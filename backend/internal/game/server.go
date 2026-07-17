// Package game implements the real-time multiplayer hub for Global Conquest.
// Server is a single-goroutine event loop that owns all connected WebSocket
// clients, lobby and game chat room membership, and the authoritative
// dispatch of game actions and the broadcasts they produce. All state
// mutations, whether triggered by a human client or a bot runner, flow
// through the hub's inbox channel and are processed serially inside Run.
package game

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"github.com/nmiano1111/global-conquest/backend/internal/proto/wsmsg"
	"log"
	"strings"
	"time"
)

// Server is the single-goroutine hub that owns all WebSocket client state,
// chat room membership, and game dispatch. Its unexported fields must only
// be read or written from inside Run(); everything else communicates with
// it by sending messages on the inbox channel.
type Server struct {
	inbox chan any

	clients   map[string]*Client
	games     map[string]*Game
	typing    map[string]typingPresence
	chatRooms map[string]map[string]struct{}
	chatLog   GameChatLogStore
	actions   GameActionService
	access    GameAccessChecker

	// botTrigger is invoked after every committed game action (human or
	// bot-submitted) with the affected game ID, so a bot manager can check
	// whether the new current player needs a runner started. It is called
	// from inside Run(), never concurrently.
	botTrigger BotTrigger
}

// BotTrigger is notified after a game action commits. Implementations must
// not block the hub goroutine for long; typical implementations just
// enqueue/dedupe a background runner start.
type BotTrigger func(gameID string)

// Client represents one connected WebSocket connection registered with the
// hub.
type Client struct {
	// ID is the hub-assigned identifier for this connection.
	ID string
	// UserID is the authenticated user's ID, or "" for an anonymous client.
	UserID string
	// Name is the display name associated with this client.
	Name string
	// ChatRoom is the game ID of the chat room this client has joined, or ""
	// if it has not joined one.
	ChatRoom string

	Conn Outbound // interface so game package doesn't depend on wsconn directly
	Game string   // current game id, "" if none
}

// Outbound abstracts sending a message to a connected client so the game
// package does not need to depend on the wsconn package directly.
type Outbound interface {
	Send(env wsmsg.Envelope) bool
}

// Game is the hub's in-memory record of a lobby/game and its connected
// players.
type Game struct {
	// ID is the game's unique identifier.
	ID string
	// OwnerID is the client ID of the player who created the game.
	OwnerID string
	// MaxPlayers is the maximum number of players allowed to join.
	MaxPlayers int
	// Players maps client ID to the connected Client for each player in the
	// game.
	Players map[string]*Client
	// CreatedAt is when the game was created.
	CreatedAt time.Time
}

type typingPresence struct {
	Name     string
	LastSeen time.Time
}

// GameChatLogMessage is a persisted chat message belonging to a game's chat
// room.
type GameChatLogMessage struct {
	// GameID identifies the game chat room the message belongs to.
	GameID string
	// UserName is the display name of the message's sender.
	UserName string
	// Body is the message text.
	Body string
	// CreatedAt is when the message was saved.
	CreatedAt time.Time
}

// GameChatLogStore persists and retrieves per-game chat history.
type GameChatLogStore interface {
	SaveGameMessage(ctx context.Context, gameID, senderClientID, senderName, body string) (GameChatLogMessage, error)
	ListGameMessages(ctx context.Context, gameID string, limit int) ([]GameChatLogMessage, error)
}

// GameActionInput describes a single game action to apply, whether it
// originates from a human WebSocket client or a bot runner.
type GameActionInput struct {
	// GameID identifies the game the action applies to.
	GameID string
	// PlayerUserID is the acting player's user ID.
	PlayerUserID string
	// Action names the action to perform (e.g. "attack", "fortify").
	Action string
	// Territory is the single territory targeted by the action, when
	// applicable.
	Territory string
	// From is the source territory for actions that move armies.
	From string
	// To is the destination territory for actions that move armies.
	To string
	// Armies is the number of armies involved in the action.
	Armies int
	// AttackerDice is the number of dice the attacker rolls.
	AttackerDice int
	// DefenderDice is the number of dice the defender rolls.
	DefenderDice int
	// CardIndices identifies the up-to-three cards selected from the
	// player's hand, for card-trading actions.
	CardIndices [3]int
}

// GameActionPlayer is the per-player state included in a GameActionUpdate.
type GameActionPlayer struct {
	// UserID identifies the player.
	UserID string
	// CardCount is the number of cards currently in the player's hand.
	CardCount int
	// SetupArmies is the number of armies the player has left to place
	// during setup.
	SetupArmies int
	// Eliminated is true if the player has been eliminated from the game.
	Eliminated bool
}

// GameActionUpdate is the resulting state and outcome of a successfully
// applied game action, as returned by GameActionService and broadcast to
// the game's chat room.
type GameActionUpdate struct {
	// GameID identifies the game that was updated.
	GameID string
	// Action names the action that was applied.
	Action string
	// ActorUserID is the user ID of the player who performed the action.
	ActorUserID string
	// Phase is the game's phase after the action was applied.
	Phase string
	// CurrentPlayer is the index of the player whose turn it is after the
	// action was applied.
	CurrentPlayer int
	// PendingReinforcements is the number of reinforcement armies the
	// current player still has to place.
	PendingReinforcements int
	// SetsTraded is the running count of card sets traded in for armies.
	SetsTraded int
	// Occupy describes the pending post-conquest occupy move, or nil if the
	// game is not awaiting one.
	Occupy *GameOccupyRequirement
	// Players is the updated per-player state for all players in the game.
	Players []GameActionPlayer
	// Territories is the updated territory state, serialized as raw JSON.
	Territories json.RawMessage
	// Result carries action-specific outcome data (e.g. combat results).
	Result any
	// Event is the game event record generated by this action, or nil if
	// none was generated.
	Event *GameEventMessage
	// ActorCards is the acting player's current hand, sent privately back
	// to the actor.
	ActorCards []wsmsg.CardPayload

	// ActionTerritory is the territory the action targeted, echoed back for
	// client display.
	ActionTerritory string
	// ActionFrom is the source territory the action used, echoed back for
	// client display.
	ActionFrom string
	// ActionTo is the destination territory the action used, echoed back
	// for client display.
	ActionTo string
}

// GameOccupyRequirement describes the post-conquest occupy move a player
// must make, including the legal range of armies that can be moved.
type GameOccupyRequirement struct {
	// From is the attacking territory armies are moved out of.
	From string
	// To is the newly conquered territory armies are moved into.
	To string
	// MinMove is the minimum number of armies that must be moved.
	MinMove int
	// MaxMove is the maximum number of armies that may be moved.
	MaxMove int
}

// GameEventMessage is a persisted, narratable event generated by a game
// action (e.g. a conquest or elimination).
type GameEventMessage struct {
	// ID is the event's unique identifier.
	ID string
	// GameID identifies the game the event belongs to.
	GameID string
	// ActorUserID is the user ID of the player whose action produced the
	// event.
	ActorUserID string
	// EventType names the kind of event.
	EventType string
	// Body is the human-readable event text.
	Body string
	// CreatedAt is when the event was recorded.
	CreatedAt time.Time
}

// GameActionService applies a game action against authoritative game state
// and returns the resulting update. Implementations own all game-rule
// enforcement; the hub only dispatches to it and broadcasts the result.
type GameActionService interface {
	ApplyGameAction(ctx context.Context, in GameActionInput) (GameActionUpdate, error)
}

// GameAccessChecker reports whether userID is allowed to access gameID,
// enforcing the same sandbox visibility rule the REST list/join/bootstrap
// endpoints use. The hub consults it before letting a client join a game's
// chat room (see the CLAUDE.md invariant that game_chat_join gates
// game_state_updated delivery) -- without this check, any authenticated
// client that guessed or was given a sandboxed game's ID could still
// receive its state broadcasts even though every other access path is
// closed to them. If unset (SetGameAccessChecker never called), the hub
// allows every join, matching pre-sandbox behavior.
type GameAccessChecker interface {
	CanAccessGame(ctx context.Context, gameID, userID string) (bool, error)
}

// --- inbox messages ---

// Register is an inbox message that registers a newly connected Client with
// the hub.
type Register struct {
	// C is the newly connected client to register.
	C *Client
}

// Unregister is an inbox message that removes a disconnected client from
// the hub, cleaning up its game, chat room, and typing state.
type Unregister struct {
	// ClientID identifies the client to remove.
	ClientID string
}

// Incoming is an inbox message carrying a decoded envelope received from a
// connected client.
type Incoming struct {
	// ClientID identifies the client the message was received from.
	ClientID string
	// Env is the received message envelope.
	Env wsmsg.Envelope
}

// PublishLobbyChat is an inbox message that broadcasts a lobby chat message
// to every connected client.
type PublishLobbyChat struct {
	// Message is the raw lobby chat payload to broadcast.
	Message map[string]any
}

// GameActionRequest submits a game action through the normal command path
// without a connected websocket client (used by bot runners). Result must
// be buffered (capacity >= 1) so Run() never blocks sending the reply.
type GameActionRequest struct {
	// Input is the game action to apply.
	Input GameActionInput
	// Result receives the outcome of applying Input. It must be buffered
	// with capacity >= 1 so Run() never blocks sending the reply.
	Result chan<- GameActionResult
}

// GameActionResult is the outcome of applying a GameActionRequest, sent
// back to the requester.
type GameActionResult struct {
	// Update is the resulting game state update, valid only if Err is nil.
	Update GameActionUpdate
	// Err is the error encountered applying the action, if any.
	Err error
}

// NewServer creates a Server with its internal state initialized. The
// returned Server must have Run started (typically in its own goroutine)
// before it can process inbox messages.
func NewServer() *Server {
	return &Server{
		inbox:     make(chan any, 256),
		clients:   make(map[string]*Client),
		games:     make(map[string]*Game),
		typing:    make(map[string]typingPresence),
		chatRooms: make(map[string]map[string]struct{}),
	}
}

// Inbox returns the send-only channel used to deliver messages to the hub.
// It is safe to call and send on from any goroutine.
func (s *Server) Inbox() chan<- any { return s.inbox }

// SetGameChatLogStore configures the store used to persist and load game
// chat history. It must be called before Run starts processing messages
// that depend on chat history.
func (s *Server) SetGameChatLogStore(chatLog GameChatLogStore) {
	s.chatLog = chatLog
}

// SetGameActionService configures the service used to apply game actions.
// It must be called before Run starts processing game actions.
func (s *Server) SetGameActionService(actions GameActionService) {
	s.actions = actions
}

// SetGameAccessChecker configures the checker used to gate game_chat_join
// against a game's sandbox visibility. It must be called before Run starts
// processing messages; if never called, every join is allowed.
func (s *Server) SetGameAccessChecker(access GameAccessChecker) {
	s.access = access
}

// SetBotTrigger configures the callback invoked after every committed game
// action, allowing a bot manager to check whether the next player needs a
// bot runner started.
func (s *Server) SetBotTrigger(trigger BotTrigger) {
	s.botTrigger = trigger
}

// SubmitGameAction runs a game action through the same normal command path
// human WebSocket clients use (ApplyGameAction, event/outbox persistence,
// commit, broadcast), without requiring a connected Client. It is safe to
// call from any goroutine: the work happens inside Run() via the inbox.
func (s *Server) SubmitGameAction(ctx context.Context, in GameActionInput) (GameActionUpdate, error) {
	resultCh := make(chan GameActionResult, 1)
	select {
	case s.inbox <- GameActionRequest{Input: in, Result: resultCh}:
	case <-ctx.Done():
		return GameActionUpdate{}, ctx.Err()
	}
	select {
	case res := <-resultCh:
		return res.Update, res.Err
	case <-ctx.Done():
		return GameActionUpdate{}, ctx.Err()
	}
}

// Run processes inbox messages one at a time until the inbox channel is
// closed. It is the only place s.clients and s.chatRooms are read or
// written, and must be run from a single, dedicated goroutine.
func (s *Server) Run() {
	for msg := range s.inbox {
		switch m := msg.(type) {
		case Register:
			s.clients[m.C.ID] = m.C
			log.Printf("ws: hub register client=%s user=%s name=%s", m.C.ID, m.C.UserID, m.C.Name)
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
		case GameActionRequest:
			update, err := s.commitGameAction(m.Input)
			m.Result <- GameActionResult{Update: update, Err: err}
		}
	}
}

func (s *Server) handleDisconnect(clientID string) {
	c, ok := s.clients[clientID]
	if !ok {
		log.Printf("ws: hub unregister client=%s (already gone)", clientID)
		return
	}
	log.Printf("ws: hub unregister client=%s user=%s name=%s game=%s chatRoom=%s", clientID, c.UserID, c.Name, c.Game, c.ChatRoom)
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
		log.Printf("ws: recv from unknown client=%s type=%s (dropped)", clientID, env.Type)
		return
	}

	t := env.Type
	id := env.ID
	gameID := env.GameID

	log.Printf("ws: recv client=%s user=%s type=%s game=%s", clientID, c.UserID, t, gameID)

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
		if s.access != nil {
			ok, err := s.access.CanAccessGame(context.Background(), gameID, c.UserID)
			if err != nil || !ok {
				c.Conn.Send(errEnv(id, "forbidden", "You do not have access to this game"))
				return
			}
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
		updated, err := s.commitGameAction(GameActionInput{
			GameID:       gameID,
			PlayerUserID: c.UserID,
			Action:       strings.TrimSpace(payload.Action),
			Territory:    strings.TrimSpace(payload.Territory),
			From:         strings.TrimSpace(payload.From),
			To:           strings.TrimSpace(payload.To),
			Armies:       payload.Armies,
			AttackerDice: payload.AttackerDice,
			DefenderDice: payload.DefenderDice,
			CardIndices:  payload.CardIndices,
		})
		if err != nil {
			c.Conn.Send(errEnv(id, "invalid_action", err.Error()))
			return
		}

		// Send the actor's current hand privately so they can see their cards.
		if updated.ActorCards != nil {
			c.Conn.Send(envelope(string(wsmsg.TypeYourCards), newID("s"), id, gameID, wsmsg.YourCardsPayload{
				Cards: updated.ActorCards,
			}))
		}

	case wsmsg.TypeTerritorySelect:
		// Purely a live-cursor relay: never touches game state, never
		// persisted, no authoritative meaning. A client with nothing
		// selected sends an all-empty payload, which is relayed the same
		// way — that's how everyone else learns "they deselected."
		if gameID == "" {
			c.Conn.Send(errEnv(id, "invalid_message", "game_id is required"))
			return
		}
		if c.ChatRoom != gameID {
			c.Conn.Send(errEnv(id, "not_in_room", "join the game chat room first"))
			return
		}
		var payload wsmsg.TerritorySelectPayload
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			c.Conn.Send(errEnv(id, "invalid_message", "invalid payload"))
			return
		}
		s.broadcastTerritorySelected(gameID, wsmsg.TerritorySelectedPayload{
			UserID:    c.UserID,
			Territory: strings.TrimSpace(payload.Territory),
			From:      strings.TrimSpace(payload.From),
			To:        strings.TrimSpace(payload.To),
		})

	default:
		// generic ack
		c.Conn.Send(envelope("ack", newID("s"), id, gameID, nil))
	}
}

// commitGameAction runs a game action through the application service,
// broadcasts the resulting state to the game's chat room, and notifies the
// bot trigger. It must only be called from within Run(), since it touches
// s.chatRooms/s.clients via broadcastGameStateUpdate. It is the single
// choke point both human (TypeGameAction) and bot (GameActionRequest)
// submissions go through, so both get identical persistence and broadcast
// behavior.
func (s *Server) commitGameAction(in GameActionInput) (GameActionUpdate, error) {
	if s.actions == nil {
		return GameActionUpdate{}, fmt.Errorf("game action service is not configured")
	}
	updated, err := s.actions.ApplyGameAction(context.Background(), in)
	if err != nil {
		return GameActionUpdate{}, err
	}

	statePlayers := make([]wsmsg.GameStatePlayerPayload, 0, len(updated.Players))
	for _, p := range updated.Players {
		statePlayers = append(statePlayers, wsmsg.GameStatePlayerPayload{
			UserID:      p.UserID,
			CardCount:   p.CardCount,
			SetupArmies: p.SetupArmies,
			Eliminated:  p.Eliminated,
		})
	}
	s.broadcastGameStateUpdate(in.GameID, wsmsg.GameStateUpdatedPayload{
		GameID:                updated.GameID,
		Action:                updated.Action,
		ActorUserID:           updated.ActorUserID,
		Phase:                 updated.Phase,
		CurrentPlayer:         updated.CurrentPlayer,
		PendingReinforcements: updated.PendingReinforcements,
		SetsTraded:            updated.SetsTraded,
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

		ActionTerritory: updated.ActionTerritory,
		ActionFrom:      updated.ActionFrom,
		ActionTo:        updated.ActionTo,

		Event: func() *wsmsg.GameEventPayload {
			if updated.Event == nil {
				return nil
			}
			return &wsmsg.GameEventPayload{
				ID:          updated.Event.ID,
				GameID:      updated.Event.GameID,
				ActorUserID: updated.Event.ActorUserID,
				EventType:   updated.Event.EventType,
				Body:        updated.Event.Body,
				CreatedAt:   updated.Event.CreatedAt.UTC().Format(time.RFC3339Nano),
			}
		}(),
	})

	if s.botTrigger != nil {
		s.botTrigger(in.GameID)
	}

	return updated, nil
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
	s.broadcastGamePresence(roomID)

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
	s.broadcastGamePresence(roomID)
}

// broadcastGamePresence sends the complete, current set of connected
// viewers' user IDs to every client still in roomID's chat room. Called
// after any change to that room's membership (join, explicit leave, or
// disconnect) so every member's presence list stays accurate -- this is a
// full resend of the set, not a diff, matching game_state_updated's own
// "clients replace state wholesale" convention.
func (s *Server) broadcastGamePresence(roomID string) {
	clientIDs := s.chatRooms[roomID]
	userIDs := make([]string, 0, len(clientIDs))
	for clientID := range clientIDs {
		c, ok := s.clients[clientID]
		if !ok || c.UserID == "" {
			continue
		}
		userIDs = append(userIDs, c.UserID)
	}
	ev := envelope(string(wsmsg.TypeGamePresence), newID("s"), "", roomID, wsmsg.GamePresencePayload{
		GameID:  roomID,
		UserIDs: userIDs,
	})
	for clientID := range clientIDs {
		if c, ok := s.clients[clientID]; ok {
			c.Conn.Send(ev)
		}
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

func (s *Server) broadcastTerritorySelected(roomID string, payload wsmsg.TerritorySelectedPayload) {
	ev := envelope(string(wsmsg.TypeTerritorySelected), newID("s"), "", roomID, payload)
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
	log.Printf("ws: error reply corrID=%s code=%s msg=%q", corrID, code, msg)
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
