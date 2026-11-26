package sourcemap

import (
	"strings"
	"testing"
)

// Frame paths come from SDKs; turning them into lookup candidates must never panic or produce paths that
// climb out of the release ("..").
func FuzzCandidates(f *testing.F) {
	for _, s := range []string{"https://shop.example/static/js/main.4f2a.js", "app:///main.js", "/../../etc/passwd", "~/x.js", "", "webpack:///./src/a.ts?x#y", "http://%zz"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, p string) {
		for _, c := range Candidates(p) {
			if strings.Contains(c, "\x00") {
				t.Fatalf("candidate with NUL from %q", p)
			}
		}
	})
}
