package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"backend/internal/bot"
	"backend/internal/db"
	"backend/internal/gamename"
	"backend/internal/risk"
	"backend/internal/store"
	"github.com/jackc/pgx/v5"
)

type gameDB interface {
	Queryer() db.Querier
	WithTxQ(ctx context.Context, fn func(q db.Querier) error) error
}

type gamePlayersStore interface {
	InsertGamePlayers(ctx context.Context, q db.Querier, players []store.NewGamePlayer) error
	SetGameWinner(ctx context.Context, q db.Querier, gameID, winnerUserID string) error
	GetLeaderboard(ctx context.Context, q db.Querier, limit int) ([]store.LeaderboardEntry, error)
}

type gameDomainEventStore interface {
	InsertDomainEvent(ctx context.Context, q db.Querier, gameID string, ev risk.DomainEvent, payload []byte) (store.GameDomainEvent, error)
}

type discordOutboxStore interface {
	EnqueueTurnStarted(ctx context.Context, q db.Querier, gameID, gameName, previousPlayerDisplayName, playerID, playerDisplayName string, previousPlayerDiscordName, playerDiscordName *string, turnNumber int) error
	EnqueueCardsTrade(ctx context.Context, q db.Querier, gameID, gameName, playerID, playerDisplayName string, playerDiscordName *string, armies int) error
	EnqueuePlayerEliminated(ctx context.Context, q db.Querier, gameID, gameName, attackerID, attackerDisplayName string, attackerDiscordName *string, eliminatedPlayerID, eliminatedPlayerDisplayName string, eliminatedPlayerDiscordName *string) error
	EnqueueGameOver(ctx context.Context, q db.Querier, gameID, gameName, winnerID, winnerDisplayName string, winnerDiscordName *string) error
	EnqueueGameStarted(ctx context.Context, q db.Querier, gameID, gameName, playerID, playerDisplayName string, playerDiscordName *string) error
}

type GamesService struct {
	db               gameDB
	games            store.GamesStore
	gameEvent        gameEventStore
	gamePlayers      gamePlayersStore
	gameDomainEvents gameDomainEventStore
	discordOutbox    discordOutboxStore
	assignBotNames   func(count int, exclude []string) []string
	gameStarted      func(gameID string)
}

var (
	ErrGameNotFound        = errors.New("game not found")
	ErrInvalidGameInput    = errors.New("invalid game input")
	ErrUnknownPlayerIDs    = errors.New("one or more player_ids do not exist")
	ErrGameNotJoinable     = errors.New("game is not joinable")
	ErrGameAlreadyJoined   = errors.New("player already joined this game")
	ErrGamePlayerCountFull = errors.New("game is already full")
	ErrGameForbidden       = errors.New("game access forbidden")
	ErrInvalidGameAction   = errors.New("invalid game action")
)

func NewGamesService(db gameDB, games store.GamesStore) *GamesService {
	return &GamesService{
		db:    db,
		games: games,
		assignBotNames: func(count int, exclude []string) []string {
			return bot.AssignBotNames(nil, count, exclude)
		},
	}
}

// SetBotNameAssigner overrides how bot display names are chosen. Production
// wiring never needs to call this; it exists so tests can inject a
// deterministic fake selector instead of real randomness.
func (s *GamesService) SetBotNameAssigner(assign func(count int, exclude []string) []string) {
	s.assignBotNames = assign
}

// SetGameStartedHook registers a callback invoked whenever CreateClassicGame
// or JoinClassicGame transitions a game to in_progress. This is the only
// way a bot runner ever gets triggered for a game that starts this way:
// unlike a normal game_action, starting a game here never goes through
// game.Server, so nothing else would ever notice a bot-controlled player
// is now current. Production wiring sets this to the bot manager's
// Trigger method; it is nil-safe (never required) for tests.
func (s *GamesService) SetGameStartedHook(fn func(gameID string)) {
	s.gameStarted = fn
}

func (s *GamesService) notifyGameStarted(gameID string) {
	if s.gameStarted != nil {
		s.gameStarted(gameID)
	}
}

type gameEventStore interface {
	SaveGameEvent(ctx context.Context, q db.Querier, gameID, actorUserID, eventType, body string) (store.GameEvent, error)
	ListGameEvents(ctx context.Context, q db.Querier, gameID string, limit int) ([]store.GameEvent, error)
}

func (s *GamesService) SetGameEventStore(gameEvent gameEventStore) {
	s.gameEvent = gameEvent
}

func (s *GamesService) SetGamePlayersStore(gp gamePlayersStore) {
	s.gamePlayers = gp
}

func (s *GamesService) SetGameDomainEventStore(ds gameDomainEventStore) {
	s.gameDomainEvents = ds
}

func (s *GamesService) SetDiscordOutboxStore(store discordOutboxStore) {
	s.discordOutbox = store
}

type lobbyState struct {
	PlayerCount int      `json:"player_count"`
	PlayerIDs   []string `json:"player_ids"`
	SetupMode   string   `json:"setup_mode,omitempty"`

	// BotCount is how many of PlayerIDs are bot-controlled. BotNames maps
	// each bot's player ID to its assigned display name — bots occupy a
	// slot in PlayerIDs immediately at creation (see CreateClassicGame), so
	// existing lobby-fullness/start logic in JoinClassicGame needs no
	// changes to account for them.
	BotCount int               `json:"bot_count,omitempty"`
	BotNames map[string]string `json:"bot_names,omitempty"`
}

type GameBootstrapPlayer struct {
	UserID      string      `json:"user_id"`
	UserName    string      `json:"user_name"`
	Color       string      `json:"color"`
	CardCount   int         `json:"card_count"`
	Cards       []risk.Card `json:"cards,omitempty"`
	SetupArmies int         `json:"setup_armies"`
	Eliminated  bool        `json:"eliminated"`
	IsBot       bool        `json:"is_bot"`
}

type GameBootstrap struct {
	ID                    string                 `json:"id"`
	OwnerUserID           string                 `json:"owner_user_id"`
	Name                  string                 `json:"name"`
	Status                string                 `json:"status"`
	Phase                 string                 `json:"phase"`
	Winner                string                 `json:"winner,omitempty"`
	PlayerCount           int                    `json:"player_count"`
	CurrentPlayer         int                    `json:"current_player"`
	PendingReinforcements int                    `json:"pending_reinforcements"`
	SetsTraded            int                    `json:"sets_traded"`
	Occupy                *GameOccupyRequirement `json:"occupy,omitempty"`
	Players               []GameBootstrapPlayer  `json:"players"`
	Territories           json.RawMessage        `json:"territories"`
	Events                []GameEventEntry       `json:"events"`
	CreatedAt             time.Time              `json:"created_at"`
	UpdatedAt             time.Time              `json:"updated_at"`
}

type GameActionInput struct {
	GameID       string
	PlayerUserID string
	Action       string
	Territory    string
	From         string
	To           string
	Armies       int
	AttackerDice int
	DefenderDice int
	CardIndices  [3]int
}

type GameActionPlayer struct {
	UserID      string `json:"user_id"`
	CardCount   int    `json:"card_count"`
	SetupArmies int    `json:"setup_armies"`
	Eliminated  bool   `json:"eliminated"`
}

type GameActionUpdate struct {
	GameID                string                 `json:"game_id"`
	Action                string                 `json:"action"`
	ActorUserID           string                 `json:"actor_user_id"`
	Phase                 string                 `json:"phase"`
	Winner                string                 `json:"winner,omitempty"`
	CurrentPlayer         int                    `json:"current_player"`
	PendingReinforcements int                    `json:"pending_reinforcements"`
	SetsTraded            int                    `json:"sets_traded"`
	Occupy                *GameOccupyRequirement `json:"occupy,omitempty"`
	Players               []GameActionPlayer     `json:"players"`
	Territories           json.RawMessage        `json:"territories"`
	Result                any                    `json:"result,omitempty"`
	Event                 *GameEventEntry        `json:"event,omitempty"`
	ActorCards            []risk.Card            `json:"-"`

	// ActionTerritory/ActionFrom/ActionTo tell the frontend which
	// territory (or territory pair) this action touched, so it can
	// highlight them the same way a human's own click would — this is
	// the only signal for bot-driven actions, which have no click at all.
	ActionTerritory string `json:"action_territory,omitempty"`
	ActionFrom      string `json:"action_from,omitempty"`
	ActionTo        string `json:"action_to,omitempty"`
}

type GameEventEntry struct {
	ID          string    `json:"id"`
	GameID      string    `json:"game_id"`
	ActorUserID string    `json:"actor_user_id,omitempty"`
	EventType   string    `json:"event_type"`
	Body        string    `json:"body"`
	CreatedAt   time.Time `json:"created_at"`
}

type GameOccupyRequirement struct {
	From    string `json:"from"`
	To      string `json:"to"`
	MinMove int    `json:"min_move"`
	MaxMove int    `json:"max_move"`
}

func (s *GamesService) CreateClassicGame(ctx context.Context, ownerUserID string, playerCount int, setupMode string, botCount int) (store.Game, error) {
	if ownerUserID == "" {
		return store.Game{}, ErrInvalidGameInput
	}
	if playerCount < 3 || playerCount > 6 {
		return store.Game{}, ErrInvalidGameInput
	}
	// The creator always occupies one human slot, so at most playerCount-1
	// slots may be bots.
	if botCount < 0 || botCount > playerCount-1 {
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

	// Best-effort: exclude the creator's own username from the bot name
	// pool. A lookup failure here is not fatal to game creation — it just
	// means bot names aren't deduplicated against this one human name.
	var ownerName string
	if names, err := s.userNamesByIDsQ(ctx, s.db.Queryer(), []string{ownerUserID}); err == nil {
		ownerName = names[ownerUserID]
	}

	playerIDs := []string{ownerUserID}
	botNames := map[string]string{}
	if botCount > 0 {
		assigned := s.assignBotNames(botCount, []string{ownerName})
		for _, name := range assigned {
			id, err := newBotPlayerID()
			if err != nil {
				return store.Game{}, err
			}
			playerIDs = append(playerIDs, id)
			botNames[id] = name
		}
	}

	lobby := lobbyState{
		PlayerCount: playerCount,
		PlayerIDs:   playerIDs,
		SetupMode:   setupMode,
		BotCount:    botCount,
		BotNames:    botNames,
	}

	// If bots already fill every non-creator slot, the lobby is full the
	// instant it's created — no other human will ever call JoinClassicGame
	// to trigger the start, so it must start immediately here instead.
	status := "lobby"
	var stateJSON []byte
	var startedEngine *risk.Game
	var stateErr error
	if len(playerIDs) == playerCount {
		startedEngine, stateJSON, stateErr = s.startEngineForFullLobby(lobby)
		status = "in_progress"
	} else {
		stateJSON, stateErr = json.Marshal(lobby)
	}
	if stateErr != nil {
		return store.Game{}, stateErr
	}

	g, err := s.games.Create(ctx, s.db.Queryer(), store.NewGame{
		OwnerUserID: ownerUserID,
		Name:        gamename.Generate(),
		Status:      status,
		State:       stateJSON,
	})
	if err != nil {
		return store.Game{}, err
	}

	if startedEngine != nil && s.gamePlayers != nil {
		if err := s.gamePlayers.InsertGamePlayers(ctx, s.db.Queryer(), humanGamePlayers(g.ID, startedEngine.Players)); err != nil {
			return store.Game{}, err
		}
	}
	if startedEngine != nil {
		firstPlayerID := startedEngine.Players[startedEngine.CurrentPlayer].ID
		firstPlayerNames, err := s.userNamesByIDsQ(ctx, s.db.Queryer(), []string{firstPlayerID})
		if err != nil {
			return store.Game{}, err
		}
		overlayBotNames(firstPlayerNames, botNames)
		firstPlayerName := displayName(firstPlayerNames, firstPlayerID)
		if s.gameEvent != nil {
			startBody := fmt.Sprintf("All bot slots filled. %s goes first.", firstPlayerName)
			if _, err := s.gameEvent.SaveGameEvent(ctx, s.db.Queryer(), g.ID, ownerUserID, "game_started", startBody); err != nil {
				return store.Game{}, err
			}
		}
		if s.discordOutbox != nil && anyHuman(startedEngine.Players, firstPlayerID) {
			discordNames, err := s.discordNamesByIDsQ(ctx, s.db.Queryer(), []string{firstPlayerID})
			if err != nil {
				return store.Game{}, err
			}
			if err := s.discordOutbox.EnqueueGameStarted(ctx, s.db.Queryer(), g.ID, g.Name, firstPlayerID, firstPlayerName, discordNames[firstPlayerID]); err != nil {
				return store.Game{}, err
			}
		}
	}
	if startedEngine != nil {
		s.notifyGameStarted(g.ID)
	}

	return g, nil
}

// startEngineForFullLobby starts the classic engine for a lobby that has
// just become full — whether because bots already occupied every
// non-creator slot at creation, or because a human join just filled the
// last one — and marks the bot players in the resulting state. Both
// CreateClassicGame and JoinClassicGame call this so the two paths can
// never drift apart.
func (s *GamesService) startEngineForFullLobby(lobby lobbyState) (*risk.Game, []byte, error) {
	var startedEngine *risk.Game
	var err error
	if lobby.SetupMode == "manual" {
		startedEngine, err = risk.NewClassicRandomTerritoryGame(lobby.PlayerIDs, nil)
	} else {
		startedEngine, err = risk.NewClassicAutoStartGame(lobby.PlayerIDs, nil)
	}
	if err != nil {
		return nil, nil, err
	}
	applyBotMetadata(startedEngine, lobby.BotNames)
	nextState, err := json.Marshal(startedEngine)
	if err != nil {
		return nil, nil, err
	}
	return startedEngine, nextState, nil
}

// humanGamePlayers filters out bot players: bots have no row in `users`
// and no session, so inserting them would violate game_players.user_id's
// FK to users(id). They still exist as full players in the engine state;
// they just never appear in game_players/leaderboard bookkeeping.
func humanGamePlayers(gameID string, players []risk.PlayerState) []store.NewGamePlayer {
	out := make([]store.NewGamePlayer, 0, len(players))
	for i, p := range players {
		if p.IsBot() {
			continue
		}
		out = append(out, store.NewGamePlayer{GameID: gameID, UserID: p.ID, PlayerIndex: i})
	}
	return out
}

// anyHuman reports whether at least one of the given player IDs belongs to
// a human. Discord notifications name one or more specific players (e.g.
// "X ended their turn, Y is up"); a Discord channel only cares about a
// notification if a human is actually one of the players it names — a
// bot-vs-bot handoff or trade is pure noise for anyone watching.
func anyHuman(players []risk.PlayerState, ids ...string) bool {
	idSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id != "" {
			idSet[id] = struct{}{}
		}
	}
	for _, p := range players {
		if _, ok := idSet[p.ID]; ok && !p.IsBot() {
			return true
		}
	}
	return false
}

// newBotPlayerID mints a synthetic player ID for a bot in the same
// textual format Postgres's gen_random_uuid() produces for real users, so
// bot IDs are indistinguishable in logs/events. Bots are never inserted
// into the `users` table or the `game_players` table (no fake accounts,
// no session), so this never needs to be DB-unique against real users —
// only unique enough to not collide within one game, which 122 bits of
// randomness guarantees for any practical player count.
func newBotPlayerID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate bot player id: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func (s *GamesService) JoinClassicGame(ctx context.Context, gameID, playerID string) (store.Game, error) {
	if gameID == "" || playerID == "" {
		return store.Game{}, ErrInvalidGameInput
	}

	var out store.Game
	started := false
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
		names, err := s.userNamesByIDsQ(ctx, q, lobby.PlayerIDs)
		if err != nil {
			return err
		}
		overlayBotNames(names, lobby.BotNames)
		nextStatus := "lobby"
		var nextState []byte
		var startedEngine *risk.Game
		if len(lobby.PlayerIDs) == lobby.PlayerCount {
			startedEngine, nextState, err = s.startEngineForFullLobby(lobby)
			if err != nil {
				return err
			}
			nextStatus = "in_progress"
			if s.gameEvent != nil {
				joinBody := fmt.Sprintf(
					"%s joined the game lobby (%d/%d players).",
					displayName(names, playerID),
					len(lobby.PlayerIDs),
					lobby.PlayerCount,
				)
				if _, err := s.gameEvent.SaveGameEvent(ctx, q, g.ID, playerID, "player_joined", joinBody); err != nil {
					return err
				}
				var startBody string
				if lobby.SetupMode == "manual" {
					startBody = fmt.Sprintf("All players joined. Territories have been randomly assigned. %s places first.", displayName(names, startedEngine.Players[startedEngine.CurrentPlayer].ID))
				} else {
					startBody = fmt.Sprintf("All players joined. Armies have been randomly distributed. %s goes first.", displayName(names, startedEngine.Players[startedEngine.CurrentPlayer].ID))
				}
				if _, err := s.gameEvent.SaveGameEvent(ctx, q, g.ID, playerID, "game_started", startBody); err != nil {
					return err
				}
			}
			firstPlayerID := startedEngine.Players[startedEngine.CurrentPlayer].ID
			if s.discordOutbox != nil && anyHuman(startedEngine.Players, firstPlayerID) {
				discordNames, err := s.discordNamesByIDsQ(ctx, q, []string{firstPlayerID})
				if err != nil {
					return err
				}
				if err := s.discordOutbox.EnqueueGameStarted(ctx, q, g.ID, g.Name, firstPlayerID, displayName(names, firstPlayerID), discordNames[firstPlayerID]); err != nil {
					return err
				}
			}
		} else {
			nextState, err = json.Marshal(lobby)
			if err != nil {
				return err
			}
			if s.gameEvent != nil {
				joinBody := fmt.Sprintf(
					"%s joined the game lobby (%d/%d players).",
					displayName(names, playerID),
					len(lobby.PlayerIDs),
					lobby.PlayerCount,
				)
				if _, err := s.gameEvent.SaveGameEvent(ctx, q, g.ID, playerID, "player_joined", joinBody); err != nil {
					return err
				}
			}
		}

		out, err = s.games.UpdateState(ctx, q, store.UpdateGameState{
			GameID: g.ID,
			Status: nextStatus,
			State:  nextState,
		})
		if err != nil {
			return err
		}

		if startedEngine != nil && s.gamePlayers != nil {
			if err := s.gamePlayers.InsertGamePlayers(ctx, q, humanGamePlayers(g.ID, startedEngine.Players)); err != nil {
				return err
			}
		}
		started = startedEngine != nil

		return nil
	})
	if err == nil && started {
		s.notifyGameStarted(out.ID)
	}
	return out, err
}

func (s *GamesService) GetLeaderboard(ctx context.Context, limit int) ([]store.LeaderboardEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	if s.gamePlayers == nil {
		return []store.LeaderboardEntry{}, nil
	}
	return s.gamePlayers.GetLeaderboard(ctx, s.db.Queryer(), limit)
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

// DeleteGame permanently removes a game and everything derived from it
// (events, chat history, discord outbox rows, player records). Callers are
// responsible for authorization — this is admin-only at the HTTP layer.
func (s *GamesService) DeleteGame(ctx context.Context, gameID string) error {
	if gameID == "" {
		return ErrInvalidGameInput
	}
	if err := s.games.Delete(ctx, s.db.Queryer(), gameID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrGameNotFound
		}
		return err
	}
	return nil
}

// GameSummary is a list-view projection of a game that adds the current
// player's turn (name + phase) for in-progress games, resolved server-side
// since only the raw user ID lives in the persisted engine state.
type GameSummary struct {
	store.Game
	Phase             string `json:"phase,omitempty"`
	CurrentPlayerName string `json:"current_player_name,omitempty"`
}

func (s *GamesService) ListGames(ctx context.Context, ownerUserID, status string, limit, offset int) ([]GameSummary, error) {
	if limit < 0 || offset < 0 {
		return nil, ErrInvalidGameInput
	}
	games, err := s.games.List(ctx, s.db.Queryer(), store.GameListFilter{
		OwnerUserID: ownerUserID,
		Status:      status,
		Limit:       limit,
		Offset:      offset,
	})
	if err != nil {
		return nil, err
	}

	out := make([]GameSummary, len(games))
	turnUserIDByGame := make(map[int]string, len(games))
	userIDs := make([]string, 0, len(games))
	seenUserIDs := make(map[string]struct{}, len(games))
	botNames := make(map[string]string, len(games))
	for i, g := range games {
		out[i] = GameSummary{Game: g}
		if g.Status != "in_progress" {
			continue
		}
		var engine risk.Game
		if err := json.Unmarshal(g.State, &engine); err != nil {
			continue
		}
		if engine.CurrentPlayer < 0 || engine.CurrentPlayer >= len(engine.Players) {
			continue
		}
		out[i].Phase = string(engine.Phase)
		current := engine.Players[engine.CurrentPlayer]
		turnUserIDByGame[i] = current.ID
		if current.Name != "" {
			botNames[current.ID] = current.Name
		}
		if _, ok := seenUserIDs[current.ID]; !ok {
			seenUserIDs[current.ID] = struct{}{}
			userIDs = append(userIDs, current.ID)
		}
	}

	if len(userIDs) > 0 {
		names, err := s.userNamesByIDs(ctx, userIDs)
		if err != nil {
			return nil, err
		}
		overlayBotNames(names, botNames)
		for i, userID := range turnUserIDByGame {
			name := names[userID]
			if name == "" {
				name = userID
			}
			out[i].CurrentPlayerName = name
		}
	}

	return out, nil
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

func (s *GamesService) ApplyGameAction(ctx context.Context, in GameActionInput) (GameActionUpdate, error) {
	if in.GameID == "" || in.PlayerUserID == "" || in.Action == "" {
		return GameActionUpdate{}, ErrInvalidGameInput
	}

	var out GameActionUpdate
	err := s.db.WithTxQ(ctx, func(q db.Querier) error {
		g, err := s.games.GetByIDForUpdate(ctx, q, in.GameID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrGameNotFound
			}
			return err
		}
		if g.Status != "in_progress" {
			return ErrGameNotJoinable
		}

		var engine risk.Game
		if err := json.Unmarshal(g.State, &engine); err != nil {
			return ErrInvalidGameInput
		}
		playerIDs := make([]string, 0, len(engine.Players))
		for _, p := range engine.Players {
			playerIDs = append(playerIDs, p.ID)
		}
		if !containsID(playerIDs, in.PlayerUserID) {
			return ErrGameForbidden
		}
		names, err := s.userNamesByIDsQ(ctx, q, playerIDs)
		if err != nil {
			return err
		}
		for _, p := range engine.Players {
			if p.Name != "" {
				names[p.ID] = p.Name
			}
		}
		prevOccupy := occupyRequirement(engine.Occupy)

		var result any
		var eventType, eventBody string
		var domainEv *risk.DomainEvent
		// actionTerritory/actionFrom/actionTo tell the frontend which
		// territories this action touched, so it can highlight them the
		// same way a human's own click would — including for bot-driven
		// actions, which have no click to derive a highlight from.
		var actionTerritory, actionFrom, actionTo string
		switch in.Action {
		case "place_reinforcement":
			if in.Territory == "" || in.Armies <= 0 {
				return ErrInvalidGameAction
			}
			if err := engine.PlaceReinforcement(in.PlayerUserID, risk.Territory(in.Territory), in.Armies); err != nil {
				return err
			}
			actionTerritory = in.Territory
			eventType = "reinforcement_placed"
			eventBody = fmt.Sprintf("%s placed %d %s on %s.", displayName(names, in.PlayerUserID), in.Armies, pluralize("army", in.Armies), in.Territory)
		case "attack":
			if in.From == "" || in.To == "" || in.AttackerDice <= 0 {
				return ErrInvalidGameAction
			}
			src, ok := engine.Territories[risk.Territory(in.From)]
			if !ok {
				return ErrInvalidGameAction
			}
			dst, ok := engine.Territories[risk.Territory(in.To)]
			if !ok {
				return ErrInvalidGameAction
			}
			defenderUserID := ""
			if dst.Owner >= 0 && dst.Owner < len(engine.Players) {
				defenderUserID = engine.Players[dst.Owner].ID
			}
			maxAttackerDice := min(3, src.Armies-1)
			if maxAttackerDice < 1 {
				return ErrInvalidGameAction
			}
			attackerDice := min(max(1, in.AttackerDice), maxAttackerDice)
			defenderDice := min(2, dst.Armies)
			if defenderDice < 1 {
				return ErrInvalidGameAction
			}
			ar, ev, err := engine.Attack(
				in.PlayerUserID,
				risk.Territory(in.From),
				risk.Territory(in.To),
				attackerDice,
				defenderDice,
			)
			if err != nil {
				return err
			}
			domainEv = ev
			result = ar
			actionFrom = in.From
			actionTo = in.To
			eventType = "attack_resolved"
			eventBody = fmt.Sprintf(
				"%s attacked %s from %s. Dice: attacker [%s], defender [%s]. Losses: %s %d, %s %d.",
				displayName(names, in.PlayerUserID),
				in.To,
				in.From,
				joinDice(ar.AttackerRolls),
				joinDice(ar.DefenderRolls),
				displayName(names, in.PlayerUserID),
				ar.AttackerLoss,
				displayName(names, defenderUserID),
				ar.DefenderLoss,
			)
			if ar.Conquered {
				eventBody += fmt.Sprintf(" %s conquered %s.", displayName(names, in.PlayerUserID), in.To)
			}
			if ar.Eliminated != "" {
				eventBody += fmt.Sprintf(" %s was eliminated.", displayName(names, ar.Eliminated))
				if s.discordOutbox != nil && anyHuman(engine.Players, in.PlayerUserID, ar.Eliminated) {
					discordNames, err := s.discordNamesByIDsQ(ctx, q, []string{in.PlayerUserID, ar.Eliminated})
					if err != nil {
						return err
					}
					if err := s.discordOutbox.EnqueuePlayerEliminated(ctx, q, g.ID, g.Name,
						in.PlayerUserID, displayName(names, in.PlayerUserID), discordNames[in.PlayerUserID],
						ar.Eliminated, displayName(names, ar.Eliminated), discordNames[ar.Eliminated],
					); err != nil {
						return err
					}
				}
			}
		case "occupy":
			if in.Armies <= 0 {
				return ErrInvalidGameAction
			}
			if err := engine.OccupyTerritory(in.PlayerUserID, in.Armies); err != nil {
				return err
			}
			from := in.From
			to := in.To
			if prevOccupy != nil {
				from = prevOccupy.From
				to = prevOccupy.To
			}
			actionFrom = from
			actionTo = to
			eventType = "territory_occupied"
			eventBody = fmt.Sprintf("%s moved %d %s from %s to %s.", displayName(names, in.PlayerUserID), in.Armies, pluralize("army", in.Armies), from, to)
			// Winning the game is only detected here: checkWinner() runs inside
			// OccupyTerritory (and EndTurn, defensively), never inside Attack —
			// a conquering attack always transitions to PhaseOccupy first.
			if engine.Phase == risk.PhaseGameOver && engine.Winner != "" && s.discordOutbox != nil && anyHuman(engine.Players, engine.Winner) {
				discordNames, err := s.discordNamesByIDsQ(ctx, q, []string{engine.Winner})
				if err != nil {
					return err
				}
				if err := s.discordOutbox.EnqueueGameOver(ctx, q, g.ID, g.Name,
					engine.Winner, displayName(names, engine.Winner), discordNames[engine.Winner],
				); err != nil {
					return err
				}
			}
		case "end_attack":
			if err := engine.EndAttackPhase(in.PlayerUserID); err != nil {
				return err
			}
			eventType = "attack_phase_ended"
			eventBody = fmt.Sprintf("%s ended the attack phase.", displayName(names, in.PlayerUserID))
		case "fortify":
			if in.From == "" || in.To == "" || in.Armies <= 0 {
				return ErrInvalidGameAction
			}
			if err := engine.Fortify(in.PlayerUserID, risk.Territory(in.From), risk.Territory(in.To), in.Armies); err != nil {
				return err
			}
			actionFrom = in.From
			actionTo = in.To
			eventType = "fortified"
			eventBody = fmt.Sprintf("%s fortified %s from %s with %d %s.", displayName(names, in.PlayerUserID), in.To, in.From, in.Armies, pluralize("army", in.Armies))
		case "end_turn":
			if err := engine.EndTurn(in.PlayerUserID); err != nil {
				return err
			}
			nextPlayer := ""
			if engine.CurrentPlayer >= 0 && engine.CurrentPlayer < len(engine.Players) {
				nextPlayer = engine.Players[engine.CurrentPlayer].ID
			}
			eventType = "turn_ended"
			eventBody = fmt.Sprintf("%s ended their turn. %s is up next.", displayName(names, in.PlayerUserID), displayName(names, nextPlayer))
			if s.discordOutbox != nil && nextPlayer != "" && engine.Phase != risk.PhaseGameOver && anyHuman(engine.Players, in.PlayerUserID, nextPlayer) {
				discordNames, err := s.discordNamesByIDsQ(ctx, q, []string{in.PlayerUserID, nextPlayer})
				if err != nil {
					return err
				}
				prevDiscord := discordNames[in.PlayerUserID]
				nextDiscord := discordNames[nextPlayer]
				if err := s.discordOutbox.EnqueueTurnStarted(ctx, q, g.ID, g.Name, displayName(names, in.PlayerUserID), nextPlayer, displayName(names, nextPlayer), prevDiscord, nextDiscord, engine.TurnNumber); err != nil {
					return err
				}
			}
		case "trade_cards":
			armies, err := engine.TradeCards(in.PlayerUserID, in.CardIndices)
			if err != nil {
				return err
			}
			result = map[string]int{"armies": armies}
			eventType = "cards_traded"
			eventBody = fmt.Sprintf("%s traded cards for %d armies.", displayName(names, in.PlayerUserID), armies)
			if s.discordOutbox != nil && anyHuman(engine.Players, in.PlayerUserID) {
				discordNames, err := s.discordNamesByIDsQ(ctx, q, []string{in.PlayerUserID})
				if err != nil {
					return err
				}
				if err := s.discordOutbox.EnqueueCardsTrade(ctx, q, g.ID, g.Name, in.PlayerUserID, displayName(names, in.PlayerUserID), discordNames[in.PlayerUserID], armies); err != nil {
					return err
				}
			}
		case "place_initial_army":
			if in.Territory == "" {
				return ErrInvalidGameAction
			}
			if err := engine.PlaceInitialArmy(in.PlayerUserID, risk.Territory(in.Territory)); err != nil {
				return err
			}
			actionTerritory = in.Territory
			eventType = "initial_army_placed"
			eventBody = fmt.Sprintf("%s placed an army on %s.", displayName(names, in.PlayerUserID), in.Territory)
		default:
			return ErrInvalidGameAction
		}

		nextState, err := json.Marshal(engine)
		if err != nil {
			return err
		}
		gameStatus := "in_progress"
		if engine.Phase == risk.PhaseGameOver {
			gameStatus = "completed"
		}
		if _, err := s.games.UpdateState(ctx, q, store.UpdateGameState{
			GameID: g.ID,
			Status: gameStatus,
			State:  nextState,
		}); err != nil {
			return err
		}
		if engine.Phase == risk.PhaseGameOver && engine.Winner != "" && s.gamePlayers != nil {
			if err := s.gamePlayers.SetGameWinner(ctx, q, g.ID, engine.Winner); err != nil {
				return err
			}
		}
		if domainEv != nil && s.gameDomainEvents != nil {
			evPayload, err := json.Marshal(domainEv.Payload)
			if err != nil {
				return err
			}
			if _, err := s.gameDomainEvents.InsertDomainEvent(ctx, q, g.ID, *domainEv, evPayload); err != nil {
				return err
			}
		}

		territories, err := json.Marshal(engine.Territories)
		if err != nil {
			return err
		}
		players := make([]GameActionPlayer, 0, len(engine.Players))
		for i, p := range engine.Players {
			players = append(players, GameActionPlayer{
				UserID:      p.ID,
				CardCount:   len(p.Cards),
				SetupArmies: engine.SetupReserves[i],
				Eliminated:  p.Eliminated,
			})
		}
		var actorCards []risk.Card
		for _, p := range engine.Players {
			if p.ID == in.PlayerUserID {
				actorCards = p.Cards
				break
			}
		}
		out = GameActionUpdate{
			GameID:                g.ID,
			Action:                in.Action,
			ActorUserID:           in.PlayerUserID,
			Phase:                 string(engine.Phase),
			Winner:                engine.Winner,
			CurrentPlayer:         engine.CurrentPlayer,
			PendingReinforcements: engine.PendingReinforcements,
			SetsTraded:            engine.SetsTraded,
			Occupy:                occupyRequirement(engine.Occupy),
			Players:               players,
			Territories:           territories,
			Result:                result,
			ActorCards:            actorCards,
			ActionTerritory:       actionTerritory,
			ActionFrom:            actionFrom,
			ActionTo:              actionTo,
		}
		if s.gameEvent != nil && strings.TrimSpace(eventBody) != "" {
			saved, err := s.gameEvent.SaveGameEvent(ctx, q, g.ID, in.PlayerUserID, eventType, eventBody)
			if err != nil {
				return err
			}
			out.Event = &GameEventEntry{
				ID:          saved.ID,
				GameID:      saved.GameID,
				ActorUserID: saved.ActorUserID,
				EventType:   saved.EventType,
				Body:        saved.Body,
				CreatedAt:   saved.CreatedAt,
			}
		}
		return nil
	})
	if err != nil {
		return GameActionUpdate{}, mapGameActionErr(err)
	}
	return out, nil
}

func (s *GamesService) GetGameBootstrap(ctx context.Context, gameID, requesterUserID string) (GameBootstrap, error) {
	if gameID == "" || requesterUserID == "" {
		return GameBootstrap{}, ErrInvalidGameInput
	}
	g, err := s.GetGame(ctx, gameID)
	if err != nil {
		return GameBootstrap{}, err
	}

	out := GameBootstrap{
		ID:          g.ID,
		OwnerUserID: g.OwnerUserID,
		Name:        g.Name,
		Status:      g.Status,
		CreatedAt:   g.CreatedAt,
		UpdatedAt:   g.UpdatedAt,
	}

	switch g.Status {
	case "lobby":
		lobby, err := decodeLobbyState(g.State)
		if err != nil {
			return GameBootstrap{}, err
		}
		names, err := s.userNamesByIDs(ctx, lobby.PlayerIDs)
		if err != nil {
			return GameBootstrap{}, err
		}
		overlayBotNames(names, lobby.BotNames)
		out.Phase = "lobby"
		out.PlayerCount = lobby.PlayerCount
		out.CurrentPlayer = -1
		out.PendingReinforcements = 0
		out.Occupy = nil
		out.Players = make([]GameBootstrapPlayer, 0, len(lobby.PlayerIDs))
		for _, id := range lobby.PlayerIDs {
			name := names[id]
			if name == "" {
				name = id
			}
			_, isBot := lobby.BotNames[id]
			out.Players = append(out.Players, GameBootstrapPlayer{
				UserID:     id,
				UserName:   name,
				Color:      bootstrapColor(len(out.Players)),
				CardCount:  0,
				Eliminated: false,
				IsBot:      isBot,
			})
		}
		out.Territories = json.RawMessage(`{}`)
		if s.gameEvent != nil {
			events, err := s.gameEvent.ListGameEvents(ctx, s.db.Queryer(), g.ID, 250)
			if err != nil {
				return GameBootstrap{}, err
			}
			out.Events = make([]GameEventEntry, 0, len(events))
			for _, ev := range events {
				out.Events = append(out.Events, GameEventEntry{
					ID:          ev.ID,
					GameID:      ev.GameID,
					ActorUserID: ev.ActorUserID,
					EventType:   ev.EventType,
					Body:        ev.Body,
					CreatedAt:   ev.CreatedAt,
				})
			}
		}
		return out, nil

	case "in_progress", "completed":
		var engine risk.Game
		if err := json.Unmarshal(g.State, &engine); err != nil {
			return GameBootstrap{}, ErrInvalidGameInput
		}
		if isLegacyUninitializedSetup(engine) {
			ids := make([]string, 0, len(engine.Players))
			for _, p := range engine.Players {
				ids = append(ids, p.ID)
			}
			auto, err := risk.NewClassicAutoStartGame(ids, nil)
			if err != nil {
				return GameBootstrap{}, err
			}
			nextState, err := json.Marshal(auto)
			if err != nil {
				return GameBootstrap{}, err
			}
			updated, err := s.games.UpdateState(ctx, s.db.Queryer(), store.UpdateGameState{
				GameID: g.ID,
				Status: "in_progress",
				State:  nextState,
			})
			if err != nil {
				return GameBootstrap{}, err
			}
			g = updated
			engine = *auto
		}
		ids := make([]string, 0, len(engine.Players))
		for _, p := range engine.Players {
			ids = append(ids, p.ID)
		}
		names, err := s.userNamesByIDs(ctx, ids)
		if err != nil {
			return GameBootstrap{}, err
		}
		out.Phase = string(engine.Phase)
		out.Winner = engine.Winner
		out.PlayerCount = len(engine.Players)
		out.CurrentPlayer = engine.CurrentPlayer
		out.PendingReinforcements = engine.PendingReinforcements
		out.SetsTraded = engine.SetsTraded
		out.Occupy = occupyRequirement(engine.Occupy)
		out.Players = make([]GameBootstrapPlayer, 0, len(engine.Players))
		for i, p := range engine.Players {
			name := p.Name
			if name == "" {
				name = names[p.ID]
			}
			if name == "" {
				name = p.ID
			}
			var cards []risk.Card
			if p.ID == requesterUserID {
				cards = p.Cards
			}
			out.Players = append(out.Players, GameBootstrapPlayer{
				UserID:      p.ID,
				UserName:    name,
				Color:       bootstrapColor(i),
				CardCount:   len(p.Cards),
				Cards:       cards,
				SetupArmies: engine.SetupReserves[i],
				Eliminated:  p.Eliminated,
				IsBot:       p.IsBot(),
			})
		}
		tb, err := json.Marshal(engine.Territories)
		if err != nil {
			return GameBootstrap{}, err
		}
		out.Territories = tb
		if s.gameEvent != nil {
			events, err := s.gameEvent.ListGameEvents(ctx, s.db.Queryer(), g.ID, 250)
			if err != nil {
				return GameBootstrap{}, err
			}
			out.Events = make([]GameEventEntry, 0, len(events))
			for _, ev := range events {
				out.Events = append(out.Events, GameEventEntry{
					ID:          ev.ID,
					GameID:      ev.GameID,
					ActorUserID: ev.ActorUserID,
					EventType:   ev.EventType,
					Body:        ev.Body,
					CreatedAt:   ev.CreatedAt,
				})
			}
		}
		return out, nil

	default:
		return GameBootstrap{}, ErrInvalidGameInput
	}
}

func (s *GamesService) userNamesByIDs(ctx context.Context, ids []string) (map[string]string, error) {
	return s.userNamesByIDsQ(ctx, s.db.Queryer(), ids)
}

func (s *GamesService) userNamesByIDsQ(ctx context.Context, q db.Querier, ids []string) (map[string]string, error) {
	if len(ids) == 0 {
		return map[string]string{}, nil
	}
	if q == nil {
		out := make(map[string]string, len(ids))
		for _, id := range ids {
			out[id] = id
		}
		return out, nil
	}
	rows, err := q.Query(
		ctx,
		`SELECT id::text, username FROM users WHERE id::text = ANY($1::text[])`,
		ids,
	)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		out := make(map[string]string, len(ids))
		for _, id := range ids {
			out[id] = id
		}
		return out, nil
	}
	defer rows.Close()

	out := make(map[string]string, len(ids))
	for rows.Next() {
		var id, username string
		if err := rows.Scan(&id, &username); err != nil {
			return nil, err
		}
		out[id] = username
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// discordNamesByIDsQ returns a map of userID → *discord_name for the given IDs.
// The value is nil when the user has no discord_name set.
func (s *GamesService) discordNamesByIDsQ(ctx context.Context, q db.Querier, ids []string) (map[string]*string, error) {
	out := make(map[string]*string, len(ids))
	if len(ids) == 0 || q == nil {
		return out, nil
	}
	rows, err := q.Query(
		ctx,
		`SELECT id::text, discord_name FROM users WHERE id::text = ANY($1::text[])`,
		ids,
	)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		return out, nil
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var name *string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}

// overlayBotNames sets each bot's assigned display name into names,
// overriding whatever (empty) result the users-table lookup produced for
// that ID. Bots never have a users row, so without this every bot would
// display as its raw player ID.
func overlayBotNames(names map[string]string, botNames map[string]string) {
	for id, name := range botNames {
		if name != "" {
			names[id] = name
		}
	}
}

// applyBotMetadata marks each player in g whose ID is a key in botNames as
// bot-controlled. NewClassicAutoStartGame/NewClassicRandomTerritoryGame
// build plain PlayerState{ID: id} entries with no notion of which IDs are
// bots, so this is applied once, right after the engine starts.
//
// New bots default to scored-v1 (the candidate-scoring strategy): only its
// attack phase is migrated onto real scoring so far, reinforce/occupy/
// fortify still fall back to basic-v1's logic under the hood — see
// bot.ScoredStrategy — but attack is the highest-leverage phase and the
// fallback keeps every game fully legal in the meantime.
func applyBotMetadata(g *risk.Game, botNames map[string]string) {
	for i := range g.Players {
		if name, ok := botNames[g.Players[i].ID]; ok {
			g.Players[i].Controller = risk.ControllerBot
			g.Players[i].Strategy = bot.StrategyScoredV1
			g.Players[i].Name = name
		}
	}
}

func containsID(ids []string, target string) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func bootstrapColor(idx int) string {
	palette := []string{"#ef4444", "#3b82f6", "#22c55e", "#f59e0b", "#a855f7", "#06b6d4"}
	if idx < 0 {
		return palette[0]
	}
	return palette[idx%len(palette)]
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

func isLegacyUninitializedSetup(g risk.Game) bool {
	if g.Phase != risk.PhaseSetupClaim {
		return false
	}
	if len(g.Players) < 3 || len(g.Players) > 6 {
		return false
	}
	return true
}

func mapGameActionErr(err error) error {
	switch {
	case errors.Is(err, risk.ErrOutOfTurn),
		errors.Is(err, risk.ErrInvalidMove),
		errors.Is(err, risk.ErrInvalidPhase):
		return fmt.Errorf("%w: %v", ErrInvalidGameAction, err)
	default:
		return err
	}
}

func occupyRequirement(o *risk.OccupyState) *GameOccupyRequirement {
	if o == nil {
		return nil
	}
	return &GameOccupyRequirement{
		From:    string(o.From),
		To:      string(o.To),
		MinMove: o.MinMove,
		MaxMove: o.MaxMove,
	}
}

func displayName(names map[string]string, userID string) string {
	if userID == "" {
		return "Unknown player"
	}
	if name := strings.TrimSpace(names[userID]); name != "" {
		return name
	}
	return userID
}

func pluralize(noun string, n int) string {
	if n == 1 {
		return noun
	}
	if strings.HasSuffix(noun, "y") && len(noun) > 1 {
		return noun[:len(noun)-1] + "ies"
	}
	return noun + "s"
}

func joinDice(values []int) string {
	if len(values) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, fmt.Sprintf("%d", v))
	}
	return strings.Join(parts, ", ")
}
