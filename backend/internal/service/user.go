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

type UsersService struct {
	db    userDB
	users store.UsersStore
}

type LoginResult struct {
	User      store.User
	Token     string
	ExpiresAt time.Time
}

var ErrUsernameTaken = errors.New("username already taken")

func NewUsersService(db userDB, users store.UsersStore) *UsersService {
	return &UsersService{db: db, users: users}
}

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

func (s *UsersService) GetUser(ctx context.Context, userName string) (store.User, error) {
	return s.users.GetUser(ctx, s.db.Queryer(), userName)
}

func (s *UsersService) ListUsers(ctx context.Context) ([]store.User, error) {
	return s.users.ListUsers(ctx, s.db.Queryer())
}

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
				ID:        u.ID,
				UserName:  u.UserName,
				Role:      u.Role,
				CreatedAt: u.CreatedAt,
				UpdatedAt: u.UpdatedAt,
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
