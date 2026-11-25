// Package alert evaluates alert rules when events arrive and queues
// notifications to the configured channels.
package alert

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/rootcausehq/rootcause/internal/db"
	"github.com/rootcausehq/rootcause/internal/notify"
	"github.com/rootcausehq/rootcause/internal/process"
	"github.com/rootcausehq/rootcause/internal/queue"
)

// Job kinds.
const (
	EvaluateJob = "alert.evaluate"
	DeliverJob  = "alert.deliver"
)

// Levels in increasing severity.
var levelRank = map[string]int{"debug": 0, "info": 1, "warning": 2, "error": 3, "fatal": 4}

// Engine evaluates rules and delivers notifications.
type Engine struct {
	DB        *db.DB
	Log       *slog.Logger
	Sender    *notify.Sender
	PublicURL *url.URL
}

type evaluatePayload struct {
	OrgID       uuid.UUID `json:"org_id"`
	ProjectID   uuid.UUID `json:"project_id"`
	IssueID     int64     `json:"issue_id"`
	New         bool      `json:"new"`
	Regressed   bool      `json:"regressed"`
	Level       string    `json:"level"`
	Environment string    `json:"environment"`
}

type deliverPayload struct {
	OrgID     uuid.UUID      `json:"org_id"`
	HistoryID int64          `json:"history_id"`
	ChannelID uuid.UUID      `json:"channel_id"`
	Message   notify.Message `json:"message"`
}

// OnOutcome is wired into the event processor: it queues rule evaluation.
func (e *Engine) OnOutcome(ctx context.Context, job *process.Job, ev *process.Event, out process.Outcome) {
	_, err := queue.Enqueue(ctx, e.DB.Pool, EvaluateJob, evaluatePayload{
		OrgID: job.OrgID, ProjectID: job.ProjectID, IssueID: out.IssueID,
		New: out.New, Regressed: out.Regressed, Level: ev.Level, Environment: ev.Environment,
	}, time.Time{})
	if err != nil {
		e.Log.Error("alert: enqueue evaluation", "err", err)
	}
}

type rule struct {
	ID         uuid.UUID
	Name       string
	Trigger    string
	Threshold  int
	Window     int
	Cooldown   int
	MinLevel   string
	ChannelIDs []uuid.UUID
}

// Evaluate is the queue handler for EvaluateJob.
func (e *Engine) Evaluate(ctx context.Context, j queue.Job) error {
	var p evaluatePayload
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		return err
	}
	type delivery struct {
		historyID int64
		channelID uuid.UUID
		msg       notify.Message
	}
	var deliveries []delivery

	err := e.DB.InScope(ctx, db.Scope{OrgID: p.OrgID}, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, name, trigger, threshold, window_minutes, cooldown_minutes, min_level, channel_ids
			FROM alert_rules
			WHERE enabled AND (project_id IS NULL OR project_id = $1)
			  AND (environment = '' OR environment = $2)`, p.ProjectID, p.Environment)
		if err != nil {
			return err
		}
		var rules []rule
		for rows.Next() {
			var r rule
			if err := rows.Scan(&r.ID, &r.Name, &r.Trigger, &r.Threshold, &r.Window, &r.Cooldown, &r.MinLevel, &r.ChannelIDs); err != nil {
				rows.Close()
				return err
			}
			rules = append(rules, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(rules) == 0 {
			return nil
		}
		var title, culprit, level, status, projectSlug, orgSlug string
		var count int64
		if err := tx.QueryRow(ctx, `
			SELECT i.title, i.culprit, i.level, i.status, i.times_seen, p.slug, o.slug
			FROM issues i JOIN projects p ON p.id = i.project_id JOIN organizations o ON o.id = i.org_id
			WHERE i.id = $1`, p.IssueID).Scan(&title, &culprit, &level, &status, &count, &projectSlug, &orgSlug); err != nil {
			return err
		}
		if status == "ignored" {
			return nil
		}

		for _, r := range rules {
			if levelRank[p.Level] < levelRank[r.MinLevel] {
				continue
			}
			reason := ""
			switch r.Trigger {
			case "new_issue":
				if p.New {
					reason = "New issue"
				}
			case "regression":
				if p.Regressed {
					reason = "Regression"
				}
			case "frequency":
				var n int
				if err := tx.QueryRow(ctx, `SELECT count(*) FROM events WHERE issue_id = $1 AND received_at > now() - make_interval(mins => $2)`,
					p.IssueID, r.Window).Scan(&n); err != nil {
					return err
				}
				if r.Threshold > 0 && n >= r.Threshold {
					reason = fmt.Sprintf("Spike: %d events in %d min", n, r.Window)
				}
			}
			if reason == "" || len(r.ChannelIDs) == 0 {
				continue
			}
			// Serialize per rule+issue and respect the cooldown.
			if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, r.ID.String()+fmt.Sprint(p.IssueID)); err != nil {
				return err
			}
			var recent bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM alert_history
				WHERE rule_id = $1 AND issue_id = $2 AND created_at > now() - make_interval(mins => $3))`,
				r.ID, p.IssueID, r.Cooldown).Scan(&recent); err != nil {
				return err
			}
			if recent && r.Trigger != "regression" {
				continue
			}
			msg := notify.Message{
				Subject:  fmt.Sprintf("[%s] %s: %s", projectSlug, reason, clip(title, 120)),
				Reason:   reason,
				Title:    title,
				Culprit:  culprit,
				Project:  projectSlug,
				Level:    level,
				IssueID:  p.IssueID,
				Count:    count,
				FiredAt:  time.Now().UTC(),
				RuleName: r.Name,
				Org:      orgSlug,
				URL:      e.issueURL(orgSlug, p.IssueID),
			}
			for _, chID := range r.ChannelIDs {
				var hid int64
				err := tx.QueryRow(ctx, `
					INSERT INTO alert_history (org_id, rule_id, issue_id, channel_id)
					SELECT $1, $2, $3, c.id FROM alert_channels c WHERE c.id = $4 AND c.enabled
					RETURNING id`, p.OrgID, r.ID, p.IssueID, chID).Scan(&hid)
				if errors.Is(err, pgx.ErrNoRows) {
					continue // channel deleted or disabled
				}
				if err != nil {
					return err
				}
				deliveries = append(deliveries, delivery{hid, chID, msg})
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, d := range deliveries {
		if _, err := queue.Enqueue(ctx, e.DB.Pool, DeliverJob, deliverPayload{
			OrgID: p.OrgID, HistoryID: d.historyID, ChannelID: d.channelID, Message: d.msg,
		}, time.Time{}); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) issueURL(org string, id int64) string {
	u := *e.PublicURL
	u.Path = fmt.Sprintf("%s/%s/issues/%d", u.Path, org, id)
	return u.String()
}

// Deliver is the queue handler for DeliverJob. Errors are retried by the queue.
func (e *Engine) Deliver(ctx context.Context, j queue.Job) error {
	var p deliverPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		return err
	}
	ch, err := LoadChannel(ctx, e.DB, p.OrgID, p.ChannelID)
	if err != nil {
		return err
	}
	sendErr := e.Sender.Send(ctx, ch, p.Message)
	status, msg := "sent", ""
	if sendErr != nil {
		status, msg = "failed", sendErr.Error()
	}
	_ = e.DB.InScope(ctx, db.Scope{OrgID: p.OrgID}, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE alert_history SET status = $2, error = $3 WHERE id = $1`, p.HistoryID, status, msg)
		return err
	})
	return sendErr
}

// LoadChannel reads a channel's destination.
func LoadChannel(ctx context.Context, d *db.DB, orgID, id uuid.UUID) (notify.Channel, error) {
	var ch notify.Channel
	var cfg []byte
	err := d.InScope(ctx, db.Scope{OrgID: orgID}, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT kind, config FROM alert_channels WHERE id = $1`, id).Scan(&ch.Kind, &cfg)
	})
	if err != nil {
		return ch, err
	}
	err = json.Unmarshal(cfg, &ch.Config)
	return ch, err
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
