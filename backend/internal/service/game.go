package service

import (
	"context"
	"encoding/json"
	"errors"

	"backend/internal/db"
	"backend/internal/risk"
	"backend/internal/store"
	"github.com/jackc/pgx/v5"
)

type gameDB interface {
	Queryer() db.Querier
}

type GamesService struct {
	db    gameDB
	games store.GamesStore
}

var (
	ErrGameNotFound      = errors.New("game not found")
	ErrInvalidGameInput  = errors.New("invalid game input")
	ErrOwnerNotInPlayers = errors.New("owner must be included in player_ids")
	ErrDuplicatePlayers  = errors.New("duplicate player ids are not allowed")
	ErrUnknownPlayerIDs  = errors.New("one or more player_ids do not exist")
)

func NewGamesService(db gameDB, games store.GamesStore) *GamesService {
	return &GamesService{db: db, games: games}
}

func (s *GamesService) CreateClassicGame(ctx context.Context, ownerUserID string, playerIDs []string) (store.Game, error) {
	if ownerUserID == "" {
		return store.Game{}, ErrInvalidGameInput
	}
	seen := map[string]struct{}{}
	ownerFound := false
	for _, p := range playerIDs {
		if p == "" {
			return store.Game{}, ErrInvalidGameInput
		}
		if _, ok := seen[p]; ok {
			return store.Game{}, ErrDuplicatePlayers
		}
		seen[p] = struct{}{}
		if p == ownerUserID {
			ownerFound = true
		}
	}
	if !ownerFound {
		return store.Game{}, ErrOwnerNotInPlayers
	}

	engine, err := risk.NewClassicGame(playerIDs, nil)
	if err != nil {
		return store.Game{}, err
	}

	var existing int
	if err := s.db.Queryer().QueryRow(
		ctx,
		`SELECT count(*) FROM users WHERE id::text = ANY($1::text[])`,
		playerIDs,
	).Scan(&existing); err != nil {
		return store.Game{}, err
	}
	if existing != len(playerIDs) {
		return store.Game{}, ErrUnknownPlayerIDs
	}

	state, err := json.Marshal(engine)
	if err != nil {
		return store.Game{}, err
	}

	return s.games.Create(ctx, s.db.Queryer(), store.NewGame{
		OwnerUserID: ownerUserID,
		Status:      "lobby",
		State:       state,
	})
}

func (s *GamesService) GetGame(ctx context.Context, gameID string) (store.Game, error) {
	g, err := s.games.GetByID(ctx, s.db.Queryer(), gameID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return store.Game{}, ErrGameNotFound
		}
		return store.Game{}, err
	}
	return g, nil
}

func (s *GamesService) ListGames(ctx context.Context, ownerUserID, status string, limit, offset int) ([]store.Game, error) {
	if limit < 0 || offset < 0 {
		return nil, ErrInvalidGameInput
	}
	return s.games.List(ctx, s.db.Queryer(), store.GameListFilter{
		OwnerUserID: ownerUserID,
		Status:      status,
		Limit:       limit,
		Offset:      offset,
	})
}

func (s *GamesService) UpdateGameState(ctx context.Context, gameID, status string, state json.RawMessage) (store.Game, error) {
	if gameID == "" || status == "" || len(state) == 0 {
		return store.Game{}, ErrInvalidGameInput
	}
	g, err := s.games.UpdateState(ctx, s.db.Queryer(), store.UpdateGameState{
		GameID: gameID,
		Status: status,
		State:  state,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return store.Game{}, ErrGameNotFound
		}
		return store.Game{}, err
	}
	return g, nil
}
