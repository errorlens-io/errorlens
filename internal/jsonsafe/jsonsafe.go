// Package jsonsafe makes JSON safe to store in Postgres.
package jsonsafe

import "bytes"

// NoNUL rewrites every \u0000 escape in a JSON document to � (the Unicode replacement character).
// Postgres can't store U+0000 in text or jsonb, so one stray NUL in an error message would otherwise make
// the whole event fail to queue and the SDK retry it forever. Escaped backslashes are honoured ("\\u0000"
// is the literal text, left alone). The length is unchanged; the input is copied only when it has a NUL.
func NoNUL(b []byte) []byte {
	if !bytes.Contains(b, []byte(`\u0000`)) {
		return b
	}
	out := append([]byte(nil), b...)
	for i := 0; i < len(out)-1; i++ {
		if out[i] != '\\' {
			continue
		}
		if out[i+1] == 'u' && i+5 < len(out) && string(out[i+2:i+6]) == "0000" {
			copy(out[i+2:i+6], "fffd")
		}
		i++ // skip the escaped character
	}
	return out
}
