package api

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/errorlenshq/errorlens/internal/audit"
	"github.com/errorlenshq/errorlens/internal/auth"
	"github.com/errorlenshq/errorlens/internal/db"
)

// User is the authenticated user attached to a request.
type User struct {
	ID            uuid.UUID `json:"id"`
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	IsSuperuser   bool      `json:"isSuperuser"`
	EmailVerified bool      `json:"emailVerified"`
}

type ctxKey int

const (
	userKey ctxKey = iota
	orgKey
	tokenKey
)

func userFrom(ctx context.Context) *User {
	u, _ := ctx.Value(userKey).(*User)
	return u
}

// loadSession attaches the user for a valid session cookie, if any.
func (s *Server) loadSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer "+tokenPrefix) {
			s.tokenSession(w, r, next, strings.TrimPrefix(a, "Bearer "))
			return
		}
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value == "" {
			next.ServeHTTP(w, r)
			return
		}
		var u User
		var lastSeen time.Time
		hash := auth.HashToken(c.Value)
		err = s.db.System(r.Context(), func(tx pgx.Tx) error {
			err := tx.QueryRow(r.Context(), `
				SELECT u.id, u.email, u.name, u.is_superuser, u.email_verified_at IS NOT NULL, s.last_seen_at
				FROM sessions s JOIN users u ON u.id = s.user_id
				WHERE s.token_hash = $1 AND s.expires_at > now()`, hash,
			).Scan(&u.ID, &u.Email, &u.Name, &u.IsSuperuser, &u.EmailVerified, &lastSeen)
			if err != nil {
				return err
			}
			if time.Since(lastSeen) > 5*time.Minute {
				_, err = tx.Exec(r.Context(), `UPDATE sessions SET last_seen_at = now() WHERE token_hash = $1`, hash)
			}
			return err
		})
		if errors.Is(err, pgx.ErrNoRows) {
			s.clearCookie(w)
			next.ServeHTTP(w, r)
			return
		}
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, &u)))
	})
}

func requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userFrom(r.Context()) == nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- first-run setup ----------------------------------------------------------

func (s *Server) needsSetup(ctx context.Context) (bool, error) {
	var exists bool
	err := s.db.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users)`).Scan(&exists)
	return !exists, err
}

func (s *Server) setupStatus(w http.ResponseWriter, r *http.Request) {
	needs, err := s.needsSetup(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"needsSetup": needs})
}

type setupRequest struct {
	Name             string `json:"name"`
	Email            string `json:"email"`
	Password         string `json:"password"`
	OrganizationName string `json:"organizationName"`
}

var errAlreadySetUp = errors.New("already set up")

// setup creates the first (superuser) account and its organization. It only
// works while the instance has no users.
func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	var req setupRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.OrganizationName = strings.TrimSpace(req.OrganizationName)
	email, ok := normalizeEmail(req.Email)
	if !ok {
		writeError(w, http.StatusBadRequest, "a valid email is required")
		return
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.OrganizationName == "" || len(req.OrganizationName) > 100 {
		writeError(w, http.StatusBadRequest, "organization name is required (max 100 characters)")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	userID, orgID := uuid.New(), uuid.New()
	slug := slugify(req.OrganizationName)
	ctx := r.Context()
	err = s.db.System(ctx, func(tx pgx.Tx) error {
		// Serialize concurrent setup attempts.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(42)`); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users)`).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return errAlreadySetUp
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO users (id, email, name, password_hash, is_superuser, email_verified_at) VALUES ($1, $2, $3, $4, true, now())`,
			userID, email, req.Name, hash); err != nil {
			return err
		}
		// Everything tenant-owned is written under Row-Level Security.
		if err := db.ApplyScope(ctx, tx, db.Scope{OrgID: orgID, UserID: userID}); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO organizations (id, name, slug) VALUES ($1, $2, $3)`,
			orgID, req.OrganizationName, slug); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO memberships (org_id, user_id, role) VALUES ($1, $2, 'owner')`,
			orgID, userID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{OrgID: orgID, ActorID: userID, Action: audit.ActionSetup, Target: email, IP: clientIP(r)})
	})
	if errors.Is(err, errAlreadySetUp) {
		writeError(w, http.StatusConflict, "this instance is already set up")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := s.startSession(w, r, userID); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"user":         User{ID: userID, Email: email, Name: req.Name, IsSuperuser: true},
		"organization": Organization{ID: orgID, Name: req.OrganizationName, Slug: slug, Role: "owner"},
	})
}

// --- login / logout -------------------------------------------------------------

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	email, _ := normalizeEmail(req.Email)
	ip := clientIP(r)
	if !s.loginByIP.Allow(ip) || !s.loginByEmail.Allow(email) {
		w.Header().Set("Retry-After", "30")
		writeError(w, http.StatusTooManyRequests, "too many login attempts, try again later")
		return
	}

	ctx := r.Context()
	var userID uuid.UUID
	var hash string
	err := s.db.Pool.QueryRow(ctx, `SELECT id, password_hash FROM users WHERE lower(email) = $1`, email).Scan(&userID, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		auth.CheckDummy(req.Password) // equalize timing
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	ok, err := auth.CheckPassword(hash, req.Password)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !ok {
		_ = s.db.System(ctx, func(tx pgx.Tx) error {
			return audit.Record(ctx, tx, audit.Entry{ActorID: userID, Action: audit.ActionLoginFailed, IP: ip})
		})
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	var mfa bool
	_ = s.db.Pool.QueryRow(ctx, `SELECT totp_secret IS NOT NULL FROM users WHERE id = $1`, userID).Scan(&mfa)
	if mfa {
		tok, err := s.newUserToken(ctx, userID, "mfa_login", 5*time.Minute)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"mfaRequired": true, "mfaToken": tok})
		return
	}
	if err := s.startSession(w, r, userID); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, userID uuid.UUID) error {
	token, hash, err := auth.NewToken()
	if err != nil {
		return err
	}
	expires := time.Now().Add(s.cfg.SessionTTL)
	ua := r.UserAgent()
	if len(ua) > 512 {
		ua = ua[:512]
	}
	ctx := r.Context()
	err = s.db.System(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO sessions (token_hash, user_id, expires_at, ip, user_agent) VALUES ($1, $2, $3, $4, $5)`,
			hash, userID, expires, clientIP(r), ua); err != nil {
			return err
		}
		// Opportunistic cleanup of expired sessions.
		if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1 AND expires_at < now()`, userID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: userID, Action: audit.ActionLogin, IP: clientIP(r)})
	})
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies(),
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		ctx := r.Context()
		err := s.db.System(ctx, func(tx pgx.Tx) error {
			var userID uuid.UUID
			err := tx.QueryRow(ctx, `DELETE FROM sessions WHERE token_hash = $1 RETURNING user_id`, auth.HashToken(c.Value)).Scan(&userID)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil {
				return err
			}
			return audit.Record(ctx, tx, audit.Entry{ActorID: userID, Action: audit.ActionLogout, IP: clientIP(r)})
		})
		if err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	s.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.cfg.SecureCookies(), SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	if r.Context().Value(tokenKey) != nil {
		writeError(w, http.StatusForbidden, "not available with API tokens")
		return
	}
	u := userFrom(r.Context())
	var orgs []Organization
	err := s.db.InScope(r.Context(), db.Scope{UserID: u.ID}, func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `
			SELECT o.id, o.name, o.slug, m.role
			FROM organizations o JOIN memberships m ON m.org_id = o.id AND m.user_id = app_user_id()
			ORDER BY o.name`)
		if err != nil {
			return err
		}
		orgs, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Organization])
		return err
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if orgs == nil {
		orgs = []Organization{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": u, "organizations": orgs})
}

func normalizeEmail(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s || len(s) > 254 {
		return s, false
	}
	return s, true
}

// tokenSession authenticates an API token. The request acts as the token's
// creator (for audit records) with the token's own role, limited to its org.
func (s *Server) tokenSession(w http.ResponseWriter, r *http.Request, next http.Handler, token string) {
	var t tokenAuth
	var u User
	var creator *uuid.UUID
	err := s.db.System(r.Context(), func(tx pgx.Tx) error {
		if err := tx.QueryRow(r.Context(), `UPDATE api_tokens SET last_used_at = now() WHERE token_hash = $1
			RETURNING org_id, role, created_by`, auth.HashToken(token)).Scan(&t.OrgID, &t.Role, &creator); err != nil {
			return err
		}
		if creator != nil {
			_ = tx.QueryRow(r.Context(), `SELECT id, email, name FROM users WHERE id = $1`, *creator).Scan(&u.ID, &u.Email, &u.Name)
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, "invalid API token")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if u.Email == "" {
		u.Email = "api-token"
	}
	ctx := context.WithValue(r.Context(), userKey, &u)
	ctx = context.WithValue(ctx, tokenKey, &t)
	next.ServeHTTP(w, r.WithContext(ctx))
}
