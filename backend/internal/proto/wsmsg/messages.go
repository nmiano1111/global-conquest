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

	// server->client
	TypeError            Type = "error"
	TypeGameCreated      Type = "game_created"
	TypeJoinedGame       Type = "joined_game"
	TypeLeftGame         Type = "left_game"
	TypeGameList         Type = "game_list"
	TypePlayerJoined     Type = "player_joined"
	TypePlayerLeft       Type = "player_left"
	TypeLobbyTypingState Type = "lobby_typing_state"
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
