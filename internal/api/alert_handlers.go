package api

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/errorlenshq/errorlens/internal/audit"
	"github.com/errorlenshq/errorlens/internal/db"
	"github.com/errorlenshq/errorlens/internal/notify"
)

// AlertChannel is a notification destination. Secrets are never returned.
type AlertChannel struct {
	ID        uuid.UUID `json:"id"`
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	Target    string    `json:"target"` // masked URL or email list
	HasSecret bool      `json:"hasSecret"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
}

type channelRequest struct {
	Kind    string   `json:"kind"`
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Secret  string   `json:"secret"`
	Emails  []string `json:"emails"`
	Enabled *bool    `json:"enabled"`
}

func maskURL(raw string) string {
	if i := strings.Index(raw, "://"); i >= 0 {
		rest := raw[i+3:]
		if j := strings.IndexByte(rest, '/'); j >= 0 {
			return raw[:i+3] + rest[:j] + "/…"
		}
	}
	return raw
}

func (s *Server) listChannels(w http.ResponseWriter, r *http.Request) {
	out := []AlertChannel{}
	err := s.db.InScope(r.Context(), scope(r), func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `SELECT id, kind, name, config, enabled, created_at FROM alert_channels ORDER BY name`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c AlertChannel
			var cfg notify.ChannelConfig
			if err := rows.Scan(&c.ID, &c.Kind, &c.Name, &cfg, &c.Enabled, &c.CreatedAt); err != nil {
				return err
			}
			if c.Kind == "email" {
				c.Target = strings.Join(cfg.Emails, ", ")
			} else {
				c.Target = maskURL(cfg.URL)
			}
			c.HasSecret = cfg.Secret != ""
			out = append(out, c)
		}
		return rows.Err()
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func validateChannel(req *channelRequest) (notify.ChannelConfig, string) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 100 {
		return notify.ChannelConfig{}, "name is required (max 100 characters)"
	}
	switch req.Kind {
	case "webhook", "slack", "discord":
		if err := notify.ValidateURL(req.URL); err != nil {
			return notify.ChannelConfig{}, "url " + err.Error()
		}
		if len(req.Secret) > 200 {
			return notify.ChannelConfig{}, "secret too long"
		}
		return notify.ChannelConfig{URL: req.URL, Secret: req.Secret}, ""
	case "email":
		var emails []string
		for _, e := range req.Emails {
			e = strings.TrimSpace(e)
			if e == "" {
				continue
			}
			if a, err := mail.ParseAddress(e); err != nil || a.Address != e {
				return notify.ChannelConfig{}, "invalid email: " + e
			}
			emails = append(emails, e)
		}
		if len(emails) == 0 || len(emails) > 50 {
			return notify.ChannelConfig{}, "between 1 and 50 email addresses are required"
		}
		return notify.ChannelConfig{Emails: emails}, ""
	}
	return notify.ChannelConfig{}, "kind must be email, slack, discord or webhook"
}

func (s *Server) createChannel(w http.ResponseWriter, r *http.Request) {
	var req channelRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	cfg, msg := validateChannel(&req)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	id := uuid.New()
	ctx := r.Context()
	err := s.db.InScope(ctx, scope(r), func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO alert_channels (id, org_id, kind, name, config) VALUES ($1, $2, $3, $4, $5)`,
			id, orgFrom(ctx).ID, req.Kind, req.Name, cfg); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{OrgID: orgFrom(ctx).ID, ActorID: userFrom(ctx).ID, Action: "alert_channel.create", Target: req.Name, IP: clientIP(r)})
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) updateChannel(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "channelID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	var req channelRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ctx := r.Context()
	err = s.db.InScope(ctx, scope(r), func(tx pgx.Tx) error {
		var old notify.ChannelConfig
		var kind string
		if err := tx.QueryRow(ctx, `SELECT kind, config FROM alert_channels WHERE id = $1`, id).Scan(&kind, &old); err != nil {
			return err
		}
		req.Kind = kind
		// Keep the stored secret/URL when the client leaves them blank.
		if req.URL == "" {
			req.URL = old.URL
		}
		if req.Secret == "" {
			req.Secret = old.Secret
		}
		if kind == "email" && len(req.Emails) == 0 {
			req.Emails = old.Emails
		}
		cfg, msg := validateChannel(&req)
		if msg != "" {
			return &httpError{http.StatusBadRequest, msg}
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		_, err := tx.Exec(ctx, `UPDATE alert_channels SET name = $2, config = $3, enabled = $4 WHERE id = $1`, id, req.Name, cfg, enabled)
		return err
	})
	s.finish(w, r, err, map[string]any{"id": id})
}

func (s *Server) deleteChannel(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "channelID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	ctx := r.Context()
	err = s.db.InScope(ctx, scope(r), func(tx pgx.Tx) error {
		var name string
		if err := tx.QueryRow(ctx, `DELETE FROM alert_channels WHERE id = $1 RETURNING name`, id).Scan(&name); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE alert_rules SET channel_ids = array_remove(channel_ids, $1)`, id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{OrgID: orgFrom(ctx).ID, ActorID: userFrom(ctx).ID, Action: "alert_channel.delete", Target: name, IP: clientIP(r)})
	})
	s.finish(w, r, err, nil)
}

// testChannel sends a sample notification so admins can verify a destination.
func (s *Server) testChannel(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "channelID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	var ch notify.Channel
	err = s.db.InScope(r.Context(), scope(r), func(tx pgx.Tx) error {
		return tx.QueryRow(r.Context(), `SELECT kind, config FROM alert_channels WHERE id = $1`, id).Scan(&ch.Kind, &ch.Config)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	org := orgFrom(r.Context())
	msg := notify.Message{
		Subject: "[ErrorLens] Test notification", Reason: "Test notification", Title: "This is a test alert from ErrorLens",
		Culprit: "settings/alerts", Project: "example", Level: "info", Count: 1, FiredAt: time.Now().UTC(),
		RuleName: "test", IsTest: true, Org: org.Slug, URL: s.cfg.PublicURL.String() + "/" + org.Slug + "/alerts",
	}
	if err := s.sender.Send(r.Context(), ch, msg); err != nil {
		writeError(w, http.StatusBadGateway, "delivery failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// AlertRule as returned by the API.
type AlertRule struct {
	ID          uuid.UUID   `json:"id"`
	Name        string      `json:"name"`
	Project     *string     `json:"project"` // slug; null = all projects
	Trigger     string      `json:"trigger"`
	Threshold   int         `json:"threshold"`
	Window      int         `json:"windowMinutes"`
	MinLevel    string      `json:"minLevel"`
	Environment string      `json:"environment"`
	ChannelIDs  []uuid.UUID `json:"channelIds"`
	Cooldown    int         `json:"cooldownMinutes"`
	Enabled     bool        `json:"enabled"`
}

type ruleRequest struct {
	Name        string      `json:"name"`
	Project     string      `json:"project"`
	Trigger     string      `json:"trigger"`
	Threshold   int         `json:"threshold"`
	Window      int         `json:"windowMinutes"`
	MinLevel    string      `json:"minLevel"`
	Environment string      `json:"environment"`
	ChannelIDs  []uuid.UUID `json:"channelIds"`
	Cooldown    int         `json:"cooldownMinutes"`
	Enabled     *bool       `json:"enabled"`
}

func (s *Server) listRules(w http.ResponseWriter, r *http.Request) {
	out := []AlertRule{}
	err := s.db.InScope(r.Context(), scope(r), func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `
			SELECT a.id, a.name, p.slug, a.trigger, a.threshold, a.window_minutes, a.min_level, a.environment,
			       a.channel_ids, a.cooldown_minutes, a.enabled
			FROM alert_rules a LEFT JOIN projects p ON p.id = a.project_id ORDER BY a.created_at`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowToStructByPos[AlertRule])
		return err
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if out == nil {
		out = []AlertRule{}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) saveRule(w http.ResponseWriter, r *http.Request) {
	var req ruleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	switch {
	case req.Name == "" || len(req.Name) > 100:
		writeError(w, http.StatusBadRequest, "name is required (max 100 characters)")
		return
	case req.Trigger != "new_issue" && req.Trigger != "regression" && req.Trigger != "frequency":
		writeError(w, http.StatusBadRequest, "trigger must be new_issue, regression or frequency")
		return
	case req.Trigger == "frequency" && (req.Threshold < 1 || req.Window < 1 || req.Window > 1440):
		writeError(w, http.StatusBadRequest, "frequency rules need threshold ≥ 1 and a window of 1–1440 minutes")
		return
	}
	if req.MinLevel == "" {
		req.MinLevel = "error"
	}
	if _, ok := map[string]bool{"debug": true, "info": true, "warning": true, "error": true, "fatal": true}[req.MinLevel]; !ok {
		writeError(w, http.StatusBadRequest, "invalid minLevel")
		return
	}
	if req.Window == 0 {
		req.Window = 60
	}
	if req.Cooldown <= 0 {
		req.Cooldown = 60
	}
	if req.ChannelIDs == nil {
		req.ChannelIDs = []uuid.UUID{}
	}
	enabled := req.Enabled == nil || *req.Enabled

	ctx := r.Context()
	ruleID := uuid.New()
	editing := chi.URLParam(r, "ruleID") != ""
	if editing {
		var err error
		if ruleID, err = uuid.Parse(chi.URLParam(r, "ruleID")); err != nil {
			writeError(w, http.StatusNotFound, "rule not found")
			return
		}
	}
	err := s.db.InScope(ctx, scope(r), func(tx pgx.Tx) error {
		var projectID *uuid.UUID
		if req.Project != "" {
			var id uuid.UUID
			if err := tx.QueryRow(ctx, `SELECT id FROM projects WHERE slug = $1`, req.Project).Scan(&id); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return &httpError{http.StatusBadRequest, "unknown project"}
				}
				return err
			}
			projectID = &id
		}
		var valid int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM alert_channels WHERE id = ANY($1)`, req.ChannelIDs).Scan(&valid); err != nil {
			return err
		}
		if valid != len(req.ChannelIDs) {
			return &httpError{http.StatusBadRequest, "unknown channel"}
		}
		if editing {
			tag, err := tx.Exec(ctx, `UPDATE alert_rules SET name=$2, project_id=$3, trigger=$4, threshold=$5, window_minutes=$6,
				min_level=$7, environment=$8, channel_ids=$9, cooldown_minutes=$10, enabled=$11 WHERE id=$1`,
				ruleID, req.Name, projectID, req.Trigger, req.Threshold, req.Window, req.MinLevel, req.Environment, req.ChannelIDs, req.Cooldown, enabled)
			if err == nil && tag.RowsAffected() == 0 {
				return pgx.ErrNoRows
			}
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO alert_rules (id, org_id, name, project_id, trigger, threshold, window_minutes,
			min_level, environment, channel_ids, cooldown_minutes, enabled) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			ruleID, orgFrom(ctx).ID, req.Name, projectID, req.Trigger, req.Threshold, req.Window, req.MinLevel, req.Environment,
			req.ChannelIDs, req.Cooldown, enabled); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{OrgID: orgFrom(ctx).ID, ActorID: userFrom(ctx).ID, Action: "alert_rule.create", Target: req.Name, IP: clientIP(r)})
	})
	status := http.StatusCreated
	if editing {
		status = http.StatusOK
	}
	if err == nil {
		writeJSON(w, status, map[string]any{"id": ruleID})
		return
	}
	s.finish(w, r, err, nil)
}

func (s *Server) deleteRule(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "ruleID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}
	err = s.db.InScope(r.Context(), scope(r), func(tx pgx.Tx) error {
		tag, err := tx.Exec(r.Context(), `DELETE FROM alert_rules WHERE id = $1`, id)
		if err == nil && tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return err
	})
	s.finish(w, r, err, nil)
}

type alertHistoryEntry struct {
	ID        int64     `json:"id"`
	Rule      *string   `json:"rule"`
	IssueID   *int64    `json:"issueId"`
	Issue     *string   `json:"issueTitle"`
	Channel   *string   `json:"channel"`
	Status    string    `json:"status"`
	Error     string    `json:"error"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Server) listAlertHistory(w http.ResponseWriter, r *http.Request) {
	out := []alertHistoryEntry{}
	err := s.db.InScope(r.Context(), scope(r), func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `
			SELECT h.id, ar.name, h.issue_id, i.title, c.name, h.status, h.error, h.created_at
			FROM alert_history h
			LEFT JOIN alert_rules ar ON ar.id = h.rule_id
			LEFT JOIN issues i ON i.id = h.issue_id
			LEFT JOIN alert_channels c ON c.id = h.channel_id
			ORDER BY h.created_at DESC LIMIT 100`)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowToStructByPos[alertHistoryEntry])
		return err
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if out == nil {
		out = []alertHistoryEntry{}
	}
	writeJSON(w, http.StatusOK, out)
}

// --- shared error plumbing ------------------------------------------------------

type httpError struct {
	status int
	msg    string
}

func (e *httpError) Error() string { return e.msg }

// finish writes the standard response for a mutating handler.
func (s *Server) finish(w http.ResponseWriter, r *http.Request, err error, ok any) {
	var he *httpError
	switch {
	case err == nil:
		if ok == nil {
			w.WriteHeader(http.StatusNoContent)
		} else {
			writeJSON(w, http.StatusOK, ok)
		}
	case errors.As(err, &he):
		writeError(w, he.status, he.msg)
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "not found")
	case db.IsUniqueViolation(err):
		writeError(w, http.StatusConflict, "already exists")
	default:
		s.internalError(w, r, err)
	}
}
