package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/errorlenshq/errorlens/internal/audit"
	"github.com/errorlenshq/errorlens/internal/auth"
	"github.com/errorlenshq/errorlens/internal/db"
)

// --- one-time user tokens ---------------------------------------------------------------

func (s *Server) newUserToken(ctx context.Context, userID uuid.UUID, purpose string, ttl time.Duration) (string, error) {
	tok, hash, err := auth.NewToken()
	if err != nil {
		return "", err
	}
	err = s.db.System(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM user_tokens WHERE user_id = $1 AND purpose = $2`, userID, purpose); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO user_tokens (token_hash, user_id, purpose, expires_at) VALUES ($1, $2, $3, $4)`,
			hash, userID, purpose, time.Now().Add(ttl))
		return err
	})
	return tok, err
}

// consumeUserToken validates and deletes a token, returning its user.
func (s *Server) consumeUserToken(ctx context.Context, token, purpose string) (uuid.UUID, error) {
	var userID uuid.UUID
	err := s.db.System(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `DELETE FROM user_tokens WHERE token_hash = $1 AND purpose = $2 AND expires_at > now() RETURNING user_id`,
			auth.HashToken(token), purpose).Scan(&userID)
	})
	return userID, err
}

// --- signup (hosted mode) ------------------------------------------------------------------

type signupRequest struct {
	Name             string `json:"name"`
	Email            string `json:"email"`
	Password         string `json:"password"`
	OrganizationName string `json:"organizationName"`
}

// signup creates an account and organization when ERRORLENS_SIGNUP_ENABLED.
func (s *Server) signup(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.SignupEnabled {
		writeError(w, http.StatusForbidden, "sign-up is disabled on this server")
		return
	}
	var req signupRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	email, ok := normalizeEmail(req.Email)
	if !ok {
		writeError(w, http.StatusBadRequest, "a valid email is required")
		return
	}
	if !s.loginByIP.Allow("signup:" + clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.OrganizationName = strings.TrimSpace(req.OrganizationName)
	if req.OrganizationName == "" || len(req.OrganizationName) > 100 {
		writeError(w, http.StatusBadRequest, "organization name is required (max 100 characters)")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	verified := s.cfg.SMTP.Host == "" // without email we cannot verify; trust the address
	userID := uuid.New()
	ctx := r.Context()
	err = s.db.System(ctx, func(tx pgx.Tx) error {
		var verifiedAt *time.Time
		if verified {
			now := time.Now()
			verifiedAt = &now
		}
		_, err := tx.Exec(ctx, `INSERT INTO users (id, email, name, password_hash, email_verified_at) VALUES ($1, $2, $3, $4, $5)`,
			userID, email, strings.TrimSpace(req.Name), hash, verifiedAt)
		return err
	})
	if db.IsUniqueViolation(err) {
		writeError(w, http.StatusConflict, "an account with this email already exists")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	org, err := s.createOrganization(ctx, userID, req.OrganizationName, clientIP(r))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !verified {
		s.sendVerification(ctx, userID, email)
	}
	if err := s.startSession(w, r, userID); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"organization": org, "emailVerified": verified})
}

// createOrganization makes a new org owned by userID, with a unique slug and
// the hosted free-plan quota when sign-up is enabled.
func (s *Server) createOrganization(ctx context.Context, userID uuid.UUID, name, ip string) (*Organization, error) {
	orgID := uuid.New()
	base := orgSlug(name)
	var slug string
	err := s.db.System(ctx, func(tx pgx.Tx) error {
		slug = base
		for i := 2; ; i++ {
			var taken bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM organizations WHERE slug = $1)`, slug).Scan(&taken); err != nil {
				return err
			}
			if !taken {
				break
			}
			slug = base + "-" + itoa(i)
		}
		plan, quota := "self-hosted", (*int64)(nil)
		if s.cfg.SignupEnabled {
			plan, quota = "free", &s.cfg.FreeEventQuota
		}
		if err := db.ApplyScope(ctx, tx, db.Scope{OrgID: orgID, UserID: userID}); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO organizations (id, name, slug) VALUES ($1, $2, $3)`, orgID, name, slug); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO memberships (org_id, user_id, role) VALUES ($1, $2, 'owner')`, orgID, userID); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{OrgID: orgID, ActorID: userID, Action: "organization.create", Target: slug, IP: ip}); err != nil {
			return err
		}
		// Plan columns are operator-controlled: set them outside the app role.
		_, err := tx.Exec(ctx, `RESET ROLE`)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE organizations SET plan = $2, monthly_event_quota = $3 WHERE id = $1`, orgID, plan, quota)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &Organization{ID: orgID, Name: name, Slug: slug, Role: "owner"}, nil
}

func itoa(i int) string { return strconv.Itoa(i) }

// createOrg lets a signed-in user add another organization (hosted mode or superusers).
func (s *Server) createOrg(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	if !s.cfg.SignupEnabled && !u.IsSuperuser {
		writeError(w, http.StatusForbidden, "only administrators can create organizations on this server")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 100 {
		writeError(w, http.StatusBadRequest, "organization name is required (max 100 characters)")
		return
	}
	org, err := s.createOrganization(r.Context(), u.ID, req.Name, clientIP(r))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, org)
}

// --- email verification & password reset ---------------------------------------------------

func (s *Server) sendVerification(ctx context.Context, userID uuid.UUID, email string) {
	tok, err := s.newUserToken(ctx, userID, "verify_email", 48*time.Hour)
	if err != nil {
		s.log.Error("verification token", "err", err)
		return
	}
	link := s.cfg.PublicURL.String() + "/verify-email/" + tok
	if err := s.sender.SendMail([]string{email}, "Confirm your ErrorLens email", "Confirm your email address:\n"+link+"\n\nThe link is valid for 48 hours."); err != nil {
		s.log.Warn("verification email failed", "err", err)
	}
}

func (s *Server) resendVerification(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	if s.cfg.SMTP.Host == "" {
		writeError(w, http.StatusBadRequest, "email is not configured on this server")
		return
	}
	s.sendVerification(r.Context(), u.ID, u.Email)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	userID, err := s.consumeUserToken(r.Context(), req.Token, "verify_email")
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusBadRequest, "this link is invalid or has expired")
		return
	}
	if err == nil {
		_, err = s.db.Pool.Exec(r.Context(), `UPDATE users SET email_verified_at = now() WHERE id = $1`, userID)
	}
	s.finish(w, r, err, map[string]bool{"verified": true})
}

func (s *Server) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if s.cfg.SMTP.Host == "" {
		writeError(w, http.StatusBadRequest, "email is not configured on this server; ask an administrator to reset your password")
		return
	}
	email, _ := normalizeEmail(req.Email)
	if !s.loginByEmail.Allow("reset:" + email) {
		writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}
	var userID uuid.UUID
	if err := s.db.Pool.QueryRow(r.Context(), `SELECT id FROM users WHERE lower(email) = $1`, email).Scan(&userID); err == nil {
		if tok, err := s.newUserToken(r.Context(), userID, "reset_password", time.Hour); err == nil {
			link := s.cfg.PublicURL.String() + "/reset-password/" + tok
			_ = s.sender.SendMail([]string{email}, "Reset your ErrorLens password", "Reset your password (valid for 1 hour):\n"+link+"\n\nIf you didn't ask for this, ignore this email.")
		}
	}
	// Same response whether or not the account exists.
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	userID, err := s.consumeUserToken(r.Context(), req.Token, "reset_password")
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusBadRequest, "this link is invalid or has expired")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	ctx := r.Context()
	err = s.db.System(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE users SET password_hash = $2, email_verified_at = COALESCE(email_verified_at, now()) WHERE id = $1`, userID, hash); err != nil {
			return err
		}
		// Sign out everywhere.
		if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: userID, Action: "user.password_reset", IP: clientIP(r)})
	})
	s.finish(w, r, err, nil)
}

// changePassword lets a signed-in user change their password.
func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Current string `json:"currentPassword"`
		New     string `json:"newPassword"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	u := userFrom(r.Context())
	var hash string
	if err := s.db.Pool.QueryRow(r.Context(), `SELECT password_hash FROM users WHERE id = $1`, u.ID).Scan(&hash); err != nil {
		s.internalError(w, r, err)
		return
	}
	if ok, _ := auth.CheckPassword(hash, req.Current); !ok {
		writeError(w, http.StatusBadRequest, "current password is incorrect")
		return
	}
	if err := auth.ValidatePassword(req.New); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	newHash, err := auth.HashPassword(req.New)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	ctx := r.Context()
	err = s.db.System(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, u.ID, newHash); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: u.ID, Action: "user.password_change", IP: clientIP(r)})
	})
	s.finish(w, r, err, nil)
}

// --- two-factor authentication -----------------------------------------------------------------

func (s *Server) mfaStatus(w http.ResponseWriter, r *http.Request) {
	var enabled bool
	var remaining int
	err := s.db.Pool.QueryRow(r.Context(), `SELECT totp_secret IS NOT NULL, cardinality(recovery_codes) FROM users WHERE id = $1`,
		userFrom(r.Context()).ID).Scan(&enabled, &remaining)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": enabled, "recoveryCodesLeft": remaining})
}

// mfaSetup creates a pending secret; it becomes active after mfaConfirm.
func (s *Server) mfaSetup(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	secret, err := auth.NewTOTPSecret()
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if _, err := s.db.Pool.Exec(r.Context(), `UPDATE users SET totp_pending = $2 WHERE id = $1`, u.ID, secret); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"secret": auth.TOTPSecretString(secret),
		"uri":    auth.TOTPURI(secret, u.Email, "ErrorLens"),
	})
}

func (s *Server) mfaConfirm(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	u := userFrom(r.Context())
	var pending []byte
	if err := s.db.Pool.QueryRow(r.Context(), `SELECT totp_pending FROM users WHERE id = $1`, u.ID).Scan(&pending); err != nil || pending == nil {
		writeError(w, http.StatusBadRequest, "start two-factor setup first")
		return
	}
	if !auth.VerifyTOTP(pending, req.Code, time.Now()) {
		writeError(w, http.StatusBadRequest, "that code is not valid; check your device's clock and try again")
		return
	}
	codes, hashes, err := auth.NewRecoveryCodes()
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	ctx := r.Context()
	err = s.db.System(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE users SET totp_secret = totp_pending, totp_pending = NULL, recovery_codes = $2 WHERE id = $1`, u.ID, hashes); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: u.ID, Action: "user.mfa_enable", IP: clientIP(r)})
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "recoveryCodes": codes})
}

func (s *Server) mfaDisable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	u := userFrom(r.Context())
	var hash string
	_ = s.db.Pool.QueryRow(r.Context(), `SELECT password_hash FROM users WHERE id = $1`, u.ID).Scan(&hash)
	if ok, _ := auth.CheckPassword(hash, req.Password); !ok {
		writeError(w, http.StatusBadRequest, "password is incorrect")
		return
	}
	ctx := r.Context()
	err := s.db.System(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE users SET totp_secret = NULL, totp_pending = NULL, recovery_codes = '{}' WHERE id = $1`, u.ID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorID: u.ID, Action: "user.mfa_disable", IP: clientIP(r)})
	})
	s.finish(w, r, err, nil)
}

// loginMFA completes a login that required a second factor.
func (s *Server) loginMFA(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"mfaToken"`
		Code  string `json:"code"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if !s.loginByIP.Allow(clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}
	ctx := r.Context()
	var userID uuid.UUID
	var secret []byte
	var recovery []string
	err := s.db.Pool.QueryRow(ctx, `SELECT u.id, u.totp_secret, u.recovery_codes FROM user_tokens t JOIN users u ON u.id = t.user_id
		WHERE t.token_hash = $1 AND t.purpose = 'mfa_login' AND t.expires_at > now()`, auth.HashToken(req.Token)).Scan(&userID, &secret, &recovery)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, "sign-in expired, enter your password again")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	ok := secret != nil && auth.VerifyTOTP(secret, req.Code, time.Now())
	usedRecovery := ""
	if !ok {
		h := auth.HashRecoveryCode(req.Code)
		if slices.Contains(recovery, h) {
			ok, usedRecovery = true, h
		}
	}
	if !ok {
		_ = s.db.System(ctx, func(tx pgx.Tx) error {
			return audit.Record(ctx, tx, audit.Entry{ActorID: userID, Action: audit.ActionLoginFailed, IP: clientIP(r), Data: map[string]any{"step": "mfa"}})
		})
		writeError(w, http.StatusUnauthorized, "invalid code")
		return
	}
	if _, err := s.consumeUserToken(ctx, req.Token, "mfa_login"); err != nil {
		writeError(w, http.StatusUnauthorized, "sign-in expired, enter your password again")
		return
	}
	if usedRecovery != "" {
		_, _ = s.db.Pool.Exec(ctx, `UPDATE users SET recovery_codes = array_remove(recovery_codes, $2) WHERE id = $1`, userID, usedRecovery)
	}
	if err := s.startSession(w, r, userID); err != nil {
		s.internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
