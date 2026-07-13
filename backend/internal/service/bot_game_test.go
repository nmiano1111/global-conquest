package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/db"
	"github.com/nmiano1111/global-conquest/backend/internal/risk"
	"github.com/nmiano1111/global-conquest/backend/internal/store"

	"github.com/jackc/pgx/v5"
)

// fixedBotNames is a deterministic bot-name assigner for tests, avoiding
// any dependency on real randomness.
func fixedBotNames(names ...string) func(count int, exclude []string) []string {
	return func(count int, _ []string) []string {
		if count > len(names) {
			count = len(names)
		}
		return append([]string(nil), names[:count]...)
	}
}

// createServiceCapturingGame builds a GamesService whose store.Create call
// captures the store.NewGame it was given, so tests can inspect the exact
// status/state a creation call produced.
func createServiceCapturingGame(t *testing.T, capture *store.NewGame) *GamesService {
	t.Helper()
	svc := NewGamesService(&fakeDB{q: countQuerier{count: 1}}, &fakeGamesStore{
		createFn: func(_ context.Context, _ db.Querier, in store.NewGame) (store.Game, error) {
			*capture = in
			return store.Game{ID: "g1", OwnerUserID: in.OwnerUserID, Status: in.Status, State: in.State}, nil
		},
		getByIDFn:        func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		listFn:           func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn:    func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error) { return store.Game{}, nil },
	})
	return svc
}

func decodeLobby(t *testing.T, raw json.RawMessage) lobbyState {
	t.Helper()
	var lobby lobbyState
	if err := json.Unmarshal(raw, &lobby); err != nil {
		t.Fatalf("decode lobby state: %v", err)
	}
	return lobby
}

// --- 2-4: valid mixed human/bot combinations ---

func TestCreateClassicGameThreePlayersOneHumanTwoBots(t *testing.T) {
	var captured store.NewGame
	svc := createServiceCapturingGame(t, &captured)
	svc.SetBotNameAssigner(fixedBotNames("Randy Savage", "Bret Hart"))

	g, err := svc.CreateClassicGame(context.Background(), "u1", 3, "", 2)
	if err != nil {
		t.Fatalf("create classic game: %v", err)
	}
	// 1 human + 2 bots == 3 total: the lobby is full immediately, so this
	// must start the engine right away rather than staying in "lobby".
	if g.Status != "in_progress" {
		t.Fatalf("expected in_progress (fully bot-filled at creation), got %s", g.Status)
	}
	var engine risk.Game
	if err := json.Unmarshal(captured.State, &engine); err != nil {
		t.Fatalf("decode engine state: %v", err)
	}
	if len(engine.Players) != 3 {
		t.Fatalf("expected 3 players, got %d", len(engine.Players))
	}
	botCount, humanCount := 0, 0
	for _, p := range engine.Players {
		if p.IsBot() {
			botCount++
		} else {
			humanCount++
		}
	}
	if botCount != 2 || humanCount != 1 {
		t.Fatalf("expected 2 bots and 1 human, got %d bots, %d humans", botCount, humanCount)
	}
}

func TestCreateClassicGameFourPlayersTwoHumansTwoBots(t *testing.T) {
	var captured store.NewGame
	svc := createServiceCapturingGame(t, &captured)
	svc.SetBotNameAssigner(fixedBotNames("Randy Savage", "Bret Hart"))

	g, err := svc.CreateClassicGame(context.Background(), "u1", 4, "", 2)
	if err != nil {
		t.Fatalf("create classic game: %v", err)
	}
	// 2 humans + 2 bots == 4 total, but only the creator has joined so far:
	// one human slot remains open, so this must stay in "lobby".
	if g.Status != "lobby" {
		t.Fatalf("expected lobby (one human slot still open), got %s", g.Status)
	}
	lobby := decodeLobby(t, captured.State)
	if lobby.BotCount != 2 {
		t.Fatalf("expected bot_count=2, got %d", lobby.BotCount)
	}
	if len(lobby.PlayerIDs) != 3 {
		t.Fatalf("expected 3 occupied slots (creator + 2 bots), got %d", len(lobby.PlayerIDs))
	}
	if len(lobby.BotNames) != 2 {
		t.Fatalf("expected 2 bot names recorded, got %d", len(lobby.BotNames))
	}
}

func TestCreateClassicGameFivePlayersOneHumanFourBots(t *testing.T) {
	var captured store.NewGame
	svc := createServiceCapturingGame(t, &captured)
	svc.SetBotNameAssigner(fixedBotNames("Randy Savage", "Bret Hart", "Ric Flair", "Sting"))

	g, err := svc.CreateClassicGame(context.Background(), "u1", 5, "", 4)
	if err != nil {
		t.Fatalf("create classic game: %v", err)
	}
	if g.Status != "in_progress" {
		t.Fatalf("expected in_progress, got %s", g.Status)
	}
	var engine risk.Game
	if err := json.Unmarshal(captured.State, &engine); err != nil {
		t.Fatalf("decode engine state: %v", err)
	}
	if len(engine.Players) != 5 {
		t.Fatalf("expected 5 players, got %d", len(engine.Players))
	}
}

// --- 5-6: bot_count validation ---

func TestCreateClassicGameRejectsNegativeBotCount(t *testing.T) {
	var captured store.NewGame
	svc := createServiceCapturingGame(t, &captured)

	_, err := svc.CreateClassicGame(context.Background(), "u1", 4, "", -1)
	if err != ErrInvalidGameInput {
		t.Fatalf("expected ErrInvalidGameInput for negative bot_count, got %v", err)
	}
}

func TestCreateClassicGameRejectsBotCountAtOrAbovePlayerCount(t *testing.T) {
	var captured store.NewGame
	svc := createServiceCapturingGame(t, &captured)

	if _, err := svc.CreateClassicGame(context.Background(), "u1", 4, "", 4); err != ErrInvalidGameInput {
		t.Fatalf("expected ErrInvalidGameInput when bot_count == player_count, got %v", err)
	}
	if _, err := svc.CreateClassicGame(context.Background(), "u1", 4, "", 5); err != ErrInvalidGameInput {
		t.Fatalf("expected ErrInvalidGameInput when bot_count > player_count, got %v", err)
	}
}

// --- 7-9: creator stays human; bots get the right controller/strategy ---

func TestCreateClassicGameCreatorRemainsHumanAndBotsUseScoredV1(t *testing.T) {
	var captured store.NewGame
	svc := createServiceCapturingGame(t, &captured)
	svc.SetBotNameAssigner(fixedBotNames("Randy Savage", "Bret Hart"))

	if _, err := svc.CreateClassicGame(context.Background(), "u1", 3, "", 2); err != nil {
		t.Fatalf("create classic game: %v", err)
	}
	var engine risk.Game
	if err := json.Unmarshal(captured.State, &engine); err != nil {
		t.Fatalf("decode engine state: %v", err)
	}
	var creator *risk.PlayerState
	for i := range engine.Players {
		if engine.Players[i].ID == "u1" {
			creator = &engine.Players[i]
		}
	}
	if creator == nil {
		t.Fatalf("creator u1 not found among players")
	}
	if creator.IsBot() {
		t.Fatalf("expected creator to remain human")
	}
	for _, p := range engine.Players {
		if !p.IsBot() {
			continue
		}
		if p.Strategy != "scored-v1" {
			t.Fatalf("expected bot strategy scored-v1, got %q", p.Strategy)
		}
	}
}

// --- 10-12: unique bot IDs and names ---

func TestCreateClassicGameBotsGetUniqueIDsAndNonEmptyNames(t *testing.T) {
	var captured store.NewGame
	svc := createServiceCapturingGame(t, &captured)
	svc.SetBotNameAssigner(fixedBotNames("Randy Savage", "Bret Hart", "Ric Flair"))

	// 5 total, 1 human + 3 bots: one human slot stays open, so this stays
	// in "lobby" and we can inspect the lobby-state shape directly.
	if _, err := svc.CreateClassicGame(context.Background(), "u1", 5, "", 3); err != nil {
		t.Fatalf("create classic game: %v", err)
	}
	lobby := decodeLobby(t, captured.State)
	if len(lobby.BotNames) != 3 {
		t.Fatalf("expected 3 bot entries, got %d", len(lobby.BotNames))
	}
	seenIDs := map[string]struct{}{}
	seenNames := map[string]struct{}{}
	for id, name := range lobby.BotNames {
		if id == "" {
			t.Fatalf("bot has an empty ID")
		}
		if name == "" {
			t.Fatalf("bot %s has an empty name", id)
		}
		if _, dup := seenIDs[id]; dup {
			t.Fatalf("duplicate bot ID: %s", id)
		}
		seenIDs[id] = struct{}{}
		if _, dup := seenNames[name]; dup {
			t.Fatalf("duplicate bot name: %s", name)
		}
		seenNames[name] = struct{}{}
	}
}

// --- 13: human name collisions avoided where practical ---

func TestCreateClassicGameExcludesOwnerNameFromBotAssigner(t *testing.T) {
	var captured store.NewGame
	svc := createServiceCapturingGame(t, &captured)

	var gotExclude []string
	svc.SetBotNameAssigner(func(count int, exclude []string) []string {
		gotExclude = exclude
		out := make([]string, count)
		for i := range out {
			out[i] = "Bot"
		}
		return out
	})

	if _, err := svc.CreateClassicGame(context.Background(), "u1", 3, "", 2); err != nil {
		t.Fatalf("create classic game: %v", err)
	}
	if len(gotExclude) != 1 || gotExclude[0] == "" {
		t.Fatalf("expected the assigner to be called with the owner's name excluded, got %v", gotExclude)
	}
}

// --- 14: random initial distribution includes bots normally ---

func TestCreateClassicGameAutoStartDistributesTerritoriesToBotsToo(t *testing.T) {
	var captured store.NewGame
	svc := createServiceCapturingGame(t, &captured)
	svc.SetBotNameAssigner(fixedBotNames("Randy Savage", "Bret Hart"))

	if _, err := svc.CreateClassicGame(context.Background(), "u1", 3, "", 2); err != nil {
		t.Fatalf("create classic game: %v", err)
	}
	var engine risk.Game
	if err := json.Unmarshal(captured.State, &engine); err != nil {
		t.Fatalf("decode engine state: %v", err)
	}
	counts := make(map[int]int, len(engine.Players))
	for _, ts := range engine.Territories {
		counts[ts.Owner]++
	}
	for i, p := range engine.Players {
		if counts[i] == 0 {
			t.Fatalf("player %d (%s, bot=%v) received no territories", i, p.ID, p.IsBot())
		}
	}
}

// --- 15-16: lobby occupancy and open human slots ---

func TestCreateClassicGameBotsCountTowardFilledSlots(t *testing.T) {
	var captured store.NewGame
	svc := createServiceCapturingGame(t, &captured)
	svc.SetBotNameAssigner(fixedBotNames("Randy Savage", "Bret Hart"))

	if _, err := svc.CreateClassicGame(context.Background(), "u1", 4, "", 2); err != nil {
		t.Fatalf("create classic game: %v", err)
	}
	lobby := decodeLobby(t, captured.State)
	// 4 total, 2 bots already occupy slots alongside the creator: exactly
	// one open human slot should remain.
	openHumanSlots := lobby.PlayerCount - len(lobby.PlayerIDs)
	if openHumanSlots != 1 {
		t.Fatalf("expected 1 open human slot, got %d", openHumanSlots)
	}

	// A second human joining should now fill the game.
	joinSvc := NewGamesService(&fakeDB{q: noopQuerier{}, txQ: noopQuerier{}}, &fakeGamesStore{
		createFn:  func(context.Context, db.Querier, store.NewGame) (store.Game, error) { return store.Game{}, nil },
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "lobby", State: captured.State}, nil
		},
		listFn: func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(_ context.Context, _ db.Querier, in store.UpdateGameState) (store.Game, error) {
			if in.Status != "in_progress" {
				t.Fatalf("expected the game to start once the last human slot fills, got status=%s", in.Status)
			}
			var g risk.Game
			if err := json.Unmarshal(in.State, &g); err != nil {
				t.Fatalf("decode started engine: %v", err)
			}
			if len(g.Players) != 4 {
				t.Fatalf("expected 4 players once started, got %d", len(g.Players))
			}
			return store.Game{ID: "g1", Status: in.Status, State: in.State}, nil
		},
	})
	out, err := joinSvc.JoinClassicGame(context.Background(), "g1", "u2")
	if err != nil {
		t.Fatalf("join classic game: %v", err)
	}
	if out.Status != "in_progress" {
		t.Fatalf("expected in_progress after the last human joins, got %s", out.Status)
	}
}

func TestGetGameBootstrapLobbyReportsBotsAndOpenSlots(t *testing.T) {
	lobby := lobbyState{
		PlayerCount: 4,
		PlayerIDs:   []string{"u1", "bot-1"},
		BotCount:    1,
		BotNames:    map[string]string{"bot-1": "Randy Savage"},
	}
	raw, err := json.Marshal(lobby)
	if err != nil {
		t.Fatalf("marshal lobby: %v", err)
	}
	svc := NewGamesService(&fakeDB{q: noopQuerier{}}, &fakeGamesStore{
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "lobby", State: raw}, nil
		},
	})

	out, err := svc.GetGameBootstrap(context.Background(), "g1", "u1")
	if err != nil {
		t.Fatalf("get game bootstrap: %v", err)
	}
	if out.PlayerCount != 4 {
		t.Fatalf("expected player_count=4, got %d", out.PlayerCount)
	}
	if len(out.Players) != 2 {
		t.Fatalf("expected 2 seated players, got %d", len(out.Players))
	}
	openHumanSlots := out.PlayerCount - len(out.Players)
	if openHumanSlots != 2 {
		t.Fatalf("expected 2 open human slots, got %d", openHumanSlots)
	}
	var botPlayer, humanPlayer *GameBootstrapPlayer
	for i := range out.Players {
		if out.Players[i].UserID == "bot-1" {
			botPlayer = &out.Players[i]
		}
		if out.Players[i].UserID == "u1" {
			humanPlayer = &out.Players[i]
		}
	}
	if botPlayer == nil || !botPlayer.IsBot {
		t.Fatalf("expected bot-1 to be flagged is_bot=true")
	}
	if botPlayer.UserName != "Randy Savage" {
		t.Fatalf("expected bot display name Randy Savage, got %q", botPlayer.UserName)
	}
	if humanPlayer == nil || humanPlayer.IsBot {
		t.Fatalf("expected u1 to be flagged is_bot=false")
	}
}

// --- 17: human-only game behavior remains unchanged ---

func TestCreateClassicGameHumanOnlyOmittedBotCountStillWorks(t *testing.T) {
	var captured store.NewGame
	svc := createServiceCapturingGame(t, &captured)

	g, err := svc.CreateClassicGame(context.Background(), "u1", 4, "", 0)
	if err != nil {
		t.Fatalf("create classic game: %v", err)
	}
	if g.Status != "lobby" {
		t.Fatalf("expected lobby status for a human-only game, got %s", g.Status)
	}
	lobby := decodeLobby(t, captured.State)
	if lobby.BotCount != 0 || len(lobby.BotNames) != 0 {
		t.Fatalf("expected no bots in a human-only game, got bot_count=%d bot_names=%v", lobby.BotCount, lobby.BotNames)
	}
	if len(lobby.PlayerIDs) != 1 || lobby.PlayerIDs[0] != "u1" {
		t.Fatalf("expected only the creator seated, got %v", lobby.PlayerIDs)
	}
}

// --- game-started hook: the only way a bot's first turn ever gets
// triggered when a game starts via REST instead of a game_action, since
// that path never touches game.Server's hub. ---

func TestCreateClassicGameNotifiesGameStartedHookWhenFullAtCreation(t *testing.T) {
	var captured store.NewGame
	svc := createServiceCapturingGame(t, &captured)
	svc.SetBotNameAssigner(fixedBotNames("Randy Savage", "Bret Hart"))

	var notified []string
	svc.SetGameStartedHook(func(gameID string) { notified = append(notified, gameID) })

	g, err := svc.CreateClassicGame(context.Background(), "u1", 3, "", 2)
	if err != nil {
		t.Fatalf("create classic game: %v", err)
	}
	if len(notified) != 1 || notified[0] != g.ID {
		t.Fatalf("expected the game-started hook to fire once with game id %q, got %v", g.ID, notified)
	}
}

func TestCreateClassicGameDoesNotNotifyWhenLobbyStaysOpen(t *testing.T) {
	var captured store.NewGame
	svc := createServiceCapturingGame(t, &captured)
	svc.SetBotNameAssigner(fixedBotNames("Randy Savage"))

	var notified []string
	svc.SetGameStartedHook(func(gameID string) { notified = append(notified, gameID) })

	if _, err := svc.CreateClassicGame(context.Background(), "u1", 4, "", 1); err != nil {
		t.Fatalf("create classic game: %v", err)
	}
	if len(notified) != 0 {
		t.Fatalf("expected no game-started notification while a human slot remains open, got %v", notified)
	}
}

func TestJoinClassicGameNotifiesGameStartedHookWhenLastSlotFills(t *testing.T) {
	lobby := json.RawMessage(`{"player_count":3,"player_ids":["u1","u2"]}`)
	svc := NewGamesService(&fakeDB{q: noopQuerier{}, txQ: noopQuerier{}}, &fakeGamesStore{
		createFn:  func(context.Context, db.Querier, store.NewGame) (store.Game, error) { return store.Game{}, nil },
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "lobby", State: lobby}, nil
		},
		listFn: func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(_ context.Context, _ db.Querier, in store.UpdateGameState) (store.Game, error) {
			return store.Game{ID: "g1", Status: in.Status, State: in.State}, nil
		},
	})
	var notified []string
	svc.SetGameStartedHook(func(gameID string) { notified = append(notified, gameID) })

	out, err := svc.JoinClassicGame(context.Background(), "g1", "u3")
	if err != nil {
		t.Fatalf("join game: %v", err)
	}
	if out.Status != "in_progress" {
		t.Fatalf("expected in_progress, got %s", out.Status)
	}
	if len(notified) != 1 || notified[0] != "g1" {
		t.Fatalf("expected the game-started hook to fire once for g1, got %v", notified)
	}
}

func TestJoinClassicGameDoesNotNotifyWhenLobbyStaysOpen(t *testing.T) {
	lobby := json.RawMessage(`{"player_count":4,"player_ids":["u1","u2"]}`)
	svc := NewGamesService(&fakeDB{q: noopQuerier{}, txQ: noopQuerier{}}, &fakeGamesStore{
		createFn:  func(context.Context, db.Querier, store.NewGame) (store.Game, error) { return store.Game{}, nil },
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "lobby", State: lobby}, nil
		},
		listFn: func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(_ context.Context, _ db.Querier, in store.UpdateGameState) (store.Game, error) {
			return store.Game{ID: "g1", Status: in.Status, State: in.State}, nil
		},
	})
	var notified []string
	svc.SetGameStartedHook(func(gameID string) { notified = append(notified, gameID) })

	if _, err := svc.JoinClassicGame(context.Background(), "g1", "u3"); err != nil {
		t.Fatalf("join game: %v", err)
	}
	if len(notified) != 0 {
		t.Fatalf("expected no game-started notification while a human slot remains open, got %v", notified)
	}
}

// --- action_territory/action_from/action_to: lets the frontend highlight
// what an action (bot or human) touched, the same way a click would. ---

func TestApplyGameActionAttackSetsActionFromTo(t *testing.T) {
	gameState, attackerID, _ := attackPhaseGameState(t)
	svc := NewGamesService(&fakeDB{q: noopQuerier{}, txQ: noopQuerier{}}, &fakeGamesStore{
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: gameState}, nil
		},
		listFn: func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: gameState}, nil
		},
	})

	out, err := svc.ApplyGameAction(context.Background(), GameActionInput{
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
	if out.ActionFrom != "Alaska" || out.ActionTo != "Kamchatka" {
		t.Fatalf("expected action_from=Alaska action_to=Kamchatka, got from=%q to=%q", out.ActionFrom, out.ActionTo)
	}
	if out.ActionTerritory != "" {
		t.Fatalf("expected no action_territory for an attack, got %q", out.ActionTerritory)
	}
}

func TestApplyGameActionReinforcementSetsActionTerritory(t *testing.T) {
	g, err := risk.NewClassicAutoStartGame([]string{"uid-p1", "uid-p2", "uid-p3"}, nil)
	if err != nil {
		t.Fatalf("new game: %v", err)
	}
	actorID := g.Players[g.CurrentPlayer].ID
	g.PendingReinforcements = 3
	var ownedTerr string
	for terr, ts := range g.Territories {
		if ts.Owner == g.CurrentPlayer {
			ownedTerr = string(terr)
			break
		}
	}
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal game: %v", err)
	}
	svc := NewGamesService(&fakeDB{q: noopQuerier{}, txQ: noopQuerier{}}, &fakeGamesStore{
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: raw}, nil
		},
		listFn: func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: raw}, nil
		},
	})

	out, err := svc.ApplyGameAction(context.Background(), GameActionInput{
		GameID:       "g1",
		PlayerUserID: actorID,
		Action:       "place_reinforcement",
		Territory:    ownedTerr,
		Armies:       1,
	})
	if err != nil {
		t.Fatalf("ApplyGameAction place_reinforcement: %v", err)
	}
	if out.ActionTerritory != ownedTerr {
		t.Fatalf("expected action_territory=%q, got %q", ownedTerr, out.ActionTerritory)
	}
	if out.ActionFrom != "" || out.ActionTo != "" {
		t.Fatalf("expected no action_from/to for a reinforcement, got from=%q to=%q", out.ActionFrom, out.ActionTo)
	}
}

// --- game_started Discord notification: the very first turn of a game,
// which turn_started never covers (it only fires when a turn *ends"). ---

func TestCreateClassicGameEnqueuesGameStartedWhenFullAtCreation(t *testing.T) {
	// Which player the engine shuffles to go first isn't controllable here
	// (CreateClassicGame doesn't expose an injectable RNG), and per the
	// human-gating rule, whether a notification fires now legitimately
	// depends on that: expect one iff the resulting first player is human.
	var captured store.NewGame
	svc := createServiceCapturingGame(t, &captured)
	svc.SetBotNameAssigner(fixedBotNames("Randy Savage", "Bret Hart"))
	outboxStore := &fakeDiscordOutboxStore{}
	svc.SetDiscordOutboxStore(outboxStore)

	if _, err := svc.CreateClassicGame(context.Background(), "u1", 3, "", 2); err != nil {
		t.Fatalf("create classic game: %v", err)
	}
	var engine risk.Game
	if err := json.Unmarshal(captured.State, &engine); err != nil {
		t.Fatalf("decode engine state: %v", err)
	}
	firstIsHuman := !engine.Players[engine.CurrentPlayer].IsBot()
	wantCalls := 0
	if firstIsHuman {
		wantCalls = 1
	}
	if outboxStore.gameStartedCalls != wantCalls {
		t.Fatalf("expected %d game_started notification(s) (first player human=%v), got %d", wantCalls, firstIsHuman, outboxStore.gameStartedCalls)
	}
}

func TestCreateClassicGameDoesNotEnqueueGameStartedWhenLobbyStaysOpen(t *testing.T) {
	var captured store.NewGame
	svc := createServiceCapturingGame(t, &captured)
	svc.SetBotNameAssigner(fixedBotNames("Randy Savage"))
	outboxStore := &fakeDiscordOutboxStore{}
	svc.SetDiscordOutboxStore(outboxStore)

	if _, err := svc.CreateClassicGame(context.Background(), "u1", 4, "", 1); err != nil {
		t.Fatalf("create classic game: %v", err)
	}
	if outboxStore.gameStartedCalls != 0 {
		t.Fatalf("expected no game_started notification while a human slot remains open, got %d", outboxStore.gameStartedCalls)
	}
}

func TestJoinClassicGameEnqueuesGameStartedWhenLastSlotFills(t *testing.T) {
	lobby := json.RawMessage(`{"player_count":3,"player_ids":["u1","u2"]}`)
	outboxStore := &fakeDiscordOutboxStore{}
	svc := NewGamesService(&fakeDB{q: noopQuerier{}, txQ: noopQuerier{}}, &fakeGamesStore{
		createFn:  func(context.Context, db.Querier, store.NewGame) (store.Game, error) { return store.Game{}, nil },
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "lobby", State: lobby}, nil
		},
		listFn: func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(_ context.Context, _ db.Querier, in store.UpdateGameState) (store.Game, error) {
			return store.Game{ID: "g1", Status: in.Status, State: in.State}, nil
		},
	})
	svc.SetDiscordOutboxStore(outboxStore)

	if _, err := svc.JoinClassicGame(context.Background(), "g1", "u3"); err != nil {
		t.Fatalf("join game: %v", err)
	}
	if outboxStore.gameStartedCalls != 1 {
		t.Fatalf("expected exactly 1 game_started notification, got %d", outboxStore.gameStartedCalls)
	}
}

func TestJoinClassicGameDoesNotEnqueueGameStartedWhenLobbyStaysOpen(t *testing.T) {
	lobby := json.RawMessage(`{"player_count":4,"player_ids":["u1","u2"]}`)
	outboxStore := &fakeDiscordOutboxStore{}
	svc := NewGamesService(&fakeDB{q: noopQuerier{}, txQ: noopQuerier{}}, &fakeGamesStore{
		createFn:  func(context.Context, db.Querier, store.NewGame) (store.Game, error) { return store.Game{}, nil },
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "lobby", State: lobby}, nil
		},
		listFn: func(context.Context, db.Querier, store.GameListFilter) ([]store.Game, error) { return nil, nil },
		updateStateFn: func(_ context.Context, _ db.Querier, in store.UpdateGameState) (store.Game, error) {
			return store.Game{ID: "g1", Status: in.Status, State: in.State}, nil
		},
	})
	svc.SetDiscordOutboxStore(outboxStore)

	if _, err := svc.JoinClassicGame(context.Background(), "g1", "u3"); err != nil {
		t.Fatalf("join game: %v", err)
	}
	if outboxStore.gameStartedCalls != 0 {
		t.Fatalf("expected no game_started notification while a human slot remains open, got %d", outboxStore.gameStartedCalls)
	}
}

// --- DeleteGame: admin-only cleanup, authorization enforced at the HTTP layer. ---

func TestDeleteGameSucceeds(t *testing.T) {
	deleteCalled := false
	svc := NewGamesService(&fakeDB{q: noopQuerier{}}, &fakeGamesStore{
		deleteFn: func(_ context.Context, _ db.Querier, gameID string) error {
			deleteCalled = true
			if gameID != "g1" {
				t.Fatalf("expected gameID=g1, got %s", gameID)
			}
			return nil
		},
	})

	if err := svc.DeleteGame(context.Background(), "g1"); err != nil {
		t.Fatalf("delete game: %v", err)
	}
	if !deleteCalled {
		t.Fatal("expected the store's Delete to be called")
	}
}

func TestDeleteGameRejectsEmptyID(t *testing.T) {
	svc := NewGamesService(&fakeDB{q: noopQuerier{}}, &fakeGamesStore{
		deleteFn: func(context.Context, db.Querier, string) error {
			t.Fatal("delete should not be called for an empty game ID")
			return nil
		},
	})

	if err := svc.DeleteGame(context.Background(), ""); err != ErrInvalidGameInput {
		t.Fatalf("expected ErrInvalidGameInput, got %v", err)
	}
}

func TestDeleteGameNotFound(t *testing.T) {
	svc := NewGamesService(&fakeDB{q: noopQuerier{}}, &fakeGamesStore{
		deleteFn: func(context.Context, db.Querier, string) error {
			return pgx.ErrNoRows
		},
	})

	if err := svc.DeleteGame(context.Background(), "missing"); !errors.Is(err, ErrGameNotFound) {
		t.Fatalf("expected ErrGameNotFound, got %v", err)
	}
}

// --- Discord notifications only publish when a human is one of the
// specific players a notification names — a pure bot-vs-bot handoff, trade,
// or elimination is noise nobody watching Discord cares about. ---

// allBotGameState builds a 3-player game, in attack phase, with every
// player marked bot-controlled — so any end_turn/trade/elimination
// necessarily involves only bots, regardless of shuffle order.
func allBotGameState(t *testing.T) (json.RawMessage, string) {
	t.Helper()
	g, err := risk.NewClassicAutoStartGame([]string{"uid-p1", "uid-p2", "uid-p3"}, nil)
	if err != nil {
		t.Fatalf("new game: %v", err)
	}
	for i := range g.Players {
		g.Players[i].Controller = risk.ControllerBot
		g.Players[i].Strategy = "basic-v1"
	}
	g.Phase = risk.PhaseAttack
	actorID := g.Players[g.CurrentPlayer].ID
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw, actorID
}

func TestEndTurnDoesNotEnqueueWhenBothPlayersAreBots(t *testing.T) {
	gameState, actorID := allBotGameState(t)
	outboxStore := &fakeDiscordOutboxStore{}
	svc := NewGamesService(&fakeDB{q: noopQuerier{}, txQ: noopQuerier{}}, &fakeGamesStore{
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: gameState}, nil
		},
		updateStateFn: func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: gameState}, nil
		},
	})
	svc.SetDiscordOutboxStore(outboxStore)

	if _, err := svc.ApplyGameAction(context.Background(), GameActionInput{
		GameID: "g1", PlayerUserID: actorID, Action: "end_turn",
	}); err != nil {
		t.Fatalf("ApplyGameAction end_turn: %v", err)
	}
	if outboxStore.calls != 0 {
		t.Fatalf("expected no turn_started notification for an all-bot handoff, got %d", outboxStore.calls)
	}
}

func TestTradeCardsDoesNotEnqueueForBotActor(t *testing.T) {
	gameState, actorID := allBotGameState(t)
	var g risk.Game
	if err := json.Unmarshal(gameState, &g); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Give the current (bot) player a valid set to trade.
	pi := g.CurrentPlayer
	g.Players[pi].Cards = []risk.Card{
		{Territory: "Alaska", Symbol: risk.Infantry},
		{Territory: "Peru", Symbol: risk.Cavalry},
		{Territory: "Egypt", Symbol: risk.Artillery},
	}
	g.Phase = risk.PhaseReinforce
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	outboxStore := &fakeDiscordOutboxStore{}
	svc := NewGamesService(&fakeDB{q: noopQuerier{}, txQ: noopQuerier{}}, &fakeGamesStore{
		getByIDFn: func(context.Context, db.Querier, string) (store.Game, error) { return store.Game{}, nil },
		getByIDForUpdate: func(context.Context, db.Querier, string) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: raw}, nil
		},
		updateStateFn: func(context.Context, db.Querier, store.UpdateGameState) (store.Game, error) {
			return store.Game{ID: "g1", Status: "in_progress", State: raw}, nil
		},
	})
	svc.SetDiscordOutboxStore(outboxStore)

	if _, err := svc.ApplyGameAction(context.Background(), GameActionInput{
		GameID: "g1", PlayerUserID: actorID, Action: "trade_cards", CardIndices: [3]int{0, 1, 2},
	}); err != nil {
		t.Fatalf("ApplyGameAction trade_cards: %v", err)
	}
	if outboxStore.calls != 0 {
		t.Fatalf("expected no cards_trade notification for a bot actor, got %d", outboxStore.calls)
	}
}

// anyHuman itself is exercised directly here rather than through a full
// attack-to-elimination flow: forcing a deterministic combat outcome
// through ApplyGameAction would require controlling the engine's internal
// RNG, which isn't exposed once a game round-trips through JSON (the same
// gate is used identically for player_eliminated, game_over, turn_started,
// and cards_trade — see anyHuman's call sites in game.go).
func TestAnyHuman(t *testing.T) {
	players := []risk.PlayerState{
		{ID: "human-1"},
		{ID: "bot-1", Controller: risk.ControllerBot},
		{ID: "bot-2", Controller: risk.ControllerBot},
	}
	if anyHuman(players, "bot-1", "bot-2") {
		t.Fatal("expected false when every named player is a bot")
	}
	if !anyHuman(players, "human-1", "bot-1") {
		t.Fatal("expected true when at least one named player is human")
	}
	if !anyHuman(players, "human-1") {
		t.Fatal("expected true for a single human ID")
	}
	if anyHuman(players) {
		t.Fatal("expected false when no IDs are given")
	}
	if anyHuman(players, "", "bot-1") {
		t.Fatal("expected empty-string IDs to be ignored, not matched")
	}
}
