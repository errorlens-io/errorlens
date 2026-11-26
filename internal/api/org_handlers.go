package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/errorlens-io/errorlens/internal/audit"
	"github.com/errorlens-io/errorlens/internal/auth"
	"github.com/errorlens-io/errorlens/internal/db"
)

// Organization as seen by the current user.
type Organization struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Slug string    `json:"slug"`
	Role string    `json:"role"`
}

// Project with its default DSN.
type Project struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Platform  string    `json:"platform"`
	DSN       string    `json:"dsn"`
	CreatedAt time.Time `json:"createdAt"`
}

func orgFrom(ctx context.Context) *Organization {
	o, _ := ctx.Value(orgKey).(*Organization)
	return o
}

// scope returns the RLS scope for an org-level request.
func scope(r *http.Request) db.Scope {
	return db.Scope{OrgID: orgFrom(r.Context()).ID, UserID: userFrom(r.Context()).ID}
}

// loadOrg resolves {org} for the current user. Row-Level Security limits
// the lookup to organizations the user belongs to, so non-members get 404.
func (s *Server) loadOrg(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := userFrom(r.Context())
		var o Organization
		if t, ok := r.Context().Value(tokenKey).(*tokenAuth); ok {
			// API token: limited to its own organization, with the token's role.
			err := s.db.InScope(r.Context(), db.Scope{OrgID: t.OrgID}, func(tx pgx.Tx) error {
				return tx.QueryRow(r.Context(), `SELECT id, name, slug FROM organizations WHERE id = $1 AND slug = $2`,
					t.OrgID, chi.URLParam(r, "org")).Scan(&o.ID, &o.Name, &o.Slug)
			})
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusNotFound, "organization not found")
				return
			}
			if err != nil {
				s.internalError(w, r, err)
				return
			}
			o.Role = t.Role
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), orgKey, &o)))
			return
		}
		err := s.db.InScope(r.Context(), db.Scope{UserID: u.ID}, func(tx pgx.Tx) error {
			return tx.QueryRow(r.Context(), `
				SELECT o.id, o.name, o.slug, m.role
				FROM organizations o JOIN memberships m ON m.org_id = o.id AND m.user_id = app_user_id()
				WHERE o.slug = $1`, chi.URLParam(r, "org"),
			).Scan(&o.ID, &o.Name, &o.Slug, &o.Role)
		})
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "organization not found")
			return
		}
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), orgKey, &o)))
	})
}

func requireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := orgFrom(r.Context()).Role
			for _, ok := range roles {
				if role == ok {
					next.ServeHTTP(w, r)
					return
				}
			}
			writeError(w, http.StatusForbidden, "insufficient permissions")
		})
	}
}

// dsn builds the DSN SDKs are configured with: scheme://key@host[/path]/projectSeq
func (s *Server) dsn(publicKey string, projectSeq int64) string {
	u := s.cfg.PublicURL
	return fmt.Sprintf("%s://%s@%s%s/%d", u.Scheme, publicKey, u.Host, u.Path, projectSeq)
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	projects := []Project{}
	err := s.db.InScope(r.Context(), scope(r), func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `
			SELECT p.id, p.name, p.slug, p.platform, p.created_at, p.seq,
			       COALESCE((SELECT k.public_key FROM project_keys k
			                 WHERE k.project_id = p.id AND k.is_active
			                 ORDER BY k.created_at LIMIT 1), '')
			FROM projects p ORDER BY p.name`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p Project
			var seq int64
			var key string
			if err := rows.Scan(&p.ID, &p.Name, &p.Slug, &p.Platform, &p.CreatedAt, &seq, &key); err != nil {
				return err
			}
			if key != "" {
				p.DSN = s.dsn(key, seq)
			}
			projects = append(projects, p)
		}
		return rows.Err()
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

// Platforms offered in the UI. Free-form values are rejected to keep data clean.
var platforms = map[string]bool{
	"other": true, "javascript": true, "node": true, "python": true, "go": true,
	"java": true, "php": true, "ruby": true, "dotnet": true, "rust": true,
	"react-native": true, "flutter": true, "android": true, "apple": true,
}

type createProjectRequest struct {
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var req createProjectRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 100 {
		writeError(w, http.StatusBadRequest, "project name is required (max 100 characters)")
		return
	}
	if req.Platform == "" {
		req.Platform = "other"
	}
	if !platforms[req.Platform] {
		writeError(w, http.StatusBadRequest, "unknown platform")
		return
	}
	key, err := auth.NewPublicKey()
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	org := orgFrom(r.Context())
	p := Project{ID: uuid.New(), Name: req.Name, Slug: projectSlug(req.Name), Platform: req.Platform}
	var seq int64
	ctx := r.Context()
	err = s.db.InScope(ctx, scope(r), func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO projects (id, org_id, name, slug, platform) VALUES ($1, $2, $3, $4, $5)
			RETURNING seq, created_at`, p.ID, org.ID, p.Name, p.Slug, p.Platform,
		).Scan(&seq, &p.CreatedAt); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO project_keys (id, org_id, project_id, public_key) VALUES ($1, $2, $3, $4)`,
			uuid.New(), org.ID, p.ID, key); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			OrgID: org.ID, ActorID: userFrom(ctx).ID, Action: audit.ActionProjectCreate,
			Target: p.Slug, IP: clientIP(r),
		})
	})
	if db.IsUniqueViolation(err) {
		writeError(w, http.StatusConflict, "a project with this name already exists")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	p.DSN = s.dsn(key, seq)
	writeJSON(w, http.StatusCreated, p)
}

type auditEntry struct {
	ID         int64     `json:"id"`
	ActorEmail *string   `json:"actorEmail"`
	Action     string    `json:"action"`
	Target     string    `json:"target"`
	IP         string    `json:"ip"`
	CreatedAt  time.Time `json:"createdAt"`
}

// listAuditLog pages newest first: ?before=<id of the last entry seen>.
func (s *Server) listAuditLog(w http.ResponseWriter, r *http.Request) {
	entries := []auditEntry{}
	before := beforeID(r)
	err := s.db.InScope(r.Context(), scope(r), func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `
			SELECT a.id, u.email, a.action, a.target, a.ip, a.created_at
			FROM audit_log a LEFT JOIN users u ON u.id = a.actor_id
			WHERE $1 = 0 OR a.id < $1
			ORDER BY a.id DESC LIMIT `+strconv.Itoa(pageSize), before)
		if err != nil {
			return err
		}
		entries, err = pgx.CollectRows(rows, pgx.RowToStructByPos[auditEntry])
		return err
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// slugify turns a display name into a URL-safe slug.
func slugify(name string) string {
	s := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(s) > 50 {
		s = strings.TrimRight(s[:50], "-")
	}
	if s == "" {
		s = "org"
	}
	return s
}

// reservedOrgSlugs can't be organization slugs: in tenant mode each slug is a
// subdomain of the hosted domain, and these are kept for the service itself.
var reservedOrgSlugs = map[string]bool{
	"www": true, "app": true, "api": true, "admin": true, "docs": true, "doc": true, "help": true,
	"support": true, "status": true, "blog": true, "mail": true, "email": true, "smtp": true,
	"send": true, "rsend": true, "auth": true, "login": true, "signup": true, "account": true,
	"accounts": true, "billing": true, "static": true, "assets": true, "cdn": true, "dashboard": true,
	"ingest": true, "events": true, "dev": true, "staging": true, "test": true, "demo": true,
	"security": true, "legal": true, "about": true, "errorlens": true, "root": true, "system": true,
}

// orgSlug is slugify for organizations: never a reserved name.
func orgSlug(name string) string {
	s := slugify(name)
	if reservedOrgSlugs[s] {
		s += "-org"
	}
	return s
}

// reservedProjectSlugs are dashboard sections: /<project>/ paths would clash.
var reservedProjectSlugs = map[string]bool{
	"issues": true, "performance": true, "traces": true, "logs": true, "monitors": true,
	"releases": true, "projects": true, "alerts": true, "settings": true, "account": true,
	"admin": true, "api": true, "invite": true, "login": true, "signup": true,
}

func projectSlug(name string) string {
	s := slugify(name)
	if reservedProjectSlugs[s] {
		s += "-app"
	}
	return s
}
