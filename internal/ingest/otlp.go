package ingest

import (
	"net/http"
	"strings"
	"time"

	collogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/rootcausehq/rootcause/internal/logs"
	"github.com/rootcausehq/rootcause/internal/scrub"
)

// keyOnly authenticates OTLP requests, which carry no project id in the path.
func (h *Handler) keyOnly(r *http.Request) (*projectKey, int, string) {
	public := strings.TrimSpace(r.Header.Get("x-rootcause-key"))
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
		err = protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(body, req)
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
