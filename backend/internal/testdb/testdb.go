// Package testdb provides real-PostgreSQL integration test databases.
//
// Tests using it are skipped unless TEST_DATABASE_URL is set. Each test gets
// its own freshly migrated schema which is dropped afterwards, so the URL can
// point at a shared development database (e.g. a Neon dev branch) safely.
package testdb

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"auction-platform/backend/internal/database"
)

// New returns a pool bound to a new migrated schema, or skips the test.
func New(t *testing.T) *pgxpool.Pool {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}
	ctx := context.Background()
	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]

	admin, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Logf("drop schema %s: %v", schema, err)
		}
		admin.Close()
	})

	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	if err := database.Migrate(u.String()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := database.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// CreateUser inserts an ACTIVE user and returns its id.
func CreateUser(t *testing.T, pool *pgxpool.Pool, name string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(),
		"INSERT INTO users (line_user_id, display_name) VALUES ($1, $2) RETURNING id",
		fmt.Sprintf("test-%s-%s", name, uuid.NewString()[:8]), name).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
