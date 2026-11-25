package scrub

import (
	"encoding/json"
	"strings"
	"testing"
)

func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func encode(m map[string]any) string {
	b, _ := json.Marshal(m)
	return string(b)
}

func TestScrubsSensitiveKeysEverywhere(t *testing.T) {
	ev := decode(t, `{
	  "event_id": "abc",
	  "request": {
	    "url": "https://app.example.com/login",
	    "headers": [["Authorization", "Bearer abc.def"], ["Accept", "text/html"], ["Cookie", "sid=1"]],
	    "cookies": "sid=12345",
	    "data": {"username": "alice", "password": "hunter22", "nested": {"api_key": "k-123"}},
	    "query_string": "page=2&access_token=xyz&q=hello"
	  },
	  "extra": {"DB_PASSWORD": "x", "Session-Id": "s", "author": "bob"},
	  "exception": {"values": [{"stacktrace": {"frames": [{"vars": {"secret_value": "v", "count": 3}}]}}]}
	}`)
	s := New(DefaultConfig)
	out := encode(s.Event(ev))

	for _, leaked := range []string{"hunter22", "k-123", "Bearer abc", "sid=1", "sid=12345", "xyz", `"secret_value":"v"`} {
		if strings.Contains(out, leaked) {
			t.Errorf("leaked %q in %s", leaked, out)
		}
	}
	for _, kept := range []string{"alice", "text/html", "page=2", "hello", `"count":3`, "https://app.example.com/login"} {
		if !strings.Contains(out, kept) {
			t.Errorf("over-scrubbed %q in %s", kept, out)
		}
	}
	if s.Count < 8 {
		t.Errorf("count = %d", s.Count)
	}
}

func TestScrubsSecretsInsideStrings(t *testing.T) {
	cases := map[string]string{
		"jwt":    "token was eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
		"aws":    "key AKIAIOSFODNN7EXAMPLE used",
		"github": "ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789",
		"stripe": "stripe sk_live_4eC39HqLyjWDarjtT1zdp7dc",
		"db url": "could not connect to postgres://app:s3cr3tpw@db.internal:5432/prod",
		"kv":     "login failed for bob password=hunter2 retry",
		"card":   "charge 4111 1111 1111 1111 declined",
		"pem":    "-----BEGIN RSA PRIVATE KEY-----\nMIIEow\n-----END RSA PRIVATE KEY-----",
		"bearer": "header Bearer abcdef0123456789",
	}
	secrets := map[string]string{
		"jwt": "eyJzdWIi", "aws": "AKIAIOSFODNN7EXAMPLE", "github": "ghp_aBc", "stripe": "sk_live_",
		"db url": "s3cr3tpw", "kv": "hunter2", "card": "4111 1111", "pem": "MIIEow", "bearer": "abcdef0123456789",
	}
	s := New(DefaultConfig)
	for name, in := range cases {
		out := s.String(in)
		if strings.Contains(out, secrets[name]) || !strings.Contains(out, Filtered) {
			t.Errorf("%s: not scrubbed: %q", name, out)
		}
	}
	// Non-secrets survive: order numbers that fail Luhn, ordinary text.
	for _, in := range []string{"order 1234567890123 shipped", "connect to postgres://db.internal:5432/prod", "ZeroDivisionError: division by zero"} {
		if out := s.String(in); out != in {
			t.Errorf("over-scrubbed %q -> %q", in, out)
		}
	}
}

func TestIPModes(t *testing.T) {
	mk := func() map[string]any {
		return decode(t, `{"user": {"id": "1", "ip_address": "203.0.113.77"}}`)
	}
	if got := New(Config{Defaults: true, IP: IPTruncate}).Event(mk())["user"].(map[string]any)["ip_address"]; got != "203.0.113.0" {
		t.Errorf("truncate = %v", got)
	}
	if _, ok := New(Config{Defaults: true, IP: IPDrop}).Event(mk())["user"].(map[string]any)["ip_address"]; ok {
		t.Error("drop kept ip")
	}
	if got := New(Config{Defaults: true, IP: IPKeep}).Event(mk())["user"].(map[string]any)["ip_address"]; got != "203.0.113.77" {
		t.Errorf("keep = %v", got)
	}
	auto := decode(t, `{"user": {"ip_address": "{{auto}}"}}`)
	if _, ok := New(Config{IP: IPKeep}).Event(auto)["user"].(map[string]any)["ip_address"]; ok {
		t.Error("{{auto}} must never be resolved")
	}
	if TruncateIP("2001:db8:abcd:1234::1") != "2001:db8:abcd::" {
		t.Errorf("ipv6 truncate = %s", TruncateIP("2001:db8:abcd:1234::1"))
	}
}

func TestCustomAndSafeFields(t *testing.T) {
	s := New(Config{Defaults: true, SensitiveFields: []string{"national_id"}, SafeFields: []string{"session_count"}})
	out := encode(s.Event(decode(t, `{"extra": {"national_id": "X1", "session_count": 4, "session_id": "abc"}}`)))
	if strings.Contains(out, "X1") || strings.Contains(out, "abc") || !strings.Contains(out, `"session_count":4`) {
		t.Fatalf("got %s", out)
	}
}

func TestDefaultsOff(t *testing.T) {
	s := New(Config{Defaults: false, IP: IPKeep})
	out := encode(s.Event(decode(t, `{"extra": {"password": "p"}, "message": "password=hunter2"}`)))
	if !strings.Contains(out, "hunter2") || !strings.Contains(out, `"password":"p"`) {
		t.Fatalf("defaults off should not scrub: %s", out)
	}
}
