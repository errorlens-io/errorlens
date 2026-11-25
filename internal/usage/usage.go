// Package usage meters accepted/dropped events per organization and
// enforces monthly event quotas at ingestion.
package usage

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/rootcausehq/rootcause/internal/db"
)

// Meter batches counters in memory and flushes them to usage_daily.
// Quota checks use a cached view of month-to-date usage that is at most a
// few seconds stale, which is fine for billing-grade soft limits.
type Meter struct {
	DB  *db.DB
	Log *slog.Logger

	mu      sync.Mutex
	pending map[key]*counts
	state   map[uuid.UUID]*orgState
}

type key struct {
	org uuid.UUID
	day string
}

type counts struct{ accepted, dropped, logs int64 }

type orgState struct {
	quota     *int64
	used      int64 // month-to-date accepted, including unflushed
	refreshed time.Time
}

// NewMeter returns a meter; call Run to flush periodically.
func NewMeter(d *db.DB, log *slog.Logger) *Meter {
	return &Meter{DB: d, Log: log, pending: map[key]*counts{}, state: map[uuid.UUID]*orgState{}}
}

func today() string { return time.Now().UTC().Format(time.DateOnly) }

// Allow reports whether org may ingest n more events this month, and
// records them as accepted or dropped accordingly.
func (m *Meter) Allow(ctx context.Context, org uuid.UUID, n int) bool {
	st := m.orgState(ctx, org)
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.counter(org)
	if st.quota != nil && st.used+int64(n) > *st.quota {
		c.dropped += int64(n)
		return false
	}
	st.used += int64(n)
	c.accepted += int64(n)
	return true
}

// Dropped records events rejected for other reasons (e.g. rate limits).
func (m *Meter) Dropped(org uuid.UUID, n int) {
	m.mu.Lock()
	m.counter(org).dropped += int64(n)
	m.mu.Unlock()
}

// Logs records accepted log lines.
func (m *Meter) Logs(org uuid.UUID, n int) {
	m.mu.Lock()
	m.counter(org).logs += int64(n)
	m.mu.Unlock()
}

func (m *Meter) counter(org uuid.UUID) *counts {
	k := key{org, today()}
	c := m.pending[k]
	if c == nil {
		c = &counts{}
		m.pending[k] = c
	}
	return c
}

func (m *Meter) orgState(ctx context.Context, org uuid.UUID) *orgState {
	m.mu.Lock()
	st := m.state[org]
	fresh := st != nil && time.Since(st.refreshed) < 15*time.Second
	m.mu.Unlock()
	if fresh {
		return st
	}
	var quota *int64
	var used int64
	err := m.DB.System(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT monthly_event_quota FROM organizations WHERE id = $1`, org).Scan(&quota); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT COALESCE(sum(events_accepted), 0) FROM usage_daily
			WHERE org_id = $1 AND day >= date_trunc('month', now() AT TIME ZONE 'UTC')::date`, org).Scan(&used)
	})
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		m.Log.Warn("usage: refresh failed", "err", err)
		if st != nil {
			return st
		}
		return &orgState{} // fail open: never lose data because metering is down
	}
	// Add events counted in memory but not yet flushed.
	if c := m.pending[key{org, today()}]; c != nil {
		used += c.accepted
	}
	st = &orgState{quota: quota, used: used, refreshed: time.Now()}
	m.state[org] = st
	return st
}

// Flush writes pending counters.
func (m *Meter) Flush(ctx context.Context) error {
	m.mu.Lock()
	batch := m.pending
	m.pending = map[key]*counts{}
	m.mu.Unlock()
	if len(batch) == 0 {
		return nil
	}
	err := m.DB.System(ctx, func(tx pgx.Tx) error {
		for k, c := range batch {
			if _, err := tx.Exec(ctx, `
				INSERT INTO usage_daily (org_id, day, events_accepted, events_dropped, logs_accepted)
				VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (org_id, day) DO UPDATE SET
					events_accepted = usage_daily.events_accepted + EXCLUDED.events_accepted,
					events_dropped  = usage_daily.events_dropped + EXCLUDED.events_dropped,
					logs_accepted   = usage_daily.logs_accepted + EXCLUDED.logs_accepted`,
				k.org, k.day, c.accepted, c.dropped, c.logs); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		// Put counts back so they are retried.
		m.mu.Lock()
		for k, c := range batch {
			p := m.pending[k]
			if p == nil {
				p = &counts{}
				m.pending[k] = p
			}
			p.accepted += c.accepted
			p.dropped += c.dropped
			p.logs += c.logs
		}
		m.mu.Unlock()
	}
	return err
}

// Run flushes every few seconds until ctx is done, then flushes once more.
func (m *Meter) Run(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = m.Flush(context.Background())
			return
		case <-t.C:
			if err := m.Flush(ctx); err != nil && ctx.Err() == nil {
				m.Log.Error("usage: flush failed", "err", err)
			}
		}
	}
}
