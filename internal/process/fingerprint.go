package process

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"regexp"
	"strings"
)

// Fingerprint returns the grouping hash for an event and the components it
// was computed from (useful for debugging grouping).
//
// Order of precedence:
//  1. SDK-supplied fingerprint ("{{ default }}" expands to the default grouping)
//  2. Stack trace: in-app frames (module/file + function) plus exception types
//  3. Exception type + normalized message
//  4. Normalized message template
func Fingerprint(ev *Event) (hash string, components []string) {
	def := defaultComponents(ev)
	if len(ev.Fingerprint) > 0 {
		for _, p := range ev.Fingerprint {
			if strings.EqualFold(strings.ReplaceAll(p, " ", ""), "{{default}}") {
				components = append(components, def...)
			} else {
				components = append(components, "custom:"+p)
			}
		}
	} else {
		components = def
	}
	h := sha256.Sum256([]byte(strings.Join(components, "\n")))
	return hex.EncodeToString(h[:16]), components
}

func defaultComponents(ev *Event) []string {
	var stack []string
	var types []string
	for _, ex := range ev.Exceptions {
		if ex.Type != "" {
			types = append(types, "type:"+ex.Type)
		}
		stack = append(stack, frameComponents(ex.Frames)...)
	}
	if len(stack) > 0 {
		return append(types, stack...)
	}
	if n := len(ev.Exceptions); n > 0 {
		ex := ev.Exceptions[n-1]
		if ex.Type != "" || ex.Value != "" {
			return append(types, "value:"+NormalizeMessage(firstLine(ex.Value)))
		}
	}
	if ev.Message != "" {
		return []string{"message:" + NormalizeMessage(messageTemplate(ev.Payload))}
	}
	return []string{"title:" + ev.Title}
}

// frameComponents uses in-app frames when the SDK marked any, else all frames.
func frameComponents(frames []Frame) []string {
	hasInApp := false
	for _, f := range frames {
		if f.InApp != nil && *f.InApp {
			hasInApp = true
			break
		}
	}
	var out []string
	for _, f := range frames {
		if hasInApp && (f.InApp == nil || !*f.InApp) {
			continue
		}
		if c := frameComponent(f); c != "" {
			out = append(out, c)
		}
	}
	return out
}

var (
	reHashInFile = regexp.MustCompile(`([.-])[0-9a-f]{6,}(\.[a-z]+)$`)                  // app.3f9a8b7c.js -> app.<hash>.js
	reLambda     = regexp.MustCompile(`(lambda\$|\$\$Lambda\$|func)\d+(/0x[0-9a-f]+)?`) // Java/Go generated names
	reAddr       = regexp.MustCompile(`0x[0-9a-fA-F]+`)
	reAnonNum    = regexp.MustCompile(`<anonymous>|\$\d+`)
)

func frameComponent(f Frame) string {
	loc := f.Module
	if loc == "" {
		loc = f.Filename
		if loc == "" {
			loc = f.AbsPath
		}
		loc = normalizePath(loc)
	}
	fn := f.Function
	if fn == "" || fn == "?" || fn == "<anonymous>" {
		// Anonymous functions: fall back to the source line, if available.
		if cl := strings.TrimSpace(f.ContextLine); cl != "" && len(cl) < 200 {
			fn = "line:" + cl
		} else {
			fn = ""
		}
	} else {
		fn = reAnonNum.ReplaceAllString(reAddr.ReplaceAllString(reLambda.ReplaceAllString(fn, "${1}N"), "0x"), "")
	}
	if loc == "" && fn == "" {
		return ""
	}
	return "frame:" + loc + ":" + fn
}

func normalizePath(p string) string {
	// Strip URL scheme/host and query for browser JS: https://cdn.x.com/static/app.js?v=3
	if i := strings.Index(p, "://"); i >= 0 {
		p = p[i+3:]
		if j := strings.IndexByte(p, '/'); j >= 0 {
			p = p[j:]
		}
	}
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	p = strings.TrimPrefix(p, "webpack:///")
	p = strings.TrimPrefix(p, "app:///")
	p = reHashInFile.ReplaceAllString(p, "$1<hash>$2")
	// Keep the last two path segments so deploy directories don't split groups.
	dir, file := path.Split(p)
	parent := path.Base(strings.TrimSuffix(dir, "/"))
	if parent == "." || parent == "/" || parent == "" {
		return file
	}
	return parent + "/" + file
}

var messageReplacements = []struct {
	re   *regexp.Regexp
	with string
}{
	{regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`), "<uuid>"},
	{regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.-]+`), "<email>"},
	{regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:?\d{2})?\b`), "<date>"},
	{regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}(:\d+)?\b`), "<ip>"},
	{regexp.MustCompile(`\b0x[0-9a-fA-F]+\b`), "<hex>"},
	{regexp.MustCompile(`\b[0-9a-fA-F]{12,}\b`), "<hex>"},
	{regexp.MustCompile(`\b\d+(\.\d+)?\b`), "<n>"},
}

// NormalizeMessage replaces variable parts of messages (ids, numbers, emails,
// timestamps, addresses) so similar messages group together.
func NormalizeMessage(s string) string {
	for _, r := range messageReplacements {
		s = r.re.ReplaceAllString(s, r.with)
	}
	return strings.TrimSpace(s)
}
