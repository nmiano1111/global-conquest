// Package service implements the transactional business-logic layer that
// sits between the httpapi/wsapi request handlers and the store/risk-engine
// layers. Services in this package own request validation, orchestrate
// store reads/writes (wrapping mutations in WithTxQ transactions with
// row-level locking where needed), and translate between the persisted
// JSONB game state and the DTOs the handler layers expect — they never
// duplicate risk-engine rules, and the risk engine remains the sole
// authority for game legality.
package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nmiano1111/global-conquest/backend/internal/bot"
	"github.com/nmiano1111/global-conquest/backend/internal/db"
	"github.com/nmiano1111/global-conquest/backend/internal/gamename"
	"github.com/nmiano1111/global-conquest/backend/internal/risk"
	"github.com/nmiano1111/global-conquest/backend/internal/store"
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

// GamesService is the transactional business-logic layer for game
// lifecycle operations — lobby creation and joining, applying in-game
// actions, listing/bootstrapping games, and the leaderboard — sitting
// between the httpapi/wsapi handlers and the games/game-players stores. It
// wraps every mutation that reads then writes the persisted risk.Game
// state in a WithTxQ transaction using GetByIDForUpdate to avoid races
// between concurrent actions on the same game.
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
	// ErrGameNotFound is returned when a lookup or update targets a game ID
	// that does not exist.
	ErrGameNotFound = errors.New("game not found")
	// ErrInvalidGameInput is returned when caller-supplied input fails
	// validation, such as an empty ID, an out-of-range player count, or a
	// game whose persisted state or status cannot be decoded.
	ErrInvalidGameInput = errors.New("invalid game input")
	// ErrUnknownPlayerIDs is returned when a referenced player ID (e.g. a
	// game's owner) does not correspond to a row in the users table.
	ErrUnknownPlayerIDs = errors.New("one or more player_ids do not exist")
	// ErrGameNotJoinable is returned when an action targets a game that is
	// not in the expected status for that action, such as joining a game
	// that has already started or acting on a game that is not in_progress.
	ErrGameNotJoinable = errors.New("game is not joinable")
	// ErrGameAlreadyJoined is returned when a player attempts to join a
	// lobby they are already a member of. It is currently unused by
	// JoinClassicGame, which instead treats a repeat join as a no-op.
	ErrGameAlreadyJoined = errors.New("player already joined this game")
	// ErrGamePlayerCountFull is returned when a player tries to join a
	// lobby that has already reached its configured player count.
	ErrGamePlayerCountFull = errors.New("game is already full")
	// ErrGameForbidden is returned when the acting player is not one of the
	// players in the targeted game.
	ErrGameForbidden = errors.New("game access forbidden")
	// ErrInvalidGameAction is returned when a requested game action is
	// unrecognized, is missing required fields, or is rejected by the risk
	// engine as illegal for the current game state.
	ErrInvalidGameAction = errors.New("invalid game action")
)

// NewGamesService constructs a GamesService backed by the given database
// and games store. The optional stores (game events, game players, domain
// events, Discord outbox) and hooks are wired in separately via the
// SetGameEventStore/SetGamePlayersStore/SetGameDomainEventStore/
// SetDiscordOutboxStore/SetBotNameAssigner/SetGameStartedHook setters, and
// every one of them is nil-safe when unset.
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

// SetGameEventStore wires in the store used to persist and list
// human-readable game event log entries (e.g. "X placed 3 armies on Y").
// It is nil-safe: until set, methods that would otherwise record an event
// simply skip that step.
func (s *GamesService) SetGameEventStore(gameEvent gameEventStore) {
	s.gameEvent = gameEvent
}

// SetGamePlayersStore wires in the store used to record game_players rows
// (for the leaderboard) and set a game's winner. It is nil-safe: until set,
// GetLeaderboard returns an empty result and player-row bookkeeping is
// skipped.
func (s *GamesService) SetGamePlayersStore(gp gamePlayersStore) {
	s.gamePlayers = gp
}

// SetGameDomainEventStore wires in the store used to persist typed
// risk.DomainEvent rows emitted by engine actions (currently only Attack).
// It is nil-safe: until set, domain events are computed by the engine but
// never persisted.
func (s *GamesService) SetGameDomainEventStore(ds gameDomainEventStore) {
	s.gameDomainEvents = ds
}

// SetDiscordOutboxStore wires in the store used to enqueue Discord
// notifications (turn started, cards traded, player eliminated, game
// over, game started). It is nil-safe: until set, no Discord notifications
// are enqueued.
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

// GameBootstrapPlayer is one player's public-facing snapshot within a
// GameBootstrap response, covering both lobby players (only UserID/
// UserName/Color/IsBot are meaningful) and in-progress/completed players.
type GameBootstrapPlayer struct {
	// UserID is the player's user ID, or a synthetic bot player ID for
	// bot-controlled players.
	UserID string `json:"user_id"`
	// UserName is the player's display name: their username for humans, or
	// their assigned wrestler name for bots.
	UserName string `json:"user_name"`
	// Color is the player's assigned UI color, derived deterministically
	// from their seat index via bootstrapColor.
	Color string `json:"color"`
	// CardCount is the number of Risk cards the player currently holds.
	CardCount int `json:"card_count"`
	// Cards holds the requesting player's own card hand; it is only
	// populated for the requester (never for other players, to avoid
	// leaking hidden information) and omitted otherwise.
	Cards []risk.Card `json:"cards,omitempty"`
	// SetupArmies is the number of reinforcement armies this player still
	// has to place during the setup phase.
	SetupArmies int `json:"setup_armies"`
	// Eliminated reports whether this player has been eliminated from the
	// game.
	Eliminated bool `json:"eliminated"`
	// IsBot reports whether this player is bot-controlled.
	IsBot bool `json:"is_bot"`
}

// GameBootstrap is the full initial-load snapshot of a game returned by
// GetGameBootstrap — everything a client needs to render a game (lobby,
// in-progress, or completed) on first load, mirroring the fields sent in
// subsequent game_state_updated broadcasts.
type GameBootstrap struct {
	// ID is the game's unique ID.
	ID string `json:"id"`
	// OwnerUserID is the user ID of the player who created the game.
	OwnerUserID string `json:"owner_user_id"`
	// Name is the game's generated display name.
	Name string `json:"name"`
	// Status is the game's persistence-layer status: "lobby",
	// "in_progress", or "completed".
	Status string `json:"status"`
	// Phase is the current risk.Phase as a string, or "lobby" when Status
	// is "lobby".
	Phase string `json:"phase"`
	// Winner is the winning player's user ID, set once the game has
	// reached PhaseGameOver.
	Winner string `json:"winner,omitempty"`
	// PlayerCount is the configured number of players for this game.
	PlayerCount int `json:"player_count"`
	// CurrentPlayer is the index into Players of whose turn it is, or -1
	// while the game is still in its lobby.
	CurrentPlayer int `json:"current_player"`
	// PendingReinforcements is the number of reinforcement armies the
	// current player still has to place before acting further.
	PendingReinforcements int `json:"pending_reinforcements"`
	// SetsTraded is the running count of Risk card sets traded in so far,
	// which determines the army value of the next set traded.
	SetsTraded int `json:"sets_traded"`
	// Occupy describes the pending post-conquest occupy move, if the game
	// is currently in the occupy phase; nil otherwise.
	Occupy *GameOccupyRequirement `json:"occupy,omitempty"`
	// Players lists every player in the game, in seat order.
	Players []GameBootstrapPlayer `json:"players"`
	// Territories is the raw JSON-encoded risk.Territories map, or `{}`
	// while the game is still in its lobby.
	Territories json.RawMessage `json:"territories"`
	// Events is the game's event log (up to the most recent 250 entries),
	// oldest to newest.
	Events []GameEventEntry `json:"events"`
	// CreatedAt is when the game row was created.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is when the game row was last updated.
	UpdatedAt time.Time `json:"updated_at"`
}

// GameActionInput is the input to GamesService.ApplyGameAction, carrying
// every field any of the supported action kinds might need; unused fields
// for a given Action are ignored.
type GameActionInput struct {
	// GameID identifies the game to act on.
	GameID string
	// PlayerUserID is the acting player's user ID (or synthetic bot player
	// ID). The action is rejected with ErrGameForbidden if this ID does not
	// belong to a player in the game.
	PlayerUserID string
	// Action names the action to perform: one of "place_reinforcement",
	// "attack", "occupy", "end_attack", "fortify", "end_turn",
	// "trade_cards", or "place_initial_army".
	Action string
	// Territory is the target territory for "place_reinforcement" and
	// "place_initial_army".
	Territory string
	// From is the origin territory for "attack" and "fortify".
	From string
	// To is the destination territory for "attack" and "fortify".
	To string
	// Armies is the army count for "place_reinforcement", "occupy", and
	// "fortify".
	Armies int
	// AttackerDice is the requested number of attacker dice for "attack";
	// it is clamped to the legal range given the attacking territory's
	// army count.
	AttackerDice int
	// DefenderDice is currently unused: the defender's dice count is always
	// computed automatically from the defending territory's army count.
	DefenderDice int
	// CardIndices are the three hand indices to trade in for "trade_cards".
	CardIndices [3]int
}

// GameActionPlayer is one player's post-action projection returned as part
// of GameActionUpdate.
type GameActionPlayer struct {
	// UserID is the player's user ID or synthetic bot player ID.
	UserID string `json:"user_id"`
	// CardCount is the number of Risk cards the player currently holds.
	CardCount int `json:"card_count"`
	// SetupArmies is the number of reinforcement armies this player still
	// has to place during the setup phase.
	SetupArmies int `json:"setup_armies"`
	// Eliminated reports whether this player has been eliminated from the
	// game.
	Eliminated bool `json:"eliminated"`
}

// GameActionUpdate is the result of a successfully applied game action,
// returned by GamesService.ApplyGameAction and broadcast to clients as the
// game_state_updated payload.
type GameActionUpdate struct {
	// GameID identifies the game that was acted on.
	GameID string `json:"game_id"`
	// Action is the action kind that was applied, echoing
	// GameActionInput.Action.
	Action string `json:"action"`
	// ActorUserID is the user ID of the player who performed the action.
	ActorUserID string `json:"actor_user_id"`
	// Phase is the resulting risk.Phase as a string.
	Phase string `json:"phase"`
	// Winner is the winning player's user ID, set once the game has
	// reached PhaseGameOver.
	Winner string `json:"winner,omitempty"`
	// CurrentPlayer is the index into Players of whose turn it is next.
	CurrentPlayer int `json:"current_player"`
	// PendingReinforcements is the number of reinforcement armies the
	// current player still has to place before acting further.
	PendingReinforcements int `json:"pending_reinforcements"`
	// SetsTraded is the running count of Risk card sets traded in so far,
	// which determines the army value of the next set traded.
	SetsTraded int `json:"sets_traded"`
	// Occupy describes the pending post-conquest occupy move, if the game
	// is now in the occupy phase; nil otherwise.
	Occupy *GameOccupyRequirement `json:"occupy,omitempty"`
	// Players lists every player's post-action projection, in seat order.
	Players []GameActionPlayer `json:"players"`
	// Territories is the raw JSON-encoded risk.Territories map after the
	// action was applied.
	Territories json.RawMessage `json:"territories"`
	// Result carries an action-specific payload: a *risk.AttackResult for
	// "attack", or a map with an "armies" key for "trade_cards"; nil for
	// actions with no extra result data.
	Result any `json:"result,omitempty"`
	// Event is the game event log entry recorded for this action, if any
	// event store is configured and the action produced a non-empty event
	// body.
	Event *GameEventEntry `json:"event,omitempty"`
	// ActorCards is the acting player's current card hand. It is excluded
	// from JSON serialization (`json:"-"`) since GameActionService
	// re-projects it into a wire-specific payload type before sending.
	ActorCards []risk.Card `json:"-"`

	// ActionTerritory/ActionFrom/ActionTo tell the frontend which
	// territory (or territory pair) this action touched, so it can
	// highlight them the same way a human's own click would — this is
	// the only signal for bot-driven actions, which have no click at all.
	ActionTerritory string `json:"action_territory,omitempty"`
	// ActionFrom is the origin territory this action touched, mirroring
	// ActionTerritory for actions with a from/to pair (attack, occupy,
	// fortify).
	ActionFrom string `json:"action_from,omitempty"`
	// ActionTo is the destination territory this action touched, mirroring
	// ActionTerritory for actions with a from/to pair (attack, occupy,
	// fortify).
	ActionTo string `json:"action_to,omitempty"`
}

// GameEventEntry is one human-readable entry in a game's event log (e.g.
// "Alice placed 3 armies on Alaska."), persisted via gameEventStore and
// returned in GameBootstrap.Events and GameActionUpdate.Event.
type GameEventEntry struct {
	// ID is the event's unique ID.
	ID string `json:"id"`
	// GameID identifies the game this event belongs to.
	GameID string `json:"game_id"`
	// ActorUserID is the user ID of the player who triggered the event, if
	// any.
	ActorUserID string `json:"actor_user_id,omitempty"`
	// EventType categorizes the event, e.g. "reinforcement_placed",
	// "attack_resolved", "turn_ended".
	EventType string `json:"event_type"`
	// Body is the human-readable event description.
	Body string `json:"body"`
	// CreatedAt is when the event was recorded.
	CreatedAt time.Time `json:"created_at"`
}

// GameOccupyRequirement describes the pending post-conquest occupy move a
// player must make after winning a territory in combat, projected from
// risk.OccupyState.
type GameOccupyRequirement struct {
	// From is the attacking territory the occupying armies must move from.
	From string `json:"from"`
	// To is the newly conquered territory the occupying armies must move
	// to.
	To string `json:"to"`
	// MinMove is the minimum number of armies that must be moved (equal to
	// the number of attacker dice used in the conquering roll).
	MinMove int `json:"min_move"`
	// MaxMove is the maximum number of armies that may be moved (limited by
	// the attacking territory's remaining army count).
	MaxMove int `json:"max_move"`
}

// CreateClassicGame creates a new classic-mode game lobby owned by
// ownerUserID with room for playerCount players (3-6), of which up to
// botCount may be bot-controlled (the creator always occupies one human
// slot, so botCount must be between 0 and playerCount-1). setupMode selects
// how territories are assigned once the lobby fills ("manual" for random
// territory claims via risk.NewClassicRandomTerritoryGame, anything else
// for auto-distributed armies via risk.NewClassicAutoStartGame). Bot
// players are assigned synthetic UUIDs and curated wrestler display names
// immediately and occupy lobby slots right away — they are never inserted
// into the users or game_players tables. If bots fill every non-creator
// slot, the game is started immediately instead of waiting for
// JoinClassicGame, and SetGameStartedHook's callback (if configured) is
// invoked. It returns ErrInvalidGameInput for invalid ownerUserID,
// playerCount, or botCount, and ErrUnknownPlayerIDs if ownerUserID does not
// correspond to an existing user.
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

	var ownerSandboxed bool
	if err := s.db.Queryer().QueryRow(
		ctx,
		`SELECT is_sandboxed FROM users WHERE id::text = $1`,
		ownerUserID,
	).Scan(&ownerSandboxed); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return store.Game{}, ErrUnknownPlayerIDs
		}
		return store.Game{}, err
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
		IsSandboxed: ownerSandboxed,
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
		if s.discordOutbox != nil && !g.IsSandboxed && anyHuman(startedEngine.Players, firstPlayerID) {
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

// gameVisible reports whether a viewer with the given admin/sandboxed
// status may see, join, or otherwise access ownerUserID's game, given that
// game's own (creation-time-snapshotted) sandboxed flag. An admin sees
// everything; a viewer always sees their own games; otherwise a sandboxed
// viewer and a sandboxed game are each invisible to everyone but the two
// exceptions above -- so a sandboxed player is isolated even from other
// sandboxed players' games, and a regular player never sees a sandboxed
// player's games.
func gameVisible(viewerUserID string, viewerIsAdmin, viewerIsSandboxed bool, ownerUserID string, gameIsSandboxed bool) bool {
	if viewerIsAdmin {
		return true
	}
	if viewerUserID != "" && viewerUserID == ownerUserID {
		return true
	}
	return !viewerIsSandboxed && !gameIsSandboxed
}

// lookupUserFlags fetches userID's admin/sandboxed status directly, for
// callers (namely the WebSocket hub's CanAccessGame path) that only have a
// bare user ID and not an already-authenticated store.User to read those
// flags off of. An empty userID (an anonymous WebSocket client) is treated
// as a non-admin, non-sandboxed viewer rather than an error.
func (s *GamesService) lookupUserFlags(ctx context.Context, q db.Querier, userID string) (isAdmin, isSandboxed bool, err error) {
	if userID == "" || q == nil {
		return false, false, nil
	}
	var role string
	err = q.QueryRow(ctx, `SELECT role, is_sandboxed FROM users WHERE id::text = $1`, userID).Scan(&role, &isSandboxed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, false, nil
		}
		return false, false, err
	}
	return strings.EqualFold(role, "admin"), isSandboxed, nil
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

// JoinClassicGame adds playerID to the lobby of the game identified by
// gameID, locking the game row for the duration of the transaction. If
// playerID is already in the lobby, this is a no-op that returns the
// current game unchanged. If the join fills the lobby to its configured
// player count, the classic engine is started via startEngineForFullLobby,
// the game's status transitions to "in_progress", game_players rows are
// inserted for human players, game-start/player-joined events are recorded
// (if an event store is configured), a Discord "game started" notification
// is enqueued (if a Discord outbox is configured and a human is involved),
// and SetGameStartedHook's callback (if configured) is invoked after the
// transaction commits. It returns ErrInvalidGameInput for an empty gameID
// or playerID, ErrGameNotFound if the game does not exist,
// ErrGameNotJoinable if the game is not in "lobby" status,
// ErrGamePlayerCountFull if the lobby has already reached its player count,
// and ErrGameForbidden if the joiner is blocked by the game's or their own
// sandbox status (see gameVisible) -- a sandboxed player may only join a
// game they themselves created, and no one but an admin may join a
// sandboxed player's game.
func (s *GamesService) JoinClassicGame(ctx context.Context, gameID, playerID string, joinerIsAdmin, joinerIsSandboxed bool) (store.Game, error) {
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
		if !gameVisible(playerID, joinerIsAdmin, joinerIsSandboxed, g.OwnerUserID, g.IsSandboxed) {
			return ErrGameForbidden
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
			if s.discordOutbox != nil && !g.IsSandboxed && anyHuman(startedEngine.Players, firstPlayerID) {
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

// GetLeaderboard returns up to limit leaderboard entries ordered by the
// underlying store's ranking. A limit of 0 or less defaults to 20. If no
// game-players store has been configured via SetGamePlayersStore, it
// returns an empty slice rather than an error.
func (s *GamesService) GetLeaderboard(ctx context.Context, limit int) ([]store.LeaderboardEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	if s.gamePlayers == nil {
		return []store.LeaderboardEntry{}, nil
	}
	return s.gamePlayers.GetLeaderboard(ctx, s.db.Queryer(), limit)
}

// GetGame fetches the game identified by gameID without locking the row.
// It returns ErrGameNotFound if no such game exists. This is an internal,
// unauthorized read used by callers that are not acting on behalf of a
// particular viewer (e.g. the bot runner reloading its own game's
// authoritative state, or GetGameBootstrap which does its own visibility
// check with the extra context it has). HTTP handlers serving a specific
// user's request should use GetGameForViewer instead.
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

// GetGameForViewer fetches the game identified by gameID, same as GetGame,
// but additionally returns ErrGameForbidden if viewerUserID is blocked by
// the game's or their own sandbox status (see gameVisible).
func (s *GamesService) GetGameForViewer(ctx context.Context, gameID, viewerUserID string, viewerIsAdmin, viewerIsSandboxed bool) (store.Game, error) {
	g, err := s.GetGame(ctx, gameID)
	if err != nil {
		return store.Game{}, err
	}
	if !gameVisible(viewerUserID, viewerIsAdmin, viewerIsSandboxed, g.OwnerUserID, g.IsSandboxed) {
		return store.Game{}, ErrGameForbidden
	}
	return g, nil
}

// CanAccessGame reports whether userID may access gameID under the sandbox
// visibility rule (see gameVisible), looking up userID's admin/sandboxed
// status itself since callers of this method (namely the WebSocket hub,
// via the GameAccessChecker interface it uses to gate game_chat_join)
// typically have nothing but a bare user ID to go on. It returns false,
// nil (rather than an error) if gameID does not exist, since the caller
// only needs a yes/no answer.
func (s *GamesService) CanAccessGame(ctx context.Context, gameID, userID string) (bool, error) {
	g, err := s.GetGame(ctx, gameID)
	if err != nil {
		if errors.Is(err, ErrGameNotFound) {
			return false, nil
		}
		return false, err
	}
	isAdmin, isSandboxed, err := s.lookupUserFlags(ctx, s.db.Queryer(), userID)
	if err != nil {
		return false, err
	}
	return gameVisible(userID, isAdmin, isSandboxed, g.OwnerUserID, g.IsSandboxed), nil
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
	// Phase is the current risk.Phase as a string, populated only for
	// games with status "in_progress" whose state decodes successfully.
	Phase string `json:"phase,omitempty"`
	// CurrentPlayerName is the display name of whoever's turn it currently
	// is, resolved server-side (username lookup for humans, assigned name
	// for bots), populated only for games with status "in_progress".
	CurrentPlayerName string `json:"current_player_name,omitempty"`
	// ViewerEliminated reports whether the requesting viewer (the
	// ListGames caller's own viewerUserID, not any other player) has been
	// eliminated in this game. Populated for "in_progress" and
	// "completed" games whose state decodes successfully and lists the
	// viewer as a player; always false for a "lobby" game, since
	// elimination cannot happen before the engine starts.
	ViewerEliminated bool `json:"viewer_eliminated,omitempty"`
}

// ListGames returns a page of GameSummary projections matching the given
// filters (ownerUserID and status may be empty to mean "any"), ordered and
// paginated by the underlying store using limit and offset, and further
// restricted to games visible to viewerUserID under the sandbox visibility
// rule (see gameVisible) unless viewerIsAdmin. For each in_progress game
// whose state decodes successfully, it also resolves and attaches the
// current player's phase and display name via batched username lookups,
// and for each in_progress or completed game, whether viewerUserID has
// been eliminated (GameSummary.ViewerEliminated). It returns
// ErrInvalidGameInput if limit or offset is negative.
func (s *GamesService) ListGames(ctx context.Context, ownerUserID, status string, limit, offset int, viewerUserID string, viewerIsAdmin, viewerIsSandboxed bool) ([]GameSummary, error) {
	if limit < 0 || offset < 0 {
		return nil, ErrInvalidGameInput
	}
	games, err := s.games.List(ctx, s.db.Queryer(), store.GameListFilter{
		OwnerUserID:       ownerUserID,
		Status:            status,
		Limit:             limit,
		Offset:            offset,
		ViewerUserID:      viewerUserID,
		ViewerIsAdmin:     viewerIsAdmin,
		ViewerIsSandboxed: viewerIsSandboxed,
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
		if g.Status != "in_progress" && g.Status != "completed" {
			continue
		}
		var engine risk.Game
		if err := json.Unmarshal(g.State, &engine); err != nil {
			continue
		}
		if viewerUserID != "" {
			for _, p := range engine.Players {
				if p.ID == viewerUserID {
					out[i].ViewerEliminated = p.Eliminated
					break
				}
			}
		}
		if g.Status != "in_progress" {
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

// UpdateGameState overwrites the given game's status and persisted JSONB
// state directly, without going through a risk-engine action. It returns
// ErrInvalidGameInput if gameID, status, or state is empty, and
// ErrGameNotFound if the game does not exist.
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

// ApplyGameAction is the authoritative entry point for every in-game move.
// It locks the game row for update inside a transaction, decodes the
// persisted risk.Game state, verifies in.PlayerUserID is a player in the
// game, dispatches on in.Action to the corresponding risk-engine method
// (place_reinforcement, attack, occupy, end_attack, fortify, end_turn,
// trade_cards, or place_initial_army), re-encodes and persists the updated
// state (transitioning the game's status to "completed" if the engine
// reached PhaseGameOver), records a human-readable game event and any
// risk.DomainEvent the action produced (if the corresponding stores are
// configured), and enqueues Discord notifications for player elimination,
// game over, cards traded, and turn started (if a Discord outbox is
// configured and a human is involved). It returns ErrInvalidGameInput if
// GameID, PlayerUserID, or Action is empty or the persisted state cannot be
// decoded, ErrGameNotFound if the game does not exist, ErrGameNotJoinable
// if the game is not in_progress, ErrGameForbidden if PlayerUserID is not a
// player in the game, ErrInvalidGameAction if Action is unrecognized or
// missing required fields, and a wrapped ErrInvalidGameAction (via
// mapGameActionErr) if the risk engine rejects the move as illegal.
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
				if s.discordOutbox != nil && !g.IsSandboxed && anyHuman(engine.Players, in.PlayerUserID, ar.Eliminated) {
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
			if engine.Phase == risk.PhaseGameOver && engine.Winner != "" && s.discordOutbox != nil && !g.IsSandboxed && anyHuman(engine.Players, engine.Winner) {
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
			if s.discordOutbox != nil && !g.IsSandboxed && nextPlayer != "" && engine.Phase != risk.PhaseGameOver && anyHuman(engine.Players, in.PlayerUserID, nextPlayer) {
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
			if s.discordOutbox != nil && !g.IsSandboxed && anyHuman(engine.Players, in.PlayerUserID) {
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

// GetGameBootstrap returns the full initial-load snapshot for gameID, as
// consumed by the frontend's GET /games/:id/bootstrap on mount. For a
// lobby-status game it projects the lobby's player list and event log. For
// an in_progress or completed game it decodes the persisted risk.Game
// state (transparently migrating any legacy game still stuck in
// PhaseSetupClaim to an auto-started game first), resolves player display
// names, includes the requester's own card hand only (never other
// players'), and includes the event log. It returns ErrInvalidGameInput if
// gameID or requesterUserID is empty, the game's status is unrecognized,
// or its persisted state cannot be decoded, propagates ErrGameNotFound from
// the underlying GetGame call, and returns ErrGameForbidden if the
// requester is blocked by the game's or their own sandbox status (see
// gameVisible).
func (s *GamesService) GetGameBootstrap(ctx context.Context, gameID, requesterUserID string, requesterIsAdmin, requesterIsSandboxed bool) (GameBootstrap, error) {
	if gameID == "" || requesterUserID == "" {
		return GameBootstrap{}, ErrInvalidGameInput
	}
	g, err := s.GetGame(ctx, gameID)
	if err != nil {
		return GameBootstrap{}, err
	}
	if !gameVisible(requesterUserID, requesterIsAdmin, requesterIsSandboxed, g.OwnerUserID, g.IsSandboxed) {
		return GameBootstrap{}, ErrGameForbidden
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
