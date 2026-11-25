// Command errorlens runs the ErrorLens server.
//
//	errorlens serve      run migrations, then serve the UI, API and background worker
//	errorlens migrate    apply database migrations and exit
//	errorlens version    print the version
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/errorlens-io/errorlens/internal/alert"
	"github.com/errorlens-io/errorlens/internal/api"
	"github.com/errorlens-io/errorlens/internal/billing"
	"github.com/errorlens-io/errorlens/internal/config"
	"github.com/errorlens-io/errorlens/internal/crypto"
	"github.com/errorlens-io/errorlens/internal/db"
	"github.com/errorlens-io/errorlens/internal/logs"
	"github.com/errorlens-io/errorlens/internal/notify"
	"github.com/errorlens-io/errorlens/internal/process"
	"github.com/errorlens-io/errorlens/internal/queue"
	"github.com/errorlens-io/errorlens/internal/retention"
	"github.com/errorlens-io/errorlens/internal/sourcemap"
	"github.com/errorlens-io/errorlens/internal/usage"
	"github.com/errorlens-io/errorlens/internal/version"
	"github.com/errorlens-io/errorlens/web"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprint(os.Stderr, `Usage: errorlens <command> [flags]

Commands:
  serve     Run the server (applies migrations first unless -no-migrate)
  migrate   Apply database migrations and exit
  version   Print version information
  sourcemaps upload   Upload JavaScript source maps for a release (see --help)
  demo --dsn DSN      Send realistic sample data to a project

Configuration is read from environment variables (see README):
  ERRORLENS_DATABASE_URL   Postgres connection URL (required)
  ERRORLENS_PUBLIC_URL     External URL, used for DSNs and cookies (default http://localhost:8080)
  ERRORLENS_LISTEN_ADDR    Listen address (default :8080)
  ERRORLENS_LOG_LEVEL      debug | info | warn | error (default info)
  ERRORLENS_SESSION_TTL    Login session lifetime (default 336h)
  ERRORLENS_TRUSTED_PROXIES  Comma-separated proxy IPs/CIDRs whose X-Forwarded-For is trusted
  ERRORLENS_SMTP_HOST, _PORT, _USERNAME, _PASSWORD, _FROM   Outgoing email
  ERRORLENS_ALLOW_PRIVATE_WEBHOOKS  Allow alert webhooks to private addresses (default false)
  ERRORLENS_SIGNUP_ENABLED   Let anyone create an organization (hosted mode, default false)
  ERRORLENS_ENCRYPTION_KEY   Base64 32-byte key; encrypts stored event payloads (openssl rand -base64 32)
  ERRORLENS_FREE_EVENT_QUOTA Monthly events for organizations created by sign-up (default 5000)
  ERRORLENS_OIDC_ISSUER, _CLIENT_ID, _CLIENT_SECRET, _NAME, _AUTO_JOIN_ORG, _DEFAULT_ROLE, _ALLOWED_DOMAINS   SSO
  ERRORLENS_STRIPE_SECRET_KEY, _WEBHOOK_SECRET, _PLANS (id:price_id:events,...)   Hosted billing
`)
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return errors.New("missing command")
	}
	switch args[0] {
	case "version", "-v", "--version":
		fmt.Printf("errorlens %s (%s)\n", version.Version, version.Commit)
		return nil
	case "migrate":
		return withDB(func(ctx context.Context, _ *config.Config, d *db.DB, log *slog.Logger) error {
			return migrate(ctx, d, log)
		})
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		noMigrate := fs.Bool("no-migrate", false, "do not apply migrations on startup")
		role := fs.String("role", roleAll, "all | web (HTTP only) | worker (background jobs only)")
		_ = fs.Parse(args[1:])
		if *role != roleAll && *role != roleWeb && *role != roleWorker {
			return fmt.Errorf("--role must be all, web or worker")
		}
		return withDB(func(ctx context.Context, cfg *config.Config, d *db.DB, log *slog.Logger) error {
			if !*noMigrate {
				if err := migrate(ctx, d, log); err != nil {
					return err
				}
			}
			return serve(ctx, cfg, d, log, *role)
		})
	case "sourcemaps":
		return sourcemapsCmd(args[1:])
	case "demo":
		return demoCmd(args[1:])
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func withDB(fn func(context.Context, *config.Config, *db.DB, *slog.Logger) error) error {
	cfg, err := config.Load(nil)
	if err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var d *db.DB
	// Retry briefly so `docker compose up` works while Postgres is starting.
	for attempt := 1; ; attempt++ {
		d, err = db.Open(ctx, cfg.DatabaseURL)
		if err == nil {
			break
		}
		if attempt == 30 || ctx.Err() != nil {
			return fmt.Errorf("database: %w", err)
		}
		log.Warn("waiting for database", "attempt", attempt, "err", err)
		select {
		case <-ctx.Done():
		case <-time.After(2 * time.Second):
		}
	}
	defer d.Close()
	return fn(ctx, cfg, d, log)
}

func migrate(ctx context.Context, d *db.DB, log *slog.Logger) error {
	applied, err := d.Migrate(ctx)
	if err != nil {
		return err
	}
	log.Info("migrations up to date", "applied", applied)
	return nil
}

// Roles let large installs run HTTP servers and background workers separately.
const (
	roleAll    = "all"
	roleWeb    = "web"
	roleWorker = "worker"
)

func serve(ctx context.Context, cfg *config.Config, d *db.DB, log *slog.Logger, role string) error {
	masterKey, err := crypto.ParseMasterKey(cfg.EncryptionKey)
	if err != nil {
		return err
	}
	keyring, err := crypto.NewKeyring(d, masterKey)
	if err != nil {
		return err
	}
	if keyring != nil {
		log.Info("event payload encryption enabled")
	}
	stripe, err := billing.FromEnv(nil)
	if err != nil {
		return err
	}
	oidcCfg := api.OIDCFromEnv(nil)

	var wg sync.WaitGroup
	bg := func(fn func(context.Context)) {
		wg.Add(1)
		go func() { defer wg.Done(); fn(ctx) }()
	}

	if role == roleAll || role == roleWorker {
		worker := queue.NewWorker(d.Pool, log)
		worker.Handle(logs.JobKind, logs.Store(d))
		alerts := &alert.Engine{
			DB:        d,
			Log:       log,
			Sender:    &notify.Sender{HTTP: notify.NewHTTPClient(cfg.AllowPrivateWebhooks), SMTP: cfg.SMTP},
			PublicURL: cfg.PublicURL,
		}
		worker.Handle(process.JobKind, (&process.Processor{
			DB: d, OnOutcome: alerts.OnOutcome, Keyring: keyring, SourceMaps: &sourcemap.Resolver{DB: d},
		}).Handle)
		worker.Handle(alert.EvaluateJob, alerts.Evaluate)
		worker.Handle(alert.DeliverJob, alerts.Deliver)
		registerExtraJobs(worker, d, log, cfg)
		bg(worker.Run)
		bg(func(ctx context.Context) { retention.Run(ctx, d, log) })
		bg(func(ctx context.Context) { runSchedulers(ctx, d, log, cfg) })
		log.Info("background worker started")
	}

	if role == roleWorker {
		<-ctx.Done()
		log.Info("shutting down")
		wg.Wait()
		return nil
	}

	meter := usage.NewMeter(d, log)
	srvAPI := api.New(cfg, d, log, web.FS()).WithKeyring(keyring).WithBilling(stripe).WithOIDC(oidcCfg)
	srvAPI.Ingest().Meter = meter
	registerExtraIngest(srvAPI, d, log, cfg)
	bg(meter.Run)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srvAPI.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("errorlens listening", "addr", cfg.ListenAddr, "public_url", cfg.PublicURL.String(), "role", role, "version", version.Version)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = srv.Shutdown(shutdownCtx)
	wg.Wait()
	return err
}
