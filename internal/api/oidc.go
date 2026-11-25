package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"

	"github.com/rootcausehq/rootcause/internal/audit"
	"github.com/rootcausehq/rootcause/internal/auth"
	"github.com/rootcausehq/rootcause/internal/db"
)

// OIDCConfig configures single sign-on with any OpenID Connect provider.
type OIDCConfig struct {
	Issuer         string
	ClientID       string
	ClientSecret   string
	Name           string   // button label, e.g. "Google" or "Okta"
	AllowedDomains []string // email domains that may sign up via SSO; empty = any
	AutoJoinOrg    string   // org slug new SSO users join; "" = no self-provisioning
	DefaultRole    string
}

// OIDCFromEnv reads ROOTCAUSE_OIDC_* variables; nil when not configured.
func OIDCFromEnv(getenv func(string) string) *OIDCConfig {
	if getenv == nil {
		getenv = os.Getenv
	}
	c := &OIDCConfig{
		Issuer:       getenv("ROOTCAUSE_OIDC_ISSUER"),
		ClientID:     getenv("ROOTCAUSE_OIDC_CLIENT_ID"),
		ClientSecret: getenv("ROOTCAUSE_OIDC_CLIENT_SECRET"),
		Name:         getenv("ROOTCAUSE_OIDC_NAME"),
		AutoJoinOrg:  getenv("ROOTCAUSE_OIDC_AUTO_JOIN_ORG"),
		DefaultRole:  getenv("ROOTCAUSE_OIDC_DEFAULT_ROLE"),
	}
	if c.Issuer == "" || c.ClientID == "" {
		return nil
	}
	if c.Name == "" {
		c.Name = "SSO"
	}
	if !roles[c.DefaultRole] || c.DefaultRole == "owner" {
		c.DefaultRole = "member"
	}
	for _, d := range strings.Split(getenv("ROOTCAUSE_OIDC_ALLOWED_DOMAINS"), ",") {
		if d = strings.ToLower(strings.TrimSpace(d)); d != "" {
			c.AllowedDomains = append(c.AllowedDomains, d)
		}
	}
	return c
}

type oidcProvider struct {
	cfg *OIDCConfig
	mu  sync.Mutex
	p   *oidc.Provider
}

// WithOIDC enables SSO.
func (s *Server) WithOIDC(c *OIDCConfig) *Server {
	if c != nil {
		s.oidc = &oidcProvider{cfg: c}
	}
	return s
}

// provider performs discovery lazily so a slow identity provider can't block startup.
func (o *oidcProvider) provider(ctx context.Context) (*oidc.Provider, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.p != nil {
		return o.p, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	p, err := oidc.NewProvider(ctx, o.cfg.Issuer)
	if err != nil {
		return nil, err
	}
	o.p = p
	return p, nil
}

func (s *Server) oauthConfig(p *oidc.Provider) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     s.oidc.cfg.ClientID,
		ClientSecret: s.oidc.cfg.ClientSecret,
		Endpoint:     p.Endpoint(),
		RedirectURL:  s.cfg.PublicURL.String() + "/api/0/auth/oidc/callback",
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}
}

// authConfig tells the login page which options to show.
func (s *Server) authConfig(w http.ResponseWriter, _ *http.Request) {
	resp := map[string]any{"signupEnabled": s.cfg.SignupEnabled, "emailEnabled": s.cfg.SMTP.Host != "", "sso": nil}
	if s.oidc != nil {
		resp["sso"] = map[string]string{"name": s.oidc.cfg.Name}
	}
	writeJSON(w, http.StatusOK, resp)
}

const oidcCookie = "rootcause_oidc"

func (s *Server) oidcStart(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil {
		http.NotFound(w, r)
		return
	}
	p, err := s.oidc.provider(r.Context())
	if err != nil {
		s.log.Error("oidc discovery", "err", err)
		http.Redirect(w, r, "/?sso_error="+url.QueryEscape("The identity provider is unreachable."), http.StatusFound)
		return
	}
	state, _, _ := auth.NewToken()
	nonce, _, _ := auth.NewToken()
	verifier := oauth2.GenerateVerifier()
	http.SetCookie(w, &http.Cookie{
		Name: oidcCookie, Value: state + "." + nonce + "." + verifier, Path: "/api/0/auth/oidc",
		MaxAge: 600, HttpOnly: true, Secure: s.cfg.SecureCookies(), SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, s.oauthConfig(p).AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), http.StatusFound)
}

func (s *Server) oidcFail(w http.ResponseWriter, r *http.Request, msg string) {
	http.Redirect(w, r, "/?sso_error="+url.QueryEscape(msg), http.StatusFound)
}

func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	if s.oidc == nil {
		http.NotFound(w, r)
		return
	}
	c, err := r.Cookie(oidcCookie)
	http.SetCookie(w, &http.Cookie{Name: oidcCookie, Path: "/api/0/auth/oidc", MaxAge: -1})
	parts := strings.Split(func() string {
		if err != nil {
			return ""
		}
		return c.Value
	}(), ".")
	if len(parts) != 3 || r.URL.Query().Get("state") != parts[0] {
		s.oidcFail(w, r, "Sign-in expired. Please try again.")
		return
	}
	if e := r.URL.Query().Get("error"); e != "" {
		s.oidcFail(w, r, "The identity provider declined the sign-in.")
		return
	}
	ctx := r.Context()
	p, err := s.oidc.provider(ctx)
	if err != nil {
		s.oidcFail(w, r, "The identity provider is unreachable.")
		return
	}
	tok, err := s.oauthConfig(p).Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(parts[2]))
	if err != nil {
		s.log.Warn("oidc exchange", "err", err)
		s.oidcFail(w, r, "Sign-in failed.")
		return
	}
	raw, _ := tok.Extra("id_token").(string)
	idt, err := p.Verifier(&oidc.Config{ClientID: s.oidc.cfg.ClientID}).Verify(ctx, raw)
	if err != nil || idt.Nonce != parts[1] {
		s.oidcFail(w, r, "Sign-in failed (invalid token).")
		return
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified *bool  `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := idt.Claims(&claims); err != nil {
		s.oidcFail(w, r, "Sign-in failed.")
		return
	}
	email, ok := normalizeEmail(claims.Email)
	if !ok || (claims.EmailVerified != nil && !*claims.EmailVerified) {
		s.oidcFail(w, r, "Your identity provider did not supply a verified email address.")
		return
	}
	userID, err := s.oidcUser(ctx, idt.Subject, email, claims.Name, clientIP(r))
	if err != nil {
		var he *httpError
		if errors.As(err, &he) {
			s.oidcFail(w, r, he.msg)
		} else {
			s.log.Error("oidc user", "err", err)
			s.oidcFail(w, r, "Sign-in failed.")
		}
		return
	}
	if err := s.startSession(w, r, userID); err != nil {
		s.oidcFail(w, r, "Sign-in failed.")
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

// oidcUser finds or provisions the account for an SSO identity.
func (s *Server) oidcUser(ctx context.Context, subject, email, name, ip string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.System(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT id FROM users WHERE oidc_subject = $1`, subject).Scan(&id)
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		// Link an existing account with the same (verified) email.
		err = tx.QueryRow(ctx, `UPDATE users SET oidc_subject = $2, email_verified_at = COALESCE(email_verified_at, now())
			WHERE lower(email) = $1 AND oidc_subject IS NULL RETURNING id`, email, subject).Scan(&id)
		if err == nil {
			return audit.Record(ctx, tx, audit.Entry{ActorID: id, Action: "user.sso_link", IP: ip})
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		cfg := s.oidc.cfg
		domain := email[strings.LastIndexByte(email, '@')+1:]
		if cfg.AutoJoinOrg == "" || (len(cfg.AllowedDomains) > 0 && !slices.Contains(cfg.AllowedDomains, domain)) {
			return &httpError{http.StatusForbidden, "No RootCause account exists for " + email + ". Ask an administrator for an invitation."}
		}
		var orgID uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM organizations WHERE slug = $1`, cfg.AutoJoinOrg).Scan(&orgID); err != nil {
			return err
		}
		// Unusable random password: SSO-only accounts can't log in with a password
		// until they reset it.
		rnd, _, _ := auth.NewToken()
		hash, err := auth.HashPassword(rnd)
		if err != nil {
			return err
		}
		id = uuid.New()
		if _, err := tx.Exec(ctx, `INSERT INTO users (id, email, name, password_hash, email_verified_at, oidc_subject) VALUES ($1, $2, $3, $4, now(), $5)`,
			id, email, name, hash, subject); err != nil {
			return err
		}
		if err := db.ApplyScope(ctx, tx, db.Scope{OrgID: orgID, UserID: id}); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO memberships (org_id, user_id, role) VALUES ($1, $2, $3)`, orgID, id, cfg.DefaultRole); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{OrgID: orgID, ActorID: id, Action: "member.sso_join", Target: email, IP: ip})
	})
	return id, err
}
