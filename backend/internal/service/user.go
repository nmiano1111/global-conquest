package service

import (
	"context"
	"errors"
	"time"

	"backend/internal/auth"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"backend/internal/db"
	"backend/internal/store"
)

type userDB interface {
	Queryer() db.Querier
	WithTxQ(ctx context.Context, fn func(q db.Querier) error) error
}

// UsersService is the transactional business-logic layer for user
// accounts and authentication — registration, profile listing, admin
// access control, and session-backed login — sitting between the httpapi
// handlers and the users store.
type UsersService struct {
	db    userDB
	users store.UsersStore
}

// LoginResult is the outcome of a successful UsersService.Login call: the
// authenticated user, the plaintext session token to hand back to the
// client, and the token's expiry time.
type LoginResult struct {
	// User is the authenticated user's public profile.
	User store.User
	// Token is the plaintext session token; only its SHA-256 hash is
	// persisted server-side, so this value is only ever available here at
	// login time.
	Token string
	// ExpiresAt is when the session token expires.
	ExpiresAt time.Time
}

// ErrUsernameTaken is returned by CreateUser when the requested username is
// already in use.
var ErrUsernameTaken = errors.New("username already taken")

// ErrUserAccessDenied is returned by Login when the authenticating user's
// access status is AccessStatusBlocked.
var ErrUserAccessDenied = errors.New("user is not allowed to access the system")

// ErrInvalidAccessState is returned by UpdateUserAccess when the requested
// access status is neither AccessStatusActive nor AccessStatusBlocked.
var ErrInvalidAccessState = errors.New("invalid access state")

const (
	// AccessStatusActive marks a user as allowed to log in and act
	// normally.
	AccessStatusActive = "active"
	// AccessStatusBlocked marks a user as denied login and session
	// authentication.
	AccessStatusBlocked = "blocked"
)

// NewUsersService constructs a UsersService backed by the given database
// and users store.
func NewUsersService(db userDB, users store.UsersStore) *UsersService {
	return &UsersService{db: db, users: users}
}

// CreateUser registers a new user with the given userName and password,
// validating the username and hashing the password with the default
// password hashing parameters before persisting it. It returns
// ErrUsernameTaken if the username is already in use, or an error from
// auth.ValidateUsername/auth.HashPassword if validation or hashing fails.
func (s *UsersService) CreateUser(ctx context.Context, userName, password string) (store.User, error) {
	validName, err := auth.ValidateUsername(userName)
	if err != nil {
		return store.User{}, err
	}
	hash, err := auth.HashPassword(password, auth.DefaultPasswordParams())
	if err != nil {
		return store.User{}, err
	}
	u, err := s.users.Create(ctx, s.db.Queryer(), store.NewUser{
		UserName:     validName,
		PasswordHash: hash,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return store.User{}, ErrUsernameTaken
		}
		return store.User{}, err
	}
	return u, nil
}

// GetUser fetches a user's public profile by username.
func (s *UsersService) GetUser(ctx context.Context, userName string) (store.User, error) {
	return s.users.GetUser(ctx, s.db.Queryer(), userName)
}

// ListUsers returns every user's public profile.
func (s *UsersService) ListUsers(ctx context.Context) ([]store.User, error) {
	return s.users.ListUsers(ctx, s.db.Queryer())
}

// ListAdminUsers returns every user's admin-facing profile, which includes
// fields (such as access status) not exposed by the public User type.
func (s *UsersService) ListAdminUsers(ctx context.Context) ([]store.AdminUser, error) {
	return s.users.ListAdminUsers(ctx, s.db.Queryer())
}

// UpdateUserAccess sets the given user's access status to accessStatus. It
// returns ErrInvalidAccessState if accessStatus is neither
// AccessStatusActive nor AccessStatusBlocked.
func (s *UsersService) UpdateUserAccess(ctx context.Context, userID, accessStatus string) (store.User, error) {
	if accessStatus != AccessStatusActive && accessStatus != AccessStatusBlocked {
		return store.User{}, ErrInvalidAccessState
	}
	return s.users.UpdateUserAccess(ctx, s.db.Queryer(), userID, accessStatus)
}

// RevokeUserSessions deletes all active sessions for the given user (e.g.
// when an admin blocks them), returning the number of sessions revoked.
func (s *UsersService) RevokeUserSessions(ctx context.Context, userID string) (int64, error) {
	return s.users.RevokeSessions(ctx, s.db.Queryer(), userID)
}

// AuthenticateSession resolves a plaintext session token to its owning
// user, hashing the token and looking it up by hash. It returns
// auth.ErrInvalidSession if the token is malformed, does not match any
// stored session, or belongs to a user whose access status is
// AccessStatusBlocked.
func (s *UsersService) AuthenticateSession(ctx context.Context, token string) (store.User, error) {
	tokenHash, err := auth.HashSessionToken(token)
	if err != nil {
		return store.User{}, auth.ErrInvalidSession
	}

	u, err := s.users.GetUserBySessionToken(ctx, s.db.Queryer(), tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return store.User{}, auth.ErrInvalidSession
		}
		return store.User{}, err
	}
	if u.AccessStatus == AccessStatusBlocked {
		return store.User{}, auth.ErrInvalidSession
	}
	return u, nil
}

// Login verifies userName and password against the stored credentials and,
// on success, creates a new 30-day session and returns it as a
// LoginResult. It returns auth.ErrInvalidUsernameOrPassword if the
// username does not exist or the password does not match, and
// ErrUserAccessDenied if the user's access status is AccessStatusBlocked.
// The credential check, access-status check, and session creation all run
// inside a single transaction.
func (s *UsersService) Login(ctx context.Context, userName, password string) (LoginResult, error) {
	var out LoginResult
	err := s.db.WithTxQ(ctx, func(q db.Querier) error {
		u, err := s.users.GetUserAuth(ctx, q, userName)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return auth.ErrInvalidUsernameOrPassword
			}
			return err
		}

		ok, err := auth.VerifyPassword(password, u.PasswordHash)
		if err != nil {
			return err
		}
		if !ok {
			return auth.ErrInvalidUsernameOrPassword
		}
		if u.AccessStatus == AccessStatusBlocked {
			return ErrUserAccessDenied
		}

		token, err := auth.NewSessionToken()
		if err != nil {
			return err
		}
		tokenHash, err := auth.HashSessionToken(token)
		if err != nil {
			return err
		}

		expiresAt := time.Now().UTC().Add(24 * time.Hour * 30)
		_, err = s.users.CreateSession(ctx, q, store.NewSession{
			UserID:    u.ID,
			TokenHash: tokenHash,
			ExpiresAt: expiresAt,
		})
		if err != nil {
			return err
		}

		out = LoginResult{
			User: store.User{
				ID:           u.ID,
				UserName:     u.UserName,
				Role:         u.Role,
				AccessStatus: u.AccessStatus,
				CreatedAt:    u.CreatedAt,
				UpdatedAt:    u.UpdatedAt,
			},
			Token:     token,
			ExpiresAt: expiresAt,
		}
		return nil
	})
	return out, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
