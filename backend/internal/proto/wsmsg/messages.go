package wsmsg

import "encoding/json"

type Type string

const (
	// client->server
	TypeHello      Type = "hello"
	TypeCreateGame Type = "create_game"
	TypeJoinGame   Type = "join_game"
	TypeLeaveGame  Type = "leave_game"
	TypeListGames  Type = "list_games"

	// server->client
	TypeError        Type = "error"
	TypeGameCreated  Type = "game_created"
	TypeJoinedGame   Type = "joined_game"
	TypeLeftGame     Type = "left_game"
	TypeGameList     Type = "game_list"
	TypePlayerJoined Type = "player_joined"
	TypePlayerLeft   Type = "player_left"
)

type Envelope struct {
	Type          Type            `json:"type"`
	ID            string          `json:"id,omitempty"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	GameID        string          `json:"game_id,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
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
