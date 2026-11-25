package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/rootcausehq/rootcause/internal/audit"
	"github.com/rootcausehq/rootcause/internal/auth"
)

// --- API tokens ------------------------------------------------------------------------

const tokenPrefix = "rc_"

// APIToken as listed (the secret is only returned once, on creation).
type APIToken struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Role       string     `json:"role"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
}

func (s *Server) listTokens(w http.ResponseWriter, r *http.Request) {
	out := []APIToken{}
	err := s.db.InScope(r.Context(), scope(r), func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `SELECT id, name, prefix, role, created_at, last_used_at FROM api_tokens ORDER BY created_at DESC`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowToStructByPos[APIToken])
		return err
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if out == nil {
		out = []APIToken{}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Role string `json:"role"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 100 {
		writeError(w, http.StatusBadRequest, "name is required (max 100 characters)")
		return
	}
	if req.Role == "" {
		req.Role = "member"
	}
	if req.Role != "admin" && req.Role != "member" && req.Role != "viewer" {
		writeError(w, http.StatusBadRequest, "role must be admin, member or viewer")
		return
	}
	raw, _, err := auth.NewToken()
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	token := tokenPrefix + raw
	ctx := r.Context()
	id := uuid.New()
	err = s.db.InScope(ctx, scope(r), func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO api_tokens (id, org_id, name, token_hash, prefix, role, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`, id, orgFrom(ctx).ID, req.Name, auth.HashToken(token), token[:10], req.Role, userFrom(ctx).ID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{OrgID: orgFrom(ctx).ID, ActorID: userFrom(ctx).ID, Action: "api_token.create", Target: req.Name, IP: clientIP(r)})
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "token": token})
}

func (s *Server) deleteToken(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "tokenID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "token not found")
		return
	}
	ctx := r.Context()
	err = s.db.InScope(ctx, scope(r), func(tx pgx.Tx) error {
		var name string
		if err := tx.QueryRow(ctx, `DELETE FROM api_tokens WHERE id = $1 RETURNING name`, id).Scan(&name); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{OrgID: orgFrom(ctx).ID, ActorID: userFrom(ctx).ID, Action: "api_token.delete", Target: name, IP: clientIP(r)})
	})
	s.finish(w, r, err, nil)
}

// tokenAuth is attached to requests authenticated with an API token.
type tokenAuth struct {
	OrgID uuid.UUID
	Role  string
}

// --- logs --------------------------------------------------------------------------------------

// LogLine as returned by the API.
type LogLine struct {
	ID          int64             `json:"id"`
	Timestamp   time.Time         `json:"timestamp"`
	Project     string            `json:"project"`
	Severity    string            `json:"severity"`
	Body        string            `json:"body"`
	TraceID     string            `json:"traceId"`
	Release     string            `json:"release"`
	Environment string            `json:"environment"`
	Attributes  map[string]string `json:"attributes"`
}

// listLogs: ?project=slug &severity=error (minimum) &query=text &before=RFC3339 &after=RFC3339 &limit=100
func (s *Server) listLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	where := []string{"true"}
	var args []any
	arg := func(v any) string { args = append(args, v); return "$" + strconv.Itoa(len(args)) }
	if p := q.Get("project"); p != "" {
		where = append(where, "p.slug = "+arg(p))
	}
	if sev := q.Get("severity"); sev != "" {
		n := map[string]int{"trace": 1, "debug": 5, "info": 9, "warn": 13, "error": 17, "fatal": 21}[sev]
		if n == 0 {
			writeError(w, http.StatusBadRequest, "invalid severity")
			return
		}
		where = append(where, "l.severity_number >= "+arg(n))
	}
	if text := strings.TrimSpace(q.Get("query")); text != "" {
		like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(text) + "%"
		a := arg(like)
		where = append(where, "(l.body ILIKE "+a+" OR l.attributes::text ILIKE "+a+")")
	}
	if t := q.Get("trace"); t != "" {
		where = append(where, "l.trace_id = "+arg(t))
	}
	for _, b := range []struct{ param, op string }{{"before", "<"}, {"after", ">"}} {
		if v := q.Get(b.param); v != "" {
			t, err := time.Parse(time.RFC3339Nano, v)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid "+b.param)
				return
			}
			where = append(where, "l.timestamp "+b.op+" "+arg(t))
		}
	}
	if q.Get("after") == "" {
		// Bound the scan to the retention window's recent part by default.
		where = append(where, "l.timestamp > now() - interval '30 days'")
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	out := []LogLine{}
	err := s.db.InScope(r.Context(), scope(r), func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `
			SELECT l.id, l.timestamp, p.slug, l.severity, l.body, l.trace_id, l.release, l.environment, l.attributes
			FROM logs l JOIN projects p ON p.id = l.project_id
			WHERE `+strings.Join(where, " AND ")+`
			ORDER BY l.timestamp DESC, l.id DESC LIMIT `+strconv.Itoa(limit), args...)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowToStructByPos[LogLine])
		return err
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if out == nil {
		out = []LogLine{}
	}
	writeJSON(w, http.StatusOK, out)
}

// --- releases ------------------------------------------------------------------------------------

// Release summary.
type Release struct {
	Version    string     `json:"version"`
	Project    string     `json:"project"`
	FirstEvent *time.Time `json:"firstEvent"`
	LastEvent  *time.Time `json:"lastEvent"`
	CreatedAt  time.Time  `json:"createdAt"`
	NewIssues  int64      `json:"newIssues"`
	Files      int64      `json:"files"`
}

func (s *Server) listReleases(w http.ResponseWriter, r *http.Request) {
	out := []Release{}
	project := r.URL.Query().Get("project")
	err := s.db.InScope(r.Context(), scope(r), func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `
			SELECT rl.version, p.slug, rl.first_event, rl.last_event, rl.created_at,
			       (SELECT count(*) FROM issues i WHERE i.project_id = rl.project_id AND i.first_release = rl.version),
			       (SELECT count(*) FROM artifacts a WHERE a.project_id = rl.project_id AND a.release = rl.version)
			FROM releases rl JOIN projects p ON p.id = rl.project_id
			WHERE ($1 = '' OR p.slug = $1)
			ORDER BY COALESCE(rl.last_event, rl.created_at) DESC LIMIT 100`, project)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Release])
		return err
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if out == nil {
		out = []Release{}
	}
	writeJSON(w, http.StatusOK, out)
}

// uploadArtifact stores a source map or minified file for a release.
// multipart/form-data with fields "file" and "name" (e.g. ~/static/js/app.js.map).
func (s *Server) uploadArtifact(w http.ResponseWriter, r *http.Request) {
	const maxSize = 64 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxSize)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "expected multipart form with 'file' and 'name' (max 64 MB)")
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file")
		return
	}
	defer file.Close()
	if name == "" {
		name = "~/" + hdr.Filename
	}
	if len(name) > 500 {
		writeError(w, http.StatusBadRequest, "name too long")
		return
	}
	content, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read file")
		return
	}
	release := chi.URLParam(r, "release")
	if release == "" || len(release) > 250 {
		writeError(w, http.StatusBadRequest, "invalid release")
		return
	}
	ctx := r.Context()
	err = s.db.InScope(ctx, scope(r), func(tx pgx.Tx) error {
		var pid uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM projects WHERE slug = $1`, chi.URLParam(r, "project")).Scan(&pid); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return &httpError{http.StatusNotFound, "project not found"}
			}
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO releases (org_id, project_id, version) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
			orgFrom(ctx).ID, pid, release); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO artifacts (id, org_id, project_id, release, name, content, size) VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (project_id, release, name) DO UPDATE SET content = EXCLUDED.content, size = EXCLUDED.size, created_at = now()`,
			uuid.New(), orgFrom(ctx).ID, pid, release, name, content, len(content))
		return err
	})
	if err != nil {
		s.finish(w, r, err, nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"name": name, "size": len(content)})
}

type artifactInfo struct {
	Name      string    `json:"name"`
	Size      int       `json:"size"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Server) listArtifacts(w http.ResponseWriter, r *http.Request) {
	out := []artifactInfo{}
	err := s.db.InScope(r.Context(), scope(r), func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `SELECT a.name, a.size, a.created_at FROM artifacts a JOIN projects p ON p.id = a.project_id
			WHERE p.slug = $1 AND a.release = $2 ORDER BY a.name`, chi.URLParam(r, "project"), chi.URLParam(r, "release"))
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowToStructByPos[artifactInfo])
		return err
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if out == nil {
		out = []artifactInfo{}
	}
	writeJSON(w, http.StatusOK, out)
}
