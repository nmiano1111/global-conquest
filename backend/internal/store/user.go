package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type User struct {
	ID          string
	Email       string
	DisplayName string
	CreatedAt   time.Time
}

type NewUser struct {
	Email       string
	DisplayName string
}

type UsersStore interface {
	Create(ctx context.Context, tx pgx.Tx, in NewUser) (User, error)
	GetByEmail(ctx context.Context, tx pgx.Tx, email string) (User, error)
}

type PostgresUsersStore struct{}

func NewPostgresUsersStore() *PostgresUsersStore { return &PostgresUsersStore{} }

func (s *PostgresUsersStore) Create(ctx context.Context, tx pgx.Tx, in NewUser) (User, error) {
	const q = `
		INSERT INTO users (email, display_name)
		VALUES ($1, $2)
		RETURNING id::text, email, display_name, created_at
	`
	var u User
	err := tx.QueryRow(ctx, q, in.Email, in.DisplayName).Scan(
		&u.ID, &u.Email, &u.DisplayName, &u.CreatedAt,
	)
	return u, err
}

func (s *PostgresUsersStore) GetByEmail(ctx context.Context, tx pgx.Tx, email string) (User, error) {
	const q = `
		SELECT id::text, email, display_name, created_at
		FROM users
		WHERE email = $1
	`
	var u User
	err := tx.QueryRow(ctx, q, email).Scan(
		&u.ID, &u.Email, &u.DisplayName, &u.CreatedAt,
	)
	return u, err
}
