package auth

import "time"

type Role string

const (
	RoleAdmin  Role = "admin"
	RolePlayer Role = "player"
)

type User struct {
	ID           string // uuid string (store as uuid in DB; keep string in core)
	Username     string
	PasswordHash string
	Role         Role
	CreatedAt    time.Time
}

type Session struct {
	ID        string
	UserID    string
	TokenHash []byte // sha256(token)
	CreatedAt time.Time
	LastSeen  time.Time
	ExpiresAt time.Time
}
