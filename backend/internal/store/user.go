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

type UserAuth struct {
	User
	PasswordHash string
}

type NewUser struct {
	UserName     string
	PasswordHash string
}

type NewSession struct {
	UserID    string
	TokenHash []byte
	ExpiresAt time.Time
}

type Session struct {
	ID         string
	UserID     string
	TokenHash  []byte
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
}

type UsersStore interface {
	Create(ctx context.Context, tx pgx.Tx, in NewUser) (User, error)
	GetUser(ctx context.Context, tx pgx.Tx, userName string) (User, error)
	GetUserAuth(ctx context.Context, tx pgx.Tx, userName string) (UserAuth, error)
	CreateSession(ctx context.Context, tx pgx.Tx, in NewSession) (Session, error)
}

type PostgresUsersStore struct{}

func NewPostgresUsersStore() *PostgresUsersStore { return &PostgresUsersStore{} }

func (s *PostgresUsersStore) Create(ctx context.Context, tx pgx.Tx, in NewUser) (User, error) {
	const q = `
		INSERT INTO users (username, password_hash)
		VALUES ($1, $2)
		RETURNING id::text, username, role, created_at, updated_at
	`
	var u User
	err := tx.QueryRow(ctx, q, in.UserName, in.PasswordHash).Scan(
		&u.ID, &u.UserName, &u.Role, &u.CreatedAt, &u.UpdatedAt,
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

func (s *PostgresUsersStore) GetUserAuth(ctx context.Context, tx pgx.Tx, userName string) (UserAuth, error) {
	const q = `
		SELECT id::text, username, role, password_hash, created_at, updated_at
		FROM users
		WHERE username = $1
	`
	var u UserAuth
	err := tx.QueryRow(ctx, q, userName).Scan(
		&u.ID, &u.UserName, &u.Role, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt,
	)
	return u, err
}

func (s *PostgresUsersStore) CreateSession(ctx context.Context, tx pgx.Tx, in NewSession) (Session, error) {
	const q = `
		INSERT INTO sessions (user_id, token_hash, expires_at)
		VALUES ($1::uuid, $2, $3)
		RETURNING id::text, user_id::text, token_hash, created_at, last_seen_at, expires_at
	`
	var out Session
	err := tx.QueryRow(ctx, q, in.UserID, in.TokenHash, in.ExpiresAt).Scan(
		&out.ID, &out.UserID, &out.TokenHash, &out.CreatedAt, &out.LastSeenAt, &out.ExpiresAt,
	)
	return out, err
}
