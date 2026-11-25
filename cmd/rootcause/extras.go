package main

import (
	"context"
	"log/slog"

	"github.com/rootcausehq/rootcause/internal/api"
	"github.com/rootcausehq/rootcause/internal/config"
	"github.com/rootcausehq/rootcause/internal/db"
	"github.com/rootcausehq/rootcause/internal/monitor"
	"github.com/rootcausehq/rootcause/internal/notify"
	"github.com/rootcausehq/rootcause/internal/perf"
	"github.com/rootcausehq/rootcause/internal/queue"
)

func monitorService(d *db.DB, log *slog.Logger, cfg *config.Config) *monitor.Service {
	return &monitor.Service{DB: d, Log: log, HTTP: notify.NewHTTPClient(cfg.AllowPrivateWebhooks), PublicURL: cfg.PublicURL}
}

// registerExtraJobs wires performance and monitor background jobs.
func registerExtraJobs(w *queue.Worker, d *db.DB, log *slog.Logger, cfg *config.Config) {
	m := monitorService(d, log, cfg)
	w.Handle(perf.JobKind, perf.Store(d))
	w.Handle(monitor.UptimeJob, m.CheckUptime)
	w.Handle(monitor.NotifyJob, m.Notify(&notify.Sender{HTTP: notify.NewHTTPClient(cfg.AllowPrivateWebhooks), SMTP: cfg.SMTP}))
}

// registerExtraIngest wires cron check-ins into ingestion.
func registerExtraIngest(s *api.Server, d *db.DB, log *slog.Logger, cfg *config.Config) {
	s.Ingest().Monitors = monitorService(d, log, cfg)
}

// runSchedulers runs periodic jobs until ctx is done.
func runSchedulers(ctx context.Context, d *db.DB, log *slog.Logger, cfg *config.Config) {
	monitorService(d, log, cfg).Run(ctx)
}
