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
	createFn      func(context.Context, db.Querier, store.NewGame) (store.Game, error)
	getByIDFn     func(context.Context, db.Querier, string) (store.Game, error)
	listFn        func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error)
	updateStateFn func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error)
}

func (f *fakeGamesStore) Create(ctx context.Context, q db.Querier, in store.NewGame) (store.Game, error) {
	return f.createFn(ctx, q, in)
}

func (f *fakeGamesStore) GetByID(ctx context.Context, q db.Querier, gameID string) (store.Game, error) {
	return f.getByIDFn(ctx, q, gameID)
}

func (f *fakeGamesStore) List(ctx context.Context, q db.Querier, filter store.GameListFilter) ([]store.Game, error) {
	return f.listFn(ctx, q, filter)
}

func (f *fakeGamesStore) UpdateState(ctx context.Context, q db.Querier, in store.UpdateGameState) (store.Game, error) {
	return f.updateStateFn(ctx, q, in)
}

func TestCreateClassicGameValidation(t *testing.T) {
	svc := NewGamesService(&fakeDB{q: countQuerier{count: 3}}, &fakeGamesStore{
		createFn: func(context.Context, db.Querier, store.NewGame) (store.Game, error) {
			t.Fatalf("create should not be called")
			return store.Game{}, nil
		},
		getByIDFn:     func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		listFn:        func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error) { return store.Game{}, nil },
	})

	_, err := svc.CreateClassicGame(context.Background(), "", []string{"u1", "u2", "u3"})
	if !errors.Is(err, ErrInvalidGameInput) {
		t.Fatalf("expected ErrInvalidGameInput, got %v", err)
	}

	_, err = svc.CreateClassicGame(context.Background(), "u1", []string{"u2", "u3", "u4"})
	if !errors.Is(err, ErrOwnerNotInPlayers) {
		t.Fatalf("expected ErrOwnerNotInPlayers, got %v", err)
	}

	_, err = svc.CreateClassicGame(context.Background(), "u1", []string{"u1", "u1", "u2"})
	if !errors.Is(err, ErrDuplicatePlayers) {
		t.Fatalf("expected ErrDuplicatePlayers, got %v", err)
	}

	_, err = svc.CreateClassicGame(context.Background(), "u1", []string{"u1", "u2"})
	if !errors.Is(err, risk.ErrInvalidPlayerCount) {
		t.Fatalf("expected risk.ErrInvalidPlayerCount, got %v", err)
	}
}

func TestCreateClassicGamePersistsState(t *testing.T) {
	called := false
	svc := NewGamesService(&fakeDB{q: countQuerier{count: 3}}, &fakeGamesStore{
		createFn: func(_ context.Context, _ db.Querier, in store.NewGame) (store.Game, error) {
			called = true
			if in.OwnerUserID != "u1" || in.Status != "lobby" {
				t.Fatalf("unexpected create input: %#v", in)
			}
			if len(in.State) == 0 {
				t.Fatalf("expected serialized state")
			}
			return store.Game{ID: "g1", OwnerUserID: in.OwnerUserID, Status: in.Status, State: in.State}, nil
		},
		getByIDFn:     func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		listFn:        func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error) { return store.Game{}, nil },
	})

	out, err := svc.CreateClassicGame(context.Background(), "u1", []string{"u1", "u2", "u3"})
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

func TestCreateClassicGameRejectsUnknownPlayers(t *testing.T) {
	svc := NewGamesService(&fakeDB{q: countQuerier{count: 2}}, &fakeGamesStore{
		createFn: func(context.Context, db.Querier, store.NewGame) (store.Game, error) {
			t.Fatalf("create should not be called when users are missing")
			return store.Game{}, nil
		},
		getByIDFn:     func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		listFn:        func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error) { return store.Game{}, nil },
	})

	_, err := svc.CreateClassicGame(context.Background(), "u1", []string{"u1", "u2", "u3"})
	if !errors.Is(err, ErrUnknownPlayerIDs) {
		t.Fatalf("expected ErrUnknownPlayerIDs, got %v", err)
	}
}

func TestGetGameMapsNotFound(t *testing.T) {
	svc := NewGamesService(&fakeDB{q: noopQuerier{}}, &fakeGamesStore{
		createFn:      func(context.Context, db.Querier, store.NewGame) (store.Game, error) { return store.Game{}, nil },
		getByIDFn:     func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, pgx.ErrNoRows },
		listFn:        func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error) { return store.Game{}, nil },
	})

	_, err := svc.GetGame(context.Background(), "missing")
	if !errors.Is(err, ErrGameNotFound) {
		t.Fatalf("expected ErrGameNotFound, got %v", err)
	}
}

func TestUpdateGameState(t *testing.T) {
	state := json.RawMessage(`{"turn":2}`)
	svc := NewGamesService(&fakeDB{q: noopQuerier{}}, &fakeGamesStore{
		createFn:  func(context.Context, db.Querier, store.NewGame) (store.Game, error) { return store.Game{}, nil },
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		listFn:    func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
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
		createFn:  func(context.Context, db.Querier, store.NewGame) (store.Game, error) { return store.Game{}, nil },
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
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
