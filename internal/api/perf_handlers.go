package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/errorlens-io/errorlens/internal/audit"
	"github.com/errorlens-io/errorlens/internal/monitor"
	"github.com/errorlens-io/errorlens/internal/notify"
)

func periodStart(r *http.Request) time.Time {
	switch r.URL.Query().Get("period") {
	case "1h":
		return time.Now().Add(-time.Hour)
	case "7d":
		return time.Now().Add(-7 * 24 * time.Hour)
	case "30d":
		return time.Now().Add(-30 * 24 * time.Hour)
	}
	return time.Now().Add(-24 * time.Hour)
}

type txSummary struct {
	Name        string  `json:"name"`
	Project     string  `json:"project"`
	Op          string  `json:"op"`
	Count       int64   `json:"count"`
	P50         float64 `json:"p50"`
	P95         float64 `json:"p95"`
	Avg         float64 `json:"avg"`
	FailureRate float64 `json:"failureRate"`
	PerMinute   float64 `json:"perMinute"`
}

// listPerformance summarizes transactions by name: ?period=1h|24h|7d|30d &project=slug &query=text &sort=count|p95
func (s *Server) listPerformance(w http.ResponseWriter, r *http.Request) {
	since := periodStart(r)
	q := r.URL.Query()
	order := "count(*) DESC"
	if q.Get("sort") == "p95" {
		order = "p95 DESC"
	}
	like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q.Get("query")) + "%"
	out := []txSummary{}
	err := s.db.InScope(r.Context(), scope(r), func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `
			SELECT t.name, p.slug, max(t.op), count(*),
			       percentile_cont(0.5) WITHIN GROUP (ORDER BY t.duration_ms),
			       percentile_cont(0.95) WITHIN GROUP (ORDER BY t.duration_ms) AS p95,
			       avg(t.duration_ms),
			       avg(CASE WHEN t.status NOT IN ('ok', 'cancelled', 'unknown') THEN 1.0 ELSE 0.0 END),
			       count(*) / GREATEST(EXTRACT(EPOCH FROM now() - $1::timestamptz) / 60, 1)
			FROM transactions t JOIN projects p ON p.id = t.project_id
			WHERE t.start_ts >= $1 AND ($2 = '' OR p.slug = $2) AND t.name ILIKE $3
			GROUP BY t.name, p.slug ORDER BY `+order+` LIMIT 200`, since, q.Get("project"), like)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowToStructByPos[txSummary])
		return err
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if out == nil {
		out = []txSummary{}
	}
	writeJSON(w, http.StatusOK, out)
}

type txRow struct {
	ID          uuid.UUID         `json:"id"`
	TraceID     string            `json:"traceId"`
	Name        string            `json:"name"`
	Project     string            `json:"project"`
	Op          string            `json:"op"`
	Status      string            `json:"status"`
	Start       time.Time         `json:"start"`
	DurationMs  float64           `json:"durationMs"`
	Release     string            `json:"release"`
	Environment string            `json:"environment"`
	Tags        map[string]string `json:"tags"`
}

// listTransactions lists individual transactions for a name, slowest first by default.
func (s *Server) listTransactions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	order := "t.duration_ms DESC"
	if q.Get("sort") == "recent" {
		order = "t.start_ts DESC"
	}
	out := []txRow{}
	err := s.db.InScope(r.Context(), scope(r), func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `
			SELECT t.id, t.trace_id, t.name, p.slug, t.op, t.status, t.start_ts, t.duration_ms, t.release, t.environment, t.tags
			FROM transactions t JOIN projects p ON p.id = t.project_id
			WHERE t.start_ts >= $1 AND t.name = $2 AND ($3 = '' OR p.slug = $3)
			ORDER BY `+order+` LIMIT 100`, periodStart(r), q.Get("name"), q.Get("project"))
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowToStructByPos[txRow])
		return err
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if out == nil {
		out = []txRow{}
	}
	writeJSON(w, http.StatusOK, out)
}

// getTrace returns everything that happened in one distributed trace:
// transactions (with spans), errors and logs.
func (s *Server) getTrace(w http.ResponseWriter, r *http.Request) {
	traceID := strings.ToLower(chi.URLParam(r, "traceID"))
	if len(traceID) > 64 {
		writeError(w, http.StatusBadRequest, "invalid trace id")
		return
	}
	type txFull struct {
		txRow
		Spans json.RawMessage `json:"spans"`
	}
	type errRow struct {
		IssueID   int64     `json:"issueId"`
		EventID   uuid.UUID `json:"eventId"`
		Title     string    `json:"title"`
		Level     string    `json:"level"`
		Timestamp time.Time `json:"timestamp"`
	}
	var txs []txFull
	var errs []errRow
	var logLines []LogLine
	err := s.db.InScope(r.Context(), scope(r), func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `
			SELECT t.id, t.trace_id, t.name, p.slug, t.op, t.status, t.start_ts, t.duration_ms, t.release, t.environment, t.tags, t.spans
			FROM transactions t JOIN projects p ON p.id = t.project_id WHERE t.trace_id = $1 ORDER BY t.start_ts LIMIT 100`, traceID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var t txFull
			if err := rows.Scan(&t.ID, &t.TraceID, &t.Name, &t.Project, &t.Op, &t.Status, &t.Start, &t.DurationMs, &t.Release, &t.Environment, &t.Tags, &t.Spans); err != nil {
				rows.Close()
				return err
			}
			txs = append(txs, t)
		}
		rows.Close()
		rows, err = tx.Query(r.Context(), `SELECT issue_id, id, title, level, timestamp FROM events
			WHERE tags->>'trace_id' = $1 ORDER BY timestamp LIMIT 100`, traceID)
		if err != nil {
			return err
		}
		if errs, err = pgx.CollectRows(rows, pgx.RowToStructByPos[errRow]); err != nil {
			return err
		}
		rows, err = tx.Query(r.Context(), `SELECT l.id, l.timestamp, p.slug, l.severity, l.body, l.trace_id, l.release, l.environment, l.attributes
			FROM logs l JOIN projects p ON p.id = l.project_id WHERE l.trace_id = $1 ORDER BY l.timestamp LIMIT 500`, traceID)
		if err != nil {
			return err
		}
		logLines, err = pgx.CollectRows(rows, pgx.RowToStructByPos[LogLine])
		return err
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if txs == nil {
		txs = []txFull{}
	}
	if errs == nil {
		errs = []errRow{}
	}
	if logLines == nil {
		logLines = []LogLine{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"traceId": traceID, "transactions": txs, "errors": errs, "logs": logLines})
}

// --- monitors ---------------------------------------------------------------------------

// Monitor as returned by the API.
type Monitor struct {
	ID             uuid.UUID   `json:"id"`
	Project        string      `json:"project"`
	Slug           string      `json:"slug"`
	Name           string      `json:"name"`
	Kind           string      `json:"kind"`
	Schedule       string      `json:"schedule"`
	Timezone       string      `json:"timezone"`
	Margin         int         `json:"checkinMarginMinutes"`
	MaxRuntime     int         `json:"maxRuntimeMinutes"`
	URL            string      `json:"url"`
	Interval       int         `json:"intervalSeconds"`
	Timeout        int         `json:"timeoutSeconds"`
	ExpectedStatus int         `json:"expectedStatus"`
	Status         string      `json:"status"`
	LastCheckIn    *time.Time  `json:"lastCheckIn"`
	NextExpected   *time.Time  `json:"nextExpected"`
	ChannelIDs     []uuid.UUID `json:"channelIds"`
	Enabled        bool        `json:"enabled"`
	Recent         []string    `json:"recent"` // last statuses, newest first
}

const monitorCols = `m.id, p.slug, m.slug, m.name, m.kind, m.schedule, m.timezone, m.checkin_margin_min, m.max_runtime_min,
	m.url, m.interval_seconds, m.timeout_seconds, m.expected_status, m.status, m.last_checkin_at, m.next_expected_at,
	m.channel_ids, m.enabled,
	ARRAY(SELECT c.status FROM monitor_checkins c WHERE c.monitor_id = m.id ORDER BY c.started_at DESC LIMIT 30)`

func (s *Server) listMonitors(w http.ResponseWriter, r *http.Request) {
	out := []Monitor{}
	err := s.db.InScope(r.Context(), scope(r), func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `SELECT `+monitorCols+` FROM monitors m JOIN projects p ON p.id = m.project_id ORDER BY m.name`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Monitor])
		return err
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if out == nil {
		out = []Monitor{}
	}
	writeJSON(w, http.StatusOK, out)
}

type checkinRow struct {
	ID          uuid.UUID  `json:"id"`
	Status      string     `json:"status"`
	StartedAt   time.Time  `json:"startedAt"`
	FinishedAt  *time.Time `json:"finishedAt"`
	DurationMs  *float64   `json:"durationMs"`
	Environment string     `json:"environment"`
	Details     string     `json:"details"`
}

func (s *Server) getMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "monitorID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "monitor not found")
		return
	}
	var m Monitor
	var checkins []checkinRow
	err = s.db.InScope(r.Context(), scope(r), func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `SELECT `+monitorCols+` FROM monitors m JOIN projects p ON p.id = m.project_id WHERE m.id = $1`, id)
		if err != nil {
			return err
		}
		if m, err = pgx.CollectOneRow(rows, pgx.RowToStructByPos[Monitor]); err != nil {
			return err
		}
		rows, err = tx.Query(r.Context(), `SELECT id, status, started_at, finished_at, duration_ms, environment, details
			FROM monitor_checkins WHERE monitor_id = $1 ORDER BY started_at DESC LIMIT 100`, id)
		if err != nil {
			return err
		}
		checkins, err = pgx.CollectRows(rows, pgx.RowToStructByPos[checkinRow])
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "monitor not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if checkins == nil {
		checkins = []checkinRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"monitor": m, "checkins": checkins})
}

type monitorRequest struct {
	Project        string      `json:"project"`
	Name           string      `json:"name"`
	Slug           string      `json:"slug"`
	Kind           string      `json:"kind"`
	Schedule       string      `json:"schedule"`
	Timezone       string      `json:"timezone"`
	Margin         int         `json:"checkinMarginMinutes"`
	MaxRuntime     int         `json:"maxRuntimeMinutes"`
	URL            string      `json:"url"`
	Interval       int         `json:"intervalSeconds"`
	Timeout        int         `json:"timeoutSeconds"`
	ExpectedStatus int         `json:"expectedStatus"`
	ChannelIDs     []uuid.UUID `json:"channelIds"`
	Enabled        *bool       `json:"enabled"`
}

func (s *Server) saveMonitor(w http.ResponseWriter, r *http.Request) {
	var req monitorRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Slug == "" {
		req.Slug = slugify(req.Name)
	}
	if req.Timezone == "" {
		req.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(req.Timezone); err != nil {
		writeError(w, http.StatusBadRequest, "unknown timezone")
		return
	}
	if req.Margin <= 0 {
		req.Margin = 5
	}
	if req.MaxRuntime <= 0 {
		req.MaxRuntime = 30
	}
	if req.Interval == 0 {
		req.Interval = 60
	}
	if req.Timeout == 0 {
		req.Timeout = 10
	}
	if req.ChannelIDs == nil {
		req.ChannelIDs = []uuid.UUID{}
	}
	switch {
	case req.Name == "" || len(req.Name) > 100:
		writeError(w, http.StatusBadRequest, "name is required (max 100 characters)")
		return
	case req.Kind == "cron" && monitor.ValidateSchedule(req.Schedule) != nil:
		writeError(w, http.StatusBadRequest, `schedule must be a crontab expression like "0 * * * *" or "@every 15m"`)
		return
	case req.Kind == "uptime" && notify.ValidateURL(req.URL) != nil:
		writeError(w, http.StatusBadRequest, "url must be an absolute http(s) URL")
		return
	case req.Kind == "uptime" && (req.Interval < 30 || req.Interval > 86400 || req.Timeout < 1 || req.Timeout > 60):
		writeError(w, http.StatusBadRequest, "check every 30s–24h with a 1–60s timeout")
		return
	case req.Kind != "cron" && req.Kind != "uptime":
		writeError(w, http.StatusBadRequest, "kind must be cron or uptime")
		return
	}
	enabled := req.Enabled == nil || *req.Enabled
	ctx := r.Context()
	editing := chi.URLParam(r, "monitorID") != ""
	id := uuid.New()
	if editing {
		var err error
		if id, err = uuid.Parse(chi.URLParam(r, "monitorID")); err != nil {
			writeError(w, http.StatusNotFound, "monitor not found")
			return
		}
	}
	var next *time.Time
	if req.Kind == "cron" {
		if n, err := monitor.Next(req.Schedule, req.Timezone, time.Now()); err == nil {
			next = &n
		}
	}
	err := s.db.InScope(ctx, scope(r), func(tx pgx.Tx) error {
		var pid uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM projects WHERE slug = $1`, req.Project).Scan(&pid); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return &httpError{http.StatusBadRequest, "unknown project"}
			}
			return err
		}
		var valid int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM alert_channels WHERE id = ANY($1)`, req.ChannelIDs).Scan(&valid); err != nil {
			return err
		}
		if valid != len(req.ChannelIDs) {
			return &httpError{http.StatusBadRequest, "unknown channel"}
		}
		if editing {
			tag, err := tx.Exec(ctx, `UPDATE monitors SET project_id=$2, name=$3, schedule=$4, timezone=$5, checkin_margin_min=$6,
				max_runtime_min=$7, url=$8, interval_seconds=$9, timeout_seconds=$10, expected_status=$11, channel_ids=$12, enabled=$13,
				next_expected_at = CASE WHEN kind = 'cron' THEN $14 ELSE NULL END WHERE id=$1`,
				id, pid, req.Name, req.Schedule, req.Timezone, req.Margin, req.MaxRuntime, req.URL, req.Interval, req.Timeout,
				req.ExpectedStatus, req.ChannelIDs, enabled, next)
			if err == nil && tag.RowsAffected() == 0 {
				return pgx.ErrNoRows
			}
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO monitors (id, org_id, project_id, slug, name, kind, schedule, timezone, checkin_margin_min,
			max_runtime_min, url, interval_seconds, timeout_seconds, expected_status, channel_ids, enabled, next_expected_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
			id, orgFrom(ctx).ID, pid, req.Slug, req.Name, req.Kind, req.Schedule, req.Timezone, req.Margin, req.MaxRuntime,
			req.URL, req.Interval, req.Timeout, req.ExpectedStatus, req.ChannelIDs, enabled, next); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{OrgID: orgFrom(ctx).ID, ActorID: userFrom(ctx).ID, Action: "monitor.create", Target: req.Slug, IP: clientIP(r)})
	})
	if err == nil {
		status := http.StatusCreated
		if editing {
			status = http.StatusOK
		}
		writeJSON(w, status, map[string]any{"id": id})
		return
	}
	s.finish(w, r, err, nil)
}

func (s *Server) deleteMonitor(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "monitorID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "monitor not found")
		return
	}
	err = s.db.InScope(r.Context(), scope(r), func(tx pgx.Tx) error {
		tag, err := tx.Exec(r.Context(), `DELETE FROM monitors WHERE id = $1`, id)
		if err == nil && tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return err
	})
	s.finish(w, r, err, nil)
}
