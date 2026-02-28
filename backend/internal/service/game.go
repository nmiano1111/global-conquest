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
	WithTxQ(ctx context.Context, fn func(q db.Querier) error) error
}

type GamesService struct {
	db    gameDB
	games store.GamesStore
}

var (
	ErrGameNotFound        = errors.New("game not found")
	ErrInvalidGameInput    = errors.New("invalid game input")
	ErrUnknownPlayerIDs    = errors.New("one or more player_ids do not exist")
	ErrGameNotJoinable     = errors.New("game is not joinable")
	ErrGameAlreadyJoined   = errors.New("player already joined this game")
	ErrGamePlayerCountFull = errors.New("game is already full")
)

func NewGamesService(db gameDB, games store.GamesStore) *GamesService {
	return &GamesService{db: db, games: games}
}

type lobbyState struct {
	PlayerCount int      `json:"player_count"`
	PlayerIDs   []string `json:"player_ids"`
}

func (s *GamesService) CreateClassicGame(ctx context.Context, ownerUserID string, playerCount int) (store.Game, error) {
	if ownerUserID == "" {
		return store.Game{}, ErrInvalidGameInput
	}
	if playerCount < 3 || playerCount > 6 {
		return store.Game{}, ErrInvalidGameInput
	}

	var existingOwner int
	if err := s.db.Queryer().QueryRow(
		ctx,
		`SELECT count(*) FROM users WHERE id::text = $1`,
		ownerUserID,
	).Scan(&existingOwner); err != nil {
		return store.Game{}, err
	}
	if existingOwner != 1 {
		return store.Game{}, ErrUnknownPlayerIDs
	}

	state, err := json.Marshal(lobbyState{
		PlayerCount: playerCount,
		PlayerIDs:   []string{ownerUserID},
	})
	if err != nil {
		return store.Game{}, err
	}

	return s.games.Create(ctx, s.db.Queryer(), store.NewGame{
		OwnerUserID: ownerUserID,
		Status:      "lobby",
		State:       state,
	})
}

func (s *GamesService) JoinClassicGame(ctx context.Context, gameID, playerID string) (store.Game, error) {
	if gameID == "" || playerID == "" {
		return store.Game{}, ErrInvalidGameInput
	}

	var out store.Game
	err := s.db.WithTxQ(ctx, func(q db.Querier) error {
		g, err := s.games.GetByIDForUpdate(ctx, q, gameID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrGameNotFound
			}
			return err
		}
		if g.Status != "lobby" {
			return ErrGameNotJoinable
		}

		lobby, err := decodeLobbyState(g.State)
		if err != nil {
			return err
		}

		for _, id := range lobby.PlayerIDs {
			if id == playerID {
				out = g
				return nil
			}
		}

		if len(lobby.PlayerIDs) >= lobby.PlayerCount {
			return ErrGamePlayerCountFull
		}

		lobby.PlayerIDs = append(lobby.PlayerIDs, playerID)
		nextStatus := "lobby"
		var nextState []byte
		if len(lobby.PlayerIDs) == lobby.PlayerCount {
			engine, err := risk.NewClassicGame(lobby.PlayerIDs, nil)
			if err != nil {
				return err
			}
			nextStatus = "in_progress"
			nextState, err = json.Marshal(engine)
			if err != nil {
				return err
			}
		} else {
			nextState, err = json.Marshal(lobby)
			if err != nil {
				return err
			}
		}

		out, err = s.games.UpdateState(ctx, q, store.UpdateGameState{
			GameID: g.ID,
			Status: nextStatus,
			State:  nextState,
		})
		return err
	})
	return out, err
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

func decodeLobbyState(raw json.RawMessage) (lobbyState, error) {
	var lobby lobbyState
	if err := json.Unmarshal(raw, &lobby); err != nil {
		return lobbyState{}, ErrInvalidGameInput
	}
	if lobby.PlayerCount < 3 || lobby.PlayerCount > 6 || len(lobby.PlayerIDs) == 0 || len(lobby.PlayerIDs) > lobby.PlayerCount {
		return lobbyState{}, ErrInvalidGameInput
	}
	seen := make(map[string]struct{}, len(lobby.PlayerIDs))
	for _, pid := range lobby.PlayerIDs {
		if pid == "" {
			return lobbyState{}, ErrInvalidGameInput
		}
		if _, ok := seen[pid]; ok {
			return lobbyState{}, ErrInvalidGameInput
		}
		seen[pid] = struct{}{}
	}
	return lobby, nil
}
