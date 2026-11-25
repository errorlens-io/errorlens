// Package testutil provides a throwaway, migrated Postgres database for tests.
//
// Set ERRORLENS_TEST_DATABASE_URL to a Postgres URL whose user may create
// databases (e.g. postgres://postgres:postgres@localhost:5434/postgres).
// Tests that need a database are skipped when it is unset.
package testutil

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/errorlens-io/errorlens/internal/db"
)

// NewDB creates a fresh database, runs migrations and drops it on cleanup.
func NewDB(t *testing.T) *db.DB {
	t.Helper()
	admin := os.Getenv("ERRORLENS_TEST_DATABASE_URL")
	if admin == "" {
		t.Skip("ERRORLENS_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()

	name := fmt.Sprintf("errorlens_test_%d", time.Now().UnixNano())
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatalf("connect admin db: %v", err)
	}
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create db: %v", err)
	}
	conn.Close(ctx)

	u, err := url.Parse(admin)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	d, err := db.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	t.Cleanup(func() {
		d.Close()
		c, err := pgx.Connect(context.Background(), admin)
		if err != nil {
			return
		}
		defer c.Close(context.Background())
		_, _ = c.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})
	return d
}

// Slug returns a short unique slug for test fixtures.
func Slug(prefix string) string {
	return strings.ToLower(fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()%1_000_000_000))
}
