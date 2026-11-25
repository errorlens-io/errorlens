package main

import (
	"context"
	"log/slog"

	"github.com/rootcausehq/rootcause/internal/api"
	"github.com/rootcausehq/rootcause/internal/config"
	"github.com/rootcausehq/rootcause/internal/db"
	"github.com/rootcausehq/rootcause/internal/queue"
)

// registerExtraJobs wires optional background features (filled in by later phases).
func registerExtraJobs(_ *queue.Worker, _ *db.DB, _ *slog.Logger, _ *config.Config) {}

// registerExtraIngest wires extra envelope item types into ingestion.
func registerExtraIngest(_ *api.Server, _ *db.DB, _ *slog.Logger) {}

// runSchedulers runs periodic jobs until ctx is done.
func runSchedulers(ctx context.Context, _ *db.DB, _ *slog.Logger, _ *config.Config) { <-ctx.Done() }
