package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/errorlenshq/errorlens/internal/monitor"
	"github.com/errorlenshq/errorlens/internal/notify"
	"github.com/errorlenshq/errorlens/internal/perf"
)

func TestPerformanceAndTraceView(t *testing.T) {
	a := newApp(t, false)
	a.worker.Handle(perf.JobKind, perf.Store(a.db))
	key, seq := a.bootstrap("python")
	auth := map[string]string{"X-Sentry-Auth": "Sentry sentry_key=" + key}
	trace := "0af7651916cd43dd8448eb211c80319c"
	now := float64(time.Now().Unix())

	for i, dur := range []float64{0.120, 0.080, 0.900} {
		tx := fmt.Sprintf(`{"type":"transaction","event_id":"%032x","transaction":"GET /checkout","start_timestamp":%f,"timestamp":%f,
			"contexts":{"trace":{"trace_id":"%s","span_id":"b7ad6b7169203331","op":"http.server","status":"%s"}},
			"spans":[{"span_id":"c1","parent_span_id":"b7ad6b7169203331","op":"db","description":"SELECT * FROM users WHERE password = 'hunter2'",
			"start_timestamp":%f,"timestamp":%f}]}`,
			i+1, now, now+dur, trace, map[bool]string{true: "internal_error", false: "ok"}[i == 2], now+0.01, now+dur/2)
		env := fmt.Sprintf("{}\n{\"type\":\"transaction\",\"length\":%d}\n%s\n", len(tx), tx)
		if code, b := a.req("POST", "/api/"+seq+"/envelope/", []byte(env), auth); code != 200 {
			t.Fatalf("transaction envelope = %d %s", code, b)
		}
	}
	// An error in the same trace.
	ev := fmt.Sprintf(`{"message":"checkout failed","contexts":{"trace":{"trace_id":"%s","span_id":"b7ad6b7169203331"}}}`, trace)
	a.req("POST", "/api/"+seq+"/store/", []byte(ev), auth)
	// OTLP trace from another service.
	otlp := fmt.Sprintf(`{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"payments"}}]},
		"scopeSpans":[{"spans":[
		{"traceId":"%s","spanId":"1111111111111111","name":"POST /charge","kind":2,"startTimeUnixNano":"%d","endTimeUnixNano":"%d"},
		{"traceId":"%s","spanId":"2222222222222222","parentSpanId":"1111111111111111","name":"stripe.charge","kind":3,"startTimeUnixNano":"%d","endTimeUnixNano":"%d","status":{"code":2}}
		]}]}]}`, b64hex(trace), int64(now*1e9), int64(now*1e9)+50e6, b64hex(trace), int64(now*1e9)+5e6, int64(now*1e9)+40e6)
	if code, b := a.req("POST", "/v1/traces", []byte(otlp), map[string]string{"Content-Type": "application/json", "Authorization": "Bearer " + key}); code != 200 {
		t.Fatalf("otlp traces = %d %s", code, b)
	}
	a.drain()

	_, body := a.req("GET", "/api/0/organizations/acme/performance", nil, nil)
	var sums []struct {
		Name        string
		Count       int
		P95         float64
		FailureRate float64
	}
	json.Unmarshal(body, &sums)
	var checkout *struct {
		Name        string
		Count       int
		P95         float64
		FailureRate float64
	}
	for i := range sums {
		if sums[i].Name == "GET /checkout" {
			checkout = &sums[i]
		}
	}
	if checkout == nil || checkout.Count != 3 || checkout.P95 < 700 || checkout.FailureRate < 0.3 {
		t.Fatalf("performance = %s", body)
	}

	code, body := a.req("GET", "/api/0/organizations/acme/traces/"+trace, nil, nil)
	var tr struct {
		Transactions []struct {
			Name  string
			Spans []perf.Span
		}
		Errors []struct{ Title string }
	}
	json.Unmarshal(body, &tr)
	if code != 200 || len(tr.Transactions) != 4 || len(tr.Errors) != 1 {
		t.Fatalf("trace = %d %s", code, body)
	}
	if strings.Contains(string(body), "hunter2") {
		t.Fatalf("secret in span description: %s", body)
	}
	found := false
	for _, x := range tr.Transactions {
		if x.Name == "POST /charge" && len(x.Spans) == 1 && x.Spans[0].Status == "internal_error" {
			found = true
		}
	}
	if !found {
		t.Fatalf("OTLP transaction missing: %s", body)
	}
}

func b64hex(h string) string {
	// OTLP/JSON encodes ids as hex strings.
	return h
}

func TestCronAndUptimeMonitors(t *testing.T) {
	a := newApp(t, false)
	key, seq := a.bootstrap("python")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	pub, _ := url.Parse(a.srv.URL)
	svc := &monitor.Service{DB: a.db, Log: log, HTTP: notify.NewHTTPClient(true), PublicURL: pub}
	// The ingest handler in newApp has no monitor service; post check-ins through the service directly via a server rebuild.
	a.worker.Handle(monitor.UptimeJob, svc.CheckUptime)

	var mu sync.Mutex
	var alerts []string
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m struct{ Data notify.Message }
		json.NewDecoder(r.Body).Decode(&m)
		mu.Lock()
		alerts = append(alerts, m.Data.Reason)
		mu.Unlock()
	}))
	defer hook.Close()
	a.worker.Handle(monitor.NotifyJob, svc.Notify(&notify.Sender{HTTP: notify.NewHTTPClient(true)}))

	_, b := a.req("POST", "/api/0/organizations/acme/alerts/channels", map[string]any{"kind": "webhook", "name": "hook", "url": hook.URL}, nil)
	var ch struct{ ID string }
	json.Unmarshal(b, &ch)

	// Cron monitor created by the API, then checked in over the plain HTTP endpoint is not wired here,
	// so record check-ins via the service (same code path the endpoints use).
	code, b := a.req("POST", "/api/0/organizations/acme/monitors", map[string]any{"project": "web", "name": "Nightly backup", "kind": "cron",
		"schedule": "0 3 * * *", "channelIds": []string{ch.ID}}, nil)
	if code != 201 {
		t.Fatalf("create cron monitor = %d %s", code, b)
	}
	var orgID, projectID string
	a.db.Pool.QueryRow(context.Background(), `SELECT org_id::text, project_id::text FROM monitors`).Scan(&orgID, &projectID)
	org, project := mustUUID(orgID), mustUUID(projectID)
	if err := svc.RecordCheckIn(context.Background(), org, project, monitor.CheckIn{Slug: "nightly-backup", Status: "ok", Duration: 12}); err != nil {
		t.Fatal(err)
	}
	// SDK upsert: an unknown slug with config creates the monitor.
	cfg := &monitor.MonitorConfig{}
	cfg.Schedule.Type, cfg.Schedule.Value, cfg.Schedule.Unit = "interval", json.RawMessage("10"), "minute"
	if err := svc.RecordCheckIn(context.Background(), org, project, monitor.CheckIn{Slug: "queue-drain", Status: "in_progress", ID: "6f1b0f7a-0000-4000-8000-000000000001", Config: cfg}); err != nil {
		t.Fatal(err)
	}

	// Simulate the backup schedule passing with no check-in.
	a.db.Pool.Exec(context.Background(), `UPDATE monitors SET next_expected_at = now() - interval '1 hour' WHERE slug = 'nightly-backup'`)
	// And the in-progress run overrunning.
	a.db.Pool.Exec(context.Background(), `UPDATE monitor_checkins SET started_at = now() - interval '2 hours' WHERE status = 'in_progress'`)
	if err := svc.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	a.drain()

	statuses := map[string]string{}
	rows, _ := a.db.Pool.Query(context.Background(), `SELECT slug, status FROM monitors`)
	for rows.Next() {
		var s, st string
		rows.Scan(&s, &st)
		statuses[s] = st
	}
	rows.Close()
	if statuses["nightly-backup"] != "missed" || statuses["queue-drain"] != "timeout" {
		t.Fatalf("statuses = %v", statuses)
	}
	mu.Lock()
	if len(alerts) != 1 || alerts[0] != "Missed check-in" {
		t.Fatalf("alerts = %v", alerts)
	}
	mu.Unlock()

	// Uptime: a flaky target goes down then recovers.
	var up = false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up {
			w.WriteHeader(503)
		}
	}))
	defer target.Close()
	code, b = a.req("POST", "/api/0/organizations/acme/monitors", map[string]any{"project": "web", "name": "Homepage", "kind": "uptime",
		"url": target.URL, "intervalSeconds": 30, "channelIds": []string{ch.ID}}, nil)
	if code != 201 {
		t.Fatalf("create uptime monitor = %d %s", code, b)
	}
	svc.Tick(context.Background())
	a.drain()
	up = true
	a.db.Pool.Exec(context.Background(), `UPDATE monitors SET next_expected_at = now() - interval '1 second' WHERE kind = 'uptime'`)
	svc.Tick(context.Background())
	a.drain()

	mu.Lock()
	got := fmt.Sprint(alerts)
	mu.Unlock()
	if got != "[Missed check-in Monitor failing Monitor recovered]" {
		t.Fatalf("alerts = %s", got)
	}
	_, b = a.req("GET", "/api/0/organizations/acme/monitors", nil, nil)
	if !strings.Contains(string(b), `"recent":["ok","error"]`) {
		t.Fatalf("uptime history = %s", b)
	}
	_ = key
	_ = seq
}
