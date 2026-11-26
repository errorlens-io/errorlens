package db_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/errorlenshq/errorlens/internal/db"
	"github.com/errorlenshq/errorlens/internal/testutil"
)

func TestMigrateIsIdempotent(t *testing.T) {
	d := testutil.NewDB(t)
	applied, err := d.Migrate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != 0 {
		t.Fatalf("second migrate applied %v", applied)
	}
}

// createOrg creates an org with one project, in the org's own scope.
func createOrg(t *testing.T, d *db.DB, slug string) uuid.UUID {
	t.Helper()
	org := uuid.New()
	err := d.InScope(context.Background(), db.Scope{OrgID: org}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(),
			`INSERT INTO organizations (id, name, slug) VALUES ($1, $2, $2)`, org, slug); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(),
			`INSERT INTO projects (id, org_id, name, slug) VALUES ($1, $2, 'api', 'api')`, uuid.New(), org)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return org
}

func TestRowLevelSecurityIsolatesTenants(t *testing.T) {
	d := testutil.NewDB(t)
	ctx := context.Background()
	orgA := createOrg(t, d, "org-a")
	orgB := createOrg(t, d, "org-b")

	count := func(s db.Scope, table string) int {
		var n int
		if err := d.InScope(ctx, s, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n)
		}); err != nil {
			t.Fatal(err)
		}
		return n
	}

	if n := count(db.Scope{OrgID: orgA}, "projects"); n != 1 {
		t.Fatalf("org A sees %d projects, want 1", n)
	}
	if n := count(db.Scope{OrgID: orgA}, "organizations"); n != 1 {
		t.Fatalf("org A sees %d orgs, want 1", n)
	}
	if n := count(db.Scope{}, "projects"); n != 0 {
		t.Fatalf("empty scope sees %d projects, want 0", n)
	}

	// Writing into another tenant must be rejected by WITH CHECK.
	err := d.InScope(ctx, db.Scope{OrgID: orgA}, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO projects (id, org_id, name, slug) VALUES ($1, $2, 'evil', 'evil')`, uuid.New(), orgB)
		return err
	})
	if err == nil {
		t.Fatal("cross-tenant insert succeeded")
	}

	// Updates cannot touch the other tenant's rows.
	var affected int64
	err = d.InScope(ctx, db.Scope{OrgID: orgA}, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE projects SET name = 'pwned' WHERE org_id = $1`, orgB)
		affected = tag.RowsAffected()
		return err
	})
	if err != nil || affected != 0 {
		t.Fatalf("cross-tenant update: affected=%d err=%v", affected, err)
	}

	// System scope sees everything.
	var total int
	if err := d.System(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT count(*) FROM projects").Scan(&total)
	}); err != nil || total != 2 {
		t.Fatalf("system sees %d projects (err %v), want 2", total, err)
	}
}

func TestUserScopeSeesOnlyMemberOrgs(t *testing.T) {
	d := testutil.NewDB(t)
	ctx := context.Background()
	orgA := createOrg(t, d, "org-a")
	createOrg(t, d, "org-b")

	user := uuid.New()
	if err := d.System(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO users (id, email, password_hash) VALUES ($1, 'a@example.com', 'x')`, user); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO memberships (org_id, user_id, role) VALUES ($1, $2, 'owner')`, orgA, user)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	var slugs []string
	if err := d.InScope(ctx, db.Scope{UserID: user}, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT slug FROM organizations ORDER BY slug`)
		if err != nil {
			return err
		}
		slugs, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(slugs) != 1 || slugs[0] != "org-a" {
		t.Fatalf("user sees orgs %v, want [org-a]", slugs)
	}
}

func TestAuditLogIsAppendOnly(t *testing.T) {
	d := testutil.NewDB(t)
	ctx := context.Background()
	org := createOrg(t, d, "org-a")
	s := db.Scope{OrgID: org}

	if err := d.InScope(ctx, s, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO audit_log (org_id, action) VALUES ($1, 'test')`, org)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`UPDATE audit_log SET action = 'x'`, `DELETE FROM audit_log`} {
		err := d.InScope(ctx, s, func(tx pgx.Tx) error { _, err := tx.Exec(ctx, q); return err })
		if err == nil {
			t.Fatalf("%q should be denied for the app role", q)
		}
	}
}

// The search functions (SECURITY DEFINER, migration 0009) and the rollup
// tables must never let one tenant see another's data.
func TestSearchAndRollupsAreTenantScoped(t *testing.T) {
	d := testutil.NewDB(t)
	ctx := context.Background()
	orgA := createOrg(t, d, "org-a")
	orgB := createOrg(t, d, "org-b")

	seed := func(org uuid.UUID, secret string) {
		t.Helper()
		err := d.InScope(ctx, db.Scope{OrgID: org}, func(tx pgx.Tx) error {
			var project uuid.UUID
			if err := tx.QueryRow(ctx, `SELECT id FROM projects`).Scan(&project); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO issues (org_id, project_id, hash, title, culprit, first_seen, last_seen)
				VALUES ($1, $2, 'h', $3, 'mod in fn', now(), now())`, org, project, "Error "+secret); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO logs (org_id, project_id, timestamp, severity, severity_number, body, attributes)
				VALUES ($1, $2, now(), 'info', 9, $3, '{}')`, org, project, "line "+secret); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO perf_hourly (org_id, project_id, name, hour, count, failures, sum_ms)
				VALUES ($1, $2, $3, date_trunc('hour', now()), 1, 0, 10)`, org, project, "GET /"+secret)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	seed(orgA, "alpha-secret")
	seed(orgB, "bravo-secret")

	query := func(s db.Scope, sql string, args ...any) int {
		t.Helper()
		var n int
		if err := d.InScope(ctx, s, func(tx pgx.Tx) error { return tx.QueryRow(ctx, sql, args...).Scan(&n) }); err != nil {
			t.Fatal(err)
		}
		return n
	}
	a := db.Scope{OrgID: orgA}
	if n := query(a, `SELECT count(*) FROM issue_search_ids('%secret%')`); n != 1 {
		t.Fatalf("issue search from org A found %d issues, want 1 (its own)", n)
	}
	if n := query(a, `SELECT count(*) FROM issue_search_ids('%bravo%')`); n != 0 {
		t.Fatalf("issue search from org A found org B's issue")
	}
	if n := query(a, `SELECT count(*) FROM log_search('%secret%', NULL, NULL, NULL, NULL, NULL, 100)`); n != 1 {
		t.Fatalf("log search from org A found %d lines, want 1", n)
	}
	if n := query(a, `SELECT count(*) FROM log_search('%bravo%', NULL, NULL, NULL, NULL, NULL, 100)`); n != 0 {
		t.Fatalf("log search from org A found org B's log line")
	}
	// Passing another tenant's project id doesn't help either.
	var projectB uuid.UUID
	if err := d.InScope(ctx, db.Scope{OrgID: orgB}, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id FROM projects`).Scan(&projectB)
	}); err != nil {
		t.Fatal(err)
	}
	if n := query(a, `SELECT count(*) FROM log_search('%%', $1, NULL, NULL, NULL, NULL, 100)`, projectB); n != 0 {
		t.Fatalf("log search with org B's project id returned rows to org A")
	}
	// No tenant set: the functions return nothing.
	if n := query(db.Scope{}, `SELECT count(*) FROM log_search('%%', NULL, NULL, NULL, NULL, NULL, 100)`); n != 0 {
		t.Fatalf("log search without a tenant returned %d rows", n)
	}
	if n := query(db.Scope{}, `SELECT count(*) FROM issue_search_ids('%%')`); n != 0 {
		t.Fatalf("issue search without a tenant returned %d rows", n)
	}
	// Rollups are under RLS like everything else.
	if n := query(a, `SELECT count(*) FROM perf_hourly`); n != 1 {
		t.Fatalf("org A sees %d rollup rows, want 1", n)
	}
	err := d.InScope(ctx, a, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO perf_hourly (org_id, project_id, name, hour, count, failures, sum_ms)
			VALUES ($1, $2, 'x', now(), 1, 0, 1)`, orgB, projectB)
		return err
	})
	if err == nil {
		t.Fatal("org A wrote a rollup row into org B")
	}
}
