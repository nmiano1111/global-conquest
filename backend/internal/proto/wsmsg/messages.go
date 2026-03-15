package wsmsg

import "encoding/json"

type Type string

const (
	// client->server
	TypeHello            Type = "hello"
	TypeCreateGame       Type = "create_game"
	TypeJoinGame         Type = "join_game"
	TypeLeaveGame        Type = "leave_game"
	TypeListGames        Type = "list_games"
	TypeLobbyTypingStart Type = "lobby_typing_start"
	TypeLobbyTypingStop  Type = "lobby_typing_stop"
	TypeGameChatJoin     Type = "game_chat_join"
	TypeGameChatLeave    Type = "game_chat_leave"
	TypeGameChatSend     Type = "game_chat_send"
	TypeGameAction       Type = "game_action"

	// server->client
	TypeError            Type = "error"
	TypeYourCards        Type = "your_cards"
	TypeGameCreated      Type = "game_created"
	TypeJoinedGame       Type = "joined_game"
	TypeLeftGame         Type = "left_game"
	TypeGameList         Type = "game_list"
	TypePlayerJoined     Type = "player_joined"
	TypePlayerLeft       Type = "player_left"
	TypeLobbyTypingState Type = "lobby_typing_state"
	TypeLobbyChatMessage Type = "lobby_chat_message"
	TypeGameChatMessage  Type = "game_chat_message"
	TypeGameChatHistory  Type = "game_chat_history"
	TypeGameStateUpdated Type = "game_state_updated"
)

type Envelope struct {
	Type          Type            `json:"type"`
	ID            string          `json:"id,omitempty"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	GameID        string          `json:"game_id,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
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

type HelloIn struct {
	Name string `json:"name"`
}
type HelloOut struct {
	ClientID string `json:"client_id"`
	Name     string `json:"name"`
}

type CreateGameIn struct {
	MaxPlayers int `json:"max_players"`
}

type GameCreatedOut struct {
	GameID     string `json:"game_id"`
	OwnerID    string `json:"owner_id"`
	MaxPlayers int    `json:"max_players"`
}

type Player struct {
	PlayerID string `json:"player_id"`
	Name     string `json:"name"`
}

type GameSnapshot struct {
	GameID     string   `json:"game_id"`
	OwnerID    string   `json:"owner_id"`
	MaxPlayers int      `json:"max_players"`
	Players    []Player `json:"players"`
}

type JoinedGameOut struct {
	Game GameSnapshot `json:"game"`
}

type PlayerJoinedOut struct {
	Player Player `json:"player"`
}
type PlayerLeftOut struct {
	Player Player `json:"player"`
}

type ErrorOut struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type GameListOut struct {
	Games []GameListItem `json:"games"`
}
type GameListItem struct {
	GameID     string `json:"game_id"`
	Players    int    `json:"players"`
	MaxPlayers int    `json:"max_players"`
}

type CreateGamePayload struct {
	MaxPlayers int `json:"max_players"`
}

type GameCreatedPayload struct {
	GameID     string `json:"game_id"`
	OwnerID    string `json:"owner_id"`
	MaxPlayers int    `json:"max_players"`
}

type JoinedGamePayload struct {
	Game any `json:"game"` // if snapshot() returns a struct, use that type instead of any
}

type GameListPayload struct {
	Games []GameListItem `json:"games"`
}

type GameChatSendPayload struct {
	Body     string `json:"body"`
	UserName string `json:"username,omitempty"`
}

type GameChatMessagePayload struct {
	GameID    string `json:"game_id"`
	UserName  string `json:"user_name"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

type GameChatHistoryPayload struct {
	Messages []GameChatMessagePayload `json:"messages"`
}

type GameActionPayload struct {
	Action       string `json:"action"`
	Territory    string `json:"territory,omitempty"`
	From         string `json:"from,omitempty"`
	To           string `json:"to,omitempty"`
	Armies       int    `json:"armies,omitempty"`
	AttackerDice int    `json:"attacker_dice,omitempty"`
	DefenderDice int    `json:"defender_dice,omitempty"`
	CardIndices  [3]int `json:"card_indices"`
}

type CardPayload struct {
	Territory string `json:"territory"`
	Symbol    string `json:"symbol"`
}

type YourCardsPayload struct {
	Cards []CardPayload `json:"cards"`
}

type GameStatePlayerPayload struct {
	UserID     string `json:"user_id"`
	CardCount  int    `json:"card_count"`
	Eliminated bool   `json:"eliminated"`
}

type GameStateUpdatedPayload struct {
	GameID                string                   `json:"game_id"`
	Action                string                   `json:"action"`
	ActorUserID           string                   `json:"actor_user_id"`
	Phase                 string                   `json:"phase"`
	CurrentPlayer         int                      `json:"current_player"`
	PendingReinforcements int                      `json:"pending_reinforcements"`
	SetsTraded            int                      `json:"sets_traded"`
	Occupy                *GameOccupyRequirement   `json:"occupy,omitempty"`
	Players               []GameStatePlayerPayload `json:"players"`
	Territories           json.RawMessage          `json:"territories"`
	Result                any                      `json:"result,omitempty"`
	Event                 *GameEventPayload        `json:"event,omitempty"`
}

type GameOccupyRequirement struct {
	From    string `json:"from"`
	To      string `json:"to"`
	MinMove int    `json:"min_move"`
	MaxMove int    `json:"max_move"`
}

type GameEventPayload struct {
	ID          string `json:"id"`
	GameID      string `json:"game_id"`
	ActorUserID string `json:"actor_user_id,omitempty"`
	EventType   string `json:"event_type"`
	Body        string `json:"body"`
	CreatedAt   string `json:"created_at"`
}
