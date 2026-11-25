// Package queue is a small Postgres-backed job queue using
// SELECT ... FOR UPDATE SKIP LOCKED, so ErrorLens needs no Kafka or Redis.
package queue

import (
	"context"
	"encoding/json"
	"errors"
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
	err = q.QueryRow(ctx,
		`INSERT INTO jobs (kind, payload, run_at) VALUES ($1, $2, COALESCE($3, now())) RETURNING id`,
		kind, body, at).Scan(&id)
	return id, err
}

// Worker polls for runnable jobs and dispatches them to registered handlers.
type Worker struct {
	pool         *pgxpool.Pool
	log          *slog.Logger
	handlers     map[string]Handler
	PollInterval time.Duration
	LockFor      time.Duration
	Concurrency  int
}

// NewWorker returns a worker with sensible defaults.
func NewWorker(pool *pgxpool.Pool, log *slog.Logger) *Worker {
	return &Worker{
		pool:         pool,
		log:          log,
		handlers:     map[string]Handler{},
		PollInterval: 500 * time.Millisecond,
		LockFor:      5 * time.Minute,
		Concurrency:  4,
	}
}

// Handle registers the handler for a job kind.
func (w *Worker) Handle(kind string, h Handler) { w.handlers[kind] = h }

// Run processes jobs until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for i := 0; i < w.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				worked, err := w.RunOnce(ctx)
				if err != nil && ctx.Err() == nil {
					w.log.Error("queue: run job", "err", err)
				}
				if !worked {
					select {
					case <-ctx.Done():
					case <-time.After(w.PollInterval):
					}
				}
			}
		}()
	}
	wg.Wait()
}

// RunOnce claims and runs at most one job. It reports whether a job was found.
func (w *Worker) RunOnce(ctx context.Context) (bool, error) {
	var job Job
	err := pgx.BeginFunc(ctx, w.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			UPDATE jobs SET locked_until = now() + $1::interval, attempts = attempts + 1
			WHERE id = (
				SELECT id FROM jobs
				WHERE run_at <= now() AND attempts < max_attempts
				  AND (locked_until IS NULL OR locked_until < now())
				ORDER BY run_at
				FOR UPDATE SKIP LOCKED
				LIMIT 1)
			RETURNING id, kind, payload, attempts`,
			fmt.Sprintf("%d seconds", int(w.LockFor.Seconds())),
		).Scan(&job.ID, &job.Kind, &job.Payload, &job.Attempts)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	h, ok := w.handlers[job.Kind]
	var runErr error
	if !ok {
		runErr = fmt.Errorf("no handler for job kind %q", job.Kind)
	} else {
		runErr = safeRun(ctx, h, job)
	}

	if runErr == nil {
		_, err = w.pool.Exec(ctx, `DELETE FROM jobs WHERE id = $1`, job.ID)
		return true, err
	}

	w.log.Warn("queue: job failed", "id", job.ID, "kind", job.Kind, "attempt", job.Attempts, "err", runErr)
	_, err = w.pool.Exec(ctx,
		`UPDATE jobs SET locked_until = NULL, last_error = $2, run_at = now() + $3::interval WHERE id = $1`,
		job.ID, runErr.Error(), fmt.Sprintf("%d seconds", backoffSeconds(job.Attempts)))
	return true, err
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
