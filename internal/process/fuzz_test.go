package process

import (
	"encoding/json"
	"testing"
	"time"
)

// Events are arbitrary JSON from SDKs. Normalizing and grouping must never panic, and grouping must be
// deterministic: the same event always lands in the same issue.
func FuzzNormalizeAndFingerprint(f *testing.F) {
	f.Add([]byte(`{"exception":{"values":[{"type":"KeyError","value":"x","stacktrace":{"frames":[{"filename":"app.py","function":"f","lineno":3,"in_app":true}]}}]}}`))
	f.Add([]byte(`{"message":"User 123 failed at 2026-11-26T10:00:00Z","level":"warning"}`))
	f.Add([]byte(`{"logentry":{"formatted":"x","params":[1,2]},"fingerprint":["{{ default }}","a"]}`))
	f.Add([]byte(`{"exception":{"values":[{"stacktrace":{"frames":[null,1,"x",{"lineno":"NaN"}]}}]},"tags":[["a","b"],null],"timestamp":"not a time"}`))
	f.Add([]byte(`{"exception":{"values":null},"threads":{"values":[{"stacktrace":{"frames":[{}]}}]},"user":7,"contexts":[]}`))
	f.Add([]byte(`{"timestamp":1e308,"start_timestamp":-1e308,"level":{"x":1},"release":["a"]}`))
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	f.Fuzz(func(t *testing.T, data []byte) {
		var raw map[string]any
		if json.Unmarshal(data, &raw) != nil || raw == nil {
			return
		}
		ev := Normalize(raw, now)
		if ev == nil {
			return
		}
		h1, _ := Fingerprint(ev)
		h2, _ := Fingerprint(Normalize(raw, now))
		if h1 == "" || h1 != h2 {
			t.Fatalf("grouping is not deterministic: %q vs %q", h1, h2)
		}
	})
}

func FuzzNormalizeMessage(f *testing.F) {
	for _, s := range []string{"Order 42 failed for 1.2.3.4 at 2026-11-26T10:00:00Z", "", "0x7fff 9ec79c33-ec99-42ab-8353-589fcb2e04dc", "\xff\xfe"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if NormalizeMessage(s) != NormalizeMessage(s) {
			t.Fatal("not deterministic")
		}
	})
}
