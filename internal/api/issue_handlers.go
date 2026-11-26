package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/errorlenshq/errorlens/internal/audit"
)

// Issue is a group of similar events.
type Issue struct {
	ID           int64      `json:"id"`
	ProjectSlug  string     `json:"project"`
	Title        string     `json:"title"`
	Culprit      string     `json:"culprit"`
	Level        string     `json:"level"`
	Platform     string     `json:"platform"`
	Status       string     `json:"status"`
	FirstSeen    time.Time  `json:"firstSeen"`
	LastSeen     time.Time  `json:"lastSeen"`
	TimesSeen    int64      `json:"count"`
	FirstRelease string     `json:"firstRelease"`
	LastRelease  string     `json:"lastRelease"`
	RegressedAt  *time.Time `json:"regressedAt"`
	Trend        []int64    `json:"trend"` // events per hour, last 24h, oldest first
}

const issueColumns = `i.id, p.slug, i.title, i.culprit, i.level, i.platform, i.status, i.first_seen,
	i.last_seen, i.times_seen, i.first_release, i.last_release, i.regressed_at`

func scanIssue(row pgx.Row, is *Issue) error {
	return row.Scan(&is.ID, &is.ProjectSlug, &is.Title, &is.Culprit, &is.Level, &is.Platform, &is.Status,
		&is.FirstSeen, &is.LastSeen, &is.TimesSeen, &is.FirstRelease, &is.LastRelease, &is.RegressedAt)
}

// listIssues: ?status=unresolved|resolved|ignored|all &project=slug &query=text
// &sort=last_seen|first_seen|count &limit=50 &offset=0
func (s *Server) listIssues(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	where := []string{"true"}
	args := []any{}
	arg := func(v any) string { args = append(args, v); return "$" + strconv.Itoa(len(args)) }

	switch st := q.Get("status"); st {
	case "", "unresolved":
		where = append(where, "i.status = 'unresolved'")
	case "resolved", "ignored":
		where = append(where, "i.status = "+arg(st))
	case "all":
	default:
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}
	if p := q.Get("project"); p != "" {
		where = append(where, "p.slug = "+arg(p))
	}
	if text := searchText(q.Get("query")); text != "" {
		like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(text) + "%"
		a := arg(like)
		// issue_search_ids uses the trigram index (see migration 0009).
		where = append(where, "i.id IN (SELECT issue_search_ids("+a+"))")
	}
	order := "i.last_seen DESC"
	switch q.Get("sort") {
	case "", "last_seen":
	case "first_seen":
		order = "i.first_seen DESC"
	case "count":
		order = "i.times_seen DESC"
	default:
		writeError(w, http.StatusBadRequest, "invalid sort")
		return
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	if offset < 0 {
		offset = 0
	}
	if offset > maxIssueTotal {
		offset = maxIssueTotal
	}

	issues := []Issue{}
	var total int64
	err := s.db.InScope(r.Context(), scope(r), func(tx pgx.Tx) error {
		cond := strings.Join(where, " AND ")
		// Counting stops at maxIssueTotal: exact totals over huge lists cost a
		// full scan and nobody pages that far.
		if err := tx.QueryRow(r.Context(), `SELECT count(*) FROM (
			SELECT 1 FROM issues i JOIN projects p ON p.id = i.project_id WHERE `+cond+`
			LIMIT `+strconv.Itoa(maxIssueTotal+1)+`) s`, args...).Scan(&total); err != nil {
			return err
		}
		rows, err := tx.Query(r.Context(), `SELECT `+issueColumns+`
			FROM issues i JOIN projects p ON p.id = i.project_id
			WHERE `+cond+` ORDER BY `+order+`, i.id DESC
			LIMIT `+strconv.Itoa(limit)+` OFFSET `+strconv.Itoa(offset), args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var is Issue
			if err := scanIssue(rows, &is); err != nil {
				return err
			}
			issues = append(issues, is)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		return s.fillTrends(r, tx, issues, 24, 1)
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	capped := total > maxIssueTotal
	if capped {
		total = maxIssueTotal
	}
	writeJSON(w, http.StatusOK, map[string]any{"issues": issues, "total": total, "totalCapped": capped})
}

// maxIssueTotal bounds the issue count shown with lists ("10,000+").
const maxIssueTotal = 10_000

// fillTrends sets each issue's Trend to its event counts in `buckets`
// consecutive buckets of `size` hours, ending with the current hour. The
// bucketing happens in SQL, so only the finished arrays cross the wire.
func (s *Server) fillTrends(r *http.Request, tx pgx.Tx, issues []Issue, buckets, size int) error {
	if len(issues) == 0 {
		return nil
	}
	ids := make([]int64, len(issues))
	idx := make(map[int64]int, len(issues))
	for i := range issues {
		ids[i] = issues[i].ID
		idx[issues[i].ID] = i
	}
	end := time.Now().UTC().Truncate(time.Hour).Add(time.Hour)
	start := end.Add(-time.Duration(buckets*size) * time.Hour)
	rows, err := tx.Query(r.Context(), `
		WITH c AS (
			SELECT issue_id, floor(extract(epoch FROM bucket - $2::timestamptz) / ($4 * 3600))::int AS k, sum(count) AS n
			FROM issue_counts
			WHERE issue_id = ANY($1) AND bucket >= $2
			GROUP BY 1, 2
		)
		SELECT ids.id, array_agg(COALESCE(c.n, 0)::bigint ORDER BY g.k)
		FROM unnest($1::bigint[]) AS ids(id)
		CROSS JOIN generate_series(0, $3::int - 1) AS g(k)
		LEFT JOIN c ON c.issue_id = ids.id AND c.k = g.k
		GROUP BY ids.id`, ids, start, buckets, size)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var trend []int64
		if err := rows.Scan(&id, &trend); err != nil {
			return err
		}
		issues[idx[id]].Trend = trend
	}
	return rows.Err()
}

func issueID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "issueID"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, "issue not found")
		return 0, false
	}
	return id, true
}

type tagValue struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

type tagSummary struct {
	Key    string     `json:"key"`
	Total  int64      `json:"total"`
	Values []tagValue `json:"values"`
}

// getIssue returns the issue, a 14-day daily trend and a tag breakdown over
// its most recent 1000 events.
func (s *Server) getIssue(w http.ResponseWriter, r *http.Request) {
	id, ok := issueID(w, r)
	if !ok {
		return
	}
	var is Issue
	var tags []tagSummary
	err := s.db.InScope(r.Context(), scope(r), func(tx pgx.Tx) error {
		if err := scanIssue(tx.QueryRow(r.Context(), `SELECT `+issueColumns+`
			FROM issues i JOIN projects p ON p.id = i.project_id WHERE i.id = $1`, id), &is); err != nil {
			return err
		}
		list := []Issue{is}
		if err := s.fillTrends(r, tx, list, 14, 24); err != nil {
			return err
		}
		is.Trend = list[0].Trend

		rows, err := tx.Query(r.Context(), `
			WITH recent AS (
				SELECT tags FROM events WHERE issue_id = $1 ORDER BY received_at DESC LIMIT 1000
			), kv AS (
				SELECT t.key, t.value, count(*) AS n FROM recent, jsonb_each_text(recent.tags) t GROUP BY 1, 2
			), ranked AS (
				SELECT key, value, n, sum(n) OVER (PARTITION BY key) AS total,
				       row_number() OVER (PARTITION BY key ORDER BY n DESC, value) AS rn
				FROM kv
			)
			SELECT key, total, value, n FROM ranked WHERE rn <= 5 ORDER BY key, n DESC, value`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key, value string
			var total, n int64
			if err := rows.Scan(&key, &total, &value, &n); err != nil {
				return err
			}
			if len(tags) == 0 || tags[len(tags)-1].Key != key {
				tags = append(tags, tagSummary{Key: key, Total: total})
			}
			tags[len(tags)-1].Values = append(tags[len(tags)-1].Values, tagValue{value, n})
		}
		return rows.Err()
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "issue not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if tags == nil {
		tags = []tagSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"issue": is, "tags": tags})
}

type updateIssueRequest struct {
	Status string `json:"status"`
}

func (s *Server) updateIssue(w http.ResponseWriter, r *http.Request) {
	id, ok := issueID(w, r)
	if !ok {
		return
	}
	var req updateIssueRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	switch req.Status {
	case "resolved", "unresolved", "ignored":
	default:
		writeError(w, http.StatusBadRequest, "status must be resolved, unresolved or ignored")
		return
	}
	var title string
	ctx := r.Context()
	err := s.db.InScope(ctx, scope(r), func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			UPDATE issues SET status = $2,
				resolved_at = CASE WHEN $2 = 'resolved' THEN now() ELSE NULL END
			WHERE id = $1 RETURNING title`, id, req.Status).Scan(&title); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			OrgID: orgFrom(ctx).ID, ActorID: userFrom(ctx).ID, Action: "issue." + req.Status,
			Target: strconv.FormatInt(id, 10), IP: clientIP(r),
		})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "issue not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": req.Status})
}

type eventSummary struct {
	ID          string            `json:"id"`
	Timestamp   time.Time         `json:"timestamp"`
	ReceivedAt  time.Time         `json:"receivedAt"`
	Level       string            `json:"level"`
	Title       string            `json:"title"`
	Release     string            `json:"release"`
	Environment string            `json:"environment"`
	Tags        map[string]string `json:"tags"`
	Scrubbed    int               `json:"scrubbed"`
}

// listIssueEvents pages through an issue's events, newest first.
// Pass ?before=<receivedAt RFC3339Nano> to get older events.
func (s *Server) listIssueEvents(w http.ResponseWriter, r *http.Request) {
	id, ok := issueID(w, r)
	if !ok {
		return
	}
	before := time.Now().Add(time.Hour)
	if b := r.URL.Query().Get("before"); b != "" {
		t, err := time.Parse(time.RFC3339Nano, b)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid before")
			return
		}
		before = t
	}
	events := []eventSummary{}
	err := s.db.InScope(r.Context(), scope(r), func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `
			SELECT id, timestamp, received_at, level, title, release, environment, tags, scrubbed
			FROM events WHERE issue_id = $1 AND received_at < $2
			ORDER BY received_at DESC LIMIT 50`, id, before)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e eventSummary
			var eid uuid.UUID
			if err := rows.Scan(&eid, &e.Timestamp, &e.ReceivedAt, &e.Level, &e.Title, &e.Release, &e.Environment, &e.Tags, &e.Scrubbed); err != nil {
				return err
			}
			e.ID = strings.ReplaceAll(eid.String(), "-", "")
			events = append(events, e)
		}
		return rows.Err()
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

// getEvent returns one full event. "latest" and "oldest" are accepted as ids.
// Every full-payload view is written to the audit log.
func (s *Server) getEvent(w http.ResponseWriter, r *http.Request) {
	id, ok := issueID(w, r)
	if !ok {
		return
	}
	eventParam := chi.URLParam(r, "eventID")
	var where string
	var args = []any{id}
	switch eventParam {
	case "latest":
		where = "ORDER BY received_at DESC LIMIT 1"
	case "oldest":
		where = "ORDER BY received_at ASC LIMIT 1"
	default:
		eid, err := uuid.Parse(eventParam)
		if err != nil {
			writeError(w, http.StatusNotFound, "event not found")
			return
		}
		where = "AND id = $2 LIMIT 1"
		args = append(args, eid)
	}

	var resp struct {
		eventSummary
		Payload json.RawMessage `json:"payload"`
		Prev    *string         `json:"prev"` // older event id
		Next    *string         `json:"next"` // newer event id
	}
	ctx := r.Context()
	var sealed []byte
	err := s.db.InScope(ctx, scope(r), func(tx pgx.Tx) error {
		var eid uuid.UUID
		if err := tx.QueryRow(ctx, `
			SELECT id, timestamp, received_at, level, title, release, environment, tags, scrubbed, payload, payload_ct
			FROM events WHERE issue_id = $1 `+where, args...,
		).Scan(&eid, &resp.Timestamp, &resp.ReceivedAt, &resp.Level, &resp.Title, &resp.Release,
			&resp.Environment, &resp.Tags, &resp.Scrubbed, &resp.Payload, &sealed); err != nil {
			return err
		}
		if sealed != nil {
			if s.keyring == nil {
				return errors.New("event is encrypted but ERRORLENS_ENCRYPTION_KEY is not set")
			}
			pt, err := s.keyring.Open(ctx, orgFrom(ctx).ID, sealed, eid[:])
			if err != nil {
				return err
			}
			resp.Payload = pt
		}
		resp.ID = strings.ReplaceAll(eid.String(), "-", "")
		neighbor := func(op, dir string) (*string, error) {
			var n uuid.UUID
			err := tx.QueryRow(ctx, `SELECT id FROM events WHERE issue_id = $1 AND received_at `+op+` $2
				ORDER BY received_at `+dir+` LIMIT 1`, id, resp.ReceivedAt).Scan(&n)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, nil
			}
			if err != nil {
				return nil, err
			}
			s := strings.ReplaceAll(n.String(), "-", "")
			return &s, nil
		}
		var err error
		if resp.Prev, err = neighbor("<", "DESC"); err != nil {
			return err
		}
		if resp.Next, err = neighbor(">", "ASC"); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			OrgID: orgFrom(ctx).ID, ActorID: userFrom(ctx).ID, Action: audit.ActionEventView,
			Target: resp.ID, IP: clientIP(r),
		})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "event not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
