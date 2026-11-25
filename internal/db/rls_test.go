package db_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/rootcausehq/rootcause/internal/db"
	"github.com/rootcausehq/rootcause/internal/testutil"
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
