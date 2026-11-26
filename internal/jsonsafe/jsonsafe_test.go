package jsonsafe

import (
	"strings"
	"testing"
)

func TestNoNUL(t *testing.T) {
	nul, repl := `\u`+"0000", `\u`+"fffd" // JSON escapes, spelled out
	for in, want := range map[string]string{
		`{"m":"a#b"}`:    `{"m":"a@b"}`,
		`{"k#":"v"}`:     `{"k@":"v"}`,
		`{"m":"\#"}`:     `{"m":"\#"}`,  // "\\" + "u0000": an escaped backslash, then literal text
		`{"m":"\\#"}`:    `{"m":"\\@"}`, // an escaped backslash, then a NUL
		`{"m":"no nul"}`: `{"m":"no nul"}`,
		`{"m":"#0"}`:     `{"m":"@0"}`,
	} {
		in = strings.ReplaceAll(in, "#", nul)
		want = strings.ReplaceAll(strings.ReplaceAll(want, "#", nul), "@", repl)
		if got := string(NoNUL([]byte(in))); got != want {
			t.Errorf("NoNUL(%s) = %s, want %s", in, got, want)
		}
	}
}
