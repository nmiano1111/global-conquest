// Package store is the raw data-access layer for Postgres-backed game, user, and chat state.
// It wraps the generated db.Querier with hand-written SQL for reads and writes, translating
// between database rows and the plain Go structs used by internal/service and the rest of the
// backend. It performs no transactional orchestration or business-rule enforcement of its own —
// callers (typically internal/service) are responsible for locking and transaction boundaries.
package store

import (
	"context"
	"github.com/nmiano1111/global-conquest/backend/internal/db"
	"time"
)

// User is a row from the users table, excluding sensitive auth data such as the password hash.
type User struct {
	// ID is the user's unique identifier.
	ID string `json:"id"`
	// UserName is the user's unique login/display name.
	UserName string `json:"username"`
	// Role is the user's authorization role (e.g. admin or regular user).
	Role string `json:"role"`
	// AccessStatus indicates whether the user is allowed to log in (e.g. active vs. revoked).
	AccessStatus string `json:"access_status"`
	// IsSandboxed marks the user as fully isolated from other players: their
	// games are invisible to and unjoinable by everyone but admins, they
	// cannot see or join anyone else's games, and no Discord notification is
	// ever enqueued for a game they created. Set by an admin.
	IsSandboxed bool `json:"is_sandboxed"`
	// CreatedAt is when the user account was created.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is when the user row was last modified.
	UpdatedAt time.Time `json:"updated_at"`
}

// AdminUser extends User with the count of currently active (unexpired) sessions, for admin listing views.
type AdminUser struct {
	User
	// ActiveSessions is the number of unexpired sessions currently held by the user.
	ActiveSessions int `json:"active_sessions"`
}

// UserAuth extends User with the password hash needed to verify login credentials.
type UserAuth struct {
	User
	// PasswordHash is the user's hashed password, used to verify login attempts.
	PasswordHash string
}

// NewUser is the input for creating a new user account.
type NewUser struct {
	// UserName is the desired login/display name for the new user.
	UserName string
	// PasswordHash is the pre-hashed password to store for the new user.
	PasswordHash string
}

// NewSession is the input for creating a new authenticated session.
type NewSession struct {
	// UserID is the identifier of the user the session belongs to.
	UserID string
	// TokenHash is the SHA-256 hash of the session token; the raw token is never stored.
	TokenHash []byte
	// ExpiresAt is when the session becomes invalid.
	ExpiresAt time.Time
}

// Session is a row from the sessions table representing one authenticated login.
type Session struct {
	// ID is the session's unique identifier.
	ID string
	// UserID is the identifier of the user the session belongs to.
	UserID string
	// TokenHash is the SHA-256 hash of the session token.
	TokenHash []byte
	// CreatedAt is when the session was created.
	CreatedAt time.Time
	// LastSeenAt is when the session was last used.
	LastSeenAt time.Time
	// ExpiresAt is when the session becomes invalid.
	ExpiresAt time.Time
}

// UsersStore defines persistence operations for users, their auth credentials, and their sessions.
type UsersStore interface {
	Create(ctx context.Context, q db.Querier, in NewUser) (User, error)
	ListUsers(ctx context.Context, q db.Querier) ([]User, error)
	ListAdminUsers(ctx context.Context, q db.Querier) ([]AdminUser, error)
	GetUser(ctx context.Context, q db.Querier, userName string) (User, error)
	GetUserBySessionToken(ctx context.Context, q db.Querier, tokenHash []byte) (User, error)
	GetUserAuth(ctx context.Context, q db.Querier, userName string) (UserAuth, error)
	CreateSession(ctx context.Context, q db.Querier, in NewSession) (Session, error)
	UpdateUserAccess(ctx context.Context, q db.Querier, userID, accessStatus string) (User, error)
	SetSandboxed(ctx context.Context, q db.Querier, userID string, sandboxed bool) (User, error)
	RevokeSessions(ctx context.Context, q db.Querier, userID string) (int64, error)
}

// PostgresUsersStore is a Postgres-backed implementation of UsersStore.
type PostgresUsersStore struct{}

// NewPostgresUsersStore constructs a PostgresUsersStore.
func NewPostgresUsersStore() *PostgresUsersStore { return &PostgresUsersStore{} }

// Create inserts a new user with the given username and password hash and returns the persisted row. It returns an error if the username is already taken (unique constraint violation) or on any other insert failure.
func (s *PostgresUsersStore) Create(ctx context.Context, exec db.Querier, in NewUser) (User, error) {
	const stmt = `
		INSERT INTO users (username, password_hash)
		VALUES ($1, $2)
		RETURNING id::text, username, role, access_status, is_sandboxed, created_at, updated_at
	`
	var u User
	err := exec.QueryRow(ctx, stmt, in.UserName, in.PasswordHash).Scan(
		&u.ID, &u.UserName, &u.Role, &u.AccessStatus, &u.IsSandboxed, &u.CreatedAt, &u.UpdatedAt,
	)
	return u, err
}

// ListUsers returns all users, ordered by creation time descending (newest first).
func (s *PostgresUsersStore) ListUsers(ctx context.Context, exec db.Querier) ([]User, error) {
	const stmt = `
		SELECT id::text, username, role, access_status, is_sandboxed, created_at, updated_at
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
		if err := rows.Scan(&u.ID, &u.UserName, &u.Role, &u.AccessStatus, &u.IsSandboxed, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

// ListAdminUsers returns all users along with each user's count of currently active (unexpired) sessions, ordered by creation time descending (newest first). Intended for admin-facing user management views.
func (s *PostgresUsersStore) ListAdminUsers(ctx context.Context, exec db.Querier) ([]AdminUser, error) {
	const stmt = `
		SELECT
			u.id::text,
			u.username,
			u.role,
			u.access_status,
			u.is_sandboxed,
			u.created_at,
			u.updated_at,
			COALESCE(COUNT(s.id), 0)::int AS active_sessions
		FROM users u
		LEFT JOIN sessions s
			ON s.user_id = u.id
			AND s.expires_at > now()
		GROUP BY u.id, u.username, u.role, u.access_status, u.is_sandboxed, u.created_at, u.updated_at
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
			&u.IsSandboxed,
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

// GetUser fetches a single user by username. It returns sql.ErrNoRows (via the underlying QueryRow.Scan) if no user with that username exists.
func (s *PostgresUsersStore) GetUser(ctx context.Context, exec db.Querier, email string) (User, error) {
	const stmt = `
		SELECT id::text, username, role, access_status, is_sandboxed, created_at, updated_at
		FROM users
		WHERE username = $1
	`
	var u User
	err := exec.QueryRow(ctx, stmt, email).Scan(
		&u.ID, &u.UserName, &u.Role, &u.AccessStatus, &u.IsSandboxed, &u.CreatedAt, &u.UpdatedAt,
	)
	return u, err
}

// GetUserBySessionToken fetches the user associated with an unexpired session whose token hash matches tokenHash. It returns sql.ErrNoRows if no matching, unexpired session exists.
func (s *PostgresUsersStore) GetUserBySessionToken(ctx context.Context, exec db.Querier, tokenHash []byte) (User, error) {
	const stmt = `
		SELECT u.id::text, u.username, u.role, u.access_status, u.is_sandboxed, u.created_at, u.updated_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1
		  AND s.expires_at > now()
	`
	var u User
	err := exec.QueryRow(ctx, stmt, tokenHash).Scan(
		&u.ID, &u.UserName, &u.Role, &u.AccessStatus, &u.IsSandboxed, &u.CreatedAt, &u.UpdatedAt,
	)
	return u, err
}

// GetUserAuth fetches a user by username along with their password hash, for verifying login credentials. It returns sql.ErrNoRows if no user with that username exists.
func (s *PostgresUsersStore) GetUserAuth(ctx context.Context, exec db.Querier, userName string) (UserAuth, error) {
	const stmt = `
		SELECT id::text, username, role, access_status, is_sandboxed, password_hash, created_at, updated_at
		FROM users
		WHERE username = $1
	`
	var u UserAuth
	err := exec.QueryRow(ctx, stmt, userName).Scan(
		&u.ID, &u.UserName, &u.Role, &u.AccessStatus, &u.IsSandboxed, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt,
	)
	return u, err
}

// CreateSession inserts a new session row for the given user, token hash, and expiry, and returns the persisted row including its generated ID, creation time, and initial last-seen time.
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

// UpdateUserAccess sets the given user's access_status and updates their updated_at timestamp, returning the updated row. It returns sql.ErrNoRows if no user with that ID exists.
func (s *PostgresUsersStore) UpdateUserAccess(ctx context.Context, exec db.Querier, userID, accessStatus string) (User, error) {
	const stmt = `
		UPDATE users
		SET
			access_status = $2,
			updated_at = now()
		WHERE id = $1::uuid
		RETURNING id::text, username, role, access_status, is_sandboxed, created_at, updated_at
	`
	var u User
	err := exec.QueryRow(ctx, stmt, userID, accessStatus).Scan(
		&u.ID, &u.UserName, &u.Role, &u.AccessStatus, &u.IsSandboxed, &u.CreatedAt, &u.UpdatedAt,
	)
	return u, err
}

// SetSandboxed sets the given user's is_sandboxed flag and updates their
// updated_at timestamp, returning the updated row. It returns sql.ErrNoRows
// if no user with that ID exists.
func (s *PostgresUsersStore) SetSandboxed(ctx context.Context, exec db.Querier, userID string, sandboxed bool) (User, error) {
	const stmt = `
		UPDATE users
		SET
			is_sandboxed = $2,
			updated_at = now()
		WHERE id = $1::uuid
		RETURNING id::text, username, role, access_status, is_sandboxed, created_at, updated_at
	`
	var u User
	err := exec.QueryRow(ctx, stmt, userID, sandboxed).Scan(
		&u.ID, &u.UserName, &u.Role, &u.AccessStatus, &u.IsSandboxed, &u.CreatedAt, &u.UpdatedAt,
	)
	return u, err
}

// RevokeSessions deletes all sessions belonging to userID and returns the number of sessions deleted, effectively logging the user out of every active session.
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
