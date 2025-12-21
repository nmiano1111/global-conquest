package game

import (
	"backend/internal/proto/wsmsg"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Server struct {
	inbox chan any

	clients map[string]*Client
	games   map[string]*Game
}

type Client struct {
	ID   string
	Name string

	Conn Outbound // interface so game package doesn't depend on wsconn directly
	Game string   // current game id, "" if none
}

type Outbound interface {
	Send(env wsmsg.Envelope) bool
}

type Game struct {
	ID         string
	OwnerID    string
	MaxPlayers int
	Players    map[string]*Client
	CreatedAt  time.Time
}

// --- inbox messages ---
type Register struct{ C *Client }
type Unregister struct{ ClientID string }
type Incoming struct {
	ClientID string
	Env      wsmsg.Envelope
}

func NewServer() *Server {
	return &Server{
		inbox:   make(chan any, 256),
		clients: make(map[string]*Client),
		games:   make(map[string]*Game),
	}
}

func (s *Server) Inbox() chan<- any { return s.inbox }

func (s *Server) Run() {
	for msg := range s.inbox {
		switch m := msg.(type) {
		case Register:
			s.clients[m.C.ID] = m.C
			// hello back with assigned id
			m.C.Conn.Send(envelope("hello", "", "", "", map[string]any{
				"client_id": m.C.ID,
				"name":      m.C.Name,
			}))
		case Unregister:
			s.handleDisconnect(m.ClientID)
		case Incoming:
			s.handleIncoming(m.ClientID, m.Env)
		}
	}
}

func (s *Server) handleDisconnect(clientID string) {
	c, ok := s.clients[clientID]
	if !ok {
		return
	}
	// If in a game, remove and broadcast
	if c.Game != "" {
		s.leaveGame(c, true)
	}
	delete(s.clients, clientID)
}

func (s *Server) handleIncoming(clientID string, env wsmsg.Envelope) {
	c, ok := s.clients[clientID]
	if !ok {
		return
	}

	t := env.Type
	id := env.ID
	gameID := env.GameID

	switch t {

	case "create_game":
		// payload.max_players (optional)
		maxPlayers := 6
		if len(env.Payload) > 0 {
			var p wsmsg.CreateGamePayload
			if err := json.Unmarshal(env.Payload, &p); err != nil {
				c.Conn.Send(errEnv(id, "invalid_message", "invalid payload"))
				return
			}
			if p.MaxPlayers > 0 {
				maxPlayers = p.MaxPlayers
			}
		}

		if c.Game != "" {
			c.Conn.Send(errEnv(id, "already_in_game", "Leave your current game first"))
			return
		}

		gid := newID("g")
		g := &Game{
			ID:         gid,
			OwnerID:    c.ID,
			MaxPlayers: maxPlayers,
			Players:    map[string]*Client{},
			CreatedAt:  time.Now(),
		}
		s.games[gid] = g

		// Auto-join creator
		s.joinGame(c, gid)

		// Tell creator game created
		c.Conn.Send(envelope("game_created", newID("s"), id, gid, wsmsg.GameCreatedPayload{
			GameID:     gid,
			OwnerID:    c.ID,
			MaxPlayers: maxPlayers,
		}))

	case "join_game":
		if c.Game != "" {
			c.Conn.Send(errEnv(id, "already_in_game", "Leave your current game first"))
			return
		}
		if gameID == "" {
			c.Conn.Send(errEnv(id, "invalid_message", "game_id is required"))
			return
		}
		g, ok := s.games[gameID]
		if !ok {
			c.Conn.Send(errEnv(id, "game_not_found", "Game does not exist"))
			return
		}

		s.joinGame(c, gameID)

		// Reply joined snapshot
		c.Conn.Send(envelope("joined_game", newID("s"), id, gameID, wsmsg.JoinedGamePayload{
			Game: snapshot(g),
		}))

	case "leave_game":
		if c.Game == "" {
			c.Conn.Send(errEnv(id, "not_in_game", "You are not in a game"))
			return
		}
		gid := c.Game
		s.leaveGame(c, false)
		c.Conn.Send(envelope("left_game", newID("s"), id, gid, nil))

	case "list_games":
		items := make([]wsmsg.GameListItem, 0, len(s.games))
		for _, g := range s.games {
			items = append(items, wsmsg.GameListItem{
				GameID:     g.ID,
				Players:    len(g.Players),
				MaxPlayers: g.MaxPlayers,
			})
		}
		c.Conn.Send(envelope("game_list", newID("s"), id, "", wsmsg.GameListPayload{
			Games: items,
		}))

	case "ping":
		c.Conn.Send(envelope("pong", newID("s"), id, gameID, nil))

	default:
		// generic ack
		c.Conn.Send(envelope("ack", newID("s"), id, gameID, nil))
	}
}

func (s *Server) joinGame(c *Client, gameID string) {
	g := s.games[gameID]
	if len(g.Players) >= g.MaxPlayers {
		c.Conn.Send(errEnv("", "game_full", "Game is full"))
		return
	}

	c.Game = gameID
	g.Players[c.ID] = c

	// Broadcast player_joined to all players in game
	ev := envelope("player_joined", newID("s"), "", gameID, map[string]any{
		"player": map[string]any{"player_id": c.ID, "name": c.Name},
	})
	for _, p := range g.Players {
		p.Conn.Send(ev)
	}

	// Also send joined snapshot to joiner (helpful even for creator)
	c.Conn.Send(envelope("joined_game", newID("s"), "", gameID, map[string]any{
		"game": snapshot(g),
	}))
}

func (s *Server) leaveGame(c *Client, disconnect bool) {
	gid := c.Game
	g, ok := s.games[gid]
	if !ok {
		c.Game = ""
		return
	}

	delete(g.Players, c.ID)
	c.Game = ""

	// Broadcast player_left
	ev := envelope("player_left", newID("s"), "", gid, map[string]any{
		"player":     map[string]any{"player_id": c.ID, "name": c.Name},
		"disconnect": disconnect,
	})
	for _, p := range g.Players {
		p.Conn.Send(ev)
	}

	// Cleanup empty games
	if len(g.Players) == 0 {
		delete(s.games, gid)
	}
}

func snapshot(g *Game) map[string]any {
	players := make([]map[string]any, 0, len(g.Players))
	for _, p := range g.Players {
		players = append(players, map[string]any{
			"player_id": p.ID,
			"name":      p.Name,
		})
	}
	return map[string]any{
		"game_id":     g.ID,
		"owner_id":    g.OwnerID,
		"max_players": g.MaxPlayers,
		"players":     players,
	}
}

// --- helpers ---

func envelope(t, id, corr, gameID string, payload any) wsmsg.Envelope {
	env := wsmsg.Envelope{
		Type:          wsmsg.Type(t),
		ID:            id,
		CorrelationID: corr,
		GameID:        gameID,
	}

	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			// In dev, crashing is fine; in prod you'd return an error instead.
			panic(err)
		}
		env.Payload = b
	}

	return env
}
func errEnv(corrID, code, msg string) wsmsg.Envelope {
	return envelope("error", newID("s"), corrID, "", map[string]any{
		"code":    code,
		"message": msg,
	})
}

func newID(prefix string) string {
	// 10 bytes => 16 base32 chars without padding-ish, short and URL-safe.
	var b [10]byte
	_, _ = rand.Read(b[:])
	s := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:]))
	return fmt.Sprintf("%s_%s", prefix, s)
}

// optional: validate that env is JSON-ish early (debug helper)
func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
