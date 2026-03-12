# Global Conquest — System-Level Canonical Patterns

> Generated 2026-03-12. Describes how the system behaves as a whole: how actions travel, how state propagates, and what invariants the architecture enforces. For implementation-level patterns (handler structure, service conventions, test fakes) see `canonical-patterns-impl.md`.

---

## 1. Client → Server Action Flow

### Game action (e.g. attack, reinforce, fortify)

```
GamePage.tsx: user clicks "Roll Dice"
→ sendAction({ action: "attack", from, to, attacker_dice, defender_dice })
→ send("game_action", payload, { game_id })          [realtime/socket.ts:468–475]
→ genId() wraps payload into WsEnvelope{type, id, game_id, payload}
→ JSON.stringify → WebSocket.send()                  [realtime/socket.ts:79–88]

→ wsconn.ReadLoop decodes JSON into wsmsg.Envelope   [wsconn/conn.go:96–104]
→ wsapi.GinHandler: s.Inbox() <- game.Incoming{...} [wsapi/handler.go:74–76]

→ game.Server.Run() dequeues from inbox              [game/server.go:148–166]
→ handleIncoming: validates client is in chat room + has UserID
→ unmarshals GameActionPayload from envelope
→ calls s.actions.ApplyGameAction(ctx, GameActionInput{...})
                                                     [game/server.go:383–393]

→ GamesService.ApplyGameAction:                      [service/game.go:298–499]
    1. WithTxQ: begin DB transaction
    2. GetByIDForUpdate (SELECT ... FOR UPDATE)       → pessimistic row lock
    3. json.Unmarshal game state into risk.Game engine
    4. verify player is in game
    5. apply action on engine (engine.Attack / engine.PlaceReinforcement / etc.)
    6. json.Marshal updated engine → nextState
    7. games.UpdateState (UPDATE games SET state = ...)
    8. gameEvent.SaveGameEvent (INSERT game_events)
    9. commit transaction
    10. return GameActionUpdate{phase, territories, players, result, event}

→ game.Server: build GameStateUpdatedPayload
→ broadcastGameStateUpdate(gameID, payload)          [game/server.go:407–441]
→ send envelope to every client in s.chatRooms[gameID]

→ GamePage.tsx: on("game_state_updated") fires        [GamePage.tsx:127–222]
→ setGame(prev => ({ ...prev, phase, currentPlayer, territories, players }))
→ React re-renders map, player list, event log
```

### Lobby create / join (REST, not WebSocket)

```
LobbyPage.tsx: onCreateGame / onJoinGame
→ createGame() / joinGame()                          [api/games.ts]
→ Axios POST /api/games/ or POST /api/games/:id/join
→ Gin router → Handler.CreateGame / JoinGame         [httpapi/handler.go]
→ GamesService.CreateClassicGame / JoinClassicGame   [service/game.go]
→ DB write (INSERT / UPDATE with FOR UPDATE)
← HTTP 201 / 200 JSON response
→ LobbyPage: loadGames() re-fetches game list via GET /api/games/
```

---

## 2. Server → Client Event Flow

### Game state broadcast

After every successful `game_action`, the hub broadcasts to all members of the game's chat room:

```
game.Server.broadcastGameStateUpdate(gameID, payload):
    clientIDs := s.chatRooms[gameID]          // only subscribed clients receive it
    for each clientID:
        client.Conn.Send(envelope{
            type:    "game_state_updated",
            game_id: gameID,
            payload: GameStateUpdatedPayload{
                game_id, action, actor_user_id,
                phase, current_player, pending_reinforcements,
                occupy (optional),
                players: [{user_id, card_count, eliminated}],
                territories: {TerritoryName: {owner, armies}},
                result (optional, attack dice rolls + losses),
                event (optional, persisted event log entry),
            },
        })
```

`Conn.Send` is non-blocking (buffered channel, capacity 16). If the send buffer is full, the message is silently dropped. `[wsconn/conn.go:85–92]`

The writer goroutine (`wsconn/conn.go:63–75`) drains the channel and calls `wsjson.Write` sequentially, ensuring a single writer per connection.

### Lobby chat broadcast (REST-initiated push)

Lobby chat goes through REST first, then gets pushed over WebSocket:

```
LobbyPage: POST /api/chat/lobby/messages       [api/chat.ts]
→ Handler.PostLobbyMessage → chats.PostLobbyMessage → DB INSERT
→ handler publishes: gameServer.Inbox() <- PublishLobbyChat{message}
                                               [httpapi/handler.go:509–520]
→ game.Server.broadcastLobbyChat(message)
→ sends lobby_chat_message to ALL connected clients (not room-scoped)
                                               [game/server.go:579–584]
→ LobbyPage: on("lobby_chat_message") → upsertMessage()
```

If the WebSocket is disconnected when the message is sent, the HTTP response is still the source of truth. `LobbyPage.tsx:193–196` manually calls `upsertMessage(created)` in that case.

### Game chat broadcast (WebSocket-only)

```
GamePage: send("game_chat_send", {body}, {game_id})
→ hub: validates c.ChatRoom == gameID
→ chatLog.SaveGameMessage → DB INSERT game_chat_messages
→ broadcastGameChatMessage(gameID, {game_id, user_name, body, created_at})
→ only clients in s.chatRooms[gameID] receive "game_chat_message"
→ GamePage: on("game_chat_message") → setChatMessages(prev => [...prev, next])
```

---

## 3. WebSocket Message Lifecycle

### Envelope structure (shared contract)

Both sides use the same envelope shape. Go definition: `wsmsg.Envelope` (`proto/wsmsg/messages.go:36–42`). TypeScript definition: `WsEnvelope` (`realtime/types.ts:3–9`).

```
{
  "type":           string,          // required — message type constant
  "id":             string,          // client-generated UUID (optional but recommended)
  "correlation_id": string,          // server echoes client's id in responses
  "game_id":        string,          // scopes message to a game (optional)
  "payload":        object | null    // type-specific data
}
```

### Send path

```
Frontend send(type, payload, opts):
    id = opts.id ?? genId()            // crypto.randomUUID() with Math.random fallback
    env = { type, id, game_id, payload }
    if ws.readyState === OPEN:
        ws.send(JSON.stringify(env))
    else:
        sendQueueRef.push(raw)         // buffered until reconnect
```

### Receive path (backend)

```
wsconn.ReadLoop: wsjson.Read → wsmsg.Envelope
→ callback: s.Inbox() <- game.Incoming{ClientID, Env}
→ game.Server.Run() (single goroutine) → handleIncoming
→ switch env.Type:
    game_action  → ApplyGameAction → broadcastGameStateUpdate
    join_game    → joinGame → broadcast player_joined + joined_game snapshot
    leave_game   → leaveGame → broadcast player_left
    game_chat_*  → chat room management + message broadcast
    list_games   → unicast game_list to requester only
    ping         → unicast pong
    unknown      → unicast ack
```

### Error response shape

Any validation failure produces a typed error envelope sent only to the originating client:

```
{ "type": "error", "correlation_id": "<client id>", "payload": { "code": "...", "message": "..." } }
```

Error codes in use: `invalid_message`, `already_in_game`, `not_in_game`, `game_not_found`, `game_full`, `not_in_room`, `unauthorized`, `invalid_action`, `not_configured`. `[game/server.go:643–648]`

Frontend `GamePage.tsx:224–234` subscribes to `"error"` and surfaces `invalid_action`, `unauthorized`, and `not_in_room` as `actionError` state.

---

## 4. Game State Update Flow

### Persistence model

The entire `risk.Game` struct is serialized as a single JSON blob and stored in `games.state` (PostgreSQL JSONB column). There are no normalized territory or player rows. Every action is a full read-modify-write cycle on that blob.

### Transaction boundary

```
ApplyGameAction (service/game.go:298):
    WithTxQ:
        SELECT ... FROM games WHERE id = $1 FOR UPDATE   ← row-level lock
        json.Unmarshal(g.State, &engine)
        engine.<Action>(...)                             ← pure in-memory mutation
        nextState = json.Marshal(engine)
        UPDATE games SET state = $nextState, status = 'in_progress'
        INSERT INTO game_events (...)
    COMMIT
```

The `FOR UPDATE` lock prevents two concurrent actions from racing on the same game. No optimistic concurrency / versioning is used.

### What the broadcast includes vs. what it omits

`game_state_updated` carries:
- Full `territories` map (`{TerritoryName: {owner: int, armies: int}}`)
- All players with `card_count` and `eliminated` flag (but **not** card identities)
- `phase`, `current_player`, `pending_reinforcements`
- Optional `occupy` constraint (after a conquest)
- Optional `result` (attack dice rolls and losses)
- Optional `event` (persisted game event log entry)

It does **not** carry player names or colors. The frontend preserves those from the bootstrap snapshot and merges them with each update:

```typescript
// GamePage.tsx:162–184
setGame((prev) => {
    const metaByID = new Map(prev.players.map((p) => [p.userId, p]));
    const nextPlayers = incomingPlayers.map((p, idx) => {
        const meta = metaByID.get(p.userId);
        return { ...p, userName: meta?.userName || p.userId, color: meta?.color || ... };
    });
    return { ...prev, phase, currentPlayer, territories, players: nextPlayers };
});
```

---

## 5. Reconnect / Resync Pattern

### Automatic reconnect

The frontend socket (`realtime/socket.ts:135–147`) reconnects automatically on close unless the user explicitly called `disconnect()`. Backoff: `250ms * 2^attempt`, capped at 8 seconds.

```
ws.onclose:
    if closedByUserRef.current: return   // intentional close, no reconnect
    delay = min(250 * 2^attempt, 8000)
    setTimeout(() => connect(), delay)
```

On reconnect, queued messages are flushed (`flushQueue`, line 69–77).

### Game state resync

There is no server-push resync. The client is responsible for re-fetching state:

```
GamePage.tsx:
    1. On mount: getGameBootstrap(gameID) via REST GET /api/games/:id/bootstrap
       → sets full initial game state (phase, territories, players, events)

    2. useEffect on wsStatus:                         [GamePage.tsx:262–268]
       if wsStatus === "connected":
           send("game_chat_join", undefined, { game_id: gameID })
           // on cleanup: send("game_chat_leave")

    3. server joinChatRoom: sends game_chat_history   [game/server.go:466–483]
       → last 200 game chat messages delivered to the rejoining client

    4. Subsequent game_state_updated messages update incremental state
```

The bootstrap (`GET /api/games/:id/bootstrap`) always reads from the database, so it reflects the canonical server state regardless of what WebSocket messages were missed.

### Lobby resync

The lobby has no WebSocket-pushed game list. After create/join operations, `LobbyPage.tsx:152,172` explicitly calls `loadGames()` (REST GET /api/games/) to refresh the list.

---

## 6. Shared Data Structures

### `wsmsg.Envelope` / `WsEnvelope`

The fundamental transport wrapper. Go: `proto/wsmsg/messages.go:36–51`. TypeScript: `realtime/types.ts:3–9`. Both sides must agree on field names (`type`, `id`, `correlation_id`, `game_id`, `payload`).

### `GameStateUpdatedPayload`

The primary game update message. Go definition: `proto/wsmsg/messages.go:160–172`. Consumed by `GamePage.tsx:128–222`.

```go
type GameStateUpdatedPayload struct {
    GameID                string
    Action                string                   // "attack", "place_reinforcement", etc.
    ActorUserID           string
    Phase                 string                   // "reinforce", "attack", "occupy", "fortify", "game_over"
    CurrentPlayer         int
    PendingReinforcements int
    Occupy                *GameOccupyRequirement   // non-nil only during occupy phase
    Players               []GameStatePlayerPayload // {user_id, card_count, eliminated}
    Territories           json.RawMessage          // {TerritoryName: {owner: int, armies: int}}
    Result                any                      // attack dice result, nil otherwise
    Event                 *GameEventPayload        // persisted event log entry, nil if none
}
```

### `GameBootstrap`

The REST response for initial page load. Go: `service/game.go:65–78`. TypeScript: `api/games.ts`. Contains everything `GameStateUpdatedPayload` carries, plus player names, colors, and game event history.

### `GameActionPayload`

Client sends this to execute any game action. Go: `proto/wsmsg/messages.go:144–152`.

```go
type GameActionPayload struct {
    Action       string  // "place_reinforcement", "attack", "occupy", "end_attack", "fortify", "end_turn"
    Territory    string  // reinforce target
    From         string  // attack/fortify source
    To           string  // attack/fortify destination
    Armies       int
    AttackerDice int
    DefenderDice int
}
```

### `GameChatLogMessage` / `wsmsg.GameChatMessagePayload`

In-game chat message. Stored in `game_chat_messages` table; broadcast as `game_chat_message` envelope.

---

## 7. Cross-System Invariants

### The server is the sole source of truth for game state

The client never computes game outcomes. It sends intent (`{action: "attack", from, to, attacker_dice}`) and receives the resolved result back. Dice rolls, army loss calculation, territory conquest, card assignment, turn transitions, and victory detection all happen exclusively in `risk.Game` methods on the server. `[service/game.go:335–442, backend/internal/risk/engine.go]`

### All state mutations go through a DB transaction with a row-level lock

`ApplyGameAction` and `JoinClassicGame` both use `GetByIDForUpdate` inside `WithTxQ`. No action is applied to an in-memory engine without first acquiring the row lock. This prevents concurrent requests from corrupting state. `[service/game.go:169–170, 305–306]`

### A client must join the game chat room before receiving game state updates or sending game actions

`broadcastGameStateUpdate` and `broadcastGameChatMessage` both send only to `s.chatRooms[gameID]`. `handleIncoming` for `game_action` checks `c.ChatRoom != gameID` before processing. `[game/server.go:364–366, 596–603]`

The frontend enforces this: on WebSocket connect, `GamePage.tsx:262–268` immediately sends `game_chat_join`.

### Authenticated identity is required for game actions; anonymous connections can observe but not act

`wsapi.GinHandler` reads `?token=` and sets `cl.UserID` if valid. `handleIncoming` rejects `game_action` if `c.UserID == ""`. `[wsapi/handler.go:34–38, game/server.go:369–372]`

WebSocket authentication does not fail the connection upgrade — an unauthenticated client still connects as `anon` and can receive broadcasts, but cannot submit game actions.

### The game list and lobby state are not pushed over WebSocket

The game list is only available via `GET /api/games/` (REST). The lobby page polls it manually after create/join operations. There is no `game_list_updated` push event. The `list_games` WebSocket message returns the hub's in-memory game map (lobby-only games), not the database.

### Lobby chat delivery is at-most-once

Lobby chat messages are persisted to the DB before broadcasting, but WebSocket delivery is non-blocking (buffered channel). If a client's send buffer is full (`cap=16`), the message is dropped silently. There is no redelivery mechanism. `[wsconn/conn.go:85–92]`

### Game state broadcast is scoped to the game's chat room, lobby chat is global

`broadcastGameStateUpdate` / `broadcastGameChatMessage` iterate `s.chatRooms[gameID]`. `broadcastLobbyChat` iterates `s.clients` — all connected clients. `[game/server.go:579–603]`

### The game engine's phase drives all action legality

The server enforces whose turn it is and which actions are legal for the current phase. The frontend reads `phase` and `currentPlayer` from state updates to show/hide controls, but these are display-only gates. The backend will reject an out-of-turn or wrong-phase action regardless of what the UI allows. `[service/game.go:335–442, risk/engine.go]`
