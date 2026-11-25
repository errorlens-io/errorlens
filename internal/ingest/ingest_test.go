package ingest_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/errorlenshq/errorlens/internal/db"
	"github.com/errorlenshq/errorlens/internal/ingest"
	"github.com/errorlenshq/errorlens/internal/process"
	"github.com/errorlenshq/errorlens/internal/queue"
	"github.com/errorlenshq/errorlens/internal/testutil"
)

type fixture struct {
	db     *db.DB
	srv    *httptest.Server
	worker *queue.Worker
	org    uuid.UUID
	key    string
	seq    int64
}

func setup(t *testing.T) *fixture {
	t.Helper()
	d := testutil.NewDB(t)
	ctx := context.Background()
	f := &fixture{db: d, org: uuid.New(), key: strings.Repeat("ab", 16)}
	err := d.InScope(ctx, db.Scope{OrgID: f.org}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO organizations (id, name, slug) VALUES ($1, 'Acme', 'acme')`, f.org); err != nil {
			return err
		}
		pid := uuid.New()
		if err := tx.QueryRow(ctx, `INSERT INTO projects (id, org_id, name, slug, platform) VALUES ($1, $2, 'API', 'api', 'python') RETURNING seq`,
			pid, f.org).Scan(&f.seq); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO project_keys (id, org_id, project_id, public_key) VALUES ($1, $2, $3, $4)`, uuid.New(), f.org, pid, f.key)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := chi.NewRouter()
	ingest.NewHandler(d, log).Routes(r)
	f.srv = httptest.NewServer(r)
	t.Cleanup(f.srv.Close)
	f.worker = queue.NewWorker(d.Pool, log)
	f.worker.Handle(process.JobKind, (&process.Processor{DB: d}).Handle)
	return f
}

func (f *fixture) drain(t *testing.T) {
	t.Helper()
	for {
		worked, err := f.worker.RunOnce(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !worked {
			return
		}
	}
}

func eventJSON(id, msg string) string {
	return fmt.Sprintf(`{"event_id":"%s","platform":"python","level":"error",
		"exception":{"values":[{"type":"KeyError","value":"%s","stacktrace":{"frames":[
		  {"filename":"app/views.py","module":"app.views","function":"index","in_app":true}]}}]},
		"request":{"url":"https://x.test/login","headers":{"Authorization":"Bearer supersecrettoken123"},
		           "data":{"password":"hunter2hunter2"}},
		"user":{"ip_address":"203.0.113.9"}}`, id, msg)
}

func envelope(eventID, event string) []byte {
	return []byte(fmt.Sprintf("{\"event_id\":\"%s\",\"sent_at\":\"2026-01-01T00:00:00Z\"}\n{\"type\":\"event\",\"length\":%d}\n%s\n{\"type\":\"client_report\"}\n{\"discarded_events\":[]}\n",
		eventID, len(event), event))
}

func (f *fixture) post(t *testing.T, path string, body []byte, headers map[string]string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest("POST", f.srv.URL+path, bytes.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func authHeader(key string) map[string]string {
	return map[string]string{"X-Sentry-Auth": "Sentry sentry_version=7, sentry_client=test/1.0, sentry_key=" + key}
}

func TestEnvelopeToIssue(t *testing.T) {
	f := setup(t)
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	path := fmt.Sprintf("/api/%d/envelope/", f.seq)

	// gzip-compressed, like most SDKs send it.
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	zw.Write(envelope(id, eventJSON(id, "'user_id'")))
	zw.Close()
	h := authHeader(f.key)
	h["Content-Encoding"] = "gzip"
	resp, body := f.post(t, path, gz.Bytes(), h)
	if resp.StatusCode != 200 || !strings.Contains(body, id) {
		t.Fatalf("envelope: %d %s", resp.StatusCode, body)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("missing CORS header")
	}

	// Before processing, the queued job must already be scrubbed.
	var queued string
	f.db.Pool.QueryRow(context.Background(), `SELECT payload::text FROM jobs`).Scan(&queued)
	for _, secret := range []string{"supersecrettoken123", "hunter2hunter2", "203.0.113.9"} {
		if strings.Contains(queued, secret) {
			t.Fatalf("secret %q reached the job queue: %s", secret, queued)
		}
	}

	f.drain(t)

	// Same error again (different id, via ?sentry_key=) groups into the same issue.
	id2 := strings.ReplaceAll(uuid.NewString(), "-", "")
	resp, body = f.post(t, path+"?sentry_key="+f.key, envelope(id2, eventJSON(id2, "'order_id'")), nil)
	if resp.StatusCode != 200 {
		t.Fatalf("second envelope: %d %s", resp.StatusCode, body)
	}
	// Retried duplicate is ignored.
	f.post(t, path, envelope(id2, eventJSON(id2, "'order_id'")), authHeader(f.key))
	f.drain(t)

	var issues, events, count int64
	var title, payload string
	err := f.db.InScope(context.Background(), db.Scope{OrgID: f.org}, func(tx pgx.Tx) error {
		ctx := context.Background()
		tx.QueryRow(ctx, `SELECT count(*), max(title), max(times_seen) FROM issues`).Scan(&issues, &title, &count)
		tx.QueryRow(ctx, `SELECT count(*), max(payload::text) FROM events`).Scan(&events, &payload)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if issues != 1 || events != 2 || count != 2 {
		t.Fatalf("issues=%d events=%d times_seen=%d", issues, events, count)
	}
	if !strings.HasPrefix(title, "KeyError: ") {
		t.Fatalf("title = %q", title)
	}
	if strings.Contains(payload, "hunter2") || !strings.Contains(payload, "203.0.113.0") {
		t.Fatalf("stored payload not scrubbed as expected: %s", payload)
	}
}

func TestStoreEndpointAndRegression(t *testing.T) {
	f := setup(t)
	path := fmt.Sprintf("/api/%d/store/", f.seq)
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	resp, body := f.post(t, path, []byte(eventJSON(id, "'a'")), authHeader(f.key))
	if resp.StatusCode != 200 {
		t.Fatalf("store: %d %s", resp.StatusCode, body)
	}
	f.drain(t)

	ctx := context.Background()
	f.db.InScope(ctx, db.Scope{OrgID: f.org}, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE issues SET status = 'resolved', resolved_at = now()`)
		return err
	})

	var outcomes []process.Outcome
	p := &process.Processor{DB: f.db, OnOutcome: func(_ context.Context, _ *process.Job, _ *process.Event, o process.Outcome) {
		outcomes = append(outcomes, o)
	}}
	f.worker.Handle(process.JobKind, p.Handle)
	id2 := strings.ReplaceAll(uuid.NewString(), "-", "")
	if resp, body := f.post(t, path, []byte(eventJSON(id2, "'b'")), authHeader(f.key)); resp.StatusCode != 200 {
		t.Fatalf("second store: %d %s", resp.StatusCode, body)
	}
	f.drain(t)

	if len(outcomes) != 1 || !outcomes[0].Regressed || outcomes[0].New {
		t.Fatalf("outcomes = %+v", outcomes)
	}
	var status string
	f.db.InScope(ctx, db.Scope{OrgID: f.org}, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT status FROM issues`).Scan(&status)
	})
	if status != "unresolved" {
		t.Fatalf("regressed issue status = %s", status)
	}
}

func TestAuthFailures(t *testing.T) {
	f := setup(t)
	ev := []byte(eventJSON(strings.Repeat("1", 32), "x"))
	cases := []struct {
		path    string
		headers map[string]string
		want    int
	}{
		{fmt.Sprintf("/api/%d/store/", f.seq), nil, 401},
		{fmt.Sprintf("/api/%d/store/", f.seq), authHeader(strings.Repeat("0", 32)), 401},
		{fmt.Sprintf("/api/%d/store/", f.seq+1), authHeader(f.key), 401}, // key for another project
		{"/api/abc/store/", authHeader(f.key), 400},
	}
	for _, c := range cases {
		if resp, body := f.post(t, c.path, ev, c.headers); resp.StatusCode != c.want {
			t.Errorf("%s: got %d want %d (%s)", c.path, resp.StatusCode, c.want, body)
		}
	}
	// Malformed envelope.
	if resp, _ := f.post(t, fmt.Sprintf("/api/%d/envelope/", f.seq), []byte("not json\n"), authHeader(f.key)); resp.StatusCode != 400 {
		t.Errorf("bad envelope = %d", resp.StatusCode)
	}
}

func TestParseEnvelopeWithoutLength(t *testing.T) {
	env, err := ingest.ParseEnvelope([]byte("{}\n{\"type\":\"event\"}\n{\"message\":\"hi\"}\n{\"type\":\"attachment\",\"length\":3}\nab\n\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(env.Items) != 2 || string(env.Items[0].Payload) != `{"message":"hi"}` || string(env.Items[1].Payload) != "ab\n" {
		var got []string
		for _, it := range env.Items {
			b, _ := json.Marshal(string(it.Payload))
			got = append(got, it.Type+"="+string(b))
		}
		t.Fatalf("items = %v", got)
	}
}
