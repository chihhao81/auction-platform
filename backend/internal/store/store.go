// Package store wraps the sqlc queries with transaction support.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"auction-platform/backend/internal/database/db"
)

// Pool is the subset of *pgxpool.Pool the application uses. pgxmock pools
// satisfy it as well, which keeps the store testable without a database.
type Pool interface {
	db.DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
	Ping(ctx context.Context) error
}

type Store struct {
	*db.Queries
	pool Pool
}

func New(pool Pool) *Store {
	return &Store{Queries: db.New(pool), pool: pool}
}

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// InTx runs fn inside a transaction. The transaction is committed when fn
// returns nil and rolled back otherwise.
func (s *Store) InTx(ctx context.Context, fn func(q *db.Queries) error) (err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
				err = errors.Join(err, fmt.Errorf("rollback: %w", rbErr))
			}
		}
	}()
	if err = fn(db.New(tx)); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// IsNoRows reports whether err means the query matched no rows.
func IsNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
