# Global Conquest — Architecture & Codebase Analysis

## Overview

**Global Conquest** is a full-stack, real-time multiplayer web game — a browser-based implementation of the classic board game Risk. It supports 3–6 players per game, live game state synchronization via WebSocket, in-game and lobby chat, and an admin interface for user management.

The codebase lives in two directories:

| Directory | Role |
|---|---|
| `backend/` | Go HTTP/WebSocket server, game engine, PostgreSQL access |
| `global-conquest/` | React/TypeScript single-page application |

---

## Technology Stack

### Backend
| Concern | Choice |
|---|---|
| Language | Go 1.24 |
| HTTP framework | Gin v1.11 |
| WebSocket | `github.com/coder/websocket` |
| Database | PostgreSQL via `pgx/v5` |
| Password hashing | Argon2id |
| API docs | Swagger (swaggo) |

### Frontend
| Concern | Choice |
|---|---|
| Framework | React 19 |
| Routing | TanStack Router v1.163 |
| HTTP client | Axios |
| Styling | Tailwind CSS v4 |
| Build tool | Vite 7 |
| Language | TypeScript 5.9 |

---

## Repository Structure

```
global-conquest/
├── backend/
│   ├── cmd/
│   │   ├── backend/main.go        # Server entry point
│   │   └── seed/main.go           # DB seeding utility
│   ├── internal/
│   │   ├── auth/                  # Argon2id + token hashing
│   │   ├── db/                    # DB pool, Querier interface, transactions
│   │   ├── game/                  # WebSocket hub / message broker
│   │   ├── httpapi/               # REST handlers + router
│   │   ├── proto/wsmsg/           # WebSocket message protocol types
│   │   ├── risk/                  # Risk game engine (rules, state machine)
│   │   ├── service/               # Business logic services
│   │   ├── store/                 # PostgreSQL data access objects
│   │   ├── wsapi/                 # WebSocket API handler
│   │   └── wsconn/                # WebSocket connection management
│   ├── migrations/                # V1–V6 SQL migration files
│   ├── docs/                      # Swagger/OpenAPI output
│   ├── docker-compose.yml         # PostgreSQL dev container
│   ├── go.mod
│   └── Makefile
│
└── global-conquest/               # Frontend
    └── src/
        ├── api/                   # Axios REST client modules
        ├── auth/                  # Auth context + session management
        ├── realtime/              # WebSocket client + SocketContext
        ├── router/                # Route definitions, pages, layouts
        ├── App.tsx
        └── main.tsx
```

---

## Backend Architecture

### Layered Design

```
┌─────────────────────────────────────────────┐
│  HTTP/WebSocket Handlers (httpapi, wsapi)    │
├─────────────────────────────────────────────┤
│  Services (game, users, chat, game-chat)     │
├─────────────────────────────────────────────┤
│  Stores (PostgresGamesStore, UsersStore …)   │
├─────────────────────────────────────────────┤
│  Database (pgxpool + Querier interface)      │
└─────────────────────────────────────────────┘
```

Handlers validate requests and delegate to services. Services own business logic and orchestrate across stores. Stores own SQL and accept a `db.Querier` interface so the same code works in a transaction or with a plain pool.

---

### Game Server — Hub / Actor Pattern

`internal/game/server.go` is the heart of real-time multiplayer. It runs a **single goroutine** that processes messages from an inbox channel, eliminating the need for explicit locking:

```
Clients (goroutines)
    │  send to inbox channel
    ▼
Game Server goroutine
    ├── Registers / unregisters WebSocket clients
    ├── Routes game actions → GameActionService
    ├── Manages pub/sub rooms (lobby chat, game chat)
    └── Broadcasts game_state_updated to all players
```

Message types the hub handles:
- `Register` / `Unregister` — client connects or disconnects
- `Incoming` — any message from a client (create_game, join_game, game_action, …)
- `PublishLobbyChat` — fan-out a chat message

---

### Risk Game Engine

`internal/risk/engine.go` + `board.go` implement all game rules as pure functions that mutate a `risk.Game` struct. No I/O happens here — the engine is isolated and easily unit-tested.

**Game Phases (state machine)**

```
PhaseSetupClaim
    → PhaseSetupReinforce
        → PhaseReinforce
            → PhaseAttack
                → PhaseOccupy  (when territory is conquered)
                    → PhaseAttack
                → PhaseFortify
                    → PhaseReinforce (next player's turn)
                        → PhaseGameOver (one player holds all territories)
```

**Key types**

```go
type Game struct {
    Board                 Board
    Players               []PlayerState
    Territories           map[Territory]TerritoryState
    CurrentPlayer         int
    Phase                 Phase
    Winner                string
    SetupReserves         map[int]int
    PendingReinforcements int
    ConqueredThisTurn     bool
    HasFortified          bool
    Occupy                *OccupyState
    Deck                  []Card
    Discard               []Card
    SetsTraded            int
}
```

**Territory system**
- 42 named territories across 6 continents
- Adjacency matrix (bidirectional) enforces legal attack/fortify paths
- Continent bonuses: NA=5, SA=2, EU=5, AF=3, AS=7, AU=2

**Combat**
- Attacker rolls up to 3 dice, defender up to 2
- Dice compared in descending order; ties go to defender
- `AttackResult` carries dice values and army losses

**Cards**
- 44-card deck (42 territories + 2 wilds); symbols: Infantry, Cavalry, Artillery
- Players exchange sets of 3 matching or mixed for bonus armies
- Deck reshuffled from discard when exhausted

---

### HTTP REST API

Base path: `/api`

| Method | Path | Auth | Purpose |
|---|---|---|---|
| POST | `/auth/login` | — | Sign in, receive session token |
| POST | `/users` | — | Register new user |
| GET | `/users` | ✓ | List users |
| GET | `/users/:username` | ✓ | Get user profile |
| GET/POST | `/games` | ✓ | List / create games |
| POST | `/games/:id/join` | ✓ | Join a lobby |
| GET | `/games/:id` | ✓ | Get game metadata |
| GET | `/games/:id/bootstrap` | ✓ | Full game state for initial load |
| PUT | `/games/:id/state` | ✓ | Apply game action (REST fallback) |
| GET/POST | `/chat/lobby/messages` | ✓ | Lobby chat history / send |
| GET | `/admin/users` | admin | List all users |
| PUT | `/admin/users/:id/access` | admin | Block / unblock user |
| POST | `/admin/users/:id/revoke-sessions` | admin | Force logout |

---

### WebSocket Protocol

**Endpoint:** `GET /ws?token=<session_token>`

All messages use a JSON envelope:

```json
{
  "type": "string",
  "id": "uuid",
  "correlation_id": "uuid",
  "game_id": "uuid",
  "payload": {}
}
```

**Client → Server**

| Type | Purpose |
|---|---|
| `create_game` | Create new game in lobby |
| `join_game` / `leave_game` | Lobby membership |
| `lobby_typing_start` / `lobby_typing_stop` | Typing presence |
| `game_chat_join` / `game_chat_leave` | Subscribe to game chat room |
| `game_chat_send` | Send in-game chat message |
| `game_action` | Execute game move (claim, reinforce, attack, fortify, …) |

**Server → Client**

| Type | Purpose |
|---|---|
| `hello` | Acknowledge connection with client_id |
| `game_created` | New game available in lobby |
| `joined_game` / `player_joined` / `player_left` | Membership events |
| `lobby_typing_state` | Updated typing-users list |
| `lobby_chat_message` / `game_chat_message` | Chat messages |
| `game_state_updated` | Full game state snapshot after any action |
| `error` | Error response |

---

### Authentication & Authorization

**Registration**
1. Validate: username 3–24 chars alphanumeric + `_-`; password 8+ chars
2. Hash password with Argon2id (64 MiB, 3 iterations, 2 parallelism)
3. Store user with `role: "player"` and `access_status: "active"`

**Login**
1. Retrieve user by username; verify password (constant-time comparison)
2. Generate 32 random bytes as session token
3. Store SHA256(token) in `sessions` table; send raw token to client
4. Token expires in 30 days

**Per-request auth**
- Client sends `Authorization: Bearer <token>`
- Middleware hashes it and checks `sessions` table; user is attached to context
- Admin routes additionally require `role: "admin"`

---

### Data Model

**Database tables** (migrations V1–V6):

| Table | Purpose |
|---|---|
| `users` | Accounts — UUID PK, unique username, Argon2id hash, role, access_status |
| `sessions` | Active sessions — token_hash (SHA256), expires_at |
| `games` | Game records — status enum (`lobby`/`in_progress`/`completed`), JSONB state |
| `chat_messages` | Lobby chat |
| `game_chat_messages` | Per-game chat |
| `game_events` | Event log per game |

Game state is stored as **JSONB** — this avoids relational schema churn as the Risk engine state evolves and keeps the data model simple.

---

## Frontend Architecture

### Component Tree

```
main.tsx
└── App
    ├── AuthProvider          (context: current user + token)
    ├── SocketProvider        (context: WebSocket client instance)
    └── RouterProvider
        └── RootLayout
            ├── /login        → LoginPage
            ├── /signup       → SignupPage
            └── (protected)   → AppShell
                ├── /         → LobbyPage
                ├── /game/:id → GamePage
                ├── /profile  → ProfilePage
                └── /admin    → AdminPage
```

Route guards (`beforeLoad`) redirect unauthenticated users to `/login`.

---

### Key Patterns

**Context API for global state**
- `AuthContext` — current user, session token, `login()` / `logout()` helpers; token persisted in `localStorage`
- `SocketContext` — shared `SocketClient` instance; one WebSocket per browser tab

**Custom hooks**
- `useAuth()` — access auth context
- `useSocket()` — access socket client
- `useGameSocket(wsUrl)` — create a WebSocket connection with auto-reconnect

**Typed API client** (`src/api/client.ts`)
- Axios instance with `Authorization` header auto-injected
- Custom `ApiError` type (status, message, details)
- Normalization functions convert raw responses to typed domain objects

---

### WebSocket Client (`src/realtime/socket.ts`)

Features:
- **Exponential backoff reconnection**: `250ms × 2^attempt`, capped at 8s
- **Message queue**: messages sent while disconnected are buffered and flushed on reconnect
- **Event-based API**: `on(type, handler)` / `emit(envelope)`
- **Connection status**: `"disconnected"` → `"connecting"` → `"connected"`

---

### Game Board (`src/router/pages/GamePage.tsx`)

The game board is an **SVG overlay** rendered on top of the official Risk board image:

- All 42 territories have pre-defined `(x, y)` pixel coordinates
- Territory nodes rendered as colored circles with army counts
- Click interactions drive game actions (claim, select attacker/defender, fortify)
- Player colors: 6 distinct colors mapped by player index
- Game state arrives via `game_state_updated` WebSocket events and is stored in local React state

---

## Game Action Flow

```
User clicks territory on board
    │
    ▼
GamePage.tsx dispatches game_action via SocketClient
    │
    ▼
backend/wsapi/handler.go receives envelope
    │
    ▼
game.Server routes to GameActionService.ApplyGameAction()
    │
    ▼
risk.Engine mutates Game struct (validate + apply rules)
    │
    ▼
GamesService persists new state to PostgreSQL (JSONB)
    │
    ▼
game.Server broadcasts game_state_updated to all players in game
    │
    ▼
All connected GamePage clients receive updated state and re-render
```

---

## Real-Time Features Summary

| Feature | Mechanism |
|---|---|
| Game state sync | Full state snapshot broadcast on every action |
| Lobby chat | Persisted to DB; broadcast to lobby subscribers |
| Game chat | Persisted to DB; broadcast to game room subscribers |
| Typing indicators | Ephemeral; server tracks per-user, broadcasts state; expires 1.5s after last event |
| Reconnection | Client-side backoff queue; server re-registers client on connect |

---

## Security Notes

| Area | Implementation |
|---|---|
| Passwords | Argon2id (memory-hard, OWASP recommended) |
| Session tokens | 32-byte CSPRNG; SHA256 hash stored in DB |
| SQL injection | Parameterized queries via pgx |
| Role authorization | Middleware checks role on admin routes |
| WebSocket origin | Origin patterns restrict to localhost in dev config |

---

## Development Setup

**Backend**
```bash
# Start PostgreSQL
docker compose up -d

# Run migrations (Flyway-style V*.sql files applied on startup)
# Start server (listens :8080)
go run ./cmd/backend/main.go

# Seed test data (users: test_alice, test_bob …; password: "password")
go run ./cmd/seed/main.go
```

**Frontend**
```bash
cd global-conquest
npm install
npm run dev   # Vite dev server; /api and /ws proxied to :8080
```

**Environment variables (backend)**

| Variable | Default |
|---|---|
| `DB_HOST` | `localhost` |
| `DB_PORT` | `5432` |
| `DB_USER` | `globalconq` |
| `DB_PASSWORD` | `globalconq` |
| `DB_NAME` | `globalconq` |

---

## Testing

| Layer | Location |
|---|---|
| Auth helpers | `internal/auth/*_test.go` |
| Risk engine | `internal/risk/engine_test.go` |
| Services | `internal/service/*_test.go` |
| Stores | `internal/store/*_test.go` |
| HTTP API | `internal/httpapi/*_test.go` |
| End-to-end | `internal/e2e/` |

---

## Key Design Decisions

### Backend

1. **Game state as JSONB** — Keeps the relational schema stable as the Risk engine evolves. Trade-off: no relational querying of game internals.
2. **Single-goroutine game server** — Sequential message processing avoids locking. Scales to many games since each game action is fast.
3. **Argon2id** — Memory-hard; tuned for a hobby server (64 MiB). More resistant to GPU cracking than bcrypt.
4. **Store interfaces accept `db.Querier`** — Same store code runs in a transaction or a plain pool. Simplifies transactional writes across multiple stores.

### Frontend

1. **Context API instead of Redux/Zustand** — Sufficient for two global concerns (auth, socket). Avoids added complexity.
2. **SVG board rendering** — Lightweight, interactive without a canvas abstraction layer; territory coordinates are just data.
3. **Full-state snapshots on each update** — Simpler than differential patches; game state is small enough (42 territories, 6 players) that this is not a performance concern.
4. **TanStack Router** — Type-safe routes with `beforeLoad` guards make auth protection declarative.
