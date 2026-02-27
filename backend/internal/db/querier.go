package db

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Querier is the minimal read/write query surface shared by pgx pool and tx.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}
