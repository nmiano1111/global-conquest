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

type UsersService struct {
	db    *db.DB
	users store.UsersStore
}

type LoginResult struct {
	User      store.User
	Token     string
	ExpiresAt time.Time
}

var ErrUsernameTaken = errors.New("username already taken")

func NewUsersService(db *db.DB, users store.UsersStore) *UsersService {
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

	var out store.User
	err = s.db.WithTx(ctx, func(tx pgx.Tx) error {
		u, err := s.users.Create(ctx, tx, store.NewUser{
			UserName:     validName,
			PasswordHash: hash,
		})
		if err != nil {
			if isUniqueViolation(err) {
				return ErrUsernameTaken
			}
			return err
		}
		out = u
		return nil
	})
	return out, err
}

func (s *UsersService) GetUser(ctx context.Context, userName string) (store.User, error) {
	var out store.User
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		u, err := s.users.GetUser(ctx, tx, userName)
		if err != nil {
			return err
		}
		out = u
		return nil
	})
	return out, err
}

func (s *UsersService) Login(ctx context.Context, userName, password string) (LoginResult, error) {
	var out LoginResult
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		u, err := s.users.GetUserAuth(ctx, tx, userName)
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
		_, err = s.users.CreateSession(ctx, tx, store.NewSession{
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
