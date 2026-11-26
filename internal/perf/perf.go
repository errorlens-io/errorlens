// Package perf ingests and stores performance data: transactions (a root
// operation such as an HTTP request) and their child spans.
package perf

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	coltrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/errorlenshq/errorlens/internal/db"
	"github.com/errorlenshq/errorlens/internal/queue"
	"github.com/errorlenshq/errorlens/internal/scrub"
)

// JobKind stores a batch of transactions.
const JobKind = "transactions.store"

const maxSpans = 1000

// Span is a timed operation inside a transaction.
type Span struct {
	SpanID      string            `json:"spanId"`
	ParentID    string            `json:"parentId,omitempty"`
	Op          string            `json:"op"`
	Description string            `json:"description"`
	Status      string            `json:"status,omitempty"`
	StartMs     float64           `json:"startMs"` // offset from transaction start
	DurationMs  float64           `json:"durationMs"`
	Data        map[string]string `json:"data,omitempty"`
}

// Transaction is a root operation with its spans.
type Transaction struct {
	ID          uuid.UUID         `json:"id"`
	TraceID     string            `json:"traceId"`
	SpanID      string            `json:"spanId"`
	Name        string            `json:"name"`
	Op          string            `json:"op"`
	Status      string            `json:"status"`
	Start       time.Time         `json:"start"`
	DurationMs  float64           `json:"durationMs"`
	Release     string            `json:"release"`
	Environment string            `json:"environment"`
	Tags        map[string]string `json:"tags"`
	Spans       []Span            `json:"spans"`
}

// Job is the queued batch.
type Job struct {
	OrgID        uuid.UUID     `json:"org_id"`
	ProjectID    uuid.UUID     `json:"project_id"`
	Transactions []Transaction `json:"transactions"`
}

func ts(v any) time.Time {
	switch x := v.(type) {
	case float64:
		sec, frac := math.Modf(x)
		return time.Unix(int64(sec), int64(frac*1e9)).UTC()
	case string:
		t, _ := time.Parse(time.RFC3339Nano, x)
		return t.UTC()
	}
	return time.Time{}
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// FromSDKEvent converts a scrubbed SDK transaction event.
func FromSDKEvent(ev map[string]any) (*Transaction, error) {
	start, end := ts(ev["start_timestamp"]), ts(ev["timestamp"])
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return nil, fmt.Errorf("transaction without valid timestamps")
	}
	trace := map[string]any{}
	if c, ok := ev["contexts"].(map[string]any); ok {
		if t, ok := c["trace"].(map[string]any); ok {
			trace = t
		}
	}
	t := &Transaction{
		TraceID:     str(trace["trace_id"]),
		SpanID:      str(trace["span_id"]),
		Name:        clip(str(ev["transaction"]), 200),
		Op:          clip(str(trace["op"]), 64),
		Status:      str(trace["status"]),
		Start:       start,
		DurationMs:  float64(end.Sub(start).Microseconds()) / 1000,
		Release:     clip(str(ev["release"]), 250),
		Environment: clip(str(ev["environment"]), 64),
		Tags:        map[string]string{},
	}
	if id, err := uuid.Parse(str(ev["event_id"])); err == nil {
		t.ID = id
	} else {
		t.ID = uuid.New()
	}
	if t.Name == "" {
		t.Name = "<unnamed transaction>"
	}
	if t.Status == "" {
		t.Status = "ok"
	}
	if t.Environment == "" {
		t.Environment = "production"
	}
	if tags, ok := ev["tags"].(map[string]any); ok {
		for k, v := range tags {
			if len(t.Tags) < 50 {
				t.Tags[clip(k, 32)] = clip(fmt.Sprint(v), 200)
			}
		}
	}
	spans, _ := ev["spans"].([]any)
	for _, s := range spans {
		m, ok := s.(map[string]any)
		if !ok || len(t.Spans) >= maxSpans {
			continue
		}
		ss, se := ts(m["start_timestamp"]), ts(m["timestamp"])
		if ss.IsZero() || se.IsZero() {
			continue
		}
		sp := Span{
			SpanID: str(m["span_id"]), ParentID: str(m["parent_span_id"]),
			Op: clip(str(m["op"]), 64), Description: clip(str(m["description"]), 1000), Status: str(m["status"]),
			StartMs:    float64(ss.Sub(start).Microseconds()) / 1000,
			DurationMs: float64(se.Sub(ss).Microseconds()) / 1000,
		}
		if data, ok := m["data"].(map[string]any); ok {
			sp.Data = map[string]string{}
			for k, v := range data {
				if len(sp.Data) < 20 {
					sp.Data[clip(k, 64)] = clip(fmt.Sprint(v), 500)
				}
			}
		}
		t.Spans = append(t.Spans, sp)
	}
	sort.Slice(t.Spans, func(i, j int) bool { return t.Spans[i].StartMs < t.Spans[j].StartMs })
	return t, nil
}

// FromOTLP groups OTLP spans into transactions: every span without a parent
// in the batch (a local root or a server/consumer entry span) becomes a
// transaction containing its descendants.
func FromOTLP(req *coltrace.ExportTraceServiceRequest) []*Transaction {
	type node struct {
		s   *tracepb.Span
		res map[string]string
	}
	byID := map[string]node{}
	for _, rs := range req.GetResourceSpans() {
		res := otlpAttrs(rs.GetResource().GetAttributes())
		for _, ss := range rs.GetScopeSpans() {
			for _, s := range ss.GetSpans() {
				byID[hex.EncodeToString(s.GetSpanId())] = node{s, res}
			}
		}
	}
	children := map[string][]string{}
	var roots []string
	for id, n := range byID {
		parent := hex.EncodeToString(n.s.GetParentSpanId())
		_, parentHere := byID[parent]
		kind := n.s.GetKind()
		if parent == "" || !parentHere || kind == tracepb.Span_SPAN_KIND_SERVER || kind == tracepb.Span_SPAN_KIND_CONSUMER {
			roots = append(roots, id)
		} else {
			children[parent] = append(children[parent], id)
		}
	}
	var out []*Transaction
	for _, rid := range roots {
		root := byID[rid]
		start := time.Unix(0, int64(root.s.GetStartTimeUnixNano())).UTC()
		t := &Transaction{
			ID:          uuid.New(),
			TraceID:     hex.EncodeToString(root.s.GetTraceId()),
			SpanID:      rid,
			Name:        clip(root.s.GetName(), 200),
			Op:          strings.ToLower(strings.TrimPrefix(root.s.GetKind().String(), "SPAN_KIND_")),
			Status:      otlpStatus(root.s),
			Start:       start,
			DurationMs:  float64(int64(root.s.GetEndTimeUnixNano())-int64(root.s.GetStartTimeUnixNano())) / 1e6,
			Release:     root.res["service.version"],
			Environment: root.res["deployment.environment.name"],
			Tags:        map[string]string{},
		}
		if t.Environment == "" {
			t.Environment = root.res["deployment.environment"]
		}
		if svc := root.res["service.name"]; svc != "" {
			t.Tags["service"] = svc
		}
		for k, v := range otlpAttrs(root.s.GetAttributes()) {
			if len(t.Tags) < 50 {
				t.Tags[clip(k, 32)] = clip(v, 200)
			}
		}
		// Breadth-first over descendants (excluding nested server spans, which are their own roots).
		queue := append([]string(nil), children[rid]...)
		for len(queue) > 0 && len(t.Spans) < maxSpans {
			id := queue[0]
			queue = queue[1:]
			n := byID[id]
			sp := Span{
				SpanID: id, ParentID: hex.EncodeToString(n.s.GetParentSpanId()),
				Op:          strings.ToLower(strings.TrimPrefix(n.s.GetKind().String(), "SPAN_KIND_")),
				Description: clip(n.s.GetName(), 1000),
				Status:      otlpStatus(n.s),
				StartMs:     float64(int64(n.s.GetStartTimeUnixNano())-int64(root.s.GetStartTimeUnixNano())) / 1e6,
				DurationMs:  float64(int64(n.s.GetEndTimeUnixNano())-int64(n.s.GetStartTimeUnixNano())) / 1e6,
				Data:        otlpAttrs(n.s.GetAttributes()),
			}
			if q, ok := sp.Data["db.statement"]; ok {
				sp.Op, sp.Description = "db", clip(q, 1000)
			}
			t.Spans = append(t.Spans, sp)
			queue = append(queue, children[id]...)
		}
		sort.Slice(t.Spans, func(i, j int) bool { return t.Spans[i].StartMs < t.Spans[j].StartMs })
		out = append(out, t)
	}
	return out
}

func otlpStatus(s *tracepb.Span) string {
	if s.GetStatus().GetCode() == tracepb.Status_STATUS_CODE_ERROR {
		return "internal_error"
	}
	return "ok"
}

func otlpAttrs(kvs []*commonpb.KeyValue) map[string]string {
	m := map[string]string{}
	for _, kv := range kvs {
		v := kv.GetValue()
		switch x := v.GetValue().(type) {
		case *commonpb.AnyValue_StringValue:
			m[kv.GetKey()] = x.StringValue
		case *commonpb.AnyValue_IntValue:
			m[kv.GetKey()] = fmt.Sprint(x.IntValue)
		case *commonpb.AnyValue_DoubleValue:
			m[kv.GetKey()] = fmt.Sprint(x.DoubleValue)
		case *commonpb.AnyValue_BoolValue:
			m[kv.GetKey()] = fmt.Sprint(x.BoolValue)
		}
	}
	return m
}

// Scrub removes secrets from names, descriptions, tags and span data
// (e.g. SQL with inline credentials, tokens in URLs).
func Scrub(t *Transaction, s *scrub.Scrubber) {
	t.Name = s.String(t.Name)
	for k, v := range t.Tags {
		t.Tags[k] = scrubValue(s, k, v)
	}
	for i := range t.Spans {
		t.Spans[i].Description = s.String(t.Spans[i].Description)
		for k, v := range t.Spans[i].Data {
			t.Spans[i].Data[k] = scrubValue(s, k, v)
		}
	}
}

func scrubValue(s *scrub.Scrubber, k, v string) string {
	m := s.Event(map[string]any{"extra": map[string]any{k: v}})
	return fmt.Sprint(m["extra"].(map[string]any)[k])
}

// Enqueue queues scrubbed transactions for storage.
func Enqueue(ctx context.Context, d *db.DB, org, project uuid.UUID, txs []Transaction) error {
	if len(txs) == 0 {
		return nil
	}
	_, err := queue.Enqueue(ctx, d.Pool, JobKind, Job{OrgID: org, ProjectID: project, Transactions: txs}, time.Time{})
	return err
}

// Store is the queue handler. One statement inserts the batch and folds the
// newly stored rows (not duplicates) into the hourly rollup and the daily
// duration histogram that long-period summaries read.
func Store(d *db.DB) queue.Handler {
	return func(ctx context.Context, j queue.Job) error {
		var job Job
		if err := json.Unmarshal(j.Payload, &job); err != nil {
			return err
		}
		n := len(job.Transactions)
		if n == 0 {
			return nil
		}
		var (
			ids                                                   = make([]uuid.UUID, n)
			traces, spanIDs, names, ops, statuses, releases, envs = make([]string, n), make([]string, n), make([]string, n), make([]string, n), make([]string, n), make([]string, n), make([]string, n)
			tagsJSON, spansJSON                                   = make([]string, n), make([]string, n)
			starts                                                = make([]time.Time, n)
			durations                                             = make([]float64, n)
		)
		for i, t := range job.Transactions {
			tags, _ := json.Marshal(t.Tags)
			spans, _ := json.Marshal(t.Spans)
			ids[i], traces[i], spanIDs[i], names[i], ops[i], statuses[i] = t.ID, t.TraceID, t.SpanID, t.Name, t.Op, t.Status
			starts[i], durations[i], releases[i], envs[i] = t.Start, t.DurationMs, t.Release, t.Environment
			tagsJSON[i], spansJSON[i] = string(tags), string(spans)
		}
		return d.InScope(ctx, db.Scope{OrgID: job.OrgID}, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `
				WITH ins AS (
					INSERT INTO transactions (id, org_id, project_id, trace_id, span_id, name, op, status, start_ts,
						duration_ms, release, environment, tags, spans)
					SELECT u.id, $1, $2, u.trace_id, u.span_id, u.name, u.op, u.status, u.start_ts,
						u.duration_ms, u.release, u.environment, u.tags::jsonb, u.spans::jsonb
					FROM unnest($3::uuid[], $4::text[], $5::text[], $6::text[], $7::text[], $8::text[], $9::timestamptz[],
						$10::float8[], $11::text[], $12::text[], $13::text[], $14::text[])
						AS u(id, trace_id, span_id, name, op, status, start_ts, duration_ms, release, environment, tags, spans)
					ON CONFLICT DO NOTHING
					RETURNING name, op, status, start_ts, duration_ms
				), hourly AS (
					INSERT INTO perf_hourly (org_id, project_id, name, hour, op, count, failures, sum_ms)
					SELECT $1, $2, name, date_trunc('hour', start_ts), max(op), count(*),
						count(*) FILTER (WHERE status NOT IN ('ok', 'cancelled', 'unknown')), sum(duration_ms)
					FROM ins GROUP BY 3, 4 ORDER BY 3, 4
					ON CONFLICT (project_id, name, hour) DO UPDATE SET
						count = perf_hourly.count + EXCLUDED.count,
						failures = perf_hourly.failures + EXCLUDED.failures,
						sum_ms = perf_hourly.sum_ms + EXCLUDED.sum_ms,
						op = EXCLUDED.op
				)
				INSERT INTO perf_daily_hist (org_id, project_id, name, day, bucket, count)
				SELECT $1, $2, name, (start_ts AT TIME ZONE 'UTC')::date, perf_bucket(duration_ms), count(*)
				FROM ins GROUP BY 3, 4, 5 ORDER BY 3, 4, 5
				ON CONFLICT (project_id, name, day, bucket) DO UPDATE SET count = perf_daily_hist.count + EXCLUDED.count`,
				job.OrgID, job.ProjectID, ids, traces, spanIDs, names, ops, statuses, starts, durations, releases, envs, tagsJSON, spansJSON)
			return err
		})
	}
}
