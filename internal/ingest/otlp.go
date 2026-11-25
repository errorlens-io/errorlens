package ingest

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	collogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	coltrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/errorlenshq/errorlens/internal/logs"
	"github.com/errorlenshq/errorlens/internal/monitor"
	"github.com/errorlenshq/errorlens/internal/perf"
	"github.com/errorlenshq/errorlens/internal/scrub"
)

// keyOnly authenticates OTLP requests, which carry no project id in the path.
func (h *Handler) keyOnly(r *http.Request) (*projectKey, int, string) {
	public := strings.TrimSpace(r.Header.Get("x-errorlens-key"))
	if public == "" {
		if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") {
			public = strings.TrimSpace(strings.TrimPrefix(a, "Bearer "))
		}
	}
	if public == "" {
		return nil, http.StatusUnauthorized, "missing key"
	}
	key, err := h.lookup(r.Context(), public)
	if err != nil {
		return nil, http.StatusServiceUnavailable, "temporarily unavailable"
	}
	if key == nil {
		return nil, http.StatusUnauthorized, "invalid key"
	}
	return key, 0, ""
}

func (h *Handler) otlpLogs(w http.ResponseWriter, r *http.Request) {
	key, status, msg := h.keyOnly(r)
	if key == nil {
		fail(w, status, msg)
		return
	}
	body, err := readBody(w, r)
	if err != nil {
		fail(w, statusFor(err), err.Error())
		return
	}
	req := &collogs.ExportLogsServiceRequest{}
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/json") {
		err = protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(otlpJSONIDs(body), req)
	} else {
		err = proto.Unmarshal(body, req)
	}
	if err != nil {
		fail(w, http.StatusBadRequest, "invalid OTLP payload")
		return
	}
	recs := logs.FromOTLP(req)
	if !h.allow(w, key, (len(recs)+99)/100) { // count 100 log lines as one event
		return
	}
	recs = logs.Clean(recs, scrub.New(key.Scrub), time.Now().UTC())
	if h.Meter != nil {
		h.Meter.Logs(key.OrgID, len(recs))
	}
	if err := logs.Enqueue(r.Context(), h.DB, key.OrgID, key.ProjectID, recs); err != nil {
		h.Log.Error("ingest: enqueue otlp logs", "err", err)
		fail(w, http.StatusServiceUnavailable, "temporarily unavailable")
		return
	}
	resp := &collogs.ExportLogsServiceResponse{}
	if strings.HasPrefix(ct, "application/json") {
		b, _ := protojson.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
		return
	}
	b, _ := proto.Marshal(resp)
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.Write(b)
}

func (h *Handler) otlpTraces(w http.ResponseWriter, r *http.Request) {
	key, status, msg := h.keyOnly(r)
	if key == nil {
		fail(w, status, msg)
		return
	}
	body, err := readBody(w, r)
	if err != nil {
		fail(w, statusFor(err), err.Error())
		return
	}
	req := &coltrace.ExportTraceServiceRequest{}
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/json") {
		err = protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(otlpJSONIDs(body), req)
	} else {
		err = proto.Unmarshal(body, req)
	}
	if err != nil {
		fail(w, http.StatusBadRequest, "invalid OTLP payload")
		return
	}
	txs := perf.FromOTLP(req)
	if !h.allow(w, key, 0) {
		return
	}
	out := make([]perf.Transaction, 0, len(txs))
	sc := scrub.New(key.Scrub)
	for _, t := range txs {
		perf.Scrub(t, sc)
		out = append(out, *t)
	}
	if err := perf.Enqueue(r.Context(), h.DB, key.OrgID, key.ProjectID, out); err != nil {
		h.Log.Error("ingest: enqueue otlp traces", "err", err)
		fail(w, http.StatusServiceUnavailable, "temporarily unavailable")
		return
	}
	resp := &coltrace.ExportTraceServiceResponse{}
	if strings.HasPrefix(ct, "application/json") {
		b, _ := protojson.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
		return
	}
	b, _ := proto.Marshal(resp)
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.Write(b)
}

// checkin accepts a cron check-in via a plain HTTP call.
func (h *Handler) checkin(w http.ResponseWriter, r *http.Request) {
	if h.Monitors == nil {
		fail(w, http.StatusNotFound, "monitors are not enabled")
		return
	}
	q := r.URL.Query()
	key, status, msg := h.authenticate(r, q.Get("key"))
	if key == nil {
		fail(w, status, msg)
		return
	}
	c := monitor.CheckIn{Slug: chi.URLParam(r, "slug"), Status: q.Get("status"), ID: q.Get("check_in_id"), Environment: q.Get("environment")}
	if c.Status == "" {
		c.Status = "ok"
	}
	if d, err := strconv.ParseFloat(q.Get("duration"), 64); err == nil {
		c.Duration = d
	}
	if err := h.Monitors.RecordCheckIn(r.Context(), key.OrgID, key.ProjectID, c); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

// otlpJSONIDs converts trace/span ids from the hex form the OTLP/JSON spec
// uses to the base64 form the generic protobuf JSON decoder expects.
func otlpJSONIDs(body []byte) []byte {
	var v any
	if json.Unmarshal(body, &v) != nil {
		return body
	}
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case map[string]any:
			for k, val := range t {
				if s, ok := val.(string); ok && (k == "traceId" || k == "spanId" || k == "parentSpanId") {
					if b, err := hex.DecodeString(s); err == nil && (len(b) == 8 || len(b) == 16) {
						t[k] = base64.StdEncoding.EncodeToString(b)
					}
					continue
				}
				walk(val)
			}
		case []any:
			for _, e := range t {
				walk(e)
			}
		}
	}
	walk(v)
	out, err := json.Marshal(v)
	if err != nil {
		return body
	}
	return out
}
