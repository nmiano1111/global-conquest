package store

import (
	"backend/internal/db"
	"context"
	"time"
)

type User struct {
	ID           string    `json:"id"`
	UserName     string    `json:"username"`
	Role         string    `json:"role"`
	AccessStatus string    `json:"access_status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type AdminUser struct {
	User
	ActiveSessions int `json:"active_sessions"`
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
	Create(ctx context.Context, q db.Querier, in NewUser) (User, error)
	ListUsers(ctx context.Context, q db.Querier) ([]User, error)
	ListAdminUsers(ctx context.Context, q db.Querier) ([]AdminUser, error)
	GetUser(ctx context.Context, q db.Querier, userName string) (User, error)
	GetUserBySessionToken(ctx context.Context, q db.Querier, tokenHash []byte) (User, error)
	GetUserAuth(ctx context.Context, q db.Querier, userName string) (UserAuth, error)
	CreateSession(ctx context.Context, q db.Querier, in NewSession) (Session, error)
	UpdateUserAccess(ctx context.Context, q db.Querier, userID, accessStatus string) (User, error)
	RevokeSessions(ctx context.Context, q db.Querier, userID string) (int64, error)
}

type PostgresUsersStore struct{}

func NewPostgresUsersStore() *PostgresUsersStore { return &PostgresUsersStore{} }

func (s *PostgresUsersStore) Create(ctx context.Context, exec db.Querier, in NewUser) (User, error) {
	const stmt = `
		INSERT INTO users (username, password_hash)
		VALUES ($1, $2)
		RETURNING id::text, username, role, access_status, created_at, updated_at
	`
	var u User
	err := exec.QueryRow(ctx, stmt, in.UserName, in.PasswordHash).Scan(
		&u.ID, &u.UserName, &u.Role, &u.AccessStatus, &u.CreatedAt, &u.UpdatedAt,
	)
	return u, err
}

func (s *PostgresUsersStore) ListUsers(ctx context.Context, exec db.Querier) ([]User, error) {
	const stmt = `
		SELECT id::text, username, role, access_status, created_at, updated_at
		FROM users
		ORDER BY created_at DESC
	`
	rows, err := exec.Query(ctx, stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.UserName, &u.Role, &u.AccessStatus, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

func (s *PostgresUsersStore) ListAdminUsers(ctx context.Context, exec db.Querier) ([]AdminUser, error) {
	const stmt = `
		SELECT
			u.id::text,
			u.username,
			u.role,
			u.access_status,
			u.created_at,
			u.updated_at,
			COALESCE(COUNT(s.id), 0)::int AS active_sessions
		FROM users u
		LEFT JOIN sessions s
			ON s.user_id = u.id
			AND s.expires_at > now()
		GROUP BY u.id, u.username, u.role, u.access_status, u.created_at, u.updated_at
		ORDER BY u.created_at DESC
	`
	rows, err := exec.Query(ctx, stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []AdminUser
	for rows.Next() {
		var u AdminUser
		if err := rows.Scan(
			&u.ID,
			&u.UserName,
			&u.Role,
			&u.AccessStatus,
			&u.CreatedAt,
			&u.UpdatedAt,
			&u.ActiveSessions,
		); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

func (s *PostgresUsersStore) GetUser(ctx context.Context, exec db.Querier, email string) (User, error) {
	const stmt = `
		SELECT id::text, username, role, access_status, created_at, updated_at
		FROM users
		WHERE username = $1
	`
	var u User
	err := exec.QueryRow(ctx, stmt, email).Scan(
		&u.ID, &u.UserName, &u.Role, &u.AccessStatus, &u.CreatedAt, &u.UpdatedAt,
	)
	return u, err
}

func (s *PostgresUsersStore) GetUserBySessionToken(ctx context.Context, exec db.Querier, tokenHash []byte) (User, error) {
	const stmt = `
		SELECT u.id::text, u.username, u.role, u.access_status, u.created_at, u.updated_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1
		  AND s.expires_at > now()
	`
	var u User
	err := exec.QueryRow(ctx, stmt, tokenHash).Scan(
		&u.ID, &u.UserName, &u.Role, &u.AccessStatus, &u.CreatedAt, &u.UpdatedAt,
	)
	return u, err
}

func (s *PostgresUsersStore) GetUserAuth(ctx context.Context, exec db.Querier, userName string) (UserAuth, error) {
	const stmt = `
		SELECT id::text, username, role, access_status, password_hash, created_at, updated_at
		FROM users
		WHERE username = $1
	`
	var u UserAuth
	err := exec.QueryRow(ctx, stmt, userName).Scan(
		&u.ID, &u.UserName, &u.Role, &u.AccessStatus, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt,
	)
	return u, err
}

func (s *PostgresUsersStore) CreateSession(ctx context.Context, exec db.Querier, in NewSession) (Session, error) {
	const stmt = `
		INSERT INTO sessions (user_id, token_hash, expires_at)
		VALUES ($1::uuid, $2, $3)
		RETURNING id::text, user_id::text, token_hash, created_at, last_seen_at, expires_at
	`
	var out Session
	err := exec.QueryRow(ctx, stmt, in.UserID, in.TokenHash, in.ExpiresAt).Scan(
		&out.ID, &out.UserID, &out.TokenHash, &out.CreatedAt, &out.LastSeenAt, &out.ExpiresAt,
	)
	return out, err
}

func (s *PostgresUsersStore) UpdateUserAccess(ctx context.Context, exec db.Querier, userID, accessStatus string) (User, error) {
	const stmt = `
		UPDATE users
		SET
			access_status = $2,
			updated_at = now()
		WHERE id = $1::uuid
		RETURNING id::text, username, role, access_status, created_at, updated_at
	`
	var u User
	err := exec.QueryRow(ctx, stmt, userID, accessStatus).Scan(
		&u.ID, &u.UserName, &u.Role, &u.AccessStatus, &u.CreatedAt, &u.UpdatedAt,
	)
	return u, err
}

func (s *PostgresUsersStore) RevokeSessions(ctx context.Context, exec db.Querier, userID string) (int64, error) {
	const stmt = `
		WITH deleted AS (
			DELETE FROM sessions
			WHERE user_id = $1::uuid
			RETURNING 1
		)
		SELECT COUNT(*)::bigint FROM deleted
	`
	var revoked int64
	if err := exec.QueryRow(ctx, stmt, userID).Scan(&revoked); err != nil {
		return 0, err
	}
	return revoked, nil
}
