package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/errorlenshq/errorlens/internal/audit"
	"github.com/errorlenshq/errorlens/internal/auth"
)

// ProjectSettings is the editable configuration of a project.
type ProjectSettings struct {
	ID              uuid.UUID    `json:"id"`
	Name            string       `json:"name"`
	Slug            string       `json:"slug"`
	Platform        string       `json:"platform"`
	RetentionDays   int          `json:"retentionDays"`
	ScrubDefaults   bool         `json:"scrubDefaults"`
	ScrubIP         string       `json:"scrubIp"`
	SensitiveFields []string     `json:"sensitiveFields"`
	SafeFields      []string     `json:"safeFields"`
	Keys            []ProjectKey `json:"keys"`
}

// ProjectKey is a DSN key.
type ProjectKey struct {
	ID        uuid.UUID `json:"id"`
	Label     string    `json:"label"`
	DSN       string    `json:"dsn"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Server) getProjectSettings(w http.ResponseWriter, r *http.Request) {
	var ps ProjectSettings
	err := s.db.InScope(r.Context(), scope(r), func(tx pgx.Tx) error {
		var seq int64
		if err := tx.QueryRow(r.Context(), `
			SELECT id, name, slug, platform, retention_days, scrub_defaults, scrub_ip, sensitive_fields, safe_fields, seq
			FROM projects WHERE slug = $1`, chi.URLParam(r, "project"),
		).Scan(&ps.ID, &ps.Name, &ps.Slug, &ps.Platform, &ps.RetentionDays, &ps.ScrubDefaults, &ps.ScrubIP,
			&ps.SensitiveFields, &ps.SafeFields, &seq); err != nil {
			return err
		}
		rows, err := tx.Query(r.Context(), `SELECT id, label, public_key, is_active, created_at FROM project_keys
			WHERE project_id = $1 ORDER BY created_at`, ps.ID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var k ProjectKey
			var pub string
			if err := rows.Scan(&k.ID, &k.Label, &pub, &k.Active, &k.CreatedAt); err != nil {
				return err
			}
			k.DSN = s.dsn(pub, seq)
			ps.Keys = append(ps.Keys, k)
		}
		return rows.Err()
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if ps.Keys == nil {
		ps.Keys = []ProjectKey{}
	}
	writeJSON(w, http.StatusOK, ps)
}

type projectSettingsRequest struct {
	Name            string   `json:"name"`
	Platform        string   `json:"platform"`
	RetentionDays   int      `json:"retentionDays"`
	ScrubDefaults   bool     `json:"scrubDefaults"`
	ScrubIP         string   `json:"scrubIp"`
	SensitiveFields []string `json:"sensitiveFields"`
	SafeFields      []string `json:"safeFields"`
}

func cleanList(in []string, max int) ([]string, bool) {
	out := []string{}
	for _, v := range in {
		if v = strings.TrimSpace(v); v != "" {
			if len(v) > 100 {
				return nil, false
			}
			out = append(out, v)
		}
	}
	return out, len(out) <= max
}

func (s *Server) updateProjectSettings(w http.ResponseWriter, r *http.Request) {
	var req projectSettingsRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	var ok1, ok2 bool
	req.SensitiveFields, ok1 = cleanList(req.SensitiveFields, 100)
	req.SafeFields, ok2 = cleanList(req.SafeFields, 100)
	switch {
	case req.Name == "" || len(req.Name) > 100:
		writeError(w, http.StatusBadRequest, "project name is required (max 100 characters)")
		return
	case !platforms[req.Platform]:
		writeError(w, http.StatusBadRequest, "unknown platform")
		return
	case req.RetentionDays < 1 || req.RetentionDays > 3650:
		writeError(w, http.StatusBadRequest, "retention must be between 1 and 3650 days")
		return
	case req.ScrubIP != "keep" && req.ScrubIP != "truncate" && req.ScrubIP != "drop":
		writeError(w, http.StatusBadRequest, "scrubIp must be keep, truncate or drop")
		return
	case !ok1 || !ok2:
		writeError(w, http.StatusBadRequest, "field lists allow up to 100 entries of 100 characters")
		return
	}
	ctx := r.Context()
	err := s.db.InScope(ctx, scope(r), func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE projects SET name=$2, platform=$3, retention_days=$4, scrub_defaults=$5,
			scrub_ip=$6, sensitive_fields=$7, safe_fields=$8 WHERE slug=$1`,
			chi.URLParam(r, "project"), req.Name, req.Platform, req.RetentionDays, req.ScrubDefaults, req.ScrubIP,
			req.SensitiveFields, req.SafeFields)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return audit.Record(ctx, tx, audit.Entry{OrgID: orgFrom(ctx).ID, ActorID: userFrom(ctx).ID,
			Action: "project.update", Target: chi.URLParam(r, "project"), IP: clientIP(r),
			Data: map[string]any{"scrubDefaults": req.ScrubDefaults, "scrubIp": req.ScrubIP, "retentionDays": req.RetentionDays}})
	})
	s.finish(w, r, err, map[string]bool{"ok": true})
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := chi.URLParam(r, "project")
	err := s.db.InScope(ctx, scope(r), func(tx pgx.Tx) error {
		var id uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM projects WHERE slug = $1`, slug).Scan(&id); err != nil {
			return err
		}
		// Events are not FK-linked (partitioned), so delete them explicitly.
		if _, err := tx.Exec(ctx, `DELETE FROM events WHERE project_id = $1`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM projects WHERE id = $1`, id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{OrgID: orgFrom(ctx).ID, ActorID: userFrom(ctx).ID, Action: "project.delete", Target: slug, IP: clientIP(r)})
	})
	s.finish(w, r, err, nil)
}

func (s *Server) createKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Label string `json:"label"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Label = strings.TrimSpace(req.Label)
	if req.Label == "" {
		req.Label = "Key " + time.Now().UTC().Format("2006-01-02")
	}
	if len(req.Label) > 100 {
		writeError(w, http.StatusBadRequest, "label too long")
		return
	}
	pub, err := auth.NewPublicKey()
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	ctx := r.Context()
	err = s.db.InScope(ctx, scope(r), func(tx pgx.Tx) error {
		var pid uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM projects WHERE slug = $1`, chi.URLParam(r, "project")).Scan(&pid); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO project_keys (id, org_id, project_id, label, public_key) VALUES ($1,$2,$3,$4,$5)`,
			uuid.New(), orgFrom(ctx).ID, pid, req.Label, pub); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{OrgID: orgFrom(ctx).ID, ActorID: userFrom(ctx).ID, Action: "project_key.create", Target: chi.URLParam(r, "project"), IP: clientIP(r)})
	})
	s.finish(w, r, err, map[string]bool{"ok": true})
}

func (s *Server) updateKey(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "keyID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "key not found")
		return
	}
	var req struct {
		Active bool `json:"active"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	err = s.db.InScope(ctx, scope(r), func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE project_keys k SET is_active = $2 FROM projects p
			WHERE k.id = $1 AND p.id = k.project_id AND p.slug = $3`, id, req.Active, chi.URLParam(r, "project"))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		action := "project_key.disable"
		if req.Active {
			action = "project_key.enable"
		}
		return audit.Record(ctx, tx, audit.Entry{OrgID: orgFrom(ctx).ID, ActorID: userFrom(ctx).ID, Action: action, Target: id.String(), IP: clientIP(r)})
	})
	s.ingestKeysChanged()
	s.finish(w, r, err, map[string]bool{"ok": true})
}
