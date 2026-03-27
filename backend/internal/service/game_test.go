package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"backend/internal/db"
	"backend/internal/risk"
	"backend/internal/store"
	"github.com/jackc/pgx/v5"
)

type scalarRow struct {
	val any
	err error
}

func (r scalarRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != 1 {
		return errors.New("expected one destination")
	}
	dv := reflect.ValueOf(dest[0])
	if dv.Kind() != reflect.Ptr || dv.IsNil() {
		return errors.New("destination must be a pointer")
	}
	dv.Elem().Set(reflect.ValueOf(r.val))
	return nil
}

type countQuerier struct {
	count int
	err   error
}

func (q countQuerier) QueryRow(context.Context, string, ...any) pgx.Row {
	return scalarRow{val: q.count, err: q.err}
}

func (q countQuerier) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, q.err
}

type fakeGamesStore struct {
	createFn         func(context.Context, db.Querier, store.NewGame) (store.Game, error)
	getByIDFn        func(context.Context, db.Querier, string) (store.Game, error)
	getByIDForUpdate func(context.Context, db.Querier, string) (store.Game, error)
	listFn           func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error)
	updateStateFn    func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error)
}

func (f *fakeGamesStore) Create(ctx context.Context, q db.Querier, in store.NewGame) (store.Game, error) {
	return f.createFn(ctx, q, in)
}

func (f *fakeGamesStore) GetByID(ctx context.Context, q db.Querier, gameID string) (store.Game, error) {
	return f.getByIDFn(ctx, q, gameID)
}

func (f *fakeGamesStore) GetByIDForUpdate(ctx context.Context, q db.Querier, gameID string) (store.Game, error) {
	return f.getByIDForUpdate(ctx, q, gameID)
}

func (f *fakeGamesStore) List(ctx context.Context, q db.Querier, filter store.GameListFilter) ([]store.Game, error) {
	return f.listFn(ctx, q, filter)
}

func (f *fakeGamesStore) UpdateState(ctx context.Context, q db.Querier, in store.UpdateGameState) (store.Game, error) {
	return f.updateStateFn(ctx, q, in)
}

func TestCreateClassicGameValidation(t *testing.T) {
	svc := NewGamesService(&fakeDB{q: countQuerier{count: 1}}, &fakeGamesStore{
		createFn: func(context.Context, db.Querier, store.NewGame) (store.Game, error) {
			t.Fatalf("create should not be called")
			return store.Game{}, nil
		},
		getByIDFn:        func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		listFn:           func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn:    func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error) { return store.Game{}, nil },
	})

	_, err := svc.CreateClassicGame(context.Background(), "", 3, "")
	if !errors.Is(err, ErrInvalidGameInput) {
		t.Fatalf("expected ErrInvalidGameInput, got %v", err)
	}

	_, err = svc.CreateClassicGame(context.Background(), "u1", 2, "")
	if !errors.Is(err, ErrInvalidGameInput) {
		t.Fatalf("expected ErrInvalidGameInput for player count, got %v", err)
	}
}

func TestCreateClassicGamePersistsLobbyState(t *testing.T) {
	called := false
	svc := NewGamesService(&fakeDB{q: countQuerier{count: 1}}, &fakeGamesStore{
		createFn: func(_ context.Context, _ db.Querier, in store.NewGame) (store.Game, error) {
			called = true
			if in.OwnerUserID != "u1" || in.Status != "lobby" {
				t.Fatalf("unexpected create input: %#v", in)
			}
			var state map[string]any
			if err := json.Unmarshal(in.State, &state); err != nil {
				t.Fatalf("unmarshal state: %v", err)
			}
			if state["player_count"].(float64) != 4 {
				t.Fatalf("expected player_count=4")
			}
			ids := state["player_ids"].([]any)
			if len(ids) != 1 || ids[0].(string) != "u1" {
				t.Fatalf("expected owner seeded in player_ids")
			}
			return store.Game{ID: "g1", OwnerUserID: in.OwnerUserID, Status: in.Status, State: in.State}, nil
		},
		getByIDFn:        func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		listFn:           func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn:    func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error) { return store.Game{}, nil },
	})

	out, err := svc.CreateClassicGame(context.Background(), "u1", 4, "")
	if err != nil {
		t.Fatalf("create classic game: %v", err)
	}
	if !called {
		t.Fatalf("expected store create call")
	}
	if out.ID != "g1" {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestCreateClassicGameRejectsUnknownOwner(t *testing.T) {
	svc := NewGamesService(&fakeDB{q: countQuerier{count: 0}}, &fakeGamesStore{
		createFn: func(context.Context, db.Querier, store.NewGame) (store.Game, error) {
			t.Fatalf("create should not be called when owner is missing")
			return store.Game{}, nil
		},
		getByIDFn:        func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		listFn:           func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn:    func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error) { return store.Game{}, nil },
	})

	_, err := svc.CreateClassicGame(context.Background(), "u1", 3, "")
	if !errors.Is(err, ErrUnknownPlayerIDs) {
		t.Fatalf("expected ErrUnknownPlayerIDs, got %v", err)
	}
}

func TestJoinClassicGameTransitionsWhenFull(t *testing.T) {
	lobby := json.RawMessage(`{"player_count":3,"player_ids":["u1","u2"]}`)
	svc := NewGamesService(&fakeDB{q: noopQuerier{}, txQ: noopQuerier{}}, &fakeGamesStore{
		createFn:  func(context.Context, db.Querier, store.NewGame) (store.Game, error) { return store.Game{}, nil },
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "lobby", State: lobby}, nil
		},
		listFn: func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(_ context.Context, _ db.Querier, in store.UpdateGameState) (store.Game, error) {
			if in.Status != "in_progress" {
				t.Fatalf("expected in_progress status")
			}
			var g risk.Game
			if err := json.Unmarshal(in.State, &g); err != nil {
				t.Fatalf("expected risk game state: %v", err)
			}
			if len(g.Players) != 3 {
				t.Fatalf("expected 3 players in risk state")
			}
			if g.Phase != risk.PhaseReinforce {
				t.Fatalf("expected game to begin at reinforce phase (random default), got %s", g.Phase)
			}
			return store.Game{ID: "g1", Status: in.Status, State: in.State}, nil
		},
	})

	out, err := svc.JoinClassicGame(context.Background(), "g1", "u3")
	if err != nil {
		t.Fatalf("join game: %v", err)
	}
	if out.Status != "in_progress" {
		t.Fatalf("unexpected status: %s", out.Status)
	}
}

func TestJoinClassicGameManualSetupMode(t *testing.T) {
	lobby := json.RawMessage(`{"player_count":3,"player_ids":["u1","u2"],"setup_mode":"manual"}`)
	svc := NewGamesService(&fakeDB{q: noopQuerier{}, txQ: noopQuerier{}}, &fakeGamesStore{
		createFn:  func(context.Context, db.Querier, store.NewGame) (store.Game, error) { return store.Game{}, nil },
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "lobby", State: lobby}, nil
		},
		listFn: func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(_ context.Context, _ db.Querier, in store.UpdateGameState) (store.Game, error) {
			var g risk.Game
			if err := json.Unmarshal(in.State, &g); err != nil {
				t.Fatalf("expected risk game state: %v", err)
			}
			if g.Phase != risk.PhaseSetupReinforce {
				t.Fatalf("expected game to begin at setup_reinforce phase for manual mode, got %s", g.Phase)
			}
			return store.Game{ID: "g1", Status: in.Status, State: in.State}, nil
		},
	})

	_, err := svc.JoinClassicGame(context.Background(), "g1", "u3")
	if err != nil {
		t.Fatalf("join game: %v", err)
	}
}

func TestJoinClassicGameLobbyUpdate(t *testing.T) {
	lobby := json.RawMessage(`{"player_count":4,"player_ids":["u1","u2"]}`)
	svc := NewGamesService(&fakeDB{q: noopQuerier{}, txQ: noopQuerier{}}, &fakeGamesStore{
		createFn:  func(context.Context, db.Querier, store.NewGame) (store.Game, error) { return store.Game{}, nil },
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "lobby", State: lobby}, nil
		},
		listFn: func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(_ context.Context, _ db.Querier, in store.UpdateGameState) (store.Game, error) {
			if in.Status != "lobby" {
				t.Fatalf("expected lobby status")
			}
			var next map[string]any
			_ = json.Unmarshal(in.State, &next)
			ids := next["player_ids"].([]any)
			if len(ids) != 3 {
				t.Fatalf("expected 3 joined players")
			}
			return store.Game{ID: "g1", Status: "lobby", State: in.State}, nil
		},
	})

	out, err := svc.JoinClassicGame(context.Background(), "g1", "u3")
	if err != nil {
		t.Fatalf("join game: %v", err)
	}
	if out.Status != "lobby" {
		t.Fatalf("unexpected status: %s", out.Status)
	}
}

func TestJoinClassicGameErrors(t *testing.T) {
	full := json.RawMessage(`{"player_count":3,"player_ids":["u1","u2","u3"]}`)
	svc := NewGamesService(&fakeDB{q: noopQuerier{}, txQ: noopQuerier{}}, &fakeGamesStore{
		createFn:  func(context.Context, db.Querier, store.NewGame) (store.Game, error) { return store.Game{}, nil },
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(_ context.Context, _ db.Querier, gameID string) (store.Game, error) {
			switch gameID {
			case "missing":
				return store.Game{}, pgx.ErrNoRows
			case "started":
				return store.Game{ID: gameID, Status: "in_progress", State: full}, nil
			default:
				return store.Game{ID: gameID, Status: "lobby", State: full}, nil
			}
		},
		listFn:        func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error) { return store.Game{}, nil },
	})

	if _, err := svc.JoinClassicGame(context.Background(), "", "u1"); !errors.Is(err, ErrInvalidGameInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
	if _, err := svc.JoinClassicGame(context.Background(), "missing", "u1"); !errors.Is(err, ErrGameNotFound) {
		t.Fatalf("expected game not found, got %v", err)
	}
	if _, err := svc.JoinClassicGame(context.Background(), "started", "u4"); !errors.Is(err, ErrGameNotJoinable) {
		t.Fatalf("expected game not joinable, got %v", err)
	}
	if _, err := svc.JoinClassicGame(context.Background(), "full", "u4"); !errors.Is(err, ErrGamePlayerCountFull) {
		t.Fatalf("expected game full, got %v", err)
	}
}

func TestGetGameMapsNotFound(t *testing.T) {
	svc := NewGamesService(&fakeDB{q: noopQuerier{}}, &fakeGamesStore{
		createFn:         func(context.Context, db.Querier, store.NewGame) (store.Game, error) { return store.Game{}, nil },
		getByIDFn:        func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, pgx.ErrNoRows },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		listFn:           func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn:    func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error) { return store.Game{}, nil },
	})

	_, err := svc.GetGame(context.Background(), "missing")
	if !errors.Is(err, ErrGameNotFound) {
		t.Fatalf("expected ErrGameNotFound, got %v", err)
	}
}

func TestUpdateGameState(t *testing.T) {
	state := json.RawMessage(`{"turn":2}`)
	svc := NewGamesService(&fakeDB{q: noopQuerier{}}, &fakeGamesStore{
		createFn:         func(context.Context, db.Querier, store.NewGame) (store.Game, error) { return store.Game{}, nil },
		getByIDFn:        func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		listFn:           func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(_ context.Context, _ db.Querier, in store.UpdateGameState) (store.Game, error) {
			if in.GameID != "g1" || in.Status != "in_progress" {
				t.Fatalf("unexpected update input: %#v", in)
			}
			return store.Game{ID: "g1", Status: in.Status, State: in.State}, nil
		},
	})

	out, err := svc.UpdateGameState(context.Background(), "g1", "in_progress", state)
	if err != nil {
		t.Fatalf("update game state: %v", err)
	}
	if out.Status != "in_progress" {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestListGamesValidationAndPassThrough(t *testing.T) {
	called := false
	svc := NewGamesService(&fakeDB{q: noopQuerier{}}, &fakeGamesStore{
		createFn:         func(context.Context, db.Querier, store.NewGame) (store.Game, error) { return store.Game{}, nil },
		getByIDFn:        func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		listFn: func(_ context.Context, _ db.Querier, filter store.GameListFilter) ([]store.Game, error) {
			called = true
			if filter.OwnerUserID != "u1" || filter.Status != "lobby" || filter.Limit != 20 || filter.Offset != 10 {
				t.Fatalf("unexpected filter: %#v", filter)
			}
			return []store.Game{{ID: "g1"}}, nil
		},
		updateStateFn: func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error) { return store.Game{}, nil },
	})

	if _, err := svc.ListGames(context.Background(), "", "", -1, 0); !errors.Is(err, ErrInvalidGameInput) {
		t.Fatalf("expected ErrInvalidGameInput, got %v", err)
	}
	if _, err := svc.ListGames(context.Background(), "", "", 0, -2); !errors.Is(err, ErrInvalidGameInput) {
		t.Fatalf("expected ErrInvalidGameInput, got %v", err)
	}

	out, err := svc.ListGames(context.Background(), "u1", "lobby", 20, 10)
	if err != nil {
		t.Fatalf("list games: %v", err)
	}
	if !called || len(out) != 1 || out[0].ID != "g1" {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestIsLegacyUninitializedSetup(t *testing.T) {
	game := risk.Game{
		Phase: risk.PhaseSetupClaim,
		Players: []risk.PlayerState{
			{ID: "u1"},
			{ID: "u2"},
			{ID: "u3"},
		},
		Territories: map[risk.Territory]risk.TerritoryState{
			"Alaska": {Owner: -1, Armies: 0},
			"Peru":   {Owner: -1, Armies: 0},
		},
	}
	if !isLegacyUninitializedSetup(game) {
		t.Fatalf("expected legacy setup state to be detected")
	}

	game.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}
	if !isLegacyUninitializedSetup(game) {
		t.Fatalf("expected setup phases to be considered legacy setup regardless of partial claims")
	}

	game.Phase = risk.PhaseReinforce
	if isLegacyUninitializedSetup(game) {
		t.Fatalf("expected non-setup phase not to be considered legacy setup")
	}
}
