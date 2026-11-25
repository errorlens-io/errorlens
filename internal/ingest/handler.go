package ingest

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/rootcausehq/rootcause/internal/auth"
	"github.com/rootcausehq/rootcause/internal/db"
	"github.com/rootcausehq/rootcause/internal/logs"
	"github.com/rootcausehq/rootcause/internal/monitor"
	"github.com/rootcausehq/rootcause/internal/perf"
	"github.com/rootcausehq/rootcause/internal/process"
	"github.com/rootcausehq/rootcause/internal/queue"
	"github.com/rootcausehq/rootcause/internal/scrub"
	"github.com/rootcausehq/rootcause/internal/usage"
)

// Size limits (SDKs keep events well below these).
const (
	maxCompressed   = 20 << 20
	maxDecompressed = 40 << 20
	maxEventSize    = 1 << 20
)

// Handler serves the ingestion endpoints.
type Handler struct {
	DB  *db.DB
	Log *slog.Logger
	// Per-key rate limit: Burst events, refilling one every Every.
	limiter *auth.Limiter

	mu    sync.Mutex
	cache map[string]cachedKey // public key -> project

	// Monitors, when set, receives cron check-ins.
	Monitors *monitor.Service

	// Meter, when set, enforces monthly quotas and records usage.
	Meter *usage.Meter

	// Extra handles additional envelope item types (e.g. transactions,
	// check-ins). It returns true if it consumed the item.
	Extra func(ctx context.Context, org, project uuid.UUID, sc scrub.Config, it Item) bool
}

type projectKey struct {
	OrgID      uuid.UUID
	ProjectID  uuid.UUID
	ProjectSeq int64
	Scrub      scrub.Config
}

type cachedKey struct {
	key     *projectKey // nil = unknown/inactive key
	expires time.Time
}

// NewHandler returns an ingest handler. Rate limit defaults to a sustained
// 100 events/s with bursts of 1000 per DSN key.
func NewHandler(d *db.DB, log *slog.Logger) *Handler {
	return &Handler{
		DB:      d,
		Log:     log,
		limiter: auth.NewLimiter(10*time.Millisecond, 1000),
		cache:   map[string]cachedKey{},
	}
}

// Routes mounts the ingest endpoints on r.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(cors)
		r.Options("/api/{projectID}/envelope/", noContent)
		r.Options("/api/{projectID}/store/", noContent)
		r.Post("/api/{projectID}/envelope/", h.envelope)
		r.Post("/api/{projectID}/store/", h.store)
		// OpenTelemetry logs (OTLP/HTTP, protobuf or JSON). Authenticate with the
		// DSN public key: "Authorization: Bearer <key>" or "x-rootcause-key: <key>".
		r.Options("/v1/logs", noContent)
		r.Post("/v1/logs", h.otlpLogs)
		r.Options("/v1/traces", noContent)
		r.Post("/v1/traces", h.otlpTraces)
		// Simple cron check-ins for shell scripts:
		//   curl "https://host/api/1/monitors/nightly-backup/checkin?key=<dsn key>&status=ok"
		r.Get("/api/{projectID}/monitors/{slug}/checkin", h.checkin)
		r.Post("/api/{projectID}/monitors/{slug}/checkin", h.checkin)
	})
}

// Browser SDKs post cross-origin. The DSN key is public by design, so any
// origin may submit; per-project origin allow-lists come later.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("Access-Control-Allow-Origin", "*")
		hd.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		hd.Set("Access-Control-Allow-Headers", "Content-Type, Content-Encoding, "+hdrAuth+", Authorization, "+hdrTrace+", baggage")
		hd.Set("Access-Control-Expose-Headers", hdrError+", "+hdrRateLimits+", Retry-After")
		hd.Set("Access-Control-Max-Age", "3600")
		next.ServeHTTP(w, r)
	})
}

func noContent(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

func fail(w http.ResponseWriter, status int, msg string) {
	w.Header().Set(hdrError, msg)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"detail": msg})
}

func ok(w http.ResponseWriter, eventID string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"id": eventID})
}

// --- endpoints ---------------------------------------------------------------

func (h *Handler) envelope(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(w, r)
	if err != nil {
		fail(w, statusFor(err), err.Error())
		return
	}
	env, err := ParseEnvelope(body)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	dsnKey := ""
	if dsn, _ := env.Header["dsn"].(string); dsn != "" {
		if u, err := url.Parse(dsn); err == nil && u.User != nil {
			dsnKey = u.User.Username()
		}
	}
	key, status, msg := h.authenticate(r, dsnKey)
	if key == nil {
		fail(w, status, msg)
		return
	}

	var events []map[string]any
	var logRecords []logs.Record
	var transactions []perf.Transaction
	for _, it := range env.Items {
		switch it.Type {
		case "event":
			if len(it.Payload) > maxEventSize {
				h.Log.Debug("ingest: dropping oversized event", "size", len(it.Payload))
				continue
			}
			var ev map[string]any
			if err := json.Unmarshal(it.Payload, &ev); err != nil {
				fail(w, http.StatusBadRequest, "invalid event JSON")
				return
			}
			events = append(events, ev)
		case "transaction":
			var ev map[string]any
			if len(it.Payload) > 4*maxEventSize || json.Unmarshal(it.Payload, &ev) != nil {
				continue
			}
			ev = scrub.New(key.Scrub).Event(ev)
			if t, err := perf.FromSDKEvent(ev); err == nil {
				perf.Scrub(t, scrub.New(key.Scrub))
				transactions = append(transactions, *t)
			}
		case "check_in":
			var c monitor.CheckIn
			if h.Monitors != nil && json.Unmarshal(it.Payload, &c) == nil {
				if err := h.Monitors.RecordCheckIn(r.Context(), key.OrgID, key.ProjectID, c); err != nil {
					h.Log.Debug("ingest: check-in rejected", "err", err)
				}
			}
		case "log":
			recs, err := logs.ParseEnvelopeItem(it.Payload)
			if err != nil {
				h.Log.Debug("ingest: bad log item", "err", err)
				continue
			}
			logRecords = append(logRecords, recs...)
		default:
			if h.Extra != nil && h.Extra(r.Context(), key.OrgID, key.ProjectID, key.Scrub, it) {
				continue
			}
			// session(s), attachment, client_report, replay, profile …:
			// accepted but not stored, so SDKs don't retry.
		}
	}
	if !h.allow(w, key, len(events)) {
		return
	}
	if len(transactions) > 0 {
		if err := perf.Enqueue(r.Context(), h.DB, key.OrgID, key.ProjectID, transactions); err != nil {
			h.Log.Error("ingest: enqueue transactions", "err", err)
			fail(w, http.StatusServiceUnavailable, "temporarily unavailable")
			return
		}
	}
	if len(logRecords) > 0 {
		recs := logs.Clean(logRecords, scrub.New(key.Scrub), time.Now().UTC())
		if h.Meter != nil {
			h.Meter.Logs(key.OrgID, len(recs))
		}
		if err := logs.Enqueue(r.Context(), h.DB, key.OrgID, key.ProjectID, recs); err != nil {
			h.Log.Error("ingest: enqueue logs", "err", err)
			fail(w, http.StatusServiceUnavailable, "temporarily unavailable")
			return
		}
	}
	eventID, _ := env.Header["event_id"].(string)
	for _, ev := range events {
		id, err := h.enqueue(r.Context(), key, ev)
		if err != nil {
			h.Log.Error("ingest: enqueue", "err", err)
			fail(w, http.StatusServiceUnavailable, "temporarily unavailable")
			return
		}
		if eventID == "" {
			eventID = id
		}
	}
	ok(w, eventID)
}

// store is the legacy single-event endpoint used by older SDKs.
func (h *Handler) store(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(w, r)
	if err != nil {
		fail(w, statusFor(err), err.Error())
		return
	}
	key, status, msg := h.authenticate(r, "")
	if key == nil {
		fail(w, status, msg)
		return
	}
	// Very old SDKs send base64(zlib(json)).
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] != '{' {
		if dec, err := base64.StdEncoding.DecodeString(string(trimmed)); err == nil {
			if zr, err := zlib.NewReader(bytes.NewReader(dec)); err == nil {
				if out, err := io.ReadAll(io.LimitReader(zr, maxEventSize+1)); err == nil {
					trimmed = out
				}
			}
		}
	}
	if len(trimmed) > maxEventSize {
		fail(w, http.StatusRequestEntityTooLarge, "event too large")
		return
	}
	var ev map[string]any
	if err := json.Unmarshal(trimmed, &ev); err != nil {
		fail(w, http.StatusBadRequest, "invalid event JSON")
		return
	}
	if !h.allow(w, key, 1) {
		return
	}
	id, err := h.enqueue(r.Context(), key, ev)
	if err != nil {
		h.Log.Error("ingest: enqueue", "err", err)
		fail(w, http.StatusServiceUnavailable, "temporarily unavailable")
		return
	}
	ok(w, id)
}

// allow applies the per-key rate limit; SDKs back off when they see the rate-limit header.
func (h *Handler) allow(w http.ResponseWriter, key *projectKey, n int) bool {
	if n == 0 {
		return true
	}
	for i := 0; i < n; i++ {
		if !h.limiter.Allow(key.ProjectID.String()) {
			if h.Meter != nil {
				h.Meter.Dropped(key.OrgID, n)
			}
			w.Header().Set("Retry-After", "1")
			w.Header().Set(hdrRateLimits, "1::key")
			fail(w, http.StatusTooManyRequests, "rate limit exceeded")
			return false
		}
	}
	if h.Meter != nil && !h.Meter.Allow(context.Background(), key.OrgID, n) {
		// Monthly quota reached: tell SDKs to back off for an hour.
		w.Header().Set("Retry-After", "3600")
		w.Header().Set(hdrRateLimits, "3600::organization:quota_exceeded")
		fail(w, http.StatusTooManyRequests, "monthly event quota exceeded")
		return false
	}
	return true
}

// enqueue scrubs the event and queues it for processing. The unscrubbed
// event never leaves this function.
func (h *Handler) enqueue(ctx context.Context, key *projectKey, ev map[string]any) (string, error) {
	s := scrub.New(key.Scrub)
	ev = s.Event(ev)
	id, _ := ev["event_id"].(string)
	if id == "" {
		id = strings.ReplaceAll(uuid.NewString(), "-", "")
		ev["event_id"] = id
	}
	_, err := queue.Enqueue(ctx, h.DB.Pool, process.JobKind, process.Job{
		OrgID:      key.OrgID,
		ProjectID:  key.ProjectID,
		ReceivedAt: time.Now().UTC(),
		Scrubbed:   s.Count,
		Event:      ev,
	}, time.Time{})
	return id, err
}

// --- authentication ------------------------------------------------------------

// authenticate finds the DSN public key in the SDK auth header, Authorization,
// the public-key query parameter or the envelope header, and checks it
// belongs to the project in the URL.
func (h *Handler) authenticate(r *http.Request, envelopeKey string) (*projectKey, int, string) {
	public := keyFromAuthHeader(r.Header.Get(hdrAuth))
	if public == "" {
		public = keyFromAuthHeader(r.Header.Get("Authorization"))
	}
	if public == "" {
		public = r.URL.Query().Get(paramPublicKey)
	}
	if public == "" {
		public = envelopeKey
	}
	if public == "" {
		return nil, http.StatusUnauthorized, "missing authentication"
	}
	seq, err := strconv.ParseInt(chi.URLParam(r, "projectID"), 10, 64)
	if err != nil {
		return nil, http.StatusBadRequest, "invalid project id"
	}
	key, err := h.lookup(r.Context(), public)
	if err != nil {
		h.Log.Error("ingest: key lookup", "err", err)
		return nil, http.StatusServiceUnavailable, "temporarily unavailable"
	}
	if key == nil || key.ProjectSeq != seq {
		return nil, http.StatusUnauthorized, "invalid DSN"
	}
	return key, 0, ""
}

// keyFromAuthHeader extracts the public key from an SDK auth header
// ("<scheme> <key-param>=abc, <version-param>=7").
func keyFromAuthHeader(header string) string {
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(strings.ToLower(header), authScheme) {
		return ""
	}
	for _, part := range strings.Split(header[len(authScheme):], ",") {
		k, v, found := strings.Cut(strings.TrimSpace(part), "=")
		if found && strings.TrimSpace(k) == paramPublicKey {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// lookup resolves a public key, caching results briefly so ingestion does
// not hit the database for every event.
func (h *Handler) lookup(ctx context.Context, public string) (*projectKey, error) {
	h.mu.Lock()
	if c, ok := h.cache[public]; ok && time.Now().Before(c.expires) {
		h.mu.Unlock()
		return c.key, nil
	}
	h.mu.Unlock()

	var k projectKey
	var ipMode string
	// Key lookup is cross-tenant by nature: the key identifies the tenant.
	err := h.DB.System(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT p.org_id, p.id, p.seq, p.scrub_defaults, p.scrub_ip, p.sensitive_fields, p.safe_fields
			FROM project_keys k JOIN projects p ON p.id = k.project_id
			WHERE k.public_key = $1 AND k.is_active`, public,
		).Scan(&k.OrgID, &k.ProjectID, &k.ProjectSeq, &k.Scrub.Defaults, &ipMode, &k.Scrub.SensitiveFields, &k.Scrub.SafeFields)
	})
	var result *projectKey
	switch {
	case err == nil:
		k.Scrub.IP = scrub.IPMode(ipMode)
		result = &k
	case errors.Is(err, pgx.ErrNoRows):
	default:
		return nil, err
	}
	h.mu.Lock()
	if len(h.cache) > 10_000 {
		h.cache = map[string]cachedKey{}
	}
	h.cache[public] = cachedKey{key: result, expires: time.Now().Add(30 * time.Second)}
	h.mu.Unlock()
	return result, nil
}

// --- body decoding -------------------------------------------------------------

var errTooLarge = errors.New("payload too large")

func statusFor(err error) int {
	if errors.Is(err, errTooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	var rd io.Reader = http.MaxBytesReader(w, r.Body, maxCompressed)
	switch enc := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Encoding"))); enc {
	case "", "identity":
	case "gzip", "x-gzip":
		gz, err := gzip.NewReader(rd)
		if err != nil {
			return nil, fmt.Errorf("invalid gzip body")
		}
		rd = gz
	case "deflate":
		// "deflate" is zlib-wrapped per HTTP; some clients send raw deflate.
		buf, err := io.ReadAll(rd)
		if err != nil {
			return nil, tooLarge(err)
		}
		if zr, err := zlib.NewReader(bytes.NewReader(buf)); err == nil {
			rd = zr
		} else {
			rd = flate.NewReader(bytes.NewReader(buf))
		}
	case "br":
		rd = brotli.NewReader(rd)
	default:
		return nil, fmt.Errorf("unsupported content encoding %q", enc)
	}
	body, err := io.ReadAll(io.LimitReader(rd, maxDecompressed+1))
	if err != nil {
		return nil, tooLarge(err)
	}
	if len(body) > maxDecompressed {
		return nil, errTooLarge
	}
	return body, nil
}

func tooLarge(err error) error {
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return errTooLarge
	}
	return fmt.Errorf("could not read body: %v", err)
}

// InvalidateCache forgets cached DSN keys (call after keys change).
func (h *Handler) InvalidateCache() {
	h.mu.Lock()
	h.cache = map[string]cachedKey{}
	h.mu.Unlock()
}
