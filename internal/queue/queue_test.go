package queue_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/errorlenshq/errorlens/internal/queue"
	"github.com/errorlenshq/errorlens/internal/testutil"
)

func TestEnqueueRunDelete(t *testing.T) {
	d := testutil.NewDB(t)
	ctx := context.Background()
	w := queue.NewWorker(d.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)))

	var got string
	w.Handle("greet", func(_ context.Context, j queue.Job) error {
		got = string(j.Payload)
		return nil
	})
	if _, err := queue.Enqueue(ctx, d.Pool, "greet", map[string]string{"to": "world"}, time.Time{}); err != nil {
		t.Fatal(err)
	}

	worked, err := w.RunOnce(ctx)
	if err != nil || !worked {
		t.Fatalf("worked=%v err=%v", worked, err)
	}
	if got != `{"to": "world"}` {
		t.Fatalf("payload = %s", got)
	}
	var n int
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM jobs`).Scan(&n)
	if n != 0 {
		t.Fatalf("job not deleted, %d remain", n)
	}
	if worked, _ := w.RunOnce(ctx); worked {
		t.Fatal("empty queue reported work")
	}
}

func TestFailedJobIsRetriedLater(t *testing.T) {
	d := testutil.NewDB(t)
	ctx := context.Background()
	w := queue.NewWorker(d.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	var calls atomic.Int32
	w.Handle("flaky", func(context.Context, queue.Job) error {
		calls.Add(1)
		return errors.New("boom")
	})
	w.Handle("panics", func(context.Context, queue.Job) error { panic("kaboom") })

	queue.Enqueue(ctx, d.Pool, "flaky", nil, time.Time{})
	queue.Enqueue(ctx, d.Pool, "panics", nil, time.Time{})
	for i := 0; i < 2; i++ {
		if _, err := w.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	// Both are rescheduled into the future, so nothing is runnable now.
	if worked, _ := w.RunOnce(ctx); worked {
		t.Fatal("failed job should be backed off")
	}
	var attempts int
	var lastErr string
	_ = d.Pool.QueryRow(ctx, `SELECT attempts, last_error FROM jobs WHERE kind = 'panics'`).Scan(&attempts, &lastErr)
	if attempts != 1 || lastErr != "panic: kaboom" {
		t.Fatalf("attempts=%d last_error=%q", attempts, lastErr)
	}
	if calls.Load() != 1 {
		t.Fatalf("flaky called %d times", calls.Load())
	}
}

func TestDelayedJobWaits(t *testing.T) {
	d := testutil.NewDB(t)
	ctx := context.Background()
	w := queue.NewWorker(d.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	w.Handle("later", func(context.Context, queue.Job) error { return nil })
	queue.Enqueue(ctx, d.Pool, "later", nil, time.Now().Add(time.Hour))
	if worked, _ := w.RunOnce(ctx); worked {
		t.Fatal("delayed job ran early")
	}
}

// Run claims jobs in batches, and an idle worker is woken by the enqueue
// notification instead of waiting for its poll interval.
func TestBatchesAndWakeUp(t *testing.T) {
	d := testutil.NewDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := queue.NewWorker(d.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	w.PollInterval = 30 * time.Second // only a notification can wake it in time
	var done atomic.Int64
	w.Handle("count", func(context.Context, queue.Job) error { done.Add(1); return nil })

	for i := 0; i < 95; i++ {
		if _, err := queue.Enqueue(ctx, d.Pool, "count", i, time.Time{}); err != nil {
			t.Fatal(err)
		}
	}
	go w.Run(ctx)
	waitFor := func(n int64) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for done.Load() < n {
			if time.Now().After(deadline) {
				t.Fatalf("processed %d of %d jobs", done.Load(), n)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	waitFor(95)

	// The workers are idle now; a new job must run long before the poll interval.
	time.Sleep(300 * time.Millisecond)
	start := time.Now()
	if _, err := queue.Enqueue(ctx, d.Pool, "count", "late", time.Time{}); err != nil {
		t.Fatal(err)
	}
	waitFor(96)
	if waited := time.Since(start); waited > 2*time.Second {
		t.Fatalf("idle worker took %v to pick up a new job", waited)
	}
	var left int
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM jobs`).Scan(&left)
	if left != 0 {
		t.Fatalf("%d jobs left in the table", left)
	}
}
