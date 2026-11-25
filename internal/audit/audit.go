// Package audit records security-relevant actions in the append-only audit log.
package audit

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Actions recorded by ErrorLens.
const (
	ActionSetup         = "instance.setup"
	ActionLogin         = "user.login"
	ActionLoginFailed   = "user.login_failed"
	ActionLogout        = "user.logout"
	ActionProjectCreate = "project.create"
	ActionEventView     = "event.view" // someone opened a full event payload
)

// Entry is one audit record.
type Entry struct {
	OrgID   uuid.UUID // uuid.Nil for instance-level events
	ActorID uuid.UUID // uuid.Nil if unknown
	Action  string
	Target  string
	IP      string
	Data    map[string]any
}

// Record writes e using tx. Call it inside the same transaction as the
// change being audited so the two commit or roll back together.
func Record(ctx context.Context, tx pgx.Tx, e Entry) error {
	data, err := json.Marshal(e.Data)
	if err != nil || e.Data == nil {
		data = []byte("{}")
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO audit_log (org_id, actor_id, action, target, ip, data) VALUES ($1, $2, $3, $4, $5, $6)`,
		nullable(e.OrgID), nullable(e.ActorID), e.Action, e.Target, e.IP, data)
	return err
}

func nullable(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id
}
