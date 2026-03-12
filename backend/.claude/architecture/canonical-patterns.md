# Canonical Backend Implementation Patterns

> Reference for AI assistants generating code in this repository.
> All patterns derived from existing code — no theoretical best practices.

---

## 1. WebSocket Handler Pattern

**Description:** WebSocket connections are accepted by the HTTP layer, upgraded, and handed off to the game hub. A read loop in the handler goroutine forwards all incoming messages into a central inbox channel.

**Canonical files:**
- `internal/wsapi/handler.go` — HTTP→WebSocket upgrade and client registration
- `internal/wsconn/conn.go` — per-connection read/write/ping goroutines
- `internal/game/server.go` — message hub, single-goroutine `Run()` loop

**Pattern:**
1. Extract session token from query param and authenticate in `wsapi.GinHandler`
2. Upgrade HTTP to WebSocket via `wsconn.Accept()`
3. Construct `game.Client{ID, UserID, Name, Conn}` and push `game.Register{C: cl}` into `s.Inbox()`
4. Block in `c.Conn.ReadLoop()`, forwarding each decoded `wsmsg.Envelope` as `game.Incoming{ClientID, Env}` into `s.Inbox()`
5. On exit (error or close), push `game.Unregister{ClientID: c.ID}` into inbox and close connection

**Why these files:** `wsapi/handler.go` owns the full lifecycle from HTTP upgrade to hub registration. `wsconn/conn.go` encapsulates the three goroutines (read, write, ping) behind a clean `Send()` method. `game/server.go` is the single consumer of all client messages.

---

## 2. Game Action Processing

**Description:** Incoming `game_action` WebSocket messages are validated in the hub, passed through a thin service adapter, and applied transactionally to the game engine.

**Canonical files:**
- `internal/game/server.go` — `handleIncoming()`, dispatch on `env.Type`
- `internal/service/game_action.go` — `GameActionService.ApplyGameAction()` (thin adapter)
- `internal/service/game.go` — `GamesService.ApplyGameAction()` (authoritative ~200 lines)
- `internal/risk/engine.go` — pure game-rule execution

**Pattern:**
1. Hub receives `game.Incoming`, switches on `env.Type == "game_action"`
2. Validate: client exists, `GameID` present, player is in the room
3. Unmarshal `wsmsg.GameActionPayload{Action, Args}`
4. Call `s.actions.ApplyGameAction(ctx, GameActionInput{...})`
5. Inside a DB transaction:
   - `SELECT … FOR UPDATE` the game row
   - Assert status `"in_progress"`, player belongs to game
   - `json.Unmarshal(state) → risk.Game`
   - Switch on `in.Action` → call engine method
   - `json.Marshal(updatedGame)` → write back to DB
   - Optionally persist a `game_events` row
6. Return `GameActionUpdate`; hub calls `broadcastGameStateUpdate()`

**Supported action names:** `place_reinforcement`, `attack`, `occupy`, `end_attack`, `fortify`, `end_turn`

**Why these files:** `service/game.go:ApplyGameAction` is the single authoritative location for the full action lifecycle. `risk/engine.go` is stateless — it receives a `risk.Game`, mutates it, and returns an error if the move is illegal. Nothing else should replicate this logic.

---

## 3. Game State Mutation

**Description:** Game state lives as a `JSONB` column in Postgres. Mutations follow a strict read-lock-mutate-write cycle inside a transaction.

**Canonical files:**
- `internal/service/game.go` — all state transitions
- `internal/store/game.go` — `GetByIDForUpdate`, `UpdateState`
- `internal/risk/engine.go` — stateless mutator methods

**Pattern:**
```
s.db.WithTxQ(ctx, func(q db.Querier) error {
    g  := store.GetByIDForUpdate(ctx, q, gameID)  // SELECT … FOR UPDATE
    json.Unmarshal(g.State, &riskGame)
    updatedGame, err := engine.SomeAction(riskGame, args)
    if err != nil { return mapGameActionErr(err) }
    newState, _ := json.Marshal(updatedGame)
    store.UpdateState(ctx, q, UpdateGameStateInput{ID, Status, State: newState})
    return nil  // auto-commit
})
```

**State shapes:**
- Lobby: `lobbyState{PlayerCount int, PlayerIDs []string}`
- In-progress: `risk.Game` (board, players, territories, phase, deck, etc.)

**Why these files:** `service/game.go` is the only place that calls `store.GetByIDForUpdate`. Bypassing it by calling `GetByID` (non-locking) would allow race conditions under concurrent player actions.

---

## 4. Event Broadcast Pattern

**Description:** After a state change, the hub iterates the set of clients in a room and delivers a typed `wsmsg.Envelope` through each connection's send channel.

**Canonical files:**
- `internal/game/server.go` — `broadcastGameStateUpdate`, `broadcastGameChatMessage`, `broadcastLobbyChat`
- `internal/proto/wsmsg/messages.go` — `Envelope` and payload types
- `internal/wsconn/conn.go` — `Conn.Send()` (non-blocking, buffered channel size 16)

**Pattern:**
```go
func (s *Server) broadcastGameStateUpdate(roomID string, payload wsmsg.GameStateUpdatedPayload) {
    for clientID := range s.chatRooms[roomID] {
        s.clients[clientID].Conn.Send(
            envelope("game_state_updated", newID("s"), "", roomID, payload),
        )
    }
}
```

**Envelope fields:**
- `Type` — message type (e.g. `"game_state_updated"`, `"error"`)
- `ID` — unique server-generated ID (`newID("s")`)
- `CorrelationID` — links server response back to originating client request
- `GameID` — scopes to a specific game room
- `Payload json.RawMessage` — arbitrary typed payload

**Why these files:** All broadcast helpers are methods on `game.Server` so they run inside the hub's single goroutine, making iteration over `s.chatRooms` safe without locks.

---

## 5. Validation Pattern

**Description:** Validation is layered across three boundaries. Each layer owns distinct concerns and uses its own error types.

**Canonical files:**
- `internal/game/server.go` — Layer 1: transport checks
- `internal/service/game.go` — Layer 2: business-rule checks
- `internal/risk/engine.go` — Layer 3: game-rule checks + error mapping in `service/game.go`

**Layer 1 — Hub (transport):**
- Client is authenticated and registered
- Required envelope fields present (`GameID`, parseable payload)
- Client is in the target chat room (member of `s.chatRooms[gameID]`)

**Layer 2 — Service (business rules):**
- Game exists and has correct status (`"lobby"` or `"in_progress"`)
- Player belongs to the game
- Lobby not full, player not already joined, player count valid (3–6)

**Layer 3 — Engine (game rules):**
- Correct player's turn → `risk.ErrOutOfTurn`
- Correct phase → `risk.ErrInvalidPhase`
- Legal move (territory reachable, enough armies, etc.) → `risk.ErrInvalidMove`

**Error mapping from engine to service:**
```go
func mapGameActionErr(err error) error {
    switch {
    case errors.Is(err, risk.ErrOutOfTurn),
         errors.Is(err, risk.ErrInvalidMove),
         errors.Is(err, risk.ErrInvalidPhase):
        return ErrInvalidGameAction
    }
    return err
}
```

**WebSocket error response:**
```go
c.Conn.Send(errEnv(env.CorrelationID, "invalid_action", err.Error()))
// → Envelope{Type: "error", Payload: {code, message}}
```

**Why these files:** Each layer rejects requests with the minimum information it has. The hub never calls the engine directly; the service never reaches into WebSocket connection state.

---

## 6. Persistence Pattern

**Description:** All persistence uses raw `pgx` SQL through a store layer. Every store method accepts a `db.Querier` parameter so it works identically inside or outside a transaction.

**Canonical files:**
- `internal/db/transaction.go` — `WithTxQ` wrapper
- `internal/db/querier.go` — `Querier` interface (`QueryRow`, `Query`)
- `internal/store/game.go` — `PostgresGamesStore`
- `internal/store/game_event.go` — `PostgresGameEventStore`
- `internal/store/user.go` — `PostgresUsersStore`

**Transaction wrapper:**
```go
// db/transaction.go
func (d *DB) WithTxQ(ctx context.Context, fn func(q db.Querier) error) error {
    tx, _ := d.pool.Begin(ctx)
    defer tx.Rollback(ctx)   // always runs; no-op after Commit
    if err := fn(tx); err != nil {
        return err
    }
    return tx.Commit(ctx)
}
```

**Store method signature convention:**
```go
func (s *PostgresGamesStore) GetByIDForUpdate(ctx context.Context, q db.Querier, id string) (Game, error)
func (s *PostgresGamesStore) UpdateState(ctx context.Context, q db.Querier, in UpdateGameStateInput) (Game, error)
```

**SQL conventions:**
- SQL is a `const` string per method — no string building, no ORMs
- `id::text` cast in SQL (pgx cannot scan `pgtype.UUID` directly into `string`)
- `RETURNING` clause on every INSERT/UPDATE — no separate SELECT after write
- `pgx.ErrNoRows` is never translated in the store; the service layer handles it

**Game event persistence** is written in the same transaction as the state update, keeping state and event history consistent.

---

## 7. Concurrency / Synchronization Pattern

**Description:** Race conditions are prevented by two mechanisms: a single-goroutine message hub for all in-memory state, and DB row locks for persistent state.

**Canonical files:**
- `internal/game/server.go` — hub inbox channel, `Run()` loop
- `internal/db/transaction.go` — `WithTxQ`
- `internal/store/game.go` — `GetByIDForUpdate` (`SELECT … FOR UPDATE`)
- `internal/wsconn/conn.go` — per-connection goroutines

**Hub (in-memory):**
```go
func (s *Server) Run() {
    for msg := range s.inbox {   // single goroutine — no mutexes needed
        switch m := msg.(type) {
        case Register:   …
        case Unregister: …
        case Incoming:   …
        }
    }
}
```
`s.clients` and `s.chatRooms` are only ever accessed inside `Run()`. The hub's `inbox` channel is buffered (size 256).

**DB concurrency:**
- `SELECT … FOR UPDATE` serializes concurrent game actions at the Postgres row level
- `WithTxQ` auto-rollback on error prevents partial writes

**Per-connection goroutines:**
- Read loop — runs in HTTP handler goroutine
- Write loop — consumes `send` channel (buffered 16); times out on slow clients
- Ping loop — 20-second ticker; closes connection on timeout

**Why:** No mutexes appear on the hot game-state path. The hub owns in-memory state; Postgres owns durable state. The combination provides correct serialization at both levels without explicit locking in Go.

---

## 8. Error Handling Pattern

**Description:** Errors are package-level sentinel `var`s. Each layer translates errors from the layer below rather than leaking internal types upward. WebSocket errors become typed envelopes; HTTP errors become status codes.

**Canonical files:**
- `internal/auth/errors.go` — auth sentinel errors
- `internal/service/game.go` — service sentinel errors + `mapGameActionErr`
- `internal/risk/engine.go` — engine sentinel errors
- `internal/httpapi/handler.go` — HTTP error mapping
- `internal/game/server.go` — `errEnv()` helper

**Sentinel error declaration (all layers):**
```go
// auth/errors.go
var ErrInvalidSession            = errors.New("invalid or expired session")
var ErrInvalidUsernameOrPassword = errors.New("invalid username or password")

// service/game.go
var ErrGameNotFound      = errors.New("game not found")
var ErrInvalidGameAction = errors.New("invalid game action")
var ErrGameForbidden     = errors.New("forbidden")

// risk/engine.go
var ErrOutOfTurn    = errors.New("out of turn")
var ErrInvalidPhase = errors.New("invalid phase")
var ErrInvalidMove  = errors.New("invalid move")
```

**Error ownership by layer:**

| Layer | Responsibility |
|---|---|
| `store/` | Returns raw pgx errors — never translates |
| `service/` | Translates pgx errors into domain sentinels; declares business-rule sentinels |
| `httpapi/` | Maps domain sentinels → HTTP codes via `errors.Is` switch |
| `game/` (hub) | Maps service errors → `wsmsg.Envelope{Type: "error"}` via `errEnv()` |

**HTTP mapping:**
```go
switch {
case errors.Is(err, auth.ErrPasswordTooShort): c.JSON(400, gin.H{"error": err.Error()})
case errors.Is(err, service.ErrUsernameTaken):  c.JSON(409, gin.H{"error": err.Error()})
default:                                         c.JSON(500, gin.H{"error": "failed to create user"})
}
```

**WebSocket error envelope:**
```go
func errEnv(corrID, code, msg string) wsmsg.Envelope {
    return envelope("error", newID("s"), corrID, "", map[string]any{
        "code": code, "message": msg,
    })
}
```

**Rules:**
- `500` responses never include internal error strings — always a safe generic message
- `errors.Is` used for all comparisons — never string matching
- Stores return raw errors; services translate; handlers map to transport

---

## 9. Testing Pattern

**Description:** Unit tests use hand-written fakes with function fields. Integration tests spin up a real HTTP server and connect via WebSocket using the `coder/websocket` library.

**Canonical files:**
- `internal/service/game_test.go` — unit tests with store/DB fakes
- `internal/httpapi/ws_integration_test.go` — WebSocket integration tests
- `internal/store/game_test.go` — store-level tests

**Unit test fake pattern:**
```go
type fakeGamesStore struct {
    createFn      func(context.Context, db.Querier, store.NewGame) (store.Game, error)
    getByIDFn     func(context.Context, db.Querier, string) (store.Game, error)
    updateStateFn func(context.Context, db.Querier, store.UpdateGameStateInput) (store.Game, error)
}
func (f *fakeGamesStore) Create(ctx context.Context, q db.Querier, in store.NewGame) (store.Game, error) {
    return f.createFn(ctx, q, in)
}

type fakeDB struct{ q db.Querier }
func (f *fakeDB) Queryer() db.Querier                                      { return f.q }
func (f *fakeDB) WithTxQ(ctx context.Context, fn func(db.Querier) error) error {
    return fn(f.q)
}
```

**WebSocket integration test pattern:**
```go
func TestSomeAction(t *testing.T) {
    g := game.NewServer()
    go g.Run()
    router := NewRouter(NewHandler(g, fakeSvc, fakeGames, fakeChat))
    base := startTestHTTPServer(t, router)

    conn, _, _ := websocket.Dial(ctx, "ws://"+base+"/ws?token=...", nil)
    defer conn.Close(websocket.StatusNormalClosure, "")

    wsjson.Write(ctx, conn, wsmsg.Envelope{Type: "game_action", Payload: ...})
    env := readUntilType(t, conn, "game_state_updated", 5)
    // assert env.Payload
}
```

**Available test helpers (`ws_integration_test.go`):**
- `startTestHTTPServer(t, handler) string` — returns base URL of real server
- `mustReadEnvelope(t, conn) wsmsg.Envelope`
- `readUntilType(t, conn, typ, maxMessages) wsmsg.Envelope`

**Rules:**
- No mock libraries — all fakes are hand-written structs with function fields
- `t.Fatalf` for all assertions (halt on first failure, not `t.Errorf`)
- Integration tests use the real `NewRouter(h)` so middleware runs against real code
- Fake fields left `nil` for methods not under test — panics on unexpected calls make missing wiring obvious
- `fakeDB.WithTxQ` passes `f.q` directly into `fn`; tests assert the querier is threaded through correctly
