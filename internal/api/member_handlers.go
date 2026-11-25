package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/rootcausehq/rootcause/internal/audit"
	"github.com/rootcausehq/rootcause/internal/auth"
	"github.com/rootcausehq/rootcause/internal/db"
)

// Member of an organization.
type Member struct {
	UserID   uuid.UUID `json:"userId"`
	Email    string    `json:"email"`
	Name     string    `json:"name"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joinedAt"`
}

// Invitation that has not been accepted yet.
type Invitation struct {
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

var roles = map[string]bool{"owner": true, "admin": true, "member": true, "viewer": true}

const inviteTTL = 7 * 24 * time.Hour

func (s *Server) listMembers(w http.ResponseWriter, r *http.Request) {
	var members []Member
	var invites []Invitation
	err := s.db.InScope(r.Context(), scope(r), func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `SELECT u.id, u.email, u.name, m.role, m.created_at
			FROM memberships m JOIN users u ON u.id = m.user_id WHERE m.org_id = app_org_id() ORDER BY u.email`)
		if err != nil {
			return err
		}
		if members, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Member]); err != nil {
			return err
		}
		rows, err = tx.Query(r.Context(), `SELECT email, role, created_at, expires_at FROM invitations
			WHERE expires_at > now() ORDER BY created_at DESC`)
		if err != nil {
			return err
		}
		invites, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Invitation])
		return err
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if invites == nil {
		invites = []Invitation{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": members, "invitations": invites})
}

// invite adds an existing user directly, or creates an invitation link for a
// new address (emailed when SMTP is configured, otherwise shown to the admin).
func (s *Server) invite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	email, ok := normalizeEmail(req.Email)
	if !ok {
		writeError(w, http.StatusBadRequest, "a valid email is required")
		return
	}
	if !roles[req.Role] {
		writeError(w, http.StatusBadRequest, "role must be owner, admin, member or viewer")
		return
	}
	ctx := r.Context()
	org := orgFrom(ctx)
	if req.Role == "owner" && org.Role != "owner" {
		writeError(w, http.StatusForbidden, "only owners can add owners")
		return
	}

	var existing uuid.UUID
	err := s.db.Pool.QueryRow(ctx, `SELECT id FROM users WHERE lower(email) = $1`, email).Scan(&existing)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		s.internalError(w, r, err)
		return
	}
	if existing != uuid.Nil {
		err := s.db.InScope(ctx, scope(r), func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `INSERT INTO memberships (org_id, user_id, role) VALUES ($1, $2, $3)`, org.ID, existing, req.Role); err != nil {
				return err
			}
			return audit.Record(ctx, tx, audit.Entry{OrgID: org.ID, ActorID: userFrom(ctx).ID, Action: "member.add", Target: email, IP: clientIP(r)})
		})
		if db.IsUniqueViolation(err) {
			writeError(w, http.StatusConflict, "already a member")
			return
		}
		s.finish(w, r, err, map[string]any{"added": true})
		return
	}

	token, hash, err := auth.NewToken()
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	err = s.db.InScope(ctx, scope(r), func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM invitations WHERE lower(email) = $1`, email); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO invitations (token_hash, org_id, email, role, invited_by, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6)`, hash, org.ID, email, req.Role, userFrom(ctx).ID, time.Now().Add(inviteTTL)); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{OrgID: org.ID, ActorID: userFrom(ctx).ID, Action: "member.invite", Target: email, IP: clientIP(r)})
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	link := s.cfg.PublicURL.String() + "/invite/" + token
	emailed := false
	if s.cfg.SMTP.Host != "" {
		body := userFrom(ctx).Email + " invited you to the " + org.Name + " organization on RootCause.\n\nAccept the invitation (valid for 7 days):\n" + link + "\n"
		if err := s.sender.SendMail([]string{email}, "You're invited to "+org.Name+" on RootCause", body); err != nil {
			s.log.Warn("invite email failed", "err", err)
		} else {
			emailed = true
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"invited": true, "emailed": emailed, "link": link})
}

func (s *Server) revokeInvite(w http.ResponseWriter, r *http.Request) {
	email, _ := normalizeEmail(chi.URLParam(r, "email"))
	err := s.db.InScope(r.Context(), scope(r), func(tx pgx.Tx) error {
		tag, err := tx.Exec(r.Context(), `DELETE FROM invitations WHERE lower(email) = $1`, email)
		if err == nil && tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return err
	})
	s.finish(w, r, err, nil)
}

func (s *Server) updateMember(w http.ResponseWriter, r *http.Request) {
	target, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "member not found")
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if !roles[req.Role] {
		writeError(w, http.StatusBadRequest, "invalid role")
		return
	}
	s.changeMembership(w, r, target, req.Role)
}

func (s *Server) removeMember(w http.ResponseWriter, r *http.Request) {
	target, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "member not found")
		return
	}
	s.changeMembership(w, r, target, "")
}

// changeMembership sets a member's role ("" removes them). The last owner
// can't be demoted or removed, and only owners can touch owners.
func (s *Server) changeMembership(w http.ResponseWriter, r *http.Request, target uuid.UUID, role string) {
	ctx := r.Context()
	org := orgFrom(ctx)
	err := s.db.InScope(ctx, scope(r), func(tx pgx.Tx) error {
		var current string
		if err := tx.QueryRow(ctx, `SELECT role FROM memberships WHERE org_id = $1 AND user_id = $2 FOR UPDATE`, org.ID, target).Scan(&current); err != nil {
			return err
		}
		if (current == "owner" || role == "owner") && org.Role != "owner" {
			return &httpError{http.StatusForbidden, "only owners can change owners"}
		}
		if current == "owner" && role != "owner" {
			var owners int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM memberships WHERE org_id = $1 AND role = 'owner'`, org.ID).Scan(&owners); err != nil {
				return err
			}
			if owners <= 1 {
				return &httpError{http.StatusBadRequest, "an organization needs at least one owner"}
			}
		}
		action := "member.remove"
		if role == "" {
			if _, err := tx.Exec(ctx, `DELETE FROM memberships WHERE org_id = $1 AND user_id = $2`, org.ID, target); err != nil {
				return err
			}
		} else {
			action = "member.role"
			if _, err := tx.Exec(ctx, `UPDATE memberships SET role = $3 WHERE org_id = $1 AND user_id = $2`, org.ID, target, role); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.Entry{OrgID: org.ID, ActorID: userFrom(ctx).ID, Action: action, Target: target.String(), IP: clientIP(r), Data: map[string]any{"role": role}})
	})
	s.finish(w, r, err, nil)
}

// --- accepting invitations (no session required) ------------------------------------

type inviteInfo struct {
	orgID   uuid.UUID
	orgName string
	orgSlug string
	email   string
	role    string
}

func (s *Server) lookupInvite(r *http.Request, token string) (*inviteInfo, error) {
	var in inviteInfo
	// The token identifies the tenant, so this lookup is cross-tenant by design.
	err := s.db.System(r.Context(), func(tx pgx.Tx) error {
		return tx.QueryRow(r.Context(), `SELECT i.org_id, o.name, o.slug, i.email, i.role
			FROM invitations i JOIN organizations o ON o.id = i.org_id
			WHERE i.token_hash = $1 AND i.expires_at > now()`, auth.HashToken(token),
		).Scan(&in.orgID, &in.orgName, &in.orgSlug, &in.email, &in.role)
	})
	return &in, err
}

func (s *Server) getInvite(w http.ResponseWriter, r *http.Request) {
	in, err := s.lookupInvite(r, chi.URLParam(r, "token"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "this invitation is invalid or has expired")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	var exists bool
	_ = s.db.Pool.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM users WHERE lower(email) = $1)`, in.email).Scan(&exists)
	writeJSON(w, http.StatusOK, map[string]any{"organization": in.orgName, "email": in.email, "role": in.role, "accountExists": exists})
}

// acceptInvite creates the account (or uses the signed-in one with the same
// email), adds the membership and signs the user in.
func (s *Server) acceptInvite(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	var req struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	in, err := s.lookupInvite(r, token)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "this invitation is invalid or has expired")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	ctx := r.Context()
	var userID uuid.UUID
	_ = s.db.Pool.QueryRow(ctx, `SELECT id FROM users WHERE lower(email) = $1`, in.email).Scan(&userID)
	if userID != uuid.Nil {
		// Existing account: must be signed in as that user.
		if u := userFrom(ctx); u == nil || u.ID != userID {
			writeError(w, http.StatusUnauthorized, "sign in as "+in.email+" to accept this invitation")
			return
		}
	} else {
		if err := auth.ValidatePassword(req.Password); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		userID = uuid.New()
		if _, err := s.db.Pool.Exec(ctx, `INSERT INTO users (id, email, name, password_hash, email_verified_at) VALUES ($1, $2, $3, $4, now())`,
			userID, in.email, strings.TrimSpace(req.Name), hash); err != nil {
			s.finish(w, r, err, nil)
			return
		}
	}
	err = s.db.InScope(ctx, db.Scope{OrgID: in.orgID, UserID: userID}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO memberships (org_id, user_id, role) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
			in.orgID, userID, in.role); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM invitations WHERE token_hash = $1`, auth.HashToken(token)); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{OrgID: in.orgID, ActorID: userID, Action: "member.join", Target: in.email, IP: clientIP(r)})
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if userFrom(ctx) == nil {
		if err := s.startSession(w, r, userID); err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"organization": in.orgSlug})
}
