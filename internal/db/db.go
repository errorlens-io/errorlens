// Package db owns the Postgres connection pool, schema migrations and the
// tenant-scoped transaction helpers that enforce Row-Level Security.
package db

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// AppRole is the restricted role that application queries run as.
const AppRole = "errorlens_app"

// DB wraps a pgx pool.
type DB struct {
	Pool *pgxpool.Pool
}

// Open connects to Postgres and verifies the connection.
func Open(ctx context.Context, url string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	// pgx defaults to max(4, CPUs) connections, which a busy server outgrows:
	// the job listener holds one, workers and requests share the rest.
	// pool_max_conns in the URL still wins.
	if !strings.Contains(url, "pool_max_conns") && cfg.MaxConns < 20 {
		cfg.MaxConns = 20
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &DB{Pool: pool}, nil
}

// Close releases all connections.
func (d *DB) Close() { d.Pool.Close() }

// Migrate applies all pending embedded migrations. It holds an advisory lock
// so several replicas starting at once do not race.
func (d *DB) Migrate(ctx context.Context) (applied []string, err error) {
	conn, err := d.Pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	const lockID = 7_316_105_118 // arbitrary, stable
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, lockID); err != nil {
		return nil, err
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, lockID) //nolint:errcheck

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return nil, err
	}

	names, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)

	for _, name := range names {
		version := strings.TrimSuffix(strings.TrimPrefix(name, "migrations/"), ".sql")
		var exists bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&exists); err != nil {
			return applied, err
		}
		if exists {
			continue
		}
		sql, err := migrationFS.ReadFile(name)
		if err != nil {
			return applied, err
		}
		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version)
			return err
		})
		if err != nil {
			return applied, fmt.Errorf("migration %s: %w", version, err)
		}
		applied = append(applied, version)
	}
	return applied, nil
}

// Scope describes who a transaction runs on behalf of.
type Scope struct {
	OrgID  uuid.UUID // uuid.Nil = no organization selected
	UserID uuid.UUID // uuid.Nil = anonymous
}

// InScope runs fn in a transaction as the restricted application role with
// the given tenant scope. Row-Level Security policies then limit every query
// to rows the scope may see. Use this for all request-driven data access.
func (d *DB) InScope(ctx context.Context, s Scope, fn func(pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, d.Pool, func(tx pgx.Tx) error {
		if err := ApplyScope(ctx, tx, s); err != nil {
			return err
		}
		return fn(tx)
	})
}

// ApplyScope drops the current transaction to the application role with the
// given scope. It cannot be undone within the transaction.
func ApplyScope(ctx context.Context, tx pgx.Tx, s Scope) error {
	if _, err := tx.Exec(ctx, `SET LOCAL ROLE `+AppRole); err != nil {
		return fmt.Errorf("set role: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`SELECT set_config('app.org_id', $1, true), set_config('app.user_id', $2, true)`,
		uuidOrEmpty(s.OrgID), uuidOrEmpty(s.UserID)); err != nil {
		return fmt.Errorf("set scope: %w", err)
	}
	return nil
}

// System runs fn in a transaction as the connecting (owner) role, bypassing
// Row-Level Security. Only use it for work that is genuinely cross-tenant:
// authentication lookups, background jobs, maintenance.
func (d *DB) System(ctx context.Context, fn func(pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, d.Pool, fn)
}

func uuidOrEmpty(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}

// IsUniqueViolation reports whether err is a Postgres unique-constraint error.
func IsUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	return errors.As(err, &pgErr) && pgErr.SQLState() == "23505"
}
