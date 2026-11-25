// Package retention maintains daily event partitions and deletes data past
// each project's retention period.
package retention

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/errorlens-io/errorlens/internal/db"
)

const (
	daysAhead = 7
	lockID    = 7_316_105_119
)

// Run performs maintenance immediately and then hourly until ctx is done.
func Run(ctx context.Context, d *db.DB, log *slog.Logger) {
	for {
		if err := Once(ctx, d, time.Now().UTC()); err != nil && ctx.Err() == nil {
			log.Error("retention: maintenance failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Hour):
		}
	}
}

func partitionName(table string, day time.Time) string { return table + "_p" + day.Format("20060102") }

// Partitioned tables maintained here (partitioned by day on their time column).
var tables = []string{"events", "logs", "transactions"}

// Once creates upcoming partitions, drops expired ones and deletes expired
// rows for projects with shorter retention. Safe to run on several replicas.
func Once(ctx context.Context, d *db.DB, now time.Time) error {
	return d.System(ctx, func(tx pgx.Tx) error {
		var got bool
		if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock($1)`, lockID).Scan(&got); err != nil || !got {
			return err // another replica is doing it
		}
		today := now.Truncate(24 * time.Hour)

		for _, table := range tables {
			for i := 0; i <= daysAhead; i++ {
				day := today.AddDate(0, 0, i)
				name := partitionName(table, day)
				var exists bool
				if err := tx.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, name).Scan(&exists); err != nil {
					return err
				}
				if exists {
					continue
				}
				// If rows for this day already landed in the default partition, skip
				// it rather than fail; they are still queryable and age out below.
				_, err := tx.Exec(ctx, `SAVEPOINT part`)
				if err != nil {
					return err
				}
				_, err = tx.Exec(ctx, fmt.Sprintf(`CREATE TABLE %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')`,
					name, table, day.Format(time.DateOnly), day.AddDate(0, 0, 1).Format(time.DateOnly)))
				if err != nil {
					if _, rbErr := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT part`); rbErr != nil {
						return rbErr
					}
					continue
				}
				if _, err := tx.Exec(ctx, `RELEASE SAVEPOINT part`); err != nil {
					return err
				}
			}

		}
		// Drop whole partitions older than the longest retention of any project.
		var maxDays int
		if err := tx.QueryRow(ctx, `SELECT COALESCE(max(retention_days), 30) FROM projects`).Scan(&maxDays); err != nil {
			return err
		}
		cutoff := today.AddDate(0, 0, -maxDays)
		rows, err := tx.Query(ctx, `
			SELECT c.relname FROM pg_inherits i
			JOIN pg_class c ON c.oid = i.inhrelid
			JOIN pg_class p ON p.oid = i.inhparent
			WHERE p.relname = ANY($1) AND c.relname ~ '^(events|logs|transactions)_p[0-9]{8}$'`, tables)
		if err != nil {
			return err
		}
		names, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		for _, n := range names {
			day, err := time.Parse("20060102", n[strings.LastIndex(n, "_p")+2:])
			if err != nil || !day.AddDate(0, 0, 1).Before(cutoff.Add(time.Nanosecond)) {
				continue
			}
			if _, err := tx.Exec(ctx, `DROP TABLE `+pgx.Identifier{n}.Sanitize()); err != nil {
				return err
			}
		}

		// Per-project retention for rows still inside kept partitions.
		if _, err := tx.Exec(ctx, `
			DELETE FROM events e USING projects p
			WHERE e.project_id = p.id AND e.received_at < $1::timestamptz - make_interval(days => p.retention_days)`, now); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM issues i USING projects p
			WHERE i.project_id = p.id AND i.last_seen < $1::timestamptz - make_interval(days => p.retention_days)`, now); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM logs l USING projects p
			WHERE l.project_id = p.id AND l.timestamp < $1::timestamptz - make_interval(days => p.retention_days)`, now); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM transactions t USING projects p
			WHERE t.project_id = p.id AND t.start_ts < $1::timestamptz - make_interval(days => p.retention_days)`, now); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM monitor_checkins WHERE started_at < $1::timestamptz - interval '90 days'`, now); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM issue_counts WHERE bucket < $1::timestamptz - make_interval(days => $2)`, now, maxDays)
		return err
	})
}
