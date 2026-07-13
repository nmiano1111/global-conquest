// Package wsmsg defines the WebSocket message envelope and the typed
// command/broadcast payloads exchanged between the Go backend and the React
// frontend. Every message on the wire is an Envelope carrying a Type and a
// JSON payload; the concrete payload types in this package describe what
// each Type's payload contains, whether it is a client->server command or a
// server->client broadcast, and are shared conceptually with the
// TypeScript definitions in realtime/types.ts on the frontend.
package wsmsg

import "encoding/json"

// Type identifies the kind of message carried by an Envelope, determining
// how its Payload is decoded and handled.
type Type string

const (
	// client->server

	// TypeHello is a legacy client->server greeting message type; it is not
	// currently handled by the hub's message switch.
	TypeHello Type = "hello"
	// TypeCreateGame is sent by a client to create a new lobby game.
	TypeCreateGame Type = "create_game"
	// TypeJoinGame is sent by a client to join an existing lobby game.
	TypeJoinGame Type = "join_game"
	// TypeLeaveGame is sent by a client to leave its current game.
	TypeLeaveGame Type = "leave_game"
	// TypeListGames is sent by a client to request the current list of
	// joinable lobby games.
	TypeListGames Type = "list_games"
	// TypeLobbyTypingStart is sent by a client when it starts typing in the
	// lobby chat.
	TypeLobbyTypingStart Type = "lobby_typing_start"
	// TypeLobbyTypingStop is sent by a client when it stops typing in the
	// lobby chat.
	TypeLobbyTypingStop Type = "lobby_typing_stop"
	// TypeGameChatJoin is sent by a client to join a game's chat room; it
	// must be sent before the hub will deliver game state updates or accept
	// game actions for that game.
	TypeGameChatJoin Type = "game_chat_join"
	// TypeGameChatLeave is sent by a client to leave a game's chat room.
	TypeGameChatLeave Type = "game_chat_leave"
	// TypeGameChatSend is sent by a client to post a chat message into a
	// game's chat room.
	TypeGameChatSend Type = "game_chat_send"
	// TypeGameAction is sent by a client to submit a gameplay action (e.g.
	// reinforce, attack, occupy, fortify, trade cards) against the engine.
	TypeGameAction Type = "game_action"
	// TypeTerritorySelect is sent by a client whenever its local territory
	// selection changes, purely as a live-cursor signal relayed to other
	// clients in the same game; it never touches persisted game state.
	TypeTerritorySelect Type = "territory_select"

	// server->client

	// TypeError is a legacy server->client error message type; error
	// replies are currently sent as untyped envelopes built by errEnv
	// rather than tagged with this constant.
	TypeError Type = "error"
	// TypeYourCards is sent privately to the acting player after a game
	// action, carrying that player's current card hand.
	TypeYourCards Type = "your_cards"
	// TypeGameCreated is sent to the creator after a lobby game is
	// successfully created.
	TypeGameCreated Type = "game_created"
	// TypeJoinedGame is sent to a client after it successfully joins a
	// lobby game, carrying a snapshot of the game.
	TypeJoinedGame Type = "joined_game"
	// TypeLeftGame is sent to a client after it leaves its current game.
	TypeLeftGame Type = "left_game"
	// TypeGameList is sent in reply to TypeListGames, carrying the current
	// list of joinable lobby games.
	TypeGameList Type = "game_list"
	// TypePlayerJoined is broadcast to a lobby game's other clients when a
	// new player joins.
	TypePlayerJoined Type = "player_joined"
	// TypePlayerLeft is broadcast to a lobby game's other clients when a
	// player leaves.
	TypePlayerLeft Type = "player_left"
	// TypeLobbyTypingState is broadcast to lobby clients to reflect who is
	// currently typing in the lobby chat.
	TypeLobbyTypingState Type = "lobby_typing_state"
	// TypeLobbyChatMessage is broadcast to lobby clients carrying a new
	// lobby chat message.
	TypeLobbyChatMessage Type = "lobby_chat_message"
	// TypeGameChatMessage is broadcast to a game's chat room carrying a new
	// chat message.
	TypeGameChatMessage Type = "game_chat_message"
	// TypeGameChatHistory is sent to a client after it joins a game's chat
	// room, carrying recent chat history for that room.
	TypeGameChatHistory Type = "game_chat_history"
	// TypeGameStateUpdated is broadcast to a game's chat room after every
	// committed game action, carrying a full snapshot of the resulting
	// engine state (not a diff).
	TypeGameStateUpdated Type = "game_state_updated"
	// TypeTerritorySelected is broadcast to a game's chat room relaying
	// another client's live territory selection.
	TypeTerritorySelected Type = "territory_selected"
)

// Envelope is the outer shape of every WebSocket message exchanged between
// client and server: a Type tag, correlation identifiers, and a raw JSON
// Payload whose concrete shape depends on Type.
type Envelope struct {
	// Type identifies what kind of message this is and how Payload should
	// be decoded.
	Type Type `json:"type"`
	// ID uniquely identifies this message instance.
	ID string `json:"id,omitempty"`
	// CorrelationID, when set on a server->client reply, echoes the ID of
	// the client message that triggered it.
	CorrelationID string `json:"correlation_id,omitempty"`
	// GameID identifies which game (and chat room) this message belongs
	// to, when applicable.
	GameID string `json:"game_id,omitempty"`
	// Payload holds the message-type-specific data as raw JSON, decoded
	// via DecodePayload into the struct matching Type.
	Payload json.RawMessage `json:"payload,omitempty"`
}

// DecodePayload unmarshals the payload into dst.
// If the payload is empty, it does nothing.
func (e Envelope) DecodePayload(dst any) error {
	if len(e.Payload) == 0 {
		return nil
	}
	return json.Unmarshal(e.Payload, dst)
}

// ----- payloads -----

// HelloIn is a legacy client->server greeting payload (paired with
// TypeHello) that is not currently produced by any handler; the Payload
// structs below are what the hub actually uses.
type HelloIn struct {
	// Name is the greeting client's display name.
	Name string `json:"name"`
}

// HelloOut is a legacy server->client greeting reply payload, not currently
// produced by any handler.
type HelloOut struct {
	// ClientID is the server-assigned identifier for the connection.
	ClientID string `json:"client_id"`
	// Name echoes the client's display name.
	Name string `json:"name"`
}

// CreateGameIn is a legacy client->server payload for creating a game,
// superseded in practice by CreateGamePayload.
type CreateGameIn struct {
	// MaxPlayers is the requested player capacity for the new game.
	MaxPlayers int `json:"max_players"`
}

// GameCreatedOut is a legacy server->client reply payload for game
// creation, superseded in practice by GameCreatedPayload.
type GameCreatedOut struct {
	// GameID is the identifier of the newly created game.
	GameID string `json:"game_id"`
	// OwnerID identifies the client that created the game.
	OwnerID string `json:"owner_id"`
	// MaxPlayers is the player capacity of the new game.
	MaxPlayers int `json:"max_players"`
}

// Player is a legacy lobby player summary used by GameSnapshot and the
// unused *Out payload types below; the hub's actual lobby snapshot is built
// as an untyped map (see the snapshot function in package game) rather than
// this struct.
type Player struct {
	// PlayerID identifies the player's connection.
	PlayerID string `json:"player_id"`
	// Name is the player's display name.
	Name string `json:"name"`
}

// GameSnapshot is a legacy full-lobby-state payload, not currently produced
// by any handler.
type GameSnapshot struct {
	// GameID identifies the game.
	GameID string `json:"game_id"`
	// OwnerID identifies the client that created the game.
	OwnerID string `json:"owner_id"`
	// MaxPlayers is the player capacity of the game.
	MaxPlayers int `json:"max_players"`
	// Players lists the players currently in the game.
	Players []Player `json:"players"`
}

// JoinedGameOut is a legacy server->client reply payload sent after joining
// a game, superseded in practice by JoinedGamePayload.
type JoinedGameOut struct {
	// Game is a snapshot of the joined game's lobby state.
	Game GameSnapshot `json:"game"`
}

// PlayerJoinedOut is a legacy broadcast payload for a player joining a
// game, not currently produced by any handler.
type PlayerJoinedOut struct {
	// Player is the player who joined.
	Player Player `json:"player"`
}

// PlayerLeftOut is a legacy broadcast payload for a player leaving a game,
// not currently produced by any handler.
type PlayerLeftOut struct {
	// Player is the player who left.
	Player Player `json:"player"`
}

// ErrorOut is a legacy server->client error payload; error replies are
// currently built as untyped envelopes by the errEnv helper in package
// game rather than this struct.
type ErrorOut struct {
	// Code is a short machine-readable error identifier.
	Code string `json:"code"`
	// Message is a human-readable description of the error.
	Message string `json:"message"`
}

// GameListOut is a legacy server->client reply payload for TypeListGames,
// superseded in practice by GameListPayload (though both share
// GameListItem).
type GameListOut struct {
	// Games lists the currently joinable lobby games.
	Games []GameListItem `json:"games"`
}

// GameListItem summarizes one joinable lobby game for inclusion in a game
// list payload.
type GameListItem struct {
	// GameID identifies the game.
	GameID string `json:"game_id"`
	// Players is the current number of players in the game.
	Players int `json:"players"`
	// MaxPlayers is the game's player capacity.
	MaxPlayers int `json:"max_players"`
}

// CreateGamePayload is the TypeCreateGame client->server payload requesting
// a new lobby game be created.
type CreateGamePayload struct {
	// MaxPlayers is the requested player capacity for the new game
	// (defaults to 6 when omitted).
	MaxPlayers int `json:"max_players"`
}

// GameCreatedPayload is the TypeGameCreated server->client payload sent to
// the creator after a lobby game is created.
type GameCreatedPayload struct {
	// GameID is the identifier of the newly created game.
	GameID string `json:"game_id"`
	// OwnerID identifies the client that created the game.
	OwnerID string `json:"owner_id"`
	// MaxPlayers is the player capacity of the new game.
	MaxPlayers int `json:"max_players"`
}

// JoinedGamePayload is the TypeJoinedGame server->client payload sent to a
// client after it joins a lobby game.
type JoinedGamePayload struct {
	// Game is the lobby snapshot (game ID, owner, capacity, and current
	// players) built by the snapshot helper in package game; it is typed
	// as any because that helper currently returns an untyped map rather
	// than a dedicated struct.
	Game any `json:"game"` // if snapshot() returns a struct, use that type instead of any
}

// GameListPayload is the TypeGameList server->client payload sent in reply
// to TypeListGames, carrying the currently joinable lobby games.
type GameListPayload struct {
	// Games lists the currently joinable lobby games.
	Games []GameListItem `json:"games"`
}

// GameChatSendPayload is the TypeGameChatSend client->server payload
// posting a new chat message into a game's chat room.
type GameChatSendPayload struct {
	// Body is the chat message text.
	Body string `json:"body"`
	// UserName is the display name to attribute the message to; if
	// omitted, the server resolves it from the authenticated user.
	UserName string `json:"username,omitempty"`
}

// GameChatMessagePayload is the TypeGameChatMessage server->client payload
// broadcasting a single chat message to a game's chat room, and is also
// used as the element type of GameChatHistoryPayload.Messages.
type GameChatMessagePayload struct {
	// GameID identifies the game whose chat room the message belongs to.
	GameID string `json:"game_id"`
	// UserName is the display name of the message's author.
	UserName string `json:"user_name"`
	// Body is the chat message text.
	Body string `json:"body"`
	// CreatedAt is the message's creation timestamp, RFC 3339 formatted.
	CreatedAt string `json:"created_at"`
}

// GameChatHistoryPayload is the TypeGameChatHistory server->client payload
// sent to a client immediately after it joins a game's chat room, carrying
// recent chat history for that room.
type GameChatHistoryPayload struct {
	// Messages is the recent chat history, oldest constraints applied by
	// the server (currently capped at 200 messages).
	Messages []GameChatMessagePayload `json:"messages"`
}

// GameActionPayload is the TypeGameAction client->server payload submitting
// a gameplay action to the engine. Action selects which fields are
// meaningful; legal values include "place_initial_army",
// "place_reinforcement", "attack", "occupy", "end_attack", "fortify",
// "end_turn", and "trade_cards".
type GameActionPayload struct {
	// Action names the gameplay action being submitted (e.g. "attack",
	// "fortify", "occupy", "trade_cards").
	Action string `json:"action"`
	// Territory is the target territory for actions that operate on a
	// single territory (e.g. placing reinforcements or initial armies).
	Territory string `json:"territory,omitempty"`
	// From is the source territory for actions that move armies between
	// two territories (attack, fortify, occupy).
	From string `json:"from,omitempty"`
	// To is the destination territory for actions that move armies between
	// two territories (attack, fortify, occupy).
	To string `json:"to,omitempty"`
	// Armies is the number of armies involved in the action (reinforcement
	// count, fortify move size, or occupy move size).
	Armies int `json:"armies,omitempty"`
	// AttackerDice is the number of dice the attacker rolls for an
	// "attack" action.
	AttackerDice int `json:"attacker_dice,omitempty"`
	// DefenderDice is the number of dice the defender rolls for an
	// "attack" action.
	DefenderDice int `json:"defender_dice,omitempty"`
	// CardIndices holds the indices, into the acting player's hand, of the
	// three cards being turned in for a "trade_cards" action.
	CardIndices [3]int `json:"card_indices"`
}

// TerritorySelectPayload is sent by a client whenever its local player
// selection changes (including clearing it — an all-empty payload means
// "I deselected"). This is purely a live-cursor-style signal: it never
// touches games.state, is never persisted, and carries no authority — the
// hub only relays it to the rest of the game's chat room.
type TerritorySelectPayload struct {
	// Territory is the single territory currently selected, if the
	// selection is a single territory rather than a from/to pair.
	Territory string `json:"territory,omitempty"`
	// From is the source territory of the current selection, if the
	// selection is a from/to pair (e.g. mid-attack or mid-fortify).
	From string `json:"from,omitempty"`
	// To is the destination territory of the current selection, if the
	// selection is a from/to pair.
	To string `json:"to,omitempty"`
}

// TerritorySelectedPayload is the relayed broadcast, tagged with whose
// selection it is so clients can ignore their own echo and can eventually
// distinguish multiple simultaneous selections.
type TerritorySelectedPayload struct {
	// UserID identifies the client whose selection this is, so clients can
	// ignore their own echo.
	UserID string `json:"user_id"`
	// Territory is the single territory currently selected, if the
	// selection is a single territory rather than a from/to pair.
	Territory string `json:"territory,omitempty"`
	// From is the source territory of the current selection, if the
	// selection is a from/to pair.
	From string `json:"from,omitempty"`
	// To is the destination territory of the current selection, if the
	// selection is a from/to pair.
	To string `json:"to,omitempty"`
}

// CardPayload describes a single Risk card in a player's hand.
type CardPayload struct {
	// Territory is the territory depicted on the card.
	Territory string `json:"territory"`
	// Symbol is the card's army symbol (e.g. infantry, cavalry, artillery,
	// or wild).
	Symbol string `json:"symbol"`
}

// YourCardsPayload is the TypeYourCards server->client payload sent
// privately to the acting player after a game action, carrying that
// player's current card hand.
type YourCardsPayload struct {
	// Cards is the acting player's current hand.
	Cards []CardPayload `json:"cards"`
}

// GameStatePlayerPayload summarizes one player's public state within a
// GameStateUpdatedPayload broadcast.
type GameStatePlayerPayload struct {
	// UserID identifies the player.
	UserID string `json:"user_id"`
	// CardCount is the number of cards in the player's hand.
	CardCount int `json:"card_count"`
	// SetupArmies is the number of armies the player has left to place
	// during the setup-reinforce phase.
	SetupArmies int `json:"setup_armies"`
	// Eliminated reports whether the player has been eliminated from the
	// game.
	Eliminated bool `json:"eliminated"`
}

// GameStateUpdatedPayload is the TypeGameStateUpdated server->client
// payload broadcast to a game's chat room after every committed game
// action. It is a full snapshot of the resulting engine state, not a
// diff — clients replace their local game state wholesale on receipt.
type GameStateUpdatedPayload struct {
	// GameID identifies the game this state belongs to.
	GameID string `json:"game_id"`
	// Action names the gameplay action that produced this update (see
	// GameActionPayload.Action for the set of legal values).
	Action string `json:"action"`
	// ActorUserID identifies the player who performed the action.
	ActorUserID string `json:"actor_user_id"`
	// Phase is the engine's current phase (e.g. "reinforce", "attack",
	// "occupy", "fortify", "game_over").
	Phase string `json:"phase"`
	// CurrentPlayer is the index, into Players, of the player whose turn it
	// currently is.
	CurrentPlayer int `json:"current_player"`
	// PendingReinforcements is the number of reinforcement armies the
	// current player still has to place.
	PendingReinforcements int `json:"pending_reinforcements"`
	// SetsTraded is the running count of card sets traded in during the
	// game, which determines the next set's reinforcement bonus.
	SetsTraded int `json:"sets_traded"`
	// Occupy, when non-nil, describes the pending post-conquest army move
	// the current player must make before the engine leaves the occupy
	// phase.
	Occupy *GameOccupyRequirement `json:"occupy,omitempty"`
	// Players lists the public state of every player in the game.
	Players []GameStatePlayerPayload `json:"players"`
	// Territories is the raw JSON encoding of the engine's territory
	// ownership and army counts, passed through unmodified.
	Territories json.RawMessage `json:"territories"`
	// Result carries action-specific outcome data: a *risk.AttackResult
	// (dice rolls, losses, conquest info) for an "attack" action, a
	// map[string]int{"armies": n} for a "trade_cards" action, and nil for
	// every other action.
	Result any `json:"result,omitempty"`
	// Event, when non-nil, is the latest persisted game_events row this
	// action produced (e.g. an attack resolution, whose Body may note a
	// resulting player elimination, or a card trade-in) for display in the
	// game log.
	Event *GameEventPayload `json:"event,omitempty"`

	// ActionTerritory/ActionFrom/ActionTo name the territory (or pair) this
	// action touched, so the frontend can highlight them exactly as it
	// would for a human's own click — the only such signal for bot moves.
	ActionTerritory string `json:"action_territory,omitempty"`
	ActionFrom      string `json:"action_from,omitempty"` // ActionFrom is the source territory this action touched, if any.
	ActionTo        string `json:"action_to,omitempty"`   // ActionTo is the destination territory this action touched, if any.
}

// GameOccupyRequirement describes the pending post-conquest army move a
// player must make after winning an attack, before the engine can proceed
// out of the occupy phase.
type GameOccupyRequirement struct {
	// From is the attacking territory that must send armies.
	From string `json:"from"`
	// To is the newly conquered territory that must receive armies.
	To string `json:"to"`
	// MinMove is the minimum number of armies that must be moved (the
	// number of dice the attacker committed).
	MinMove int `json:"min_move"`
	// MaxMove is the maximum number of armies that may be moved (all but
	// one of the armies remaining on From).
	MaxMove int `json:"max_move"`
}

// GameEventPayload describes a single persisted game_events row (e.g. an
// attack resolution or a card-set trade-in), surfaced to clients via
// GameStateUpdatedPayload.Event.
type GameEventPayload struct {
	// ID uniquely identifies the event.
	ID string `json:"id"`
	// GameID identifies the game the event belongs to.
	GameID string `json:"game_id"`
	// ActorUserID identifies the player who caused the event, if
	// applicable.
	ActorUserID string `json:"actor_user_id,omitempty"`
	// EventType is a free-form category label for the event (e.g.
	// "attack_resolved", "cards_traded", "fortified", "turn_ended",
	// "game_started", "player_joined") assigned by the service layer; there
	// is no fixed Go enum of values. A player elimination is not a
	// distinct type — it is noted as text within an "attack_resolved"
	// event's Body.
	EventType string `json:"event_type"`
	// Body is a human-readable description of the event, suitable for
	// display in the game log.
	Body string `json:"body"`
	// CreatedAt is the event's creation timestamp, RFC 3339 formatted.
	CreatedAt string `json:"created_at"`
}
