package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type User struct {
	ID        string
	UserName  string
	Role      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type NewUser struct {
	UserName string
}

type UsersStore interface {
	Create(ctx context.Context, tx pgx.Tx, in NewUser) (User, error)
	GetUser(ctx context.Context, tx pgx.Tx, userName string) (User, error)
}

type PostgresUsersStore struct{}

func NewPostgresUsersStore() *PostgresUsersStore { return &PostgresUsersStore{} }

func (s *PostgresUsersStore) Create(ctx context.Context, tx pgx.Tx, in NewUser) (User, error) {
	const q = `
		INSERT INTO users (username, password_hash)
		VALUES ($1, $2)
		RETURNING id::text, username, created_at
	`
	var u User
	err := tx.QueryRow(ctx, q, in.UserName, "temp_pw_hash").Scan(
		&u.ID, &u.UserName, &u.CreatedAt,
	)
	return u, err
}

func (s *PostgresUsersStore) GetUser(ctx context.Context, tx pgx.Tx, email string) (User, error) {
	const q = `
		SELECT id::text, username, role, created_at, updated_at
		FROM users
		WHERE username = $1
	`
	var u User
	err := tx.QueryRow(ctx, q, email).Scan(
		&u.ID, &u.UserName, &u.Role, &u.CreatedAt, &u.UpdatedAt,
	)
	return u, err
}
