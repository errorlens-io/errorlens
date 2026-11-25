package api

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// serveUI serves the embedded single-page app. Unknown paths fall back to
// index.html so client-side routes work on reload.
func (s *Server) serveUI(w http.ResponseWriter, r *http.Request) {
	if s.ui == nil || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name != "" {
		if st, err := fs.Stat(s.ui, name); err == nil && !st.IsDir() {
			if strings.HasPrefix(name, "assets/") { // Vite content-hashed files
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			http.ServeFileFS(w, r, s.ui, name)
			return
		}
	}
	if _, err := fs.Stat(s.ui, "index.html"); err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("ErrorLens is running, but the web UI was not built into this binary. Run `make web` and rebuild."))
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFileFS(w, r, s.ui, "index.html")
}
