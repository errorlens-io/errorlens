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

	payload, err := json.Marshal(ev.Payload)
	if err != nil {
		return out, err
	}
	var sealed []byte
	if p.Keyring != nil {
		if sealed, err = p.Keyring.Seal(ctx, job.OrgID, payload, ev.ID[:]); err != nil {
			return out, err
		}
		payload = []byte("{}")
	}
	tags, _ := json.Marshal(ev.Tags)

	// One statement per event: skip recent duplicates (SDKs retry), upsert
	// the issue (reopening it if it was resolved), store the event, and bump
	// the release and the hourly count. xmax = 0 marks a freshly inserted
	// issue; regressed_at = now() marks one this statement reopened.
	err = p.DB.InScope(ctx, db.Scope{OrgID: job.OrgID}, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			WITH dup AS (
				SELECT 1 FROM events
				WHERE project_id = $2 AND id = $3 AND received_at > $4::timestamptz - interval '24 hours'
				LIMIT 1
			), up AS (
				INSERT INTO issues (org_id, project_id, hash, title, culprit, level, platform,
				                    first_seen, last_seen, first_release, last_release)
				SELECT $1, $2, $5, $6, $7, $8, $9, $10, $10, $11, $11
				WHERE NOT EXISTS (SELECT 1 FROM dup)
				ON CONFLICT (project_id, hash) DO UPDATE SET
					times_seen   = issues.times_seen + 1,
					last_seen    = GREATEST(issues.last_seen, EXCLUDED.last_seen),
					first_seen   = LEAST(issues.first_seen, EXCLUDED.first_seen),
					title        = EXCLUDED.title,
					culprit      = EXCLUDED.culprit,
					level        = EXCLUDED.level,
					last_release = CASE WHEN EXCLUDED.last_release <> '' THEN EXCLUDED.last_release ELSE issues.last_release END,
					status       = CASE WHEN issues.status = 'resolved' THEN 'unresolved' ELSE issues.status END,
					regressed_at = CASE WHEN issues.status = 'resolved' THEN now() ELSE issues.regressed_at END,
					resolved_at  = CASE WHEN issues.status = 'resolved' THEN NULL ELSE issues.resolved_at END
				RETURNING id, xmax = 0 AS inserted, regressed_at IS NOT DISTINCT FROM now() AS regressed
			), ev AS (
				INSERT INTO events (id, org_id, project_id, issue_id, received_at, timestamp, level, title,
				                    release, environment, platform, tags, scrubbed, payload, payload_ct)
				SELECT $3, $1, $2, up.id, $4, $10, $8, $6, $11, $12, $9, $13, $14, $15, $16 FROM up
			), rel AS (
				INSERT INTO releases (org_id, project_id, version, first_event, last_event)
				SELECT $1, $2, $11, $4, $4 FROM up WHERE $11 <> ''
				ON CONFLICT (project_id, version) DO UPDATE SET
					first_event = LEAST(COALESCE(releases.first_event, EXCLUDED.first_event), EXCLUDED.first_event),
					last_event  = GREATEST(COALESCE(releases.last_event, EXCLUDED.last_event), EXCLUDED.last_event)
			), cnt AS (
				INSERT INTO issue_counts (org_id, project_id, issue_id, bucket, count)
				SELECT $1, $2, up.id, date_trunc('hour', $4::timestamptz), 1 FROM up
				ON CONFLICT (issue_id, bucket) DO UPDATE SET count = issue_counts.count + 1
			)
			SELECT id, inserted, regressed FROM up`,
			job.OrgID, job.ProjectID, ev.ID, job.ReceivedAt, hash, ev.Title, ev.Culprit, ev.Level, ev.Platform,
			ev.Timestamp, ev.Release, ev.Environment, tags, job.Scrubbed, payload, sealed,
		).Scan(&out.IssueID, &out.New, &out.Regressed)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		out.Duplicate = true // the duplicate check stopped the insert
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if p.OnOutcome != nil && !out.Duplicate {
		p.OnOutcome(ctx, job, ev, out)
	}
	return out, nil
}
