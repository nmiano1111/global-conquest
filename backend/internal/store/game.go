package store

import (
	"backend/internal/db"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Game struct {
	ID          string          `json:"id"`
	OwnerUserID string          `json:"owner_user_id"`
	Status      string          `json:"status"`
	State       json.RawMessage `swaggertype:"object" json:"state,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type NewGame struct {
	OwnerUserID string
	Status      string
	State       json.RawMessage
}

type UpdateGameState struct {
	GameID string
	Status string
	State  json.RawMessage
}

type GameListFilter struct {
	OwnerUserID string
	Status      string
	Limit       int
	Offset      int
}

type GamesStore interface {
	Create(ctx context.Context, q db.Querier, in NewGame) (Game, error)
	GetByID(ctx context.Context, q db.Querier, gameID string) (Game, error)
	List(ctx context.Context, q db.Querier, filter GameListFilter) ([]Game, error)
	UpdateState(ctx context.Context, q db.Querier, in UpdateGameState) (Game, error)
}

type PostgresGamesStore struct{}

func NewPostgresGamesStore() *PostgresGamesStore { return &PostgresGamesStore{} }

func (s *PostgresGamesStore) Create(ctx context.Context, exec db.Querier, in NewGame) (Game, error) {
	const stmt = `
		INSERT INTO games (owner_user_id, status, state)
		VALUES ($1::uuid, $2, $3::jsonb)
		RETURNING id::text, owner_user_id::text, status, state, created_at, updated_at
	`
	var g Game
	err := exec.QueryRow(ctx, stmt, in.OwnerUserID, in.Status, in.State).Scan(
		&g.ID, &g.OwnerUserID, &g.Status, &g.State, &g.CreatedAt, &g.UpdatedAt,
	)
	return g, err
}

func (s *PostgresGamesStore) GetByID(ctx context.Context, exec db.Querier, gameID string) (Game, error) {
	const stmt = `
		SELECT id::text, owner_user_id::text, status, state, created_at, updated_at
		FROM games
		WHERE id = $1::uuid
	`
	var g Game
	err := exec.QueryRow(ctx, stmt, gameID).Scan(
		&g.ID, &g.OwnerUserID, &g.Status, &g.State, &g.CreatedAt, &g.UpdatedAt,
	)
	return g, err
}

func (s *PostgresGamesStore) List(ctx context.Context, exec db.Querier, filter GameListFilter) ([]Game, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	stmt := `
		SELECT id::text, owner_user_id::text, status, state, created_at, updated_at
		FROM games
	`
	conds := make([]string, 0, 2)
	args := make([]any, 0, 4)
	if filter.OwnerUserID != "" {
		conds = append(conds, fmt.Sprintf("owner_user_id = $%d::uuid", len(args)+1))
		args = append(args, filter.OwnerUserID)
	}
	if filter.Status != "" {
		conds = append(conds, fmt.Sprintf("status = $%d", len(args)+1))
		args = append(args, filter.Status)
	}
	if len(conds) > 0 {
		stmt += " WHERE " + strings.Join(conds, " AND ")
	}
	stmt += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	rows, err := exec.Query(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Game, 0, limit)
	for rows.Next() {
		var g Game
		if err := rows.Scan(&g.ID, &g.OwnerUserID, &g.Status, &g.State, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *PostgresGamesStore) UpdateState(ctx context.Context, exec db.Querier, in UpdateGameState) (Game, error) {
	const stmt = `
		UPDATE games
		SET status = $2,
		    state = $3::jsonb,
		    updated_at = now()
		WHERE id = $1::uuid
		RETURNING id::text, owner_user_id::text, status, state, created_at, updated_at
	`
	var g Game
	err := exec.QueryRow(ctx, stmt, in.GameID, in.Status, in.State).Scan(
		&g.ID, &g.OwnerUserID, &g.Status, &g.State, &g.CreatedAt, &g.UpdatedAt,
	)
	return g, err
}
