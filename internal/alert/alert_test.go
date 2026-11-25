package alert_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/errorlenshq/errorlens/internal/alert"
	"github.com/errorlenshq/errorlens/internal/db"
	"github.com/errorlenshq/errorlens/internal/notify"
	"github.com/errorlenshq/errorlens/internal/process"
	"github.com/errorlenshq/errorlens/internal/queue"
	"github.com/errorlenshq/errorlens/internal/testutil"
)

func TestNewIssueAndRegressionAlerts(t *testing.T) {
	d := testutil.NewDB(t)
	ctx := context.Background()

	var mu sync.Mutex
	var got []map[string]any
	var sigs []string
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		json.Unmarshal(b, &m)
		mu.Lock()
		got = append(got, m)
		sigs = append(sigs, r.Header.Get("X-ErrorLens-Signature"))
		mu.Unlock()
	}))
	defer hook.Close()

	org, project := uuid.New(), uuid.New()
	channel := uuid.New()
	err := d.InScope(ctx, db.Scope{OrgID: org}, func(tx pgx.Tx) error {
		for _, q := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO organizations (id, name, slug) VALUES ($1, 'Acme', 'acme')`, []any{org}},
			{`INSERT INTO projects (id, org_id, name, slug) VALUES ($1, $2, 'API', 'api')`, []any{project, org}},
			{`INSERT INTO alert_channels (id, org_id, kind, name, config) VALUES ($1, $2, 'webhook', 'hook', $3)`,
				[]any{channel, org, map[string]string{"url": hook.URL, "secret": "s3cret"}}},
			{`INSERT INTO alert_rules (id, org_id, name, trigger, channel_ids) VALUES ($1, $2, 'new', 'new_issue', $3)`,
				[]any{uuid.New(), org, []uuid.UUID{channel}}},
			{`INSERT INTO alert_rules (id, org_id, name, trigger, channel_ids) VALUES ($1, $2, 'regressed', 'regression', $3)`,
				[]any{uuid.New(), org, []uuid.UUID{channel}}},
			{`INSERT INTO alert_rules (id, org_id, name, trigger, threshold, window_minutes, channel_ids) VALUES ($1, $2, 'spike', 'frequency', 3, 60, $3)`,
				[]any{uuid.New(), org, []uuid.UUID{channel}}},
		} {
			if _, err := tx.Exec(ctx, q.sql, q.args...); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	pub, _ := url.Parse("https://errors.example.com")
	eng := &alert.Engine{DB: d, Log: log, Sender: &notify.Sender{HTTP: notify.NewHTTPClient(true)}, PublicURL: pub}
	w := queue.NewWorker(d.Pool, log)
	proc := &process.Processor{DB: d, OnOutcome: eng.OnOutcome}
	w.Handle(alert.EvaluateJob, eng.Evaluate)
	w.Handle(alert.DeliverJob, eng.Deliver)
	drain := func() {
		for {
			worked, err := w.RunOnce(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !worked {
				return
			}
		}
	}
	send := func() {
		_, err := proc.Process(ctx, &process.Job{OrgID: org, ProjectID: project, ReceivedAt: time.Now(),
			Event: map[string]any{"message": "disk full on /var", "level": "error"}})
		if err != nil {
			t.Fatal(err)
		}
		drain()
	}

	send() // new issue -> 1 alert
	send()
	send() // third event -> spike (3 in 60m)
	send() // still spiking, but cooldown suppresses it

	reasons := func() []string {
		mu.Lock()
		defer mu.Unlock()
		var out []string
		for _, m := range got {
			out = append(out, m["data"].(map[string]any)["reason"].(string))
		}
		return out
	}
	if r := reasons(); len(r) != 2 || r[0] != "New issue" || !strings.HasPrefix(r[1], "Spike: 3 events") {
		t.Fatalf("reasons = %v", r)
	}
	if !strings.HasPrefix(sigs[0], "sha256=") {
		t.Fatalf("missing signature: %q", sigs[0])
	}
	if u := got[0]["data"].(map[string]any)["url"].(string); !strings.HasPrefix(u, "https://errors.example.com/acme/issues/") {
		t.Fatalf("issue url = %s", u)
	}

	// Resolve then send again -> regression alert, even within cooldown.
	d.InScope(ctx, db.Scope{OrgID: org}, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE issues SET status = 'resolved'`)
		return err
	})
	send()
	if r := reasons(); len(r) != 3 || r[2] != "Regression" {
		t.Fatalf("after regression reasons = %v", r)
	}

	var sent int
	d.InScope(ctx, db.Scope{OrgID: org}, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM alert_history WHERE status = 'sent'`).Scan(&sent)
	})
	if sent != 3 {
		t.Fatalf("history sent = %d", sent)
	}
}

func TestPrivateAddressesBlockedByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	s := &notify.Sender{HTTP: notify.NewHTTPClient(false)}
	err := s.Send(context.Background(), notify.Channel{Kind: "webhook", Config: notify.ChannelConfig{URL: srv.URL}}, notify.Message{})
	if err == nil || !strings.Contains(err.Error(), "private") {
		t.Fatalf("expected private address error, got %v", err)
	}
}
