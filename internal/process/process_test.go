package process

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func ev(t *testing.T, s string) *Event {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err)
	}
	return Normalize(m, time.Now())
}

const pyEvent = `{
  "event_id": "9ec79c33ec9942ab8353589fcb2e04dc",
  "timestamp": %TS%,
  "platform": "python",
  "level": "error",
  "release": "api@1.4.0",
  "server_name": "web-1",
  "exception": {"values": [{
    "type": "ZeroDivisionError",
    "value": "division by zero (order %ORDER%)",
    "module": "builtins",
    "stacktrace": {"frames": [
      {"filename": "/usr/lib/python3.12/site-packages/flask/app.py", "module": "flask.app", "function": "dispatch_request", "in_app": false},
      {"filename": "/srv/%DEPLOY%/app/views.py", "module": "app.views", "function": "checkout", "lineno": %LINE%, "in_app": true, "context_line": "    total = amount / count"}
    ]}
  }]},
  "contexts": {"os": {"name": "Linux", "version": "6.8"}, "runtime": {"name": "CPython", "version": "3.12.1"}},
  "user": {"id": "42"}
}`

func py(ts, order, deploy, line string) string {
	return strings.NewReplacer("%TS%", ts, "%ORDER%", order, "%DEPLOY%", deploy, "%LINE%", line).Replace(pyEvent)
}

func TestNormalizePythonEvent(t *testing.T) {
	now := float64(time.Now().Unix())
	e := ev(t, py(jsonNum(now), "17", "rel-1", "10"))
	if e.Title != "ZeroDivisionError: division by zero (order 17)" {
		t.Errorf("title = %q", e.Title)
	}
	if e.Culprit != "app.views in checkout" {
		t.Errorf("culprit = %q", e.Culprit)
	}
	if e.Environment != "production" || e.Release != "api@1.4.0" || e.Level != "error" {
		t.Errorf("fields: %+v", e)
	}
	for k, want := range map[string]string{"os": "Linux 6.8", "runtime": "CPython 3.12.1", "user": "id:42", "server_name": "web-1"} {
		if e.Tags[k] != want {
			t.Errorf("tag %s = %q, want %q", k, e.Tags[k], want)
		}
	}
	if e.ID.String() != "9ec79c33-ec99-42ab-8353-589fcb2e04dc" {
		t.Errorf("id = %s", e.ID)
	}
}

func jsonNum(f float64) string { b, _ := json.Marshal(f); return string(b) }

func TestGroupingIsStableAcrossNoise(t *testing.T) {
	now := jsonNum(float64(time.Now().Unix()))
	a, _ := Fingerprint(ev(t, py(now, "17", "rel-1", "10")))
	// Different order number, deploy directory and line number: same issue.
	b, _ := Fingerprint(ev(t, py(now, "99", "rel-2", "14")))
	if a != b {
		t.Fatal("same bug with different noise should group together")
	}
	// A different in-app function is a different issue.
	other := strings.Replace(py(now, "17", "rel-1", "10"), `"function": "checkout"`, `"function": "refund"`, 1)
	c, _ := Fingerprint(ev(t, other))
	if a == c {
		t.Fatal("different code paths must not group together")
	}
}

func TestMessageGrouping(t *testing.T) {
	a, _ := Fingerprint(ev(t, `{"message": "User 1234 failed login from 10.0.0.1 at 2026-01-02T10:00:00Z"}`))
	b, _ := Fingerprint(ev(t, `{"message": "User 98 failed login from 192.168.1.9 at 2026-03-04T11:22:33Z"}`))
	if a != b {
		t.Fatal("messages differing only in numbers/ips/dates should group")
	}
	c, _ := Fingerprint(ev(t, `{"logentry": {"message": "Payment %s failed", "formatted": "Payment abc failed"}}`))
	d, _ := Fingerprint(ev(t, `{"logentry": {"message": "Payment %s failed", "formatted": "Payment xyz failed"}}`))
	if c != d {
		t.Fatal("logentry template should be used for grouping")
	}
}

func TestCustomFingerprint(t *testing.T) {
	now := jsonNum(float64(time.Now().Unix()))
	base := py(now, "1", "r", "1")
	withFP := strings.Replace(base, `"level": "error",`, `"level": "error", "fingerprint": ["database-down"],`, 1)
	other := strings.Replace(base, `"function": "checkout"`, `"function": "refund"`, 1)
	otherFP := strings.Replace(other, `"level": "error",`, `"level": "error", "fingerprint": ["database-down"],`, 1)
	a, _ := Fingerprint(ev(t, withFP))
	b, _ := Fingerprint(ev(t, otherFP))
	if a != b {
		t.Fatal("custom fingerprint should override stack grouping")
	}
	def := strings.Replace(base, `"level": "error",`, `"level": "error", "fingerprint": ["{{ default }}", "tenant-a"],`, 1)
	d, comps := Fingerprint(ev(t, def))
	plain, _ := Fingerprint(ev(t, base))
	if d == plain || comps[len(comps)-1] != "custom:tenant-a" {
		t.Fatalf("{{ default }} expansion wrong: %v", comps)
	}
}

func TestJavaScriptPathNormalization(t *testing.T) {
	for in, want := range map[string]string{
		"https://cdn.example.com/static/js/main.3f9a8b7c.js?v=2": "js/main.<hash>.js",
		"webpack:///src/components/Cart.tsx":                     "components/Cart.tsx",
		"app:///index.js":                                        "index.js",
	} {
		if got := normalizePath(in); got != want {
			t.Errorf("normalizePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTimestampClamping(t *testing.T) {
	recv := time.Now()
	future := jsonNum(float64(recv.Add(48 * time.Hour).Unix()))
	e := ev(t, `{"timestamp": `+future+`, "message": "x"}`)
	if e.Timestamp.Sub(recv).Abs() > 2*time.Second {
		t.Fatalf("future timestamp not clamped: %v", e.Timestamp)
	}
	e = ev(t, `{"timestamp": "`+recv.Add(-time.Hour).UTC().Format(time.RFC3339)+`", "message": "x"}`)
	if recv.Sub(e.Timestamp) < 59*time.Minute {
		t.Fatalf("RFC3339 timestamp not parsed: %v", e.Timestamp)
	}
}

func TestLimits(t *testing.T) {
	var frames []string
	for i := 0; i < 400; i++ {
		frames = append(frames, `{"function": "f`+jsonNum(float64(i))+`", "filename": "a.py"}`)
	}
	e := ev(t, `{"exception": {"values": [{"type": "E", "stacktrace": {"frames": [`+strings.Join(frames, ",")+`]}}]}, "message": "`+strings.Repeat("x", 20000)+`"}`)
	if n := len(e.Exceptions[0].Frames); n != maxFrames {
		t.Errorf("frames = %d", n)
	}
	if len(e.Message) > maxMessageLen+4 {
		t.Errorf("message len = %d", len(e.Message))
	}
}
