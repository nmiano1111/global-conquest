package game

import (
	"backend/internal/proto/wsmsg"
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Server struct {
	inbox chan any

	clients   map[string]*Client
	games     map[string]*Game
	typing    map[string]typingPresence
	chatRooms map[string]map[string]struct{}
	chatLog   GameChatLogStore
	actions   GameActionService
}

type Client struct {
	ID       string
	UserID   string
	Name     string
	ChatRoom string

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

type typingPresence struct {
	Name     string
	LastSeen time.Time
}

type GameChatLogMessage struct {
	GameID    string
	UserName  string
	Body      string
	CreatedAt time.Time
}

type GameChatLogStore interface {
	SaveGameMessage(ctx context.Context, gameID, senderClientID, senderName, body string) (GameChatLogMessage, error)
	ListGameMessages(ctx context.Context, gameID string, limit int) ([]GameChatLogMessage, error)
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
}

type GameActionPlayer struct {
	UserID     string
	CardCount  int
	Eliminated bool
}

type GameActionUpdate struct {
	GameID                string
	Action                string
	ActorUserID           string
	Phase                 string
	CurrentPlayer         int
	PendingReinforcements int
	Occupy                *GameOccupyRequirement
	Players               []GameActionPlayer
	Territories           json.RawMessage
	Result                any
}

type GameOccupyRequirement struct {
	From    string
	To      string
	MinMove int
	MaxMove int
}

type GameActionService interface {
	ApplyGameAction(ctx context.Context, in GameActionInput) (GameActionUpdate, error)
}

// --- inbox messages ---
type Register struct{ C *Client }
type Unregister struct{ ClientID string }
type Incoming struct {
	ClientID string
	Env      wsmsg.Envelope
}
type PublishLobbyChat struct {
	Message map[string]any
}

func NewServer() *Server {
	return &Server{
		inbox:     make(chan any, 256),
		clients:   make(map[string]*Client),
		games:     make(map[string]*Game),
		typing:    make(map[string]typingPresence),
		chatRooms: make(map[string]map[string]struct{}),
	}
}

func (s *Server) Inbox() chan<- any { return s.inbox }

func (s *Server) SetGameChatLogStore(chatLog GameChatLogStore) {
	s.chatLog = chatLog
}

func (s *Server) SetGameActionService(actions GameActionService) {
	s.actions = actions
}

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
			s.sendTypingStateTo(m.C)
		case Unregister:
			s.handleDisconnect(m.ClientID)
		case Incoming:
			s.handleIncoming(m.ClientID, m.Env)
		case PublishLobbyChat:
			s.broadcastLobbyChat(m.Message)
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
	if _, ok := s.typing[clientID]; ok {
		delete(s.typing, clientID)
		s.broadcastTypingState()
	}
	if c.ChatRoom != "" {
		s.leaveChatRoom(c)
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

	case wsmsg.TypeLobbyTypingStart:
		name := c.Name
		if len(env.Payload) > 0 {
			var payload struct {
				UserName string `json:"username"`
			}
			if err := json.Unmarshal(env.Payload, &payload); err == nil && strings.TrimSpace(payload.UserName) != "" {
				name = strings.TrimSpace(payload.UserName)
			}
		}
		if name == "" {
			name = "anon"
		}
		s.typing[c.ID] = typingPresence{Name: name, LastSeen: time.Now().UTC()}
		s.broadcastTypingState()

	case wsmsg.TypeLobbyTypingStop:
		if _, ok := s.typing[c.ID]; ok {
			delete(s.typing, c.ID)
			s.broadcastTypingState()
		}

	case wsmsg.TypeGameChatJoin:
		if gameID == "" {
			c.Conn.Send(errEnv(id, "invalid_message", "game_id is required"))
			return
		}
		s.joinChatRoom(c, gameID)

	case wsmsg.TypeGameChatLeave:
		if c.ChatRoom != "" && (gameID == "" || gameID == c.ChatRoom) {
			s.leaveChatRoom(c)
		}

	case wsmsg.TypeGameChatSend:
		if gameID == "" {
			c.Conn.Send(errEnv(id, "invalid_message", "game_id is required"))
			return
		}
		if c.ChatRoom != gameID {
			c.Conn.Send(errEnv(id, "not_in_room", "join the game chat room first"))
			return
		}
		var payload wsmsg.GameChatSendPayload
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			c.Conn.Send(errEnv(id, "invalid_message", "invalid payload"))
			return
		}
		body := strings.TrimSpace(payload.Body)
		if body == "" {
			c.Conn.Send(errEnv(id, "invalid_message", "message body is required"))
			return
		}
		name := strings.TrimSpace(payload.UserName)
		if name == "" {
			name = c.Name
		}
		if name == "" {
			name = "anon"
		}
		createdAt := time.Now().UTC()
		if s.chatLog != nil {
			if saved, err := s.chatLog.SaveGameMessage(context.Background(), gameID, c.ID, name, body); err == nil {
				createdAt = saved.CreatedAt
				name = saved.UserName
				body = saved.Body
			}
		}
		s.broadcastGameChatMessage(gameID, map[string]any{
			"game_id":    gameID,
			"user_name":  name,
			"body":       body,
			"created_at": createdAt.UTC().Format(time.RFC3339Nano),
		})

	case wsmsg.TypeGameAction:
		if gameID == "" {
			c.Conn.Send(errEnv(id, "invalid_message", "game_id is required"))
			return
		}
		if c.ChatRoom != gameID {
			c.Conn.Send(errEnv(id, "not_in_room", "join the game chat room first"))
			return
		}
		if c.UserID == "" {
			c.Conn.Send(errEnv(id, "unauthorized", "authenticated user required"))
			return
		}
		if s.actions == nil {
			c.Conn.Send(errEnv(id, "not_configured", "game action service is not configured"))
			return
		}

		var payload wsmsg.GameActionPayload
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			c.Conn.Send(errEnv(id, "invalid_message", "invalid payload"))
			return
		}
		updated, err := s.actions.ApplyGameAction(context.Background(), GameActionInput{
			GameID:       gameID,
			PlayerUserID: c.UserID,
			Action:       strings.TrimSpace(payload.Action),
			Territory:    strings.TrimSpace(payload.Territory),
			From:         strings.TrimSpace(payload.From),
			To:           strings.TrimSpace(payload.To),
			Armies:       payload.Armies,
			AttackerDice: payload.AttackerDice,
			DefenderDice: payload.DefenderDice,
		})
		if err != nil {
			c.Conn.Send(errEnv(id, "invalid_action", err.Error()))
			return
		}

		statePlayers := make([]wsmsg.GameStatePlayerPayload, 0, len(updated.Players))
		for _, p := range updated.Players {
			statePlayers = append(statePlayers, wsmsg.GameStatePlayerPayload{
				UserID:     p.UserID,
				CardCount:  p.CardCount,
				Eliminated: p.Eliminated,
			})
		}
		s.broadcastGameStateUpdate(gameID, wsmsg.GameStateUpdatedPayload{
			GameID:                updated.GameID,
			Action:                updated.Action,
			ActorUserID:           updated.ActorUserID,
			Phase:                 updated.Phase,
			CurrentPlayer:         updated.CurrentPlayer,
			PendingReinforcements: updated.PendingReinforcements,
			Occupy: func() *wsmsg.GameOccupyRequirement {
				if updated.Occupy == nil {
					return nil
				}
				return &wsmsg.GameOccupyRequirement{
					From:    updated.Occupy.From,
					To:      updated.Occupy.To,
					MinMove: updated.Occupy.MinMove,
					MaxMove: updated.Occupy.MaxMove,
				}
			}(),
			Players:     statePlayers,
			Territories: updated.Territories,
			Result:      updated.Result,
		})

	default:
		// generic ack
		c.Conn.Send(envelope("ack", newID("s"), id, gameID, nil))
	}
}

func (s *Server) joinChatRoom(c *Client, roomID string) {
	if c.ChatRoom == roomID {
		return
	}
	if c.ChatRoom != "" {
		s.leaveChatRoom(c)
	}
	clients, ok := s.chatRooms[roomID]
	if !ok {
		clients = make(map[string]struct{})
		s.chatRooms[roomID] = clients
	}
	clients[c.ID] = struct{}{}
	c.ChatRoom = roomID

	if s.chatLog == nil {
		return
	}
	msgs, err := s.chatLog.ListGameMessages(context.Background(), roomID, 200)
	if err != nil || len(msgs) == 0 {
		return
	}
	out := make([]wsmsg.GameChatMessagePayload, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, wsmsg.GameChatMessagePayload{
			GameID:    m.GameID,
			UserName:  m.UserName,
			Body:      m.Body,
			CreatedAt: m.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	c.Conn.Send(envelope(string(wsmsg.TypeGameChatHistory), newID("s"), "", roomID, wsmsg.GameChatHistoryPayload{
		Messages: out,
	}))
}

func (s *Server) leaveChatRoom(c *Client) {
	roomID := c.ChatRoom
	if roomID == "" {
		return
	}
	if clients, ok := s.chatRooms[roomID]; ok {
		delete(clients, c.ID)
		if len(clients) == 0 {
			delete(s.chatRooms, roomID)
		}
	}
	c.ChatRoom = ""
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
	if c.ChatRoom == gid {
		s.leaveChatRoom(c)
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

func (s *Server) broadcastTypingState() {
	const typingTTL = 4 * time.Second
	cutoff := time.Now().UTC().Add(-typingTTL)
	for clientID, state := range s.typing {
		if state.LastSeen.Before(cutoff) {
			delete(s.typing, clientID)
		}
	}

	for _, c := range s.clients {
		s.sendTypingStateTo(c)
	}
}

func (s *Server) sendTypingStateTo(c *Client) {
	users := make([]string, 0, len(s.typing))
	for typingClientID, state := range s.typing {
		if typingClientID == c.ID {
			continue
		}
		users = append(users, state.Name)
	}
	c.Conn.Send(envelope(string(wsmsg.TypeLobbyTypingState), newID("s"), "", "", map[string]any{
		"users": users,
	}))
}

func (s *Server) broadcastLobbyChat(message map[string]any) {
	ev := envelope(string(wsmsg.TypeLobbyChatMessage), newID("s"), "", "", message)
	for _, c := range s.clients {
		c.Conn.Send(ev)
	}
}

func (s *Server) broadcastGameChatMessage(roomID string, message map[string]any) {
	ev := envelope(string(wsmsg.TypeGameChatMessage), newID("s"), "", roomID, message)
	clientIDs := s.chatRooms[roomID]
	for clientID := range clientIDs {
		if c, ok := s.clients[clientID]; ok {
			c.Conn.Send(ev)
		}
	}
}

func (s *Server) broadcastGameStateUpdate(roomID string, payload wsmsg.GameStateUpdatedPayload) {
	ev := envelope(string(wsmsg.TypeGameStateUpdated), newID("s"), "", roomID, payload)
	clientIDs := s.chatRooms[roomID]
	for clientID := range clientIDs {
		if c, ok := s.clients[clientID]; ok {
			c.Conn.Send(ev)
		}
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
