// Package auth provides password hashing/verification (Argon2id), session
// token generation and hashing, and the core User/Session/Role types shared
// by the store and service layers. Sessions are opaque 32-byte CSPRNG
// tokens; only their SHA-256 hash is ever persisted.
package auth

import "time"

// Role identifies a user's authorization level.
type Role string

const (
	// RoleAdmin grants administrative privileges (user management, etc.).
	RoleAdmin Role = "admin"
	// RolePlayer is the default role for a regular player account.
	RolePlayer Role = "player"
)

// User is a registered account.
type User struct {
	ID string // uuid string (store as uuid in DB; keep string in core)
	// Username is the account's unique, case-sensitive login name.
	Username string
	// PasswordHash is the encoded Argon2id hash produced by HashPassword.
	PasswordHash string
	// Role is the user's authorization level.
	Role Role
	// CreatedAt is when the account was created.
	CreatedAt time.Time
}

// Session represents an authenticated login session belonging to a User.
type Session struct {
	// ID is the session's unique identifier.
	ID string
	// UserID is the ID of the User this session belongs to.
	UserID    string
	TokenHash []byte // sha256(token)
	// CreatedAt is when the session was created.
	CreatedAt time.Time
	// LastSeen is when the session was last used to authenticate a request.
	LastSeen time.Time
	// ExpiresAt is when the session becomes invalid.
	ExpiresAt time.Time
}
