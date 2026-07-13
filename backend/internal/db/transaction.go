package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool and provides transaction helpers used throughout
// the store layer.
type DB struct {
	// Pool is the underlying Postgres connection pool.
	Pool *pgxpool.Pool
}

// New wraps pool in a DB.
func New(pool *pgxpool.Pool) *DB {
	return &DB{Pool: pool}
}

// Queryer returns the pool as a Querier, for non-transactional queries.
func (d *DB) Queryer() Querier {
	return d.Pool
}

// WithTx runs fn inside a Postgres transaction, committing if fn returns nil
// and rolling back otherwise. The transaction is unconditionally rolled back
// via defer as a safety net; that rollback is a no-op once Commit has
// already succeeded.
func (d *DB) WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// WithTxQ runs fn inside a Postgres transaction, passing it the transaction
// as a Querier. It is a convenience wrapper around WithTx for callers that
// only need to issue queries rather than access the pgx.Tx directly.
func (d *DB) WithTxQ(ctx context.Context, fn func(q Querier) error) error {
	return d.WithTx(ctx, func(tx pgx.Tx) error {
		return fn(tx)
	})
}
