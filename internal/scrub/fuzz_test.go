package scrub

import (
	"encoding/json"
	"strings"
	"testing"
)

// The scrubber runs on everything before it is stored: it must never panic, scrubbing twice must change
// nothing more, and its output must still be valid JSON.
func FuzzString(f *testing.F) {
	for _, s := range []string{
		"password=hunter2 token: abc", "postgres://user:secret@db/x", "Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.abc",
		"4111 1111 1111 1111", "sk_" + "live_51HxQabcdefghijklmnopqrstuv" /* split so secret scanners don't flag a fake key */, "-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----",
		"", "\xff\xfe\x00", strings.Repeat("a=", 1000),
	} {
		f.Add(s)
	}
	s := New(DefaultConfig)
	f.Fuzz(func(t *testing.T, in string) {
		once := s.String(in)
		if twice := s.String(once); twice != once {
			t.Fatalf("scrubbing is not stable:\n once: %q\ntwice: %q", once, twice)
		}
	})
}

func FuzzEvent(f *testing.F) {
	f.Add([]byte(`{"request":{"headers":{"Authorization":"Bearer x","Cookie":"a=b"},"data":{"password":"p"}},"user":{"ip_address":"1.2.3.4"}}`))
	f.Add([]byte(`{"extra":{"nested":[{"api_key":"k"},["token=abc"],null,1.5,true]},"exception":{"values":[{"value":"secret=x"}]}}`))
	f.Add([]byte(`{"breadcrumbs":{"values":[{"data":{"url":"https://u:p@h/x?token=1"}}]},"contexts":{"a":{"b":{"c":{"d":{"e":"f"}}}}}}`))
	s := New(DefaultConfig)
	f.Fuzz(func(t *testing.T, data []byte) {
		var ev map[string]any
		if json.Unmarshal(data, &ev) != nil || ev == nil {
			return
		}
		out := s.Event(ev)
		if _, err := json.Marshal(out); err != nil {
			t.Fatalf("scrubbed event is not JSON: %v", err)
		}
	})
}

func FuzzTruncateIP(f *testing.F) {
	for _, s := range []string{"1.2.3.4", "2001:db8::1", "::ffff:10.0.0.1", "", "999.1.1.1", "fe80::1%eth0", "[::1]:80"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, ip string) { _ = TruncateIP(ip) })
}
