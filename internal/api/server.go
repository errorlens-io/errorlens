// Package api serves ErrorLens's HTTP endpoints: the JSON API used by the
// web UI, health checks, and the embedded single-page app.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/errorlens-io/errorlens/internal/auth"
	"github.com/errorlens-io/errorlens/internal/billing"
	"github.com/errorlens-io/errorlens/internal/config"
	"github.com/errorlens-io/errorlens/internal/crypto"
	"github.com/errorlens-io/errorlens/internal/db"
	"github.com/errorlens-io/errorlens/internal/ingest"
	"github.com/errorlens-io/errorlens/internal/notify"
	"github.com/errorlens-io/errorlens/internal/version"
)

const sessionCookie = "errorlens_session"

// Server holds dependencies for HTTP handlers.
type Server struct {
	cfg          *config.Config
	db           *db.DB
	log          *slog.Logger
	ui           fs.FS // built web UI; may be nil
	loginByIP    *auth.Limiter
	loginByEmail *auth.Limiter
	sender       *notify.Sender
	ingest       *ingest.Handler
	keyring      *crypto.Keyring
	billing      *billing.Stripe
	oidc         *oidcProvider
}

// WithKeyring enables reading encrypted event payloads.
func (s *Server) WithKeyring(k *crypto.Keyring) *Server { s.keyring = k; return s }

// Ingest exposes the ingest handler so other features can register item types.
func (s *Server) Ingest() *ingest.Handler { return s.ingest }

// New builds a Server. ui is the static web bundle (nil disables the UI).
func New(cfg *config.Config, d *db.DB, log *slog.Logger, ui fs.FS) *Server {
	return &Server{
		cfg:          cfg,
		db:           d,
		log:          log,
		ui:           ui,
		loginByIP:    auth.NewLimiter(2*time.Second, 30),
		loginByEmail: auth.NewLimiter(30*time.Second, 10),
		sender:       &notify.Sender{HTTP: notify.NewHTTPClient(cfg.AllowPrivateWebhooks), SMTP: cfg.SMTP},
		ingest:       ingest.NewHandler(d, log),
	}
}

// ingestKeysChanged drops cached DSN keys so changes apply immediately.
func (s *Server) ingestKeysChanged() { s.ingest.InvalidateCache() }

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(realIP(s.cfg.TrustedProxies))
	r.Use(middleware.Recoverer)
	r.Use(securityHeaders)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	r.Get("/readyz", s.readyz)

	// SDK event ingestion: /api/{projectID}/envelope/ and /store/.
	s.ingest.Routes(r)

	// Payment provider webhooks authenticate by signature, not session.
	r.Post("/webhooks/stripe", s.stripeWebhook)

	r.Route("/api/0", func(r chi.Router) {
		r.Use(limitBody(1<<20, 64<<20))
		r.Use(s.sameOrigin)
		r.Use(s.loadSession)

		r.Get("/version", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{"version": version.Version})
		})
		r.Get("/setup/status", s.setupStatus)
		r.Post("/setup", s.setup)
		r.Post("/auth/login", s.login)
		r.Post("/auth/logout", s.logout)
		r.Post("/auth/login/mfa", s.loginMFA)
		r.Post("/auth/signup", s.signup)
		r.Post("/auth/verify-email", s.verifyEmail)
		r.Post("/auth/password/forgot", s.forgotPassword)
		r.Post("/auth/password/reset", s.resetPassword)
		r.Get("/auth/config", s.authConfig)
		r.Get("/auth/oidc/start", s.oidcStart)
		r.Get("/auth/oidc/callback", s.oidcCallback)
		r.Get("/invitations/{token}", s.getInvite)
		r.Post("/invitations/{token}/accept", s.acceptInvite)

		r.Group(func(r chi.Router) {
			r.Use(requireUser)
			r.Get("/auth/me", s.me)
			r.Post("/auth/verify-email/resend", s.resendVerification)
			r.Post("/auth/password", s.changePassword)
			r.Get("/auth/mfa", s.mfaStatus)
			r.Post("/auth/mfa/setup", s.mfaSetup)
			r.Post("/auth/mfa/confirm", s.mfaConfirm)
			r.Post("/auth/mfa/disable", s.mfaDisable)
			r.Post("/organizations", s.createOrg)
			r.Route("/admin", func(r chi.Router) {
				r.Use(requireSuperuser)
				r.Get("/organizations", s.adminListOrgs)
				r.Put("/organizations/{orgID}", s.adminUpdateOrg)
			})
			r.Route("/organizations/{org}", func(r chi.Router) {
				r.Use(s.loadOrg)
				r.Get("/projects", s.listProjects)
				r.With(requireRole("owner", "admin")).Post("/projects", s.createProject)
				r.With(requireRole("owner", "admin")).Get("/audit-log", s.listAuditLog)

				r.Get("/issues", s.listIssues)
				r.Get("/issues/{issueID}", s.getIssue)
				r.With(requireRole("owner", "admin", "member")).Put("/issues/{issueID}", s.updateIssue)
				r.Get("/issues/{issueID}/events", s.listIssueEvents)
				r.Get("/issues/{issueID}/events/{eventID}", s.getEvent)

				admin := requireRole("owner", "admin")
				r.Route("/projects/{project}", func(r chi.Router) {
					r.Use(admin)
					r.Get("/", s.getProjectSettings)
					r.Put("/", s.updateProjectSettings)
					r.Delete("/", s.deleteProject)
					r.Post("/keys", s.createKey)
					r.Put("/keys/{keyID}", s.updateKey)
				})

				r.With(admin).Get("/alerts/channels", s.listChannels)
				r.With(admin).Post("/alerts/channels", s.createChannel)
				r.With(admin).Put("/alerts/channels/{channelID}", s.updateChannel)
				r.With(admin).Delete("/alerts/channels/{channelID}", s.deleteChannel)
				r.With(admin).Post("/alerts/channels/{channelID}/test", s.testChannel)
				r.With(admin).Get("/alerts/rules", s.listRules)
				r.With(admin).Post("/alerts/rules", s.saveRule)
				r.With(admin).Put("/alerts/rules/{ruleID}", s.saveRule)
				r.With(admin).Delete("/alerts/rules/{ruleID}", s.deleteRule)
				r.With(admin).Get("/alerts/history", s.listAlertHistory)

				r.Get("/logs", s.listLogs)
				r.Get("/performance", s.listPerformance)
				r.Get("/transactions", s.listTransactions)
				r.Get("/traces/{traceID}", s.getTrace)
				r.Get("/monitors", s.listMonitors)
				r.Get("/monitors/{monitorID}", s.getMonitor)
				r.With(admin).Post("/monitors", s.saveMonitor)
				r.With(admin).Put("/monitors/{monitorID}", s.saveMonitor)
				r.With(admin).Delete("/monitors/{monitorID}", s.deleteMonitor)
				r.Get("/releases", s.listReleases)
				r.Get("/projects/{project}/releases/{release}/files", s.listArtifacts)
				r.With(requireRole("owner", "admin", "member")).Post("/projects/{project}/releases/{release}/files", s.uploadArtifact)
				r.With(admin).Get("/tokens", s.listTokens)
				r.With(admin).Post("/tokens", s.createToken)
				r.With(admin).Delete("/tokens/{tokenID}", s.deleteToken)

				r.With(requireRole("owner")).Get("/billing", s.getBilling)
				r.With(requireRole("owner")).Post("/billing/checkout", s.billingCheckout)
				r.With(requireRole("owner")).Post("/billing/portal", s.billingPortal)

				r.Get("/members", s.listMembers)
				r.With(admin).Post("/members", s.invite)
				r.With(admin).Delete("/invitations/{email}", s.revokeInvite)
				r.With(admin).Put("/members/{userID}", s.updateMember)
				r.With(admin).Delete("/members/{userID}", s.removeMember)
			})
		})
		r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
			writeError(w, http.StatusNotFound, "not found")
		})
	})

	r.NotFound(s.serveUI)
	return r
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.Pool.Ping(ctx); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Write([]byte("ok"))
}

// --- middleware -----------------------------------------------------------

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// The UI loads nothing from third parties: no CDNs, fonts or analytics.
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

// limitBody caps request bodies; release file uploads get the larger limit.
func limitBody(n, upload int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limit := n
			if strings.HasSuffix(r.URL.Path, "/files") && r.Method == http.MethodPost {
				limit = upload
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

// sameOrigin is the CSRF defence for cookie-authenticated API calls:
// state-changing requests must be JSON and, if the browser sends an Origin,
// it must match the configured public URL.
func (s *Server) sameOrigin(next http.Handler) http.Handler {
	want := s.cfg.PublicURL.Scheme + "://" + s.cfg.PublicURL.Host
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer "+tokenPrefix) {
			next.ServeHTTP(w, r) // API tokens are not ambient credentials: no CSRF risk
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if origin := r.Header.Get("Origin"); origin != "" && origin != want {
				writeError(w, http.StatusForbidden, "cross-origin request rejected")
				return
			}
			// Bodyless DELETEs are safe: browsers can't send cross-site DELETEs without a CORS preflight.
			bodyless := r.Method == http.MethodDelete && r.ContentLength <= 0
			if !bodyless && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				writeError(w, http.StatusUnsupportedMediaType, "content type must be application/json")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// --- helpers ----------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
		}
		return false
	}
	return true
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
