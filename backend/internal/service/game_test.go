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

// fakeDomainEventStore records InsertDomainEvent calls for assertions.
type fakeDomainEventStore struct {
	insertFn func(context.Context, db.Querier, string, risk.DomainEvent, []byte) (store.GameDomainEvent, error)
	calls    int
}

func (f *fakeDomainEventStore) InsertDomainEvent(ctx context.Context, q db.Querier, gameID string, ev risk.DomainEvent, payload []byte) (store.GameDomainEvent, error) {
	f.calls++
	if f.insertFn != nil {
		return f.insertFn(ctx, q, gameID, ev, payload)
	}
	return store.GameDomainEvent{ID: "ev1", GameID: gameID, GameSequence: int64(f.calls), EventType: ev.Type}, nil
}

// attackPhaseGameState returns JSON for a risk.Game in PhaseAttack with Alaska owned by
// the first shuffled player (currentPlayer) and Kamchatka owned by the second.
func attackPhaseGameState(t *testing.T) (json.RawMessage, string, string) {
	t.Helper()
	g, err := risk.NewClassicAutoStartGame([]string{"uid-p1", "uid-p2", "uid-p3"}, nil)
	if err != nil {
		t.Fatalf("new game: %v", err)
	}
	g.Phase = risk.PhaseAttack
	g.PendingReinforcements = 0
	attackerIdx := g.CurrentPlayer
	defenderIdx := (attackerIdx + 1) % len(g.Players)
	attackerID := g.Players[attackerIdx].ID
	defenderID := g.Players[defenderIdx].ID
	g.Territories["Alaska"] = risk.TerritoryState{Owner: attackerIdx, Armies: 5}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: defenderIdx, Armies: 2}
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal game: %v", err)
	}
	return raw, attackerID, defenderID
}

func TestApplyAttackProducesDomainEvent(t *testing.T) {
	gameState, attackerID, _ := attackPhaseGameState(t)

	domainStore := &fakeDomainEventStore{}
	svc := NewGamesService(&fakeDB{q: noopQuerier{}, txQ: noopQuerier{}}, &fakeGamesStore{
		createFn:  func(context.Context, db.Querier, store.NewGame) (store.Game, error) { return store.Game{}, nil },
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: gameState}, nil
		},
		listFn: func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: gameState}, nil
		},
	})
	svc.SetGameDomainEventStore(domainStore)

	_, err := svc.ApplyGameAction(context.Background(), GameActionInput{
		GameID:       "g1",
		PlayerUserID: attackerID,
		Action:       "attack",
		From:         "Alaska",
		To:           "Kamchatka",
		AttackerDice: 3,
		DefenderDice: 2,
	})
	if err != nil {
		t.Fatalf("ApplyGameAction attack: %v", err)
	}
	if domainStore.calls != 1 {
		t.Fatalf("expected 1 InsertDomainEvent call, got %d", domainStore.calls)
	}
}

func TestApplyAttackDomainEventPayloadType(t *testing.T) {
	gameState, attackerID, _ := attackPhaseGameState(t)

	var capturedEv risk.DomainEvent
	domainStore := &fakeDomainEventStore{
		insertFn: func(_ context.Context, _ db.Querier, _ string, ev risk.DomainEvent, payload []byte) (store.GameDomainEvent, error) {
			capturedEv = ev
			return store.GameDomainEvent{ID: "ev1", EventType: ev.Type, GameSequence: 1}, nil
		},
	}
	svc := NewGamesService(&fakeDB{q: noopQuerier{}, txQ: noopQuerier{}}, &fakeGamesStore{
		createFn:  func(context.Context, db.Querier, store.NewGame) (store.Game, error) { return store.Game{}, nil },
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: gameState}, nil
		},
		listFn: func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: gameState}, nil
		},
	})
	svc.SetGameDomainEventStore(domainStore)

	_, err := svc.ApplyGameAction(context.Background(), GameActionInput{
		GameID:       "g1",
		PlayerUserID: attackerID,
		Action:       "attack",
		From:         "Alaska",
		To:           "Kamchatka",
		AttackerDice: 3,
		DefenderDice: 2,
	})
	if err != nil {
		t.Fatalf("ApplyGameAction: %v", err)
	}
	if capturedEv.Type != risk.EventTypeCombatRollResolved {
		t.Fatalf("expected event type %q, got %q", risk.EventTypeCombatRollResolved, capturedEv.Type)
	}
	if capturedEv.ActorPlayerID != attackerID {
		t.Fatalf("expected actor %q, got %q", attackerID, capturedEv.ActorPlayerID)
	}
	pl, ok := capturedEv.Payload.(risk.CombatRollResolvedPayload)
	if !ok {
		t.Fatalf("expected CombatRollResolvedPayload, got %T", capturedEv.Payload)
	}
	if pl.SourceTerritoryID != "Alaska" || pl.TargetTerritoryID != "Kamchatka" {
		t.Fatalf("unexpected territories: src=%q tgt=%q", pl.SourceTerritoryID, pl.TargetTerritoryID)
	}
	if pl.AttackerPlayerID != attackerID {
		t.Fatalf("unexpected attacker player id: %q", pl.AttackerPlayerID)
	}
}

func TestNonAttackActionProducesNoDomainEvent(t *testing.T) {
	g, err := risk.NewClassicAutoStartGame([]string{"uid-p1", "uid-p2", "uid-p3"}, nil)
	if err != nil {
		t.Fatalf("new game: %v", err)
	}
	actorIdx := g.CurrentPlayer
	actorID := g.Players[actorIdx].ID
	// Find a territory owned by the current player to place a reinforcement on.
	var ownedTerr string
	for terr, ts := range g.Territories {
		if ts.Owner == actorIdx {
			ownedTerr = string(terr)
			break
		}
	}
	if ownedTerr == "" {
		t.Fatal("no owned territory found for current player")
	}
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	domainStore := &fakeDomainEventStore{}
	svc := NewGamesService(&fakeDB{q: noopQuerier{}, txQ: noopQuerier{}}, &fakeGamesStore{
		createFn:  func(context.Context, db.Querier, store.NewGame) (store.Game, error) { return store.Game{}, nil },
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: raw}, nil
		},
		listFn: func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: raw}, nil
		},
	})
	svc.SetGameDomainEventStore(domainStore)

	// place_reinforcement never produces a domain event
	_, err = svc.ApplyGameAction(context.Background(), GameActionInput{
		GameID:       "g1",
		PlayerUserID: actorID,
		Action:       "place_reinforcement",
		Territory:    ownedTerr,
		Armies:       1,
	})
	if err != nil {
		t.Fatalf("ApplyGameAction place_reinforcement: %v", err)
	}
	if domainStore.calls != 0 {
		t.Fatalf("expected 0 domain event calls for non-attack action, got %d", domainStore.calls)
	}
}

func TestApplyAttackDomainEventStoreErrorRollsBack(t *testing.T) {
	gameState, attackerID, _ := attackPhaseGameState(t)

	storeErr := errors.New("event store failure")
	domainStore := &fakeDomainEventStore{
		insertFn: func(context.Context, db.Querier, string, risk.DomainEvent, []byte) (store.GameDomainEvent, error) {
			return store.GameDomainEvent{}, storeErr
		},
	}
	updateCalled := false
	svc := NewGamesService(&fakeDB{q: noopQuerier{}, txQ: noopQuerier{}}, &fakeGamesStore{
		createFn:  func(context.Context, db.Querier, store.NewGame) (store.Game, error) { return store.Game{}, nil },
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: gameState}, nil
		},
		listFn: func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error) {
			updateCalled = true
			return store.Game{ID: "g1", Status: "in_progress", State: gameState}, nil
		},
	})
	svc.SetGameDomainEventStore(domainStore)

	_, err := svc.ApplyGameAction(context.Background(), GameActionInput{
		GameID:       "g1",
		PlayerUserID: attackerID,
		Action:       "attack",
		From:         "Alaska",
		To:           "Kamchatka",
		AttackerDice: 3,
		DefenderDice: 2,
	})
	if err == nil {
		t.Fatal("expected error when domain event store fails")
	}
	if !errors.Is(err, storeErr) {
		t.Fatalf("expected storeErr, got: %v", err)
	}
	if !updateCalled {
		t.Fatal("expected state update to have been attempted before event store failed")
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

// --- fakeDiscordOutboxStore and end_turn outbox tests ---

type fakeDiscordOutboxStore struct {
	enqueueFn func(ctx context.Context, q db.Querier, gameID, previousPlayerDisplayName, playerID, playerDisplayName string, previousPlayerDiscordName, playerDiscordName *string, turnNumber int) error
	calls     int
	lastQ     db.Querier
}

func (f *fakeDiscordOutboxStore) EnqueueTurnStarted(ctx context.Context, q db.Querier, gameID, previousPlayerDisplayName, playerID, playerDisplayName string, previousPlayerDiscordName, playerDiscordName *string, turnNumber int) error {
	f.calls++
	f.lastQ = q
	if f.enqueueFn != nil {
		return f.enqueueFn(ctx, q, gameID, previousPlayerDisplayName, playerID, playerDisplayName, previousPlayerDiscordName, playerDiscordName, turnNumber)
	}
	return nil
}

// endTurnGameState builds a 3-player game in attack phase suitable for end_turn.
func endTurnGameState(t *testing.T) (json.RawMessage, string) {
	t.Helper()
	g, err := risk.NewClassicAutoStartGame([]string{"uid-p1", "uid-p2", "uid-p3"}, nil)
	if err != nil {
		t.Fatalf("new game: %v", err)
	}
	// Force into attack phase so end_turn is valid.
	g.Phase = risk.PhaseAttack
	actorID := g.Players[g.CurrentPlayer].ID
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw, actorID
}

func TestEndTurnEnqueuesOutboxNotification(t *testing.T) {
	gameState, actorID := endTurnGameState(t)

	outboxStore := &fakeDiscordOutboxStore{}
	svc := NewGamesService(&fakeDB{q: noopQuerier{}, txQ: noopQuerier{}}, &fakeGamesStore{
		createFn:  func(context.Context, db.Querier, store.NewGame) (store.Game, error) { return store.Game{}, nil },
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: gameState}, nil
		},
		listFn: func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: gameState}, nil
		},
	})
	svc.SetDiscordOutboxStore(outboxStore)

	_, err := svc.ApplyGameAction(context.Background(), GameActionInput{
		GameID:       "g1",
		PlayerUserID: actorID,
		Action:       "end_turn",
	})
	if err != nil {
		t.Fatalf("ApplyGameAction end_turn: %v", err)
	}
	if outboxStore.calls != 1 {
		t.Fatalf("expected 1 EnqueueTurnStarted call, got %d", outboxStore.calls)
	}
}

func TestEndTurnOutboxPayloadCorrect(t *testing.T) {
	gameState, actorID := endTurnGameState(t)

	var capturedPlayerID, capturedDisplayName string
	var capturedTurnNumber int
	outboxStore := &fakeDiscordOutboxStore{
		enqueueFn: func(_ context.Context, _ db.Querier, _, _, playerID, displayName string, _, _ *string, turnNumber int) error {
			capturedPlayerID = playerID
			capturedDisplayName = displayName
			capturedTurnNumber = turnNumber
			return nil
		},
	}

	svc := NewGamesService(&fakeDB{q: noopQuerier{}, txQ: noopQuerier{}}, &fakeGamesStore{
		createFn:  func(context.Context, db.Querier, store.NewGame) (store.Game, error) { return store.Game{}, nil },
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: gameState}, nil
		},
		listFn: func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: gameState}, nil
		},
	})
	svc.SetDiscordOutboxStore(outboxStore)

	_, err := svc.ApplyGameAction(context.Background(), GameActionInput{
		GameID:       "g1",
		PlayerUserID: actorID,
		Action:       "end_turn",
	})
	if err != nil {
		t.Fatalf("ApplyGameAction: %v", err)
	}

	// The next player (not the actor) should be the enqueued player.
	if capturedPlayerID == actorID {
		t.Fatalf("enqueued player should be the NEXT player, not the actor")
	}
	if capturedPlayerID == "" {
		t.Fatalf("enqueued player ID must not be empty")
	}
	if capturedDisplayName == "" {
		t.Fatalf("enqueued player display name must not be empty")
	}
	if capturedTurnNumber <= 0 {
		t.Fatalf("turn number must be positive, got %d", capturedTurnNumber)
	}
}

func TestEndTurnOutboxUsesTransactionQuerier(t *testing.T) {
	gameState, actorID := endTurnGameState(t)

	txQ := noopQuerier{}
	var qUsedForUpdate, qUsedForOutbox db.Querier
	outboxStore := &fakeDiscordOutboxStore{
		enqueueFn: func(_ context.Context, q db.Querier, _, _, _, _ string, _, _ *string, _ int) error {
			qUsedForOutbox = q
			return nil
		},
	}

	svc := NewGamesService(&fakeDB{q: noopQuerier{}, txQ: txQ}, &fakeGamesStore{
		createFn:  func(context.Context, db.Querier, store.NewGame) (store.Game, error) { return store.Game{}, nil },
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: gameState}, nil
		},
		listFn: func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(_ context.Context, q db.Querier, _ store.UpdateGameState) (store.Game, error) {
			qUsedForUpdate = q
			return store.Game{ID: "g1", Status: "in_progress", State: gameState}, nil
		},
	})
	svc.SetDiscordOutboxStore(outboxStore)

	_, err := svc.ApplyGameAction(context.Background(), GameActionInput{
		GameID:       "g1",
		PlayerUserID: actorID,
		Action:       "end_turn",
	})
	if err != nil {
		t.Fatalf("ApplyGameAction: %v", err)
	}
	if qUsedForUpdate == nil || qUsedForOutbox == nil {
		t.Fatalf("expected both update and outbox to receive a querier")
	}
	// Both must use the transaction querier, not the pool querier.
	if qUsedForUpdate != qUsedForOutbox {
		t.Fatalf("update and outbox must use the same transaction querier")
	}
}

func TestEndTurnOutboxErrorRollsBack(t *testing.T) {
	gameState, actorID := endTurnGameState(t)
	outboxErr := errors.New("outbox enqueue failure")

	outboxStore := &fakeDiscordOutboxStore{
		enqueueFn: func(_ context.Context, _ db.Querier, _, _, _, _ string, _, _ *string, _ int) error {
			return outboxErr
		},
	}

	svc := NewGamesService(&fakeDB{q: noopQuerier{}, txQ: noopQuerier{}}, &fakeGamesStore{
		createFn:  func(context.Context, db.Querier, store.NewGame) (store.Game, error) { return store.Game{}, nil },
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: gameState}, nil
		},
		listFn: func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: gameState}, nil
		},
	})
	svc.SetDiscordOutboxStore(outboxStore)

	_, err := svc.ApplyGameAction(context.Background(), GameActionInput{
		GameID:       "g1",
		PlayerUserID: actorID,
		Action:       "end_turn",
	})
	if err == nil {
		t.Fatal("expected error when outbox enqueue fails")
	}
	if !errors.Is(err, outboxErr) {
		t.Fatalf("expected outboxErr, got: %v", err)
	}
}

func TestInvalidActionProducesNoOutboxNotification(t *testing.T) {
	gameState, actorID := endTurnGameState(t)

	outboxStore := &fakeDiscordOutboxStore{}
	svc := NewGamesService(&fakeDB{q: noopQuerier{}, txQ: noopQuerier{}}, &fakeGamesStore{
		createFn:  func(context.Context, db.Querier, store.NewGame) (store.Game, error) { return store.Game{}, nil },
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: gameState}, nil
		},
		listFn: func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: gameState}, nil
		},
	})
	svc.SetDiscordOutboxStore(outboxStore)

	// fortify with empty territory is an invalid action
	_, err := svc.ApplyGameAction(context.Background(), GameActionInput{
		GameID:       "g1",
		PlayerUserID: actorID,
		Action:       "fortify",
		From:         "",
		To:           "",
		Armies:       0,
	})
	if err == nil {
		t.Fatal("expected error for invalid action")
	}
	if outboxStore.calls != 0 {
		t.Fatalf("invalid action must not produce outbox notification, got %d calls", outboxStore.calls)
	}
}

func TestNonEndTurnActionProducesNoOutboxNotification(t *testing.T) {
	g, err := risk.NewClassicAutoStartGame([]string{"uid-p1", "uid-p2", "uid-p3"}, nil)
	if err != nil {
		t.Fatalf("new game: %v", err)
	}
	actorIdx := g.CurrentPlayer
	actorID := g.Players[actorIdx].ID
	var ownedTerr string
	for terr, ts := range g.Territories {
		if ts.Owner == actorIdx {
			ownedTerr = string(terr)
			break
		}
	}
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	outboxStore := &fakeDiscordOutboxStore{}
	svc := NewGamesService(&fakeDB{q: noopQuerier{}, txQ: noopQuerier{}}, &fakeGamesStore{
		createFn:  func(context.Context, db.Querier, store.NewGame) (store.Game, error) { return store.Game{}, nil },
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: raw}, nil
		},
		listFn: func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: raw}, nil
		},
	})
	svc.SetDiscordOutboxStore(outboxStore)

	_, err = svc.ApplyGameAction(context.Background(), GameActionInput{
		GameID:       "g1",
		PlayerUserID: actorID,
		Action:       "place_reinforcement",
		Territory:    ownedTerr,
		Armies:       1,
	})
	if err != nil {
		t.Fatalf("ApplyGameAction: %v", err)
	}
	if outboxStore.calls != 0 {
		t.Fatalf("place_reinforcement must not produce outbox notification, got %d calls", outboxStore.calls)
	}
}
