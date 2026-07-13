package auth

import "errors"

var (
	// ErrInvalidUsernameOrPassword indicates a login attempt failed because
	// the username does not exist or the password does not match.
	ErrInvalidUsernameOrPassword = errors.New("invalid username or password")
	// ErrInvalidSession indicates a session token is missing, expired, or
	// does not correspond to a known session.
	ErrInvalidSession = errors.New("invalid or expired session")
	// ErrPasswordTooShort indicates a supplied plaintext password is shorter
	// than the minimum allowed length.
	ErrPasswordTooShort = errors.New("password too short")
	// ErrUsernameInvalid indicates a supplied username fails length or
	// character-set validation.
	ErrUsernameInvalid = errors.New("username invalid")
	// ErrHashFormatInvalid indicates an encoded password hash does not match
	// the expected $argon2id$... format.
	ErrHashFormatInvalid = errors.New("password hash format invalid")
)
