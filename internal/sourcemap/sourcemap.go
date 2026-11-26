// Package sourcemap rewrites minified JavaScript stack frames to original
// source locations using source maps uploaded for a release.
package sourcemap

import (
	"context"
	"errors"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-sourcemap/sourcemap"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/errorlenshq/errorlens/internal/db"
)

// Resolver loads artifacts and maps frames. Parsed maps are cached briefly.
type Resolver struct {
	DB *db.DB

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	c       *sourcemap.Consumer // nil = not found
	expires time.Time
}

var mappingURL = regexp.MustCompile(`(?m)^//[#@]\s*sourceMappingURL=(\S+)\s*$`)

// Candidates returns artifact names to try for a frame URL, most specific
// first: "~/path/file.js" (any host) then the full URL. A path with a NUL
// byte has none: no real file has one, and Postgres rejects it in text.
func Candidates(absPath string) []string {
	if strings.ContainsRune(absPath, 0) {
		return nil
	}
	u, err := url.Parse(absPath)
	if err != nil || u.Path == "" {
		return []string{absPath}
	}
	p := u.Path
	if u.Scheme == "app" || u.Scheme == "file" {
		p = "/" + strings.TrimPrefix(u.Host+u.Path, "/")
	}
	if strings.ContainsRune(p, 0) { // a decoded %00
		return []string{absPath}
	}
	return []string{"~" + p, absPath}
}

// Apply rewrites JS frames in the raw event payload in place. It returns
// the number of frames it mapped.
func (r *Resolver) Apply(ctx context.Context, org, project uuid.UUID, ev map[string]any) int {
	release, _ := ev["release"].(string)
	if release == "" {
		return 0
	}
	mapped := 0
	for _, st := range stacktraces(ev) {
		frames, _ := st["frames"].([]any)
		for _, f := range frames {
			fr, ok := f.(map[string]any)
			if !ok {
				continue
			}
			abs, _ := fr["abs_path"].(string)
			if abs == "" {
				abs, _ = fr["filename"].(string)
			}
			line, lok := num(fr["lineno"])
			col, cok := num(fr["colno"])
			if abs == "" || !lok || !cok {
				continue
			}
			c := r.consumer(ctx, org, project, release, abs)
			if c == nil {
				continue
			}
			file, fn, oline, ocol, ok := c.Source(line, col-1) // frames use 1-based columns
			if !ok {
				continue
			}
			fr["data"] = map[string]any{"symbolicated": true, "minified_path": abs, "minified_line": line, "minified_col": col}
			fr["abs_path"] = file
			fr["filename"] = cleanSource(file)
			fr["lineno"] = oline
			fr["colno"] = ocol + 1
			if fn != "" {
				fr["function"] = fn
			}
			fr["in_app"] = !strings.Contains(file, "node_modules")
			if src := c.SourceContent(file); src != "" {
				pre, cur, post := context5(src, oline)
				fr["pre_context"], fr["context_line"], fr["post_context"] = pre, cur, post
			}
			mapped++
		}
	}
	return mapped
}

func cleanSource(s string) string {
	for _, p := range []string{"webpack:///", "webpack://", "file://", "app:///"} {
		s = strings.TrimPrefix(s, p)
	}
	return strings.TrimPrefix(s, "./")
}

func context5(src string, line int) (pre []any, cur string, post []any) {
	lines := strings.Split(src, "\n")
	if line < 1 || line > len(lines) {
		return nil, "", nil
	}
	for i := max(0, line-6); i < line-1; i++ {
		pre = append(pre, lines[i])
	}
	cur = lines[line-1]
	for i := line; i < min(len(lines), line+5); i++ {
		post = append(post, lines[i])
	}
	return pre, cur, post
}

func num(v any) (int, bool) {
	f, ok := v.(float64)
	return int(f), ok && f > 0
}

func stacktraces(ev map[string]any) []map[string]any {
	var out []map[string]any
	if ex, ok := ev["exception"].(map[string]any); ok {
		vals, _ := ex["values"].([]any)
		for _, v := range vals {
			if m, ok := v.(map[string]any); ok {
				if st, ok := m["stacktrace"].(map[string]any); ok {
					out = append(out, st)
				}
			}
		}
	}
	if st, ok := ev["stacktrace"].(map[string]any); ok {
		out = append(out, st)
	}
	return out
}

func (r *Resolver) consumer(ctx context.Context, org, project uuid.UUID, release, abs string) *sourcemap.Consumer {
	// NUL separates the cache key's parts and can't be stored in Postgres text: nothing to look up.
	if strings.ContainsRune(release, 0) || strings.ContainsRune(abs, 0) {
		return nil
	}
	key := project.String() + "\x00" + release + "\x00" + abs
	r.mu.Lock()
	if r.cache == nil {
		r.cache = map[string]cached{}
	}
	if c, ok := r.cache[key]; ok && time.Now().Before(c.expires) {
		r.mu.Unlock()
		return c.c
	}
	r.mu.Unlock()

	c, _ := r.load(ctx, org, project, release, abs)
	r.mu.Lock()
	if len(r.cache) > 500 {
		r.cache = map[string]cached{}
	}
	r.cache[key] = cached{c: c, expires: time.Now().Add(time.Minute)}
	r.mu.Unlock()
	return c
}

func (r *Resolver) load(ctx context.Context, org, project uuid.UUID, release, abs string) (*sourcemap.Consumer, error) {
	var mapName string
	var mapData []byte
	err := r.DB.InScope(ctx, db.Scope{OrgID: org}, func(tx pgx.Tx) error {
		get := func(name string) []byte {
			var b []byte
			_ = tx.QueryRow(ctx, `SELECT content FROM artifacts WHERE project_id = $1 AND release = $2 AND name = $3`,
				project, release, name).Scan(&b)
			return b
		}
		for _, cand := range Candidates(abs) {
			// 1. A map uploaded next to the file: app.js.map
			if b := get(cand + ".map"); b != nil {
				mapName, mapData = cand+".map", b
				return nil
			}
			// 2. The minified file itself, following its sourceMappingURL comment.
			if src := get(cand); src != nil {
				m := mappingURL.FindSubmatch(src)
				if m == nil || strings.HasPrefix(string(m[1]), "data:") {
					continue
				}
				ref := string(m[1])
				name := ref
				if !strings.HasPrefix(ref, "~/") && !strings.Contains(ref, "://") {
					name = path.Join(path.Dir(cand), ref)
					if strings.HasPrefix(cand, "~/") && !strings.HasPrefix(name, "~/") {
						name = "~/" + strings.TrimPrefix(name, "~")
					}
				}
				if b := get(name); b != nil {
					mapName, mapData = name, b
					return nil
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if mapData == nil {
		return nil, errors.New("no source map")
	}
	return sourcemap.Parse(mapName, mapData)
}
