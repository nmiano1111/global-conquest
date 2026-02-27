package auth

import "errors"

var (
	ErrInvalidUsernameOrPassword = errors.New("invalid username or password")
	ErrPasswordTooShort          = errors.New("password too short")
	ErrUsernameInvalid           = errors.New("username invalid")
	ErrHashFormatInvalid         = errors.New("password hash format invalid")
)
