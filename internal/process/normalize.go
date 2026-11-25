// Package process turns scrubbed SDK events into stored events grouped into issues.
package process

import (
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Limits applied during normalization.
const (
	maxBreadcrumbs = 100
	maxFrames      = 250
	maxStringLen   = 16 * 1024
	maxMessageLen  = 8 * 1024
	maxTagValueLen = 200
	maxTags        = 50
)

// Event is the normalized view of an event the processor needs. The full
// normalized payload is kept in Payload.
type Event struct {
	ID          uuid.UUID
	Timestamp   time.Time
	Level       string
	Platform    string
	Release     string
	Environment string
	Title       string
	Culprit     string
	Tags        map[string]string
	Payload     map[string]any
	Exceptions  []Exception
	Message     string // logentry template or formatted message
	Fingerprint []string
}

// Exception is one entry of an exception chain.
type Exception struct {
	Type   string
	Value  string
	Module string
	Frames []Frame
}

// Frame is a stack frame.
type Frame struct {
	Filename    string
	AbsPath     string
	Module      string
	Function    string
	ContextLine string
	InApp       *bool
}

var levels = map[string]bool{"fatal": true, "error": true, "warning": true, "info": true, "debug": true}

// Normalize validates and cleans a decoded event in place. received is the
// server receive time, used to correct missing or implausible timestamps.
func Normalize(raw map[string]any, received time.Time) *Event {
	ev := &Event{Payload: raw, Tags: map[string]string{}}

	// Event id: SDKs send 32 hex chars; accept UUID form too.
	ev.ID = parseEventID(str(raw["event_id"]))
	raw["event_id"] = strings.ReplaceAll(ev.ID.String(), "-", "")

	ev.Timestamp = parseTimestamp(raw["timestamp"], received)
	raw["timestamp"] = float64(ev.Timestamp.UnixMilli()) / 1000

	ev.Level = strings.ToLower(str(raw["level"]))
	if ev.Level == "warn" {
		ev.Level = "warning"
	}
	if !levels[ev.Level] {
		ev.Level = "error"
	}
	raw["level"] = ev.Level

	ev.Platform = clip(str(raw["platform"]), 32)
	if ev.Platform == "" {
		ev.Platform = "other"
	}
	raw["platform"] = ev.Platform
	ev.Release = clip(str(raw["release"]), 250)
	ev.Environment = clip(str(raw["environment"]), 64)
	if ev.Environment == "" {
		ev.Environment = "production"
	}
	raw["environment"] = ev.Environment

	// Message: "message" may be a string or {formatted, message}; logentry likewise.
	ev.Message = clip(messageOf(raw), maxMessageLen)

	ev.Exceptions = exceptionsOf(raw)
	if fp, ok := raw["fingerprint"].([]any); ok {
		for _, p := range fp {
			if s := fmt.Sprint(p); s != "" {
				ev.Fingerprint = append(ev.Fingerprint, clip(s, 200))
			}
		}
	}

	trimBreadcrumbs(raw)
	clipStrings(raw, 0)

	ev.Title, ev.Culprit = titleAndCulprit(ev, raw)
	ev.Tags = tagsOf(ev, raw)
	return ev
}

func parseEventID(s string) uuid.UUID {
	s = strings.ReplaceAll(strings.TrimSpace(s), "-", "")
	if b, err := hex.DecodeString(s); err == nil && len(b) == 16 {
		var id uuid.UUID
		copy(id[:], b)
		return id
	}
	return uuid.New()
}

func parseTimestamp(v any, received time.Time) time.Time {
	var t time.Time
	switch x := v.(type) {
	case float64:
		sec, frac := math.Modf(x)
		t = time.Unix(int64(sec), int64(frac*1e9))
	case string:
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999", "2006-01-02T15:04:05"} {
			if p, err := time.Parse(layout, x); err == nil {
				t = p
				break
			}
		}
	}
	// Clock skew protection: future or very old timestamps use receive time.
	if t.IsZero() || t.After(received.Add(time.Minute)) || t.Before(received.Add(-30*24*time.Hour)) {
		return received.UTC()
	}
	return t.UTC()
}

func messageOf(raw map[string]any) string {
	for _, key := range []string{"logentry", "message"} {
		switch m := raw[key].(type) {
		case string:
			if m != "" {
				return m
			}
		case map[string]any:
			if s := str(m["formatted"]); s != "" {
				return s
			}
			if s := str(m["message"]); s != "" {
				return s
			}
		}
	}
	return ""
}

// messageTemplate prefers the un-formatted template, which groups better.
func messageTemplate(raw map[string]any) string {
	for _, key := range []string{"logentry", "message"} {
		if m, ok := raw[key].(map[string]any); ok {
			if s := str(m["message"]); s != "" {
				return s
			}
		}
	}
	return messageOf(raw)
}

func exceptionsOf(raw map[string]any) []Exception {
	var values []any
	switch e := raw["exception"].(type) {
	case map[string]any:
		values, _ = e["values"].([]any)
	case []any:
		values = e
	}
	var out []Exception
	for _, v := range values {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		ex := Exception{Type: str(m["type"]), Value: str(m["value"]), Module: str(m["module"])}
		if st, ok := m["stacktrace"].(map[string]any); ok {
			ex.Frames = framesOf(st)
		}
		out = append(out, ex)
	}
	// Fall back to top-level or first thread's stack trace (e.g. captureMessage with attachStacktrace).
	if len(out) == 0 {
		var st map[string]any
		if s, ok := raw["stacktrace"].(map[string]any); ok {
			st = s
		} else if th, ok := raw["threads"].(map[string]any); ok {
			if vals, ok := th["values"].([]any); ok {
				for _, t := range vals {
					if tm, ok := t.(map[string]any); ok {
						if s, ok := tm["stacktrace"].(map[string]any); ok {
							st = s
							break
						}
					}
				}
			}
		}
		if st != nil {
			out = append(out, Exception{Frames: framesOf(st)})
		}
	}
	return out
}

func framesOf(st map[string]any) []Frame {
	raw, _ := st["frames"].([]any)
	// Keep the frames closest to the crash and the entry point; drop the middle.
	if len(raw) > maxFrames {
		raw = append(raw[:maxFrames/2:maxFrames/2], raw[len(raw)-maxFrames/2:]...)
		st["frames"] = raw
	}
	frames := make([]Frame, 0, len(raw))
	for _, f := range raw {
		m, ok := f.(map[string]any)
		if !ok {
			continue
		}
		fr := Frame{
			Filename:    str(m["filename"]),
			AbsPath:     str(m["abs_path"]),
			Module:      str(m["module"]),
			Function:    str(m["function"]),
			ContextLine: str(m["context_line"]),
		}
		if b, ok := m["in_app"].(bool); ok {
			fr.InApp = &b
		}
		frames = append(frames, fr)
	}
	return frames
}

func trimBreadcrumbs(raw map[string]any) {
	switch b := raw["breadcrumbs"].(type) {
	case map[string]any:
		if vals, ok := b["values"].([]any); ok && len(vals) > maxBreadcrumbs {
			b["values"] = vals[len(vals)-maxBreadcrumbs:]
		}
	case []any:
		if len(b) > maxBreadcrumbs {
			b = b[len(b)-maxBreadcrumbs:]
		}
		raw["breadcrumbs"] = map[string]any{"values": b}
	}
}

// clipStrings bounds every string in the payload and the nesting depth.
func clipStrings(v any, depth int) any {
	switch t := v.(type) {
	case map[string]any:
		if depth > 12 {
			return "[too deep]"
		}
		for k, c := range t {
			t[k] = clipStrings(c, depth+1)
		}
	case []any:
		if depth > 12 {
			return "[too deep]"
		}
		for i, c := range t {
			t[i] = clipStrings(c, depth+1)
		}
	case string:
		return clip(t, maxStringLen)
	}
	return v
}

func titleAndCulprit(ev *Event, raw map[string]any) (title, culprit string) {
	if n := len(ev.Exceptions); n > 0 && (ev.Exceptions[n-1].Type != "" || ev.Exceptions[n-1].Value != "") {
		ex := ev.Exceptions[n-1] // last value is the outermost exception
		switch {
		case ex.Type != "" && ex.Value != "":
			title = ex.Type + ": " + firstLine(ex.Value)
		case ex.Type != "":
			title = ex.Type
		default:
			title = firstLine(ex.Value)
		}
	} else if ev.Message != "" {
		title = firstLine(ev.Message)
	} else {
		title = "<unlabeled event>"
	}

	if tr := str(raw["transaction"]); tr != "" {
		culprit = tr
	} else if f := topFrame(ev); f != nil {
		loc := f.Module
		if loc == "" {
			loc = f.Filename
		}
		if f.Function != "" {
			culprit = loc + " in " + f.Function
		} else {
			culprit = loc
		}
	}
	return clip(title, 200), clip(culprit, 200)
}

// topFrame returns the most recent in-app frame (frames are oldest-first).
func topFrame(ev *Event) *Frame {
	if len(ev.Exceptions) == 0 {
		return nil
	}
	frames := ev.Exceptions[len(ev.Exceptions)-1].Frames
	for i := len(frames) - 1; i >= 0; i-- {
		if frames[i].InApp != nil && *frames[i].InApp {
			return &frames[i]
		}
	}
	if len(frames) > 0 {
		return &frames[len(frames)-1]
	}
	return nil
}

func tagsOf(ev *Event, raw map[string]any) map[string]string {
	tags := map[string]string{}
	set := func(k, v string) {
		k, v = clip(strings.TrimSpace(k), 32), clip(strings.TrimSpace(v), maxTagValueLen)
		if k != "" && v != "" && len(tags) < maxTags {
			tags[k] = v
		}
	}
	switch t := raw["tags"].(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			set(k, fmt.Sprint(t[k]))
		}
	case []any:
		for _, p := range t {
			if pair, ok := p.([]any); ok && len(pair) == 2 {
				set(fmt.Sprint(pair[0]), fmt.Sprint(pair[1]))
			}
		}
	}
	set("level", ev.Level)
	set("environment", ev.Environment)
	set("release", ev.Release)
	set("server_name", str(raw["server_name"]))
	set("transaction", str(raw["transaction"]))
	if ctx, ok := raw["contexts"].(map[string]any); ok {
		for _, c := range []struct{ ctx, tag string }{{"browser", "browser"}, {"os", "os"}, {"runtime", "runtime"}, {"device", "device"}} {
			if m, ok := ctx[c.ctx].(map[string]any); ok {
				name, ver := str(m["name"]), str(m["version"])
				if c.ctx == "device" {
					name, ver = str(m["model"]), ""
				}
				if name != "" {
					set(c.tag, strings.TrimSpace(name+" "+ver))
				}
			}
		}
	}
	if u, ok := raw["user"].(map[string]any); ok {
		for _, k := range []string{"id", "username", "email", "ip_address"} {
			if v := str(u[k]); v != "" {
				set("user", k+":"+v)
				break
			}
		}
	}
	return tags
}

func str(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	case float64, bool, int, int64:
		return fmt.Sprint(t)
	}
	return ""
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// Cut on a rune boundary.
	for n > 0 && n < len(s) && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n] + "…"
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
