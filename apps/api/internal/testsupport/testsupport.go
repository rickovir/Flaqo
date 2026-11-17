// Package testsupport provides helpers for integration tests that need a
// real PostgreSQL database.
//
// Each test gets its own schema so tests are independent and can run
// concurrently without interfering with one another. The schema is
// dropped when the test finishes.
package testsupport

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SkipIfNoDatabase skips the test when no test database is configured.
//
// It reads TEST_DATABASE_URL, falling back to MIGRATION_DATABASE_URL.
// Integration tests need the migration role because they create schemas.
func SkipIfNoDatabase(t *testing.T) string {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("MIGRATION_DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL or MIGRATION_DATABASE_URL is not set; skipping integration test")
	}
	return dsn
}

// NewSchema creates an isolated schema containing the full application
// schema and returns a pool connected to it.
//
// The returned cleanup function drops the schema.
func NewSchema(t *testing.T, ddl string) (*pgxpool.Pool, func()) {
	t.Helper()

	dsn := SkipIfNoDatabase(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}

	schema := "ff_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, fmt.Sprintf("CREATE SCHEMA %q", schema)); err != nil {
		admin.Close()
		t.Fatalf("create test schema: %v", err)
	}

	// Point the pool at the new schema and apply the DDL there.
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		admin.Close()
		t.Fatalf("parse database config: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.MaxConns = 10
	cfg.MinConns = 0

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		admin.Close()
		t.Fatalf("create schema pool: %v", err)
	}

	if _, err := pool.Exec(ctx, ddl); err != nil {
		pool.Close()
		_, _ = admin.Exec(ctx, fmt.Sprintf("DROP SCHEMA %q CASCADE", schema))
		admin.Close()
		t.Fatalf("apply test schema: %v", err)
	}

	cleanup := func() {
		pool.Close()
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer dropCancel()
		_, _ = admin.Exec(dropCtx, fmt.Sprintf("DROP SCHEMA %q CASCADE", schema))
		admin.Close()
	}

	return pool, cleanup
}

// SeedUser inserts a user and returns its ID.
func SeedUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool, email string) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash)
		VALUES ($1, 'not-a-real-hash')
		RETURNING id
	`, email).Scan(&id)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

// Exec runs a statement, failing the test on error.
func Exec(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// QueryRow runs a query expecting exactly one row.
func QueryRow(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) pgx.Row {
	t.Helper()
	return pool.QueryRow(ctx, sql, args...)
}
