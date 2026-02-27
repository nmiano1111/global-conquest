package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *DB {
	return &DB{Pool: pool}
}

func (d *DB) Queryer() Querier {
	return d.Pool
}

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

func (d *DB) WithTxQ(ctx context.Context, fn func(q Querier) error) error {
	return d.WithTx(ctx, func(tx pgx.Tx) error {
		return fn(tx)
	})
}
