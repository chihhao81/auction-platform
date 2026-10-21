// Package database opens the PostgreSQL pool and applies migrations.
package database

import (
	"context"
	"errors"
	"fmt"
	neturl "net/url"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // registers pgx5://
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"

	"auction-platform/backend/migrations"
)

// Open creates a small connection pool sized for a 1 GB VM talking to Neon.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MinConns = 0 // let Neon scale to zero when idle
	cfg.MaxConnIdleTime = 5 * time.Minute
	// Every session runs in UTC so NOW() and timestamp rendering are consistent.
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// Migrate applies all pending up migrations embedded in the binary. It always
// connects directly (see DirectURL): golang-migrate relies on session-level
// advisory locks, which a transaction-mode pooler does not preserve.
func Migrate(url string) error {
	url = DirectURL(url)
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("migration source: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, toMigrateURL(url))
	if err != nil {
		return fmt.Errorf("init migrate: %w", err)
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

// DirectURL turns a Neon pooled URL (host "ep-xxx-pooler.region...") into
// the direct endpoint. Session-level features (advisory locks, SET,
// search_path startup parameters) need a direct connection. Other URLs are
// returned unchanged.
func DirectURL(raw string) string {
	u, err := neturl.Parse(raw)
	if err != nil {
		return raw
	}
	host, port := u.Hostname(), u.Port()
	first, rest, ok := strings.Cut(host, ".")
	if !ok || !strings.HasSuffix(first, "-pooler") {
		return raw
	}
	host = strings.TrimSuffix(first, "-pooler") + "." + rest
	if port != "" {
		host += ":" + port
	}
	u.Host = host
	return u.String()
}

// toMigrateURL rewrites postgres:// URLs to the pgx5:// scheme golang-migrate
// uses for its pgx v5 driver.
func toMigrateURL(url string) string {
	for _, p := range []string{"postgresql://", "postgres://"} {
		if rest, ok := strings.CutPrefix(url, p); ok {
			return "pgx5://" + rest
		}
	}
	return url
}
