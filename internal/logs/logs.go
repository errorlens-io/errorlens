// Package logs parses, stores and queries structured logs.
package logs

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	collogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"

	"github.com/rootcausehq/rootcause/internal/db"
	"github.com/rootcausehq/rootcause/internal/queue"
	"github.com/rootcausehq/rootcause/internal/scrub"
)

// JobKind stores a batch of logs.
const JobKind = "logs.store"

const (
	maxBody   = 16 * 1024
	maxAttrs  = 64
	maxBatch  = 1000
	maxAttrSz = 2048
)

// Record is one log line.
type Record struct {
	Timestamp      time.Time         `json:"ts"`
	Severity       string            `json:"sev"`
	SeverityNumber int               `json:"sevn"`
	Body           string            `json:"body"`
	TraceID        string            `json:"trace,omitempty"`
	Release        string            `json:"rel,omitempty"`
	Environment    string            `json:"env,omitempty"`
	Attributes     map[string]string `json:"attrs,omitempty"`
}

// Job is the queued batch.
type Job struct {
	OrgID     uuid.UUID `json:"org_id"`
	ProjectID uuid.UUID `json:"project_id"`
	Records   []Record  `json:"records"`
}

var severityNumbers = map[string]int{"trace": 1, "debug": 5, "info": 9, "warn": 13, "error": 17, "fatal": 21}

// SeverityFromNumber maps an OpenTelemetry severity number to a name.
func SeverityFromNumber(n int) string {
	switch {
	case n >= 21:
		return "fatal"
	case n >= 17:
		return "error"
	case n >= 13:
		return "warn"
	case n >= 9:
		return "info"
	case n >= 5:
		return "debug"
	case n >= 1:
		return "trace"
	}
	return "info"
}

func normSeverity(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "warning":
		return "warn"
	case "critical", "panic", "fatal":
		return "fatal"
	case "err":
		return "error"
	}
	if _, ok := severityNumbers[s]; ok {
		return s
	}
	return "info"
}

// Clean bounds sizes, fixes timestamps and scrubs secrets. It must run in
// the ingest path before anything is queued.
func Clean(recs []Record, s *scrub.Scrubber, received time.Time) []Record {
	if len(recs) > maxBatch {
		recs = recs[:maxBatch]
	}
	for i := range recs {
		r := &recs[i]
		if r.Timestamp.IsZero() || r.Timestamp.After(received.Add(time.Minute)) || r.Timestamp.Before(received.Add(-30*24*time.Hour)) {
			r.Timestamp = received
		}
		r.Severity = normSeverity(r.Severity)
		if r.SeverityNumber == 0 {
			r.SeverityNumber = severityNumbers[r.Severity]
		}
		r.Body = s.String(clip(r.Body, maxBody))
		clean := map[string]string{}
		for k, v := range r.Attributes {
			if len(clean) >= maxAttrs {
				break
			}
			// Reuse the event scrubber's key rules via a one-field map.
			m := s.Event(map[string]any{"attributes": map[string]any{k: clip(v, maxAttrSz)}})
			clean[clip(k, 128)] = fmt.Sprint(m["attributes"].(map[string]any)[k])
		}
		r.Attributes = clean
	}
	return recs
}

// --- SDK envelope log items ------------------------------------------------------

type sdkLog struct {
	Timestamp  float64                    `json:"timestamp"`
	TraceID    string                     `json:"trace_id"`
	Level      string                     `json:"level"`
	Body       string                     `json:"body"`
	Severity   int                        `json:"severity_number"`
	Attributes map[string]json.RawMessage `json:"attributes"`
}

// ParseEnvelopeItem parses a "log" envelope item: {"items": [...]}.
func ParseEnvelopeItem(payload []byte) ([]Record, error) {
	var wrapper struct {
		Items []sdkLog `json:"items"`
	}
	if err := json.Unmarshal(payload, &wrapper); err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(wrapper.Items))
	for _, l := range wrapper.Items {
		r := Record{Body: l.Body, Severity: l.Level, SeverityNumber: l.Severity, TraceID: l.TraceID, Attributes: map[string]string{}}
		if l.Timestamp > 0 {
			sec, frac := math.Modf(l.Timestamp)
			r.Timestamp = time.Unix(int64(sec), int64(frac*1e9)).UTC()
		}
		for k, raw := range l.Attributes {
			// Attributes are {"value": x, "type": "string"} objects.
			var typed struct {
				Value any `json:"value"`
			}
			if json.Unmarshal(raw, &typed) == nil && typed.Value != nil {
				r.Attributes[k] = fmt.Sprint(typed.Value)
			} else {
				r.Attributes[k] = string(raw)
			}
		}
		r.Release, r.Environment = r.Attributes["sentry.release"], r.Attributes["sentry.environment"]
		for k := range r.Attributes {
			if strings.HasPrefix(k, "sentry.") {
				delete(r.Attributes, k)
			}
		}
		out = append(out, r)
	}
	return out, nil
}

// --- OpenTelemetry (OTLP) --------------------------------------------------------------

// FromOTLP converts an OTLP export request into records.
func FromOTLP(req *collogs.ExportLogsServiceRequest) []Record {
	var out []Record
	for _, rl := range req.GetResourceLogs() {
		res := attrs(rl.GetResource().GetAttributes())
		for _, sl := range rl.GetScopeLogs() {
			for _, lr := range sl.GetLogRecords() {
				r := Record{
					SeverityNumber: int(lr.GetSeverityNumber()),
					Body:           anyString(lr.GetBody()),
					Attributes:     attrs(lr.GetAttributes()),
				}
				if r.SeverityNumber > 0 {
					r.Severity = SeverityFromNumber(r.SeverityNumber)
				} else {
					r.Severity = lr.GetSeverityText()
				}
				ts := lr.GetTimeUnixNano()
				if ts == 0 {
					ts = lr.GetObservedTimeUnixNano()
				}
				if ts > 0 {
					r.Timestamp = time.Unix(0, int64(ts)).UTC()
				}
				if id := lr.GetTraceId(); len(id) > 0 {
					r.TraceID = hex.EncodeToString(id)
				}
				r.Release = res["service.version"]
				r.Environment = res["deployment.environment.name"]
				if r.Environment == "" {
					r.Environment = res["deployment.environment"]
				}
				if svc := res["service.name"]; svc != "" {
					r.Attributes["service.name"] = svc
				}
				out = append(out, r)
			}
		}
	}
	return out
}

func attrs(kvs []*commonpb.KeyValue) map[string]string {
	m := map[string]string{}
	for _, kv := range kvs {
		m[kv.GetKey()] = anyString(kv.GetValue())
	}
	return m
}

func anyString(v *commonpb.AnyValue) string {
	if v == nil {
		return ""
	}
	switch x := v.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		return x.StringValue
	case *commonpb.AnyValue_BoolValue:
		return fmt.Sprint(x.BoolValue)
	case *commonpb.AnyValue_IntValue:
		return fmt.Sprint(x.IntValue)
	case *commonpb.AnyValue_DoubleValue:
		return fmt.Sprint(x.DoubleValue)
	case *commonpb.AnyValue_BytesValue:
		return hex.EncodeToString(x.BytesValue)
	case *commonpb.AnyValue_ArrayValue:
		parts := make([]string, 0, len(x.ArrayValue.GetValues()))
		for _, e := range x.ArrayValue.GetValues() {
			parts = append(parts, anyString(e))
		}
		b, _ := json.Marshal(parts)
		return string(b)
	case *commonpb.AnyValue_KvlistValue:
		b, _ := json.Marshal(attrs(x.KvlistValue.GetValues()))
		return string(b)
	}
	return ""
}

// --- storage ---------------------------------------------------------------------------------

// Enqueue queues already-cleaned records for storage.
func Enqueue(ctx context.Context, d *db.DB, org, project uuid.UUID, recs []Record) error {
	if len(recs) == 0 {
		return nil
	}
	_, err := queue.Enqueue(ctx, d.Pool, JobKind, Job{OrgID: org, ProjectID: project, Records: recs}, time.Time{})
	return err
}

// Store is the queue handler that writes a batch.
func Store(d *db.DB) queue.Handler {
	return func(ctx context.Context, j queue.Job) error {
		var job Job
		if err := json.Unmarshal(j.Payload, &job); err != nil {
			return err
		}
		return d.InScope(ctx, db.Scope{OrgID: job.OrgID}, func(tx pgx.Tx) error {
			b := &pgx.Batch{}
			for _, r := range job.Records {
				a, _ := json.Marshal(r.Attributes)
				b.Queue(`INSERT INTO logs (org_id, project_id, timestamp, severity, severity_number, body, trace_id, release, environment, attributes)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
					job.OrgID, job.ProjectID, r.Timestamp, r.Severity, r.SeverityNumber, r.Body, r.TraceID, r.Release, r.Environment, a)
			}
			return tx.SendBatch(ctx, b).Close()
		})
	}
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n] + "…"
}
