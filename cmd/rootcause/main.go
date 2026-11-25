// Command rootcause runs the RootCause server.
//
//	rootcause serve      run migrations, then serve the UI, API and background worker
//	rootcause migrate    apply database migrations and exit
//	rootcause version    print the version
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
	"syscall"
	"time"

	"github.com/rootcausehq/rootcause/internal/alert"
	"github.com/rootcausehq/rootcause/internal/api"
	"github.com/rootcausehq/rootcause/internal/config"
	"github.com/rootcausehq/rootcause/internal/crypto"
	"github.com/rootcausehq/rootcause/internal/db"
	"github.com/rootcausehq/rootcause/internal/logs"
	"github.com/rootcausehq/rootcause/internal/notify"
	"github.com/rootcausehq/rootcause/internal/process"
	"github.com/rootcausehq/rootcause/internal/queue"
	"github.com/rootcausehq/rootcause/internal/retention"
	"github.com/rootcausehq/rootcause/internal/sourcemap"
	"github.com/rootcausehq/rootcause/internal/version"
	"github.com/rootcausehq/rootcause/web"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `Usage: rootcause <command> [flags]

Commands:
  serve     Run the server (applies migrations first unless -no-migrate)
  migrate   Apply database migrations and exit
  version   Print version information
  sourcemaps upload   Upload JavaScript source maps for a release (see --help)

Configuration is read from environment variables (see README):
  ROOTCAUSE_DATABASE_URL   Postgres connection URL (required)
  ROOTCAUSE_PUBLIC_URL     External URL, used for DSNs and cookies (default http://localhost:8080)
  ROOTCAUSE_LISTEN_ADDR    Listen address (default :8080)
  ROOTCAUSE_LOG_LEVEL      debug | info | warn | error (default info)
  ROOTCAUSE_SESSION_TTL    Login session lifetime (default 336h)
  ROOTCAUSE_TRUSTED_PROXIES  Comma-separated proxy IPs/CIDRs whose X-Forwarded-For is trusted
  ROOTCAUSE_SMTP_HOST, _PORT, _USERNAME, _PASSWORD, _FROM   Outgoing email
  ROOTCAUSE_ALLOW_PRIVATE_WEBHOOKS  Allow alert webhooks to private addresses (default false)
  ROOTCAUSE_SIGNUP_ENABLED   Let anyone create an organization (hosted mode, default false)
  ROOTCAUSE_ENCRYPTION_KEY   Base64 32-byte key; encrypts stored event payloads (openssl rand -base64 32)
`)
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return errors.New("missing command")
	}
	switch args[0] {
	case "version", "-v", "--version":
		fmt.Printf("rootcause %s (%s)\n", version.Version, version.Commit)
		return nil
	case "migrate":
		return withDB(func(ctx context.Context, _ *config.Config, d *db.DB, log *slog.Logger) error {
			return migrate(ctx, d, log)
		})
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		noMigrate := fs.Bool("no-migrate", false, "do not apply migrations on startup")
		_ = fs.Parse(args[1:])
		return withDB(func(ctx context.Context, cfg *config.Config, d *db.DB, log *slog.Logger) error {
			if !*noMigrate {
				if err := migrate(ctx, d, log); err != nil {
					return err
				}
			}
			return serve(ctx, cfg, d, log)
		})
	case "sourcemaps":
		return sourcemapsCmd(args[1:])
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		usage()
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

func serve(ctx context.Context, cfg *config.Config, d *db.DB, log *slog.Logger) error {
	srvAPI := api.New(cfg, d, log, web.FS())
	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srvAPI.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

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
	srvAPI.WithKeyring(keyring)

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
	workerDone := make(chan struct{})
	go func() { worker.Run(ctx); close(workerDone) }()

	// Daily event partitions and retention clean-up.
	maintCtx, stopMaint := context.WithCancel(ctx)
	maintDone := make(chan struct{})
	go func() { retention.Run(maintCtx, d, log); close(maintDone) }()
	defer func() { stopMaint(); <-maintDone }()

	errCh := make(chan error, 1)
	go func() {
		log.Info("rootcause listening", "addr", cfg.ListenAddr, "public_url", cfg.PublicURL.String(), "version", version.Version)
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
	<-workerDone
	return err
}
