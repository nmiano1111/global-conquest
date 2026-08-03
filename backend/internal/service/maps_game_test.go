package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/db"
	"github.com/nmiano1111/global-conquest/backend/internal/mapgen"
	"github.com/nmiano1111/global-conquest/backend/internal/risk"
	"github.com/nmiano1111/global-conquest/backend/internal/store"
)

// fakeMapResolver is a minimal mapBoardResolver for tests, resolving a
// single known map ID to a definition generated on demand.
type fakeMapResolver struct {
	id  string
	def mapgen.MapDefinition
	err error
}

func (f *fakeMapResolver) GetMap(_ context.Context, mapID string) (MapDetail, error) {
	if f.err != nil {
		return MapDetail{}, f.err
	}
	if mapID != f.id {
		return MapDetail{}, ErrMapNotFound
	}
	return MapDetail{ID: f.id, Name: "Custom Map", Definition: f.def}, nil
}

func testMapDefinition(t *testing.T) mapgen.MapDefinition {
	t.Helper()
	def, err := mapgen.Generate(mapgen.MapSpec{
		Continents: []mapgen.ContinentSpec{
			{Name: "Redland", Bonus: 2, TerritoryCount: 3},
			{Name: "Blueland", Bonus: 1, TerritoryCount: 3},
		},
		Borders: []mapgen.ContinentBorder{{A: "Redland", B: "Blueland", Crossings: 1}},
	}, nil)
	if err != nil {
		t.Fatalf("generate test map: %v", err)
	}
	return def
}

func TestCreateGameWithMap_RejectsUnknownMap(t *testing.T) {
	svc := createServiceCapturingGame(t, new(store.NewGame))
	svc.SetMapsService(&fakeMapResolver{id: "m1", err: errors.New("not found")})

	_, err := svc.CreateGameWithMap(context.Background(), "u1", 3, "", 0, "does-not-exist")
	if !errors.Is(err, ErrInvalidGameInput) {
		t.Fatalf("expected ErrInvalidGameInput, got %v", err)
	}
}

func TestCreateGameWithMap_NoMapsServiceConfigured(t *testing.T) {
	svc := createServiceCapturingGame(t, new(store.NewGame))
	// No SetMapsService call.

	_, err := svc.CreateGameWithMap(context.Background(), "u1", 3, "", 0, "m1")
	if err == nil {
		t.Fatalf("expected an error when map_id is given but no MapsService is configured")
	}
}

func TestCreateGameWithMap_StartsEngineOnCustomBoard(t *testing.T) {
	def := testMapDefinition(t)
	captured := new(store.NewGame)
	svc := createServiceCapturingGame(t, captured)
	svc.SetMapsService(&fakeMapResolver{id: "m1", def: def})
	svc.SetBotNameAssigner(fixedBotNames("Randy Savage", "Bret Hart"))

	g, err := svc.CreateGameWithMap(context.Background(), "u1", 3, "", 2, "m1")
	if err != nil {
		t.Fatalf("create game with map: %v", err)
	}
	if g.Status != "in_progress" {
		t.Fatalf("expected in_progress (fully bot-filled at creation), got %s", g.Status)
	}

	var engine risk.Game
	if err := json.Unmarshal(captured.State, &engine); err != nil {
		t.Fatalf("decode engine state: %v", err)
	}
	if len(engine.Territories) != len(def.Board.Order) {
		t.Fatalf("expected %d territories from custom board, got %d", len(def.Board.Order), len(engine.Territories))
	}
	if _, ok := engine.Board.Continents["Redland"]; !ok {
		t.Fatalf("expected custom continent %q in engine board, got %#v", "Redland", engine.Board.Continents)
	}
	if captured.MapID != "m1" {
		t.Fatalf("expected persisted game row to carry map_id, got %q", captured.MapID)
	}
}

func TestGetGameBootstrap_CustomMapPopulatesBoardAndLayout(t *testing.T) {
	def := testMapDefinition(t)
	engine := risk.Game{
		Phase:         risk.PhaseReinforce,
		Board:         def.Board,
		Players:       []risk.PlayerState{{ID: "u1"}, {ID: "u2"}, {ID: "u3"}},
		Territories:   map[risk.Territory]risk.TerritoryState{},
		SetupReserves: map[int]int{},
	}
	for _, t2 := range def.Board.Order {
		engine.Territories[t2] = risk.TerritoryState{Owner: 0, Armies: 1}
	}
	state, err := json.Marshal(engine)
	if err != nil {
		t.Fatalf("marshal engine: %v", err)
	}

	svc := NewGamesService(&fakeDB{}, &fakeGamesStore{
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: state, MapID: "m1"}, nil
		},
	})
	svc.SetMapsService(&fakeMapResolver{id: "m1", def: def})

	out, err := svc.GetGameBootstrap(context.Background(), "g1", "u1", false, false)
	if err != nil {
		t.Fatalf("get bootstrap: %v", err)
	}
	if out.MapID != "m1" {
		t.Fatalf("expected map_id m1, got %q", out.MapID)
	}
	if out.Board == nil {
		t.Fatalf("expected board to be populated for a custom-map game")
	}
	if _, ok := out.Board.Continents["Redland"]; !ok {
		t.Fatalf("expected custom continent in bootstrap board, got %#v", out.Board.Continents)
	}
	if len(out.MapLayout) != len(def.Board.Order) {
		t.Fatalf("expected layout for every territory, got %d of %d", len(out.MapLayout), len(def.Board.Order))
	}
}

func TestGetGameBootstrap_LobbyCustomMapPopulatesBoard(t *testing.T) {
	def := testMapDefinition(t)
	lobby := lobbyState{
		PlayerCount: 3,
		PlayerIDs:   []string{"u1"},
		MapID:       "m1",
	}
	raw, err := json.Marshal(lobby)
	if err != nil {
		t.Fatalf("marshal lobby: %v", err)
	}

	svc := NewGamesService(&fakeDB{}, &fakeGamesStore{
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "lobby", State: raw, MapID: "m1"}, nil
		},
	})
	svc.SetMapsService(&fakeMapResolver{id: "m1", def: def})

	out, err := svc.GetGameBootstrap(context.Background(), "g1", "u1", false, false)
	if err != nil {
		t.Fatalf("get bootstrap: %v", err)
	}
	if out.Status != "lobby" {
		t.Fatalf("expected lobby status, got %q", out.Status)
	}
	// A client that navigates to the game page while it's still waiting for
	// players must already see the real board -- game_state_updated's
	// frontend handler only ever patches phase/players/territories from
	// live updates, never board/map_id, so this first (lobby-status) load
	// is the only chance to get it right.
	if out.MapID != "m1" {
		t.Fatalf("expected map_id m1, got %q", out.MapID)
	}
	if out.Board == nil {
		t.Fatalf("expected board to be populated for a lobby-status custom-map game")
	}
	if _, ok := out.Board.Continents["Redland"]; !ok {
		t.Fatalf("expected custom continent in lobby bootstrap board, got %#v", out.Board.Continents)
	}
	if len(out.MapLayout) != len(def.Board.Order) {
		t.Fatalf("expected layout for every territory, got %d of %d", len(out.MapLayout), len(def.Board.Order))
	}
}

func TestGetGameBootstrap_ClassicGameOmitsBoard(t *testing.T) {
	engine := risk.Game{
		Phase:         risk.PhaseReinforce,
		Board:         risk.ClassicBoard(),
		Players:       []risk.PlayerState{{ID: "u1"}, {ID: "u2"}, {ID: "u3"}},
		Territories:   map[risk.Territory]risk.TerritoryState{"Alaska": {Owner: 0, Armies: 1}},
		SetupReserves: map[int]int{},
	}
	state, err := json.Marshal(engine)
	if err != nil {
		t.Fatalf("marshal engine: %v", err)
	}

	svc := NewGamesService(&fakeDB{}, &fakeGamesStore{
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: state}, nil
		},
	})

	out, err := svc.GetGameBootstrap(context.Background(), "g1", "u1", false, false)
	if err != nil {
		t.Fatalf("get bootstrap: %v", err)
	}
	if out.MapID != "" || out.Board != nil || out.MapLayout != nil {
		t.Fatalf("expected classic game to omit map fields, got MapID=%q Board=%v MapLayout=%v", out.MapID, out.Board, out.MapLayout)
	}
}
