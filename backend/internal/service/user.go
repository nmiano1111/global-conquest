package service

import (
	"context"

	"github.com/jackc/pgx/v5"

	"backend/internal/db"
	"backend/internal/store"
)

type UsersService struct {
	db    *db.DB
	users store.UsersStore
}

func NewUsersService(db *db.DB, users store.UsersStore) *UsersService {
	return &UsersService{db: db, users: users}
}

func (s *UsersService) CreateUser(ctx context.Context, in store.NewUser) (store.User, error) {
	var out store.User
	err := s.db.WithTx(ctx, func(tx pgx.Tx) error {
		u, err := s.users.Create(ctx, tx, in)
		if err != nil {
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
