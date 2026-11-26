// Package queue is a small Postgres-backed job queue using
// SELECT ... FOR UPDATE SKIP LOCKED, so ErrorLens needs no Kafka or Redis.
package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Job is a unit of background work.
type Job struct {
	ID       int64
	Kind     string
	Payload  json.RawMessage
	Attempts int
}

// Handler processes a job. Returning an error schedules a retry with backoff.
type Handler func(ctx context.Context, job Job) error

// Querier is satisfied by *pgxpool.Pool and pgx.Tx, so jobs can be enqueued
// inside the same transaction as the data they refer to.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Enqueue inserts a job that becomes runnable at runAt (zero = now, using
// the database clock so app/DB clock skew cannot delay jobs).
func Enqueue(ctx context.Context, q Querier, kind string, payload any, runAt time.Time) (int64, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	var at *time.Time
	if !runAt.IsZero() {
		at = &runAt
	}
	var id int64
	// The notification wakes idle workers at once (see Worker.listen);
	// identical notifications in one transaction are delivered only once.
	err = q.QueryRow(ctx, `
		WITH ins AS (INSERT INTO jobs (kind, payload, run_at) VALUES ($1, $2, COALESCE($3, now())) RETURNING id),
		     n AS (SELECT pg_notify('`+notifyChannel+`', ''))
		SELECT ins.id FROM ins, n`,
		kind, body, at).Scan(&id)
	return id, err
}

// notifyChannel is the LISTEN/NOTIFY channel announcing runnable jobs.
const notifyChannel = "errorlens_jobs"

// Worker runs jobs. Idle workers are woken by LISTEN/NOTIFY when a job is
// enqueued, and also poll as a fallback.
type Worker struct {
	pool         *pgxpool.Pool
	log          *slog.Logger
	handlers     map[string]Handler
	PollInterval time.Duration
	LockFor      time.Duration
	Concurrency  int
	// BatchSize is how many jobs one claim takes: fewer round trips under load.
	BatchSize int

	wake chan struct{}
}

// NewWorker returns a worker with sensible defaults.
func NewWorker(pool *pgxpool.Pool, log *slog.Logger) *Worker {
	return &Worker{
		pool:         pool,
		log:          log,
		handlers:     map[string]Handler{},
		PollInterval: 2 * time.Second,
		LockFor:      5 * time.Minute,
		Concurrency:  4,
		BatchSize:    10,
		wake:         make(chan struct{}, 1),
	}
}

// Handle registers the handler for a job kind.
func (w *Worker) Handle(kind string, h Handler) { w.handlers[kind] = h }

// Run processes jobs until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); w.listen(ctx) }()
	for i := 0; i < w.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				n, err := w.runBatch(ctx, w.BatchSize)
				if err != nil && ctx.Err() == nil {
					w.log.Error("queue: run jobs", "err", err)
				}
				if n == 0 {
					select {
					case <-ctx.Done():
					case <-w.wake:
					case <-time.After(w.PollInterval):
					}
				}
			}
		}()
	}
	wg.Wait()
}

// listen turns job notifications into wake-ups for idle workers. If the
// connection drops it reconnects; polling covers the gap.
func (w *Worker) listen(ctx context.Context) {
	for ctx.Err() == nil {
		err := func() error {
			conn, err := w.pool.Acquire(ctx)
			if err != nil {
				return err
			}
			defer conn.Release()
			if _, err := conn.Exec(ctx, "LISTEN "+notifyChannel); err != nil {
				return err
			}
			defer conn.Exec(context.Background(), "UNLISTEN *") //nolint:errcheck
			for {
				if _, err := conn.Conn().WaitForNotification(ctx); err != nil {
					return err
				}
				select {
				case w.wake <- struct{}{}:
				default: // a wake-up is already pending
				}
			}
		}()
		if err != nil && ctx.Err() == nil {
			w.log.Warn("queue: job notifications unavailable, polling", "err", err)
			select {
			case <-ctx.Done():
			case <-time.After(5 * time.Second):
			}
		}
	}
}

// RunOnce claims and runs at most one job. It reports whether a job was found.
func (w *Worker) RunOnce(ctx context.Context) (bool, error) {
	n, err := w.runBatch(ctx, 1)
	return n > 0, err
}

// runBatch claims up to n runnable jobs in one statement, runs them in order
// and deletes the successful ones in one statement. It returns how many jobs
// it claimed.
func (w *Worker) runBatch(ctx context.Context, n int) (int, error) {
	if n < 1 {
		n = 1
	}
	rows, err := w.pool.Query(ctx, `
		UPDATE jobs SET locked_until = now() + $1::interval, attempts = attempts + 1
		WHERE id IN (
			SELECT id FROM jobs
			WHERE run_at <= now() AND attempts < max_attempts
			  AND (locked_until IS NULL OR locked_until < now())
			ORDER BY run_at
			FOR UPDATE SKIP LOCKED
			LIMIT $2)
		RETURNING id, kind, payload, attempts`,
		fmt.Sprintf("%d seconds", int(w.LockFor.Seconds())), n)
	if err != nil {
		return 0, err
	}
	jobs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Job, error) {
		var j Job
		err := r.Scan(&j.ID, &j.Kind, &j.Payload, &j.Attempts)
		return j, err
	})
	if err != nil {
		return 0, err
	}

	var done []int64
	var firstErr error
	for _, job := range jobs {
		h, ok := w.handlers[job.Kind]
		var runErr error
		if !ok {
			runErr = fmt.Errorf("no handler for job kind %q", job.Kind)
		} else {
			runErr = safeRun(ctx, h, job)
		}
		if runErr == nil {
			done = append(done, job.ID)
			continue
		}
		w.log.Warn("queue: job failed", "id", job.ID, "kind", job.Kind, "attempt", job.Attempts, "err", runErr)
		if _, err := w.pool.Exec(ctx,
			`UPDATE jobs SET locked_until = NULL, last_error = $2, run_at = now() + $3::interval WHERE id = $1`,
			job.ID, runErr.Error(), fmt.Sprintf("%d seconds", backoffSeconds(job.Attempts))); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if len(done) > 0 {
		if _, err := w.pool.Exec(ctx, `DELETE FROM jobs WHERE id = ANY($1)`, done); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return len(jobs), firstErr
}

func safeRun(ctx context.Context, h Handler, job Job) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return h(ctx, job)
}

// backoffSeconds returns 2^attempt seconds, capped at one hour.
func backoffSeconds(attempt int) int {
	if attempt > 11 {
		return 3600
	}
	s := 1 << attempt
	if s > 3600 {
		return 3600
	}
	return s
}
