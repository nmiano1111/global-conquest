# Global Conquest — System Architecture Overview

> Generated 2026-03-11. Intended to give AI assistants a fast, accurate mental model of this codebase.

---

## 1. System Purpose

**Global Conquest** is a real-time, browser-based multiplayer adaptation of the board game Risk. It supports 3–6 players per game with live state synchronization, a Risk-compliant game engine, in-game and lobby chat, and an admin interface for user management.

**Major capabilities:**
- User registration, login, and session management with Argon2id password hashing
- Lobby: browse/create/join games via WebSocket
- In-game: full Risk rules (territory claiming, reinforcement, combat, fortification, cards)
- Real-time event propagation via WebSocket (actor/hub pattern)
- Lobby and per-game chat with typing presence indicators
- Admin controls: block users, revoke sessions

---

## 2. Tech Stack

| Layer | Technology |
|---|---|
| Backend language | Go 1.24 |
| HTTP framework | Gin v1.11 (`github.com/gin-gonic/gin`) |
| WebSocket | `github.com/coder/websocket` v1.8.14 |
| Database driver | pgx/v5 v5.8 (`github.com/jackc/pgx/v5`) |
| Password hashing | Argon2id (`golang.org/x/crypto`) |
| API docs | Swagger via swaggo (`github.com/swaggo/gin-swagger`) |
| Frontend language | TypeScript 5.9 |
| Frontend framework | React 19 |
| Router | TanStack Router v1.163 |
| HTTP client | Axios v1.13 |
| Styling | Tailwind CSS v4.2 |
| Build tool | Vite 7 |
| Database | PostgreSQL 16 |
| Cloud | AWS (ECS Fargate, ALB, RDS, S3, ECR, SSM) |
| IaC | Terraform 1.9+ |
| CI/CD | GitHub Actions (OIDC — no long-lived secrets) |

---

## 3. Major Subsystems

### 3.1 Backend — `backend/`

#### HTTP API — `backend/internal/httpapi/`
Gin-based REST handlers. `router.go` defines all routes; `handler.go` implements them. `auth_middleware.go` validates Bearer session tokens on protected routes. Swagger UI is mounted at `/swagger/*any`.

#### WebSocket API — `backend/internal/wsapi/`
`handler.go` accepts the WebSocket upgrade via Gin and registers the new connection with the game hub. All real-time traffic flows through here.

#### Game Hub — `backend/internal/game/`
Single-goroutine actor pattern (`server.go`). Maintains maps of connected clients, running games, chat rooms, and typing presence. All incoming messages are serialized through an `inbox` channel — no mutexes on shared state. Broadcasts updated game state to all players in a game after each action.

#### Risk Engine — `backend/internal/risk/`
Pure Go implementation of Risk rules. `board.go` defines the classic board (6 continents, 42 territories, adjacency graph, card deck). `engine.go` is a state machine with phases: `setup_claim → setup_reinforce → reinforce → attack → occupy → fortify → game_over`. The engine is called by the service layer; the result is persisted as a JSON blob in `games.state`.

#### Service Layer — `backend/internal/service/`
Business logic decoupled from HTTP/WebSocket transport:
- `user.go` — CreateUser, Login, UpdateUserAccess, RevokeUserSessions
- `game.go` — CreateClassicGame, JoinGame, GetGame, UpdateGameState
- `game_action.go` — Validates and applies a player's game action via the Risk engine
- `chat.go` / `game_chat.go` — Lobby and per-game chat

#### Store Layer — `backend/internal/store/`
Data-access objects wrapping raw SQL via pgx:
- `user.go` — users + sessions tables
- `game.go` — games table
- `chat.go` — chat_messages table
- `game_chat.go` — game_chat_messages table
- `game_event.go` — game_events table

#### Auth — `backend/internal/auth/`
- `password.go` — Argon2id hashing (64 MiB memory, 3 iterations, parallelism 2)
- `session_token.go` — cryptographically random tokens, stored as SHA-256 hash
- `types.go` — Role constants (`admin`, `player`), User and Session structs

#### Database — `backend/internal/db/`
- `db.go` — builds a `pgxpool.Pool` from env vars
- `querier.go` — `Querier` interface satisfied by both the pool and a transaction, enabling the same store methods to run inside or outside a transaction
- `transaction.go` — `WithTxQ` helper wraps a function in a DB transaction

#### WebSocket Connection — `backend/internal/wsconn/`
Per-connection read/write loops, ping/pong keepalive, message framing. Wraps `coder/websocket`.

#### Wire Protocol — `backend/internal/proto/wsmsg/`
Go types for all client→server and server→client WebSocket message payloads. Both sides share the JSON envelope: `{type, id, correlation_id, game_id, payload}`.

#### Migrations — `backend/migrations/`
Flyway-style versioned SQL files (V1–V6). Run automatically at startup; tracked in a `schema_migrations` table.

---

### 3.2 Frontend — `global-conquest/`

#### Entry & Root — `src/main.tsx`, `src/App.tsx`
`main.tsx` mounts React. `App.tsx` composes three context providers in order: `AuthProvider → SocketProvider → Router`.

#### API Client — `src/api/`
Axios instance (`client.ts`) with a request interceptor that injects `Authorization: Bearer <token>`. Separate modules per domain: `auth.ts`, `games.ts`, `chat.ts`, `users.ts`, `health.ts`.

#### Auth — `src/auth/`
`AuthProvider.tsx` manages user identity state (login, logout, token refresh). `storage.ts` persists the token in `localStorage`. `useAuth.ts` exposes the context to components.

#### Real-time — `src/realtime/`
`socket.ts` is a custom hook managing the WebSocket lifecycle: connection, exponential-backoff reconnection (250 ms → 8 s), an offline message queue, and UUID client-ID generation with a non-secure-context fallback. `SocketProvider.tsx` makes the socket available app-wide. `types.ts` defines the envelope and listener types.

#### Router — `src/router/`
TanStack Router with route guards for authenticated pages. Pages include Login, Register, Games lobby, GameBoard, Admin, and user Profile.

---

### 3.3 Infrastructure — `infra/`

| File | Responsibility |
|---|---|
| `vpc.tf` | VPC (10.0.0.0/16), 2 public + 2 private subnets, IGW, routing |
| `alb.tf` | ALB on port 80 → ECS port 8080; sticky-session cookie (1 day) for WebSocket affinity |
| `ecs.tf` | Fargate cluster, task (256 CPU / 512 MB), 1 replica; DB_PASSWORD from SSM |
| `rds.tf` | PostgreSQL 16 db.t4g.micro, private subnets; password in SSM Parameter Store |
| `ecr.tf` | ECR repo for backend images |
| `s3.tf` | Static frontend hosting; immutable cache on assets, no-cache on `index.html` |
| `iam.tf` | ECS execution role (ECR + CloudWatch + SSM); GitHub Actions OIDC role (ECR push + ECS deploy + S3 sync) |
| `main.tf` | Terraform config; remote state in S3 + DynamoDB lock |

---

## 4. Request / Execution Flow

### 4.1 REST Request (e.g., create game)

```
Browser
  → POST /api/games/
  → ALB (port 80)
  → ECS container (port 8080)
  → Gin router  [backend/internal/httpapi/router.go]
  → auth_middleware (validates Bearer token against sessions table)
                     [backend/internal/httpapi/auth_middleware.go]
  → Handler.CreateGame [backend/internal/httpapi/handler.go]
  → GamesService.CreateClassicGame [backend/internal/service/game.go]
  → PostgresGamesStore.Create [backend/internal/store/game.go]
  → PostgreSQL (RDS, private subnet)
  ← JSON response
```

### 4.2 WebSocket Game Action (e.g., attack)

```
Browser (WebSocket client)
  → WS frame: {type: "game_action", game_id: "...", payload: {action: "attack", ...}}
  → ALB (sticky-session lb_cookie ensures same ECS task)
  → ECS container
  → wsapi.Handler upgrades HTTP → WebSocket  [backend/internal/wsapi/handler.go]
  → wsconn reads frame  [backend/internal/wsconn/]
  → game.Server.inbox channel  [backend/internal/game/server.go]
  → GameActionService.Apply  [backend/internal/service/game_action.go]
  → risk.Engine.ApplyAction  [backend/internal/risk/engine.go]
  → GamesService.UpdateGameState (persist state JSON)
  → game.Server broadcasts {type: "game_state_updated", ...} to all players in game
  ← WS frames sent to each connected client
```

### 4.3 User Login

```
Browser
  → POST /api/auth/login  {username, password}
  → Handler.Login [backend/internal/httpapi/handler.go]
  → UsersService.Login [backend/internal/service/user.go]
    → UsersStore.GetByUsername → fetch user row
    → auth.VerifyPassword (Argon2id)  [backend/internal/auth/password.go]
    → auth.NewSessionToken → random token + SHA-256 hash
    → UsersStore.CreateSession → insert into sessions
  ← {token: "<plain token>"} — stored in browser localStorage
```

---

## 5. Data Model Overview

All UUIDs generated in application code (not DB-generated).

### `users`
`id uuid PK | username unique | password_hash text | role (admin|player) | access_status (active|blocked) | created_at | updated_at`

### `sessions`
`id uuid PK | user_id FK(users) | token_hash bytea unique | created_at | last_seen_at | expires_at`
Indexed on `user_id` and `expires_at`.

### `games`
`id uuid PK | owner_user_id FK(users) | status (lobby|active|finished) | state jsonb | created_at | updated_at`
The `state` column holds the full serialized `risk.Game` struct.

### `chat_messages` (lobby chat)
`id uuid PK | room text | user_id FK(users) | body text | created_at`
Indexed on `(room, created_at DESC)`.

### `game_chat_messages`
`id uuid PK | game_id FK(games) | sender_client_id text | sender_name text | body text | created_at`
Indexed on `(game_id, created_at DESC)`.

### `game_events`
`id uuid PK | game_id FK(games) | actor_user_id FK(users) nullable | event_type text | body text | created_at`
Indexed on `(game_id, created_at DESC)`.

### In-memory Risk game state (`risk.Game` — not a DB table)
```
Board        — 6 continents, 42 territories, adjacency map, card deck definition
Players      — []PlayerState (id, name, color, card hand, eliminated flag)
Territories  — map[Territory]TerritoryState (owner player index, army count)
Phase        — setup_claim | setup_reinforce | reinforce | attack | occupy | fortify | game_over
CurrentPlayer int
SetupReserves map[int]int
PendingReinforcements int
ConqueredThisTurn bool
Deck, Discard []Card
SetsTraded   int
Winner       string
```

---

## 6. External Dependencies

| Service | Purpose | Integration point |
|---|---|---|
| AWS RDS (PostgreSQL 16) | Primary data store | `backend/internal/db/`, all stores |
| AWS S3 | Frontend static hosting | `infra/s3.tf`, deploy workflow |
| AWS ECS Fargate | Backend compute | `infra/ecs.tf` |
| AWS ALB | HTTP load balancing + WebSocket sticky sessions | `infra/alb.tf` |
| AWS ECR | Docker image registry | `infra/ecr.tf`, deploy workflow |
| AWS SSM Parameter Store | DB password secret | `infra/ecs.tf` container env |
| AWS CloudWatch Logs | Application log sink | ECS task definition, 7-day retention |
| GitHub Actions + AWS OIDC | Automated CI/CD without long-lived secrets | `.github/workflows/deploy.yml`, `infra/iam.tf` |
| Flyway (via Docker, local dev) | SQL migration runner in `docker-compose.yml` | `backend/docker-compose.yml` |

No external SaaS APIs (payment, analytics, email, etc.) are integrated.

---

## 7. Key Entry Points

| File | Role |
|---|---|
| `backend/cmd/backend/main.go` | Backend server entry point — starts hub, pools DB, runs migrations, wires services, starts Gin on `:8080` |
| `backend/cmd/seed/main.go` | One-shot DB seeding utility |
| `global-conquest/src/main.tsx` | Frontend Vite entry — mounts React into `#root` |
| `global-conquest/src/App.tsx` | Composes AuthProvider + SocketProvider + Router |
| `backend/internal/httpapi/router.go` | All REST route definitions |
| `backend/internal/game/server.go` | Real-time hub — the single goroutine arbitrating all game state |
| `backend/internal/risk/engine.go` | Risk game state machine |
| `backend/migrations/` | V1–V6 SQL migration files |

---

## 8. Important Constraints

### Single-ECS-task WebSocket affinity
The infrastructure runs **1 ECS task replica**. The ALB is configured with `lb_cookie` sticky sessions (1-day TTL) to ensure a browser always hits the same task. If horizontal scaling is added, the in-memory game hub (`game/server.go`) must be replaced with a distributed pub/sub mechanism (e.g., Redis).

### Actor pattern — no concurrent hub access
`game.Server` is intentionally single-threaded. All mutations to the shared client/game maps go through `s.inbox`. Adding direct method calls from other goroutines would introduce data races.

### Game state is a single JSON blob
`games.state jsonb` stores the entire `risk.Game` struct. There is no normalized per-territory or per-player table. This makes state updates a full read-modify-write cycle but keeps the schema simple.

### Auth is session-token only (no JWT)
Tokens are random bytes; the SHA-256 hash is stored in the DB. Every authenticated request requires a DB lookup. There is no short-lived JWT tier.

### Migrations run at startup
`main.go` applies pending migrations automatically before accepting traffic. The tracking table is `schema_migrations`. Flyway is used in local dev via `docker-compose.yml` but the production binary has its own migration runner.

### DB SSL mode defaults differ between environments
`DB_SSL_MODE` defaults to `disable` in dev (`docker-compose`) and `require` in production (set in ECS task definition via `infra/ecs.tf`).

### Frontend env vars are baked at build time
`VITE_API_BASE_URL` and `VITE_WS_URL` are injected during `vite build` in the deploy workflow. Changing the backend URL requires a frontend rebuild.

### UUID generation fallback
`global-conquest/src/realtime/socket.ts` generates client IDs using `crypto.randomUUID()` with a manual fallback for non-HTTPS contexts (Math.random hex string) — see git commit `8d93309`.
