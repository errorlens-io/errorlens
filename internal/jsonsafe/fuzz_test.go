package jsonsafe

import (
	"encoding/json"
	"strings"
	"testing"
)

// After NoNUL, no decoded string (or key) holds U+0000, and valid JSON stays valid.
func FuzzNoNUL(f *testing.F) {
	for _, s := range []string{`{"a":"x\u0000y"}`, `{"a\u0000":1}`, `"\\u0000"`, `"\\\u0000"`, `"\u0000\u0000"`, `\`, `"\u000"`} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, in []byte) {
		out := NoNUL(in)
		if len(out) != len(in) {
			t.Fatalf("length changed")
		}
		var v any
		if json.Unmarshal(in, &v) != nil {
			return
		}
		if json.Unmarshal(out, &v) != nil {
			t.Fatalf("valid JSON became invalid: %q", in)
		}
		if hasNUL(v) {
			t.Fatalf("NUL survived: %q", in)
		}
	})
}

func hasNUL(v any) bool {
	switch t := v.(type) {
	case string:
		return strings.ContainsRune(t, 0)
	case map[string]any:
		for k, c := range t {
			if strings.ContainsRune(k, 0) || hasNUL(c) {
				return true
			}
		}
	case []any:
		for _, c := range t {
			if hasNUL(c) {
				return true
			}
		}
	}
	return false
}
