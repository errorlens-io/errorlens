package main

import (
	"context"
	"log/slog"

	"github.com/errorlens-io/errorlens/internal/api"
	"github.com/errorlens-io/errorlens/internal/config"
	"github.com/errorlens-io/errorlens/internal/db"
	"github.com/errorlens-io/errorlens/internal/monitor"
	"github.com/errorlens-io/errorlens/internal/notify"
	"github.com/errorlens-io/errorlens/internal/perf"
	"github.com/errorlens-io/errorlens/internal/queue"
)

func monitorService(d *db.DB, log *slog.Logger, cfg *config.Config) *monitor.Service {
	return &monitor.Service{DB: d, Log: log, HTTP: notify.NewHTTPClient(cfg.AllowPrivateWebhooks), OrgURL: cfg.OrgURL}
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
