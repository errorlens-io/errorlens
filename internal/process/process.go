package process

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/errorlens-io/errorlens/internal/crypto"
	"github.com/errorlens-io/errorlens/internal/db"
	"github.com/errorlens-io/errorlens/internal/queue"
	"github.com/errorlens-io/errorlens/internal/sourcemap"
)

// JobKind is the queue job kind for event processing.
const JobKind = "event.process"

// Job is the queue payload produced by ingest. Event is already scrubbed.
type Job struct {
	OrgID      uuid.UUID      `json:"org_id"`
	ProjectID  uuid.UUID      `json:"project_id"`
	ReceivedAt time.Time      `json:"received_at"`
	Scrubbed   int            `json:"scrubbed"`
	Event      map[string]any `json:"event"`
}

// Outcome describes what happened to an event's issue.
type Outcome struct {
	IssueID   int64
	EventID   uuid.UUID
	New       bool // first event of a new issue
	Regressed bool // a resolved issue received a new event
	Duplicate bool // the event id was already stored
}

// Processor stores events and maintains issues.
type Processor struct {
	DB *db.DB
	// Keyring, when set, encrypts stored event payloads.
	Keyring *crypto.Keyring
	// SourceMaps, when set, maps minified JavaScript frames to original sources.
	SourceMaps *sourcemap.Resolver
	// OnOutcome is called after an event commits (e.g. to trigger alerts).
	OnOutcome func(ctx context.Context, job *Job, ev *Event, out Outcome)
}

// Handle implements queue.Handler.
func (p *Processor) Handle(ctx context.Context, qj queue.Job) error {
	var job Job
	if err := json.Unmarshal(qj.Payload, &job); err != nil {
		return fmt.Errorf("decode job: %w", err) // retried, then parked with last_error
	}
	_, err := p.Process(ctx, &job)
	return err
}

// Process normalizes, groups and stores one event.
func (p *Processor) Process(ctx context.Context, job *Job) (Outcome, error) {
	if p.SourceMaps != nil {
		p.SourceMaps.Apply(ctx, job.OrgID, job.ProjectID, job.Event)
	}
	ev := Normalize(job.Event, job.ReceivedAt)
	hash, _ := Fingerprint(ev)
	out := Outcome{EventID: ev.ID}

	err := p.DB.InScope(ctx, db.Scope{OrgID: job.OrgID}, func(tx pgx.Tx) error {
		// SDKs retry; drop exact duplicates seen recently.
		var dup bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM events
			WHERE project_id = $1 AND id = $2 AND received_at > $3)`,
			job.ProjectID, ev.ID, job.ReceivedAt.Add(-24*time.Hour)).Scan(&dup); err != nil {
			return err
		}
		if dup {
			out.Duplicate = true
			return nil
		}

		// Insert the issue, or lock the existing one.
		err := tx.QueryRow(ctx, `
			INSERT INTO issues (org_id, project_id, hash, title, culprit, level, platform,
			                    first_seen, last_seen, first_release, last_release)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8, $9, $9)
			ON CONFLICT (project_id, hash) DO NOTHING
			RETURNING id`,
			job.OrgID, job.ProjectID, hash, ev.Title, ev.Culprit, ev.Level, ev.Platform, ev.Timestamp, ev.Release,
		).Scan(&out.IssueID)
		switch {
		case err == nil:
			out.New = true
		case errors.Is(err, pgx.ErrNoRows):
			var status string
			if err := tx.QueryRow(ctx, `SELECT id, status FROM issues WHERE project_id = $1 AND hash = $2 FOR UPDATE`,
				job.ProjectID, hash).Scan(&out.IssueID, &status); err != nil {
				return err
			}
			out.Regressed = status == "resolved"
			if _, err := tx.Exec(ctx, `
				UPDATE issues SET
					times_seen   = times_seen + 1,
					last_seen    = GREATEST(last_seen, $2),
					first_seen   = LEAST(first_seen, $2),
					title        = $3,
					culprit      = $4,
					level        = $5,
					last_release = CASE WHEN $6 <> '' THEN $6 ELSE last_release END,
					status       = CASE WHEN status = 'resolved' THEN 'unresolved' ELSE status END,
					regressed_at = CASE WHEN status = 'resolved' THEN now() ELSE regressed_at END,
					resolved_at  = CASE WHEN status = 'resolved' THEN NULL ELSE resolved_at END
				WHERE id = $1`,
				out.IssueID, ev.Timestamp, ev.Title, ev.Culprit, ev.Level, ev.Release); err != nil {
				return err
			}
		default:
			return err
		}

		payload, err := json.Marshal(ev.Payload)
		if err != nil {
			return err
		}
		var sealed []byte
		if p.Keyring != nil {
			if sealed, err = p.Keyring.Seal(ctx, job.OrgID, payload, ev.ID[:]); err != nil {
				return err
			}
			payload = []byte("{}")
		}
		tags, _ := json.Marshal(ev.Tags)
		if _, err := tx.Exec(ctx, `
			INSERT INTO events (id, org_id, project_id, issue_id, received_at, timestamp, level, title,
			                    release, environment, platform, tags, scrubbed, payload, payload_ct)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
			ev.ID, job.OrgID, job.ProjectID, out.IssueID, job.ReceivedAt, ev.Timestamp, ev.Level, ev.Title,
			ev.Release, ev.Environment, ev.Platform, tags, job.Scrubbed, payload, sealed); err != nil {
			return err
		}
		if ev.Release != "" {
			if _, err := tx.Exec(ctx, `
				INSERT INTO releases (org_id, project_id, version, first_event, last_event) VALUES ($1, $2, $3, $4, $4)
				ON CONFLICT (project_id, version) DO UPDATE SET
					first_event = LEAST(COALESCE(releases.first_event, EXCLUDED.first_event), EXCLUDED.first_event),
					last_event  = GREATEST(COALESCE(releases.last_event, EXCLUDED.last_event), EXCLUDED.last_event)`,
				job.OrgID, job.ProjectID, ev.Release, job.ReceivedAt); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO issue_counts (org_id, project_id, issue_id, bucket, count)
			VALUES ($1, $2, $3, date_trunc('hour', $4::timestamptz), 1)
			ON CONFLICT (issue_id, bucket) DO UPDATE SET count = issue_counts.count + 1`,
			job.OrgID, job.ProjectID, out.IssueID, job.ReceivedAt)
		return err
	})
	if err != nil {
		return out, err
	}
	if p.OnOutcome != nil && !out.Duplicate {
		p.OnOutcome(ctx, job, ev, out)
	}
	return out, nil
}
