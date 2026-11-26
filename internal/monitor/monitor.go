// Package monitor implements cron monitors (jobs report check-ins; missed
// or overrunning runs are detected) and uptime monitors (scheduled HTTP checks).
package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/robfig/cron/v3"

	"github.com/errorlenshq/errorlens/internal/db"
	"github.com/errorlenshq/errorlens/internal/notify"
	"github.com/errorlenshq/errorlens/internal/queue"
)

// Job kinds.
const (
	UptimeJob = "monitor.uptime"
	NotifyJob = "monitor.notify"
)

var parser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// ValidateSchedule checks a crontab expression ("0 * * * *") or "@every 5m".
func ValidateSchedule(s string) error {
	_, err := parser.Parse(s)
	return err
}

// Next returns the next scheduled time after t.
func Next(schedule, tz string, t time.Time) (time.Time, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	sch, err := parser.Parse(schedule)
	if err != nil {
		return time.Time{}, err
	}
	return sch.Next(t.In(loc)).UTC(), nil
}

// CheckIn is a report from a scheduled job.
type CheckIn struct {
	ID          string         `json:"check_in_id"`
	Slug        string         `json:"monitor_slug"`
	Status      string         `json:"status"`   // in_progress | ok | error
	Duration    float64        `json:"duration"` // seconds
	Environment string         `json:"environment"`
	Config      *MonitorConfig `json:"monitor_config"`
}

// MonitorConfig lets SDKs create/update a monitor from code ("upsert").
type MonitorConfig struct {
	Schedule struct {
		Type  string          `json:"type"` // crontab | interval
		Value json.RawMessage `json:"value"`
		Unit  string          `json:"unit"`
	} `json:"schedule"`
	CheckinMargin int    `json:"checkin_margin"`
	MaxRuntime    int    `json:"max_runtime"`
	Timezone      string `json:"timezone"`
}

func (c *MonitorConfig) schedule() (string, error) {
	switch c.Schedule.Type {
	case "crontab":
		var s string
		if err := json.Unmarshal(c.Schedule.Value, &s); err != nil {
			return "", err
		}
		return s, ValidateSchedule(s)
	case "interval":
		var n int
		if err := json.Unmarshal(c.Schedule.Value, &n); err != nil || n < 1 {
			return "", errors.New("invalid interval")
		}
		unit := map[string]string{"minute": "m", "hour": "h", "day": "h", "week": "h", "month": "h", "year": "h"}[c.Schedule.Unit]
		mult := map[string]int{"minute": 1, "hour": 1, "day": 24, "week": 24 * 7, "month": 24 * 30, "year": 24 * 365}[c.Schedule.Unit]
		if unit == "" {
			return "", errors.New("invalid interval unit")
		}
		return fmt.Sprintf("@every %d%s", n*mult, unit), nil
	}
	return "", errors.New("unknown schedule type")
}

var slugRe = strings.NewReplacer(" ", "-", "_", "-")

// Service processes check-ins and runs schedules.
type Service struct {
	DB   *db.DB
	Log  *slog.Logger
	HTTP *http.Client
	// OrgURL builds dashboard links (see config.Config.OrgURL).
	OrgURL func(org, path string) string
}

// RecordCheckIn applies a check-in to the monitor identified by slug,
// creating the monitor if the check-in carries a config.
func (s *Service) RecordCheckIn(ctx context.Context, org, project uuid.UUID, c CheckIn) error {
	slug := strings.ToLower(slugRe.Replace(strings.TrimSpace(c.Slug)))
	if slug == "" || len(slug) > 64 {
		return errors.New("invalid monitor slug")
	}
	switch c.Status {
	case "in_progress", "ok", "error":
	default:
		return errors.New("invalid check-in status")
	}
	now := time.Now().UTC()
	var notifyMsg *transition
	err := s.DB.InScope(ctx, db.Scope{OrgID: org}, func(tx pgx.Tx) error {
		var id uuid.UUID
		var schedule, tz, status, name string
		err := tx.QueryRow(ctx, `SELECT id, schedule, timezone, status, name FROM monitors WHERE project_id = $1 AND slug = $2 AND kind = 'cron' FOR UPDATE`,
			project, slug).Scan(&id, &schedule, &tz, &status, &name)
		if errors.Is(err, pgx.ErrNoRows) {
			if c.Config == nil {
				return fmt.Errorf("no monitor %q; create it in ErrorLens or send monitor_config", slug)
			}
			sched, err := c.Config.schedule()
			if err != nil {
				return fmt.Errorf("monitor_config: %w", err)
			}
			id, schedule, tz, status, name = uuid.New(), sched, defaultStr(c.Config.Timezone, "UTC"), "pending", slug
			margin, runtime := defaultInt(c.Config.CheckinMargin, 5), defaultInt(c.Config.MaxRuntime, 30)
			if _, err := tx.Exec(ctx, `INSERT INTO monitors (id, org_id, project_id, slug, name, kind, schedule, timezone, checkin_margin_min, max_runtime_min)
				VALUES ($1, $2, $3, $4, $4, 'cron', $5, $6, $7, $8)`, id, org, project, slug, schedule, tz, margin, runtime); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if c.Config != nil {
			if sched, err := c.Config.schedule(); err == nil {
				schedule = sched
				_, _ = tx.Exec(ctx, `UPDATE monitors SET schedule = $2, timezone = COALESCE(NULLIF($3, ''), timezone),
					checkin_margin_min = COALESCE(NULLIF($4, 0), checkin_margin_min), max_runtime_min = COALESCE(NULLIF($5, 0), max_runtime_min)
					WHERE id = $1`, id, sched, c.Config.Timezone, c.Config.CheckinMargin, c.Config.MaxRuntime)
			}
		}

		checkinID, err := uuid.Parse(c.ID)
		if err != nil {
			checkinID = uuid.New()
		}
		var dur *float64
		if c.Duration > 0 {
			ms := c.Duration * 1000
			dur = &ms
		}
		if c.Status == "in_progress" {
			_, err = tx.Exec(ctx, `INSERT INTO monitor_checkins (id, org_id, monitor_id, status, environment) VALUES ($1, $2, $3, 'in_progress', $4)
				ON CONFLICT (id) DO NOTHING`, checkinID, org, id, c.Environment)
		} else {
			// Finish the matching in-progress check-in, or record a standalone one.
			var tag interface{ RowsAffected() int64 }
			tag, err = tx.Exec(ctx, `UPDATE monitor_checkins SET status = $3, finished_at = now(),
				duration_ms = COALESCE($4, EXTRACT(EPOCH FROM now() - started_at) * 1000)
				WHERE id = $1 AND monitor_id = $2`, checkinID, id, c.Status, dur)
			if err == nil && tag.RowsAffected() == 0 {
				_, err = tx.Exec(ctx, `INSERT INTO monitor_checkins (id, org_id, monitor_id, status, finished_at, duration_ms, environment)
					VALUES ($1, $2, $3, $4, now(), $5, $6)`, checkinID, org, id, c.Status, dur, c.Environment)
			}
		}
		if err != nil {
			return err
		}
		if c.Status == "in_progress" {
			_, err = tx.Exec(ctx, `UPDATE monitors SET last_checkin_at = now() WHERE id = $1`, id)
			return err
		}
		next, err := Next(schedule, tz, now)
		if err != nil {
			return err
		}
		newStatus := c.Status
		if _, err := tx.Exec(ctx, `UPDATE monitors SET status = $2, last_checkin_at = now(), next_expected_at = $3 WHERE id = $1`,
			id, newStatus, next); err != nil {
			return err
		}
		notifyMsg = changed(id, name, status, newStatus, "")
		return nil
	})
	if err == nil && notifyMsg != nil {
		s.enqueueNotify(ctx, org, *notifyMsg)
	}
	return err
}

func defaultStr(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func defaultInt(n, d int) int {
	if n <= 0 {
		return d
	}
	return n
}

type transition struct {
	MonitorID uuid.UUID `json:"monitor_id"`
	Name      string    `json:"name"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Detail    string    `json:"detail"`
}

func failing(s string) bool { return s == "error" || s == "missed" || s == "timeout" }

// changed returns a notification for failing<->healthy transitions.
func changed(id uuid.UUID, name, from, to, detail string) *transition {
	if failing(to) && !failing(from) || (to == "ok" && failing(from)) {
		return &transition{MonitorID: id, Name: name, From: from, To: to, Detail: detail}
	}
	return nil
}

type notifyPayload struct {
	OrgID uuid.UUID  `json:"org_id"`
	T     transition `json:"t"`
}

func (s *Service) enqueueNotify(ctx context.Context, org uuid.UUID, t transition) {
	if _, err := queue.Enqueue(ctx, s.DB.Pool, NotifyJob, notifyPayload{OrgID: org, T: t}, time.Time{}); err != nil {
		s.Log.Error("monitor: enqueue notification", "err", err)
	}
}

// Tick detects missed/timed-out cron runs and schedules due uptime checks.
// Call it every ~15s; it is safe on several replicas.
func (s *Service) Tick(ctx context.Context) error {
	var notes []notifyPayload
	var uptime []uuid.UUID
	var uptimeOrgs []uuid.UUID
	err := s.DB.System(ctx, func(tx pgx.Tx) error {
		var got bool
		if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(7316105120)`).Scan(&got); err != nil || !got {
			return err
		}
		// Missed: the expected time plus margin passed with no check-in.
		rows, err := tx.Query(ctx, `SELECT id, org_id, name, status, schedule, timezone FROM monitors
			WHERE enabled AND kind = 'cron' AND next_expected_at IS NOT NULL
			  AND next_expected_at + make_interval(mins => checkin_margin_min) < now()
			FOR UPDATE SKIP LOCKED`)
		if err != nil {
			return err
		}
		type m struct {
			id, org             uuid.UUID
			name, status, sched string
			tz                  string
		}
		var missed []m
		for rows.Next() {
			var x m
			if err := rows.Scan(&x.id, &x.org, &x.name, &x.status, &x.sched, &x.tz); err != nil {
				rows.Close()
				return err
			}
			missed = append(missed, x)
		}
		rows.Close()
		for _, x := range missed {
			next, err := Next(x.sched, x.tz, time.Now())
			if err != nil {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO monitor_checkins (id, org_id, monitor_id, status, finished_at) VALUES ($1, $2, $3, 'missed', now())`,
				uuid.New(), x.org, x.id); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE monitors SET status = 'missed', next_expected_at = $2 WHERE id = $1`, x.id, next); err != nil {
				return err
			}
			if t := changed(x.id, x.name, x.status, "missed", "No check-in arrived on schedule."); t != nil {
				notes = append(notes, notifyPayload{x.org, *t})
			}
		}

		// Timeouts: in-progress runs older than max runtime.
		rows, err = tx.Query(ctx, `UPDATE monitor_checkins c SET status = 'timeout', finished_at = now(),
			duration_ms = EXTRACT(EPOCH FROM now() - c.started_at) * 1000
			FROM monitors m
			WHERE c.monitor_id = m.id AND c.status = 'in_progress'
			  AND c.started_at + make_interval(mins => m.max_runtime_min) < now()
			RETURNING m.id, m.org_id, m.name, m.status`)
		if err != nil {
			return err
		}
		var timeouts []m
		for rows.Next() {
			var x m
			if err := rows.Scan(&x.id, &x.org, &x.name, &x.status); err != nil {
				rows.Close()
				return err
			}
			timeouts = append(timeouts, x)
		}
		rows.Close()
		for _, x := range timeouts {
			if _, err := tx.Exec(ctx, `UPDATE monitors SET status = 'timeout' WHERE id = $1`, x.id); err != nil {
				return err
			}
			if t := changed(x.id, x.name, x.status, "timeout", "A run exceeded its maximum runtime."); t != nil {
				notes = append(notes, notifyPayload{x.org, *t})
			}
		}

		// Uptime checks that are due.
		rows, err = tx.Query(ctx, `UPDATE monitors SET next_expected_at = now() + make_interval(secs => interval_seconds)
			WHERE enabled AND kind = 'uptime' AND (next_expected_at IS NULL OR next_expected_at <= now())
			RETURNING id, org_id`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id, org uuid.UUID
			if err := rows.Scan(&id, &org); err != nil {
				rows.Close()
				return err
			}
			uptime = append(uptime, id)
			uptimeOrgs = append(uptimeOrgs, org)
		}
		rows.Close()
		return rows.Err()
	})
	if err != nil {
		return err
	}
	for _, n := range notes {
		s.enqueueNotify(ctx, n.OrgID, n.T)
	}
	for i, id := range uptime {
		if _, err := queue.Enqueue(ctx, s.DB.Pool, UptimeJob, map[string]uuid.UUID{"org_id": uptimeOrgs[i], "monitor_id": id}, time.Time{}); err != nil {
			return err
		}
	}
	return nil
}

// Run ticks until ctx is done.
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		if err := s.Tick(ctx); err != nil && ctx.Err() == nil {
			s.Log.Error("monitor: tick", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// CheckUptime is the queue handler that performs one HTTP check.
func (s *Service) CheckUptime(ctx context.Context, j queue.Job) error {
	var p struct {
		OrgID     uuid.UUID `json:"org_id"`
		MonitorID uuid.UUID `json:"monitor_id"`
	}
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		return err
	}
	var target, name, prev string
	var timeout, expected int
	err := s.DB.InScope(ctx, db.Scope{OrgID: p.OrgID}, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT url, name, status, timeout_seconds, expected_status FROM monitors WHERE id = $1 AND enabled`, p.MonitorID).
			Scan(&target, &name, &prev, &timeout, &expected)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	status, detail, dur := s.probe(ctx, target, time.Duration(timeout)*time.Second, expected)
	var note *transition
	err = s.DB.InScope(ctx, db.Scope{OrgID: p.OrgID}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO monitor_checkins (id, org_id, monitor_id, status, started_at, finished_at, duration_ms, details)
			VALUES ($1, $2, $3, $4, now() - make_interval(secs => $5 / 1000.0), now(), $5, $6)`,
			uuid.New(), p.OrgID, p.MonitorID, status, dur, detail); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE monitors SET status = $2, last_checkin_at = now() WHERE id = $1`, p.MonitorID, status)
		note = changed(p.MonitorID, name, prev, status, detail)
		return err
	})
	if err == nil && note != nil {
		s.enqueueNotify(ctx, p.OrgID, *note)
	}
	return err
}

func (s *Service) probe(ctx context.Context, target string, timeout time.Duration, expected int) (status, detail string, ms float64) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "error", "invalid URL", 0
	}
	req.Header.Set("User-Agent", "ErrorLens-Uptime/1.0")
	resp, err := s.HTTP.Do(req)
	ms = float64(time.Since(start).Microseconds()) / 1000
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "timeout", fmt.Sprintf("no response within %s", timeout), ms
		}
		return "error", clip(err.Error(), 300), ms
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
	ok := resp.StatusCode < 400
	if expected > 0 {
		ok = resp.StatusCode == expected
	}
	if !ok {
		return "error", fmt.Sprintf("HTTP %d", resp.StatusCode), ms
	}
	return "ok", fmt.Sprintf("HTTP %d", resp.StatusCode), ms
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Notify is the queue handler that alerts a monitor's channels.
func (s *Service) Notify(sender *notify.Sender) queue.Handler {
	return func(ctx context.Context, j queue.Job) error {
		var p notifyPayload
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			return err
		}
		var channels []uuid.UUID
		var orgSlug, project string
		err := s.DB.InScope(ctx, db.Scope{OrgID: p.OrgID}, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT m.channel_ids, o.slug, pr.slug FROM monitors m
				JOIN organizations o ON o.id = m.org_id JOIN projects pr ON pr.id = m.project_id WHERE m.id = $1`, p.T.MonitorID).
				Scan(&channels, &orgSlug, &project)
		})
		if err != nil || len(channels) == 0 {
			return nil
		}
		reason := map[string]string{"error": "Monitor failing", "missed": "Missed check-in", "timeout": "Monitor timed out", "ok": "Monitor recovered"}[p.T.To]
		link := s.OrgURL(orgSlug, "monitors/"+p.T.MonitorID.String())
		msg := notify.Message{
			Subject: fmt.Sprintf("[%s] %s: %s", project, reason, p.T.Name), Reason: reason, Title: p.T.Name,
			Culprit: p.T.Detail, Project: project, Level: map[bool]string{true: "info", false: "error"}[p.T.To == "ok"],
			URL: link, FiredAt: time.Now().UTC(), RuleName: "monitor", Org: orgSlug,
		}
		var lastErr error
		for _, ch := range channels {
			var c notify.Channel
			err := s.DB.InScope(ctx, db.Scope{OrgID: p.OrgID}, func(tx pgx.Tx) error {
				return tx.QueryRow(ctx, `SELECT kind, config FROM alert_channels WHERE id = $1 AND enabled`, ch).Scan(&c.Kind, &c.Config)
			})
			if err != nil {
				continue
			}
			status, errText := "sent", ""
			if err := sender.Send(ctx, c, msg); err != nil {
				status, errText, lastErr = "failed", err.Error(), err
			}
			_ = s.DB.InScope(ctx, db.Scope{OrgID: p.OrgID}, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `INSERT INTO alert_history (org_id, channel_id, monitor_id, status, error) VALUES ($1, $2, $3, $4, $5)`,
					p.OrgID, ch, p.T.MonitorID, status, errText)
				return err
			})
		}
		if lastErr != nil {
			s.Log.Warn("monitor: notification failed", "err", lastErr)
		}
		return nil // don't retry the whole fan-out; failures are recorded
	}
}
