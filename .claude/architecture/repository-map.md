# Global Conquest — Repository Navigation Map

> Generated 2026-03-11. Use this to locate code quickly without needing to explore the tree.

---

## 1. Top-Level Directory Overview

```
global-conquest/
├── .claude/architecture/   AI assistant context documents (system-overview.md, this file)
├── .github/workflows/      CI/CD pipelines (deploy + infrastructure)
├── backend/                Go backend — HTTP API, WebSocket hub, game engine, data access
├── global-conquest/        React frontend — SPA served from S3
├── infra/                  Terraform — all AWS infrastructure definitions
├── ARCHITECTURE.md         Human-authored high-level overview
├── DEPLOYMENT.md           Deployment runbook
└── MISSING_FUNCTIONALITY.md Known gaps / open TODOs
```

---

## 2. Subsystem Breakdown

### Backend subsystems — `backend/`

| Subsystem | Directory | Responsibility |
|---|---|---|
| Server entry point | `backend/cmd/backend/` | Wires all components, runs migrations, starts Gin on :8080 |
| DB seed utility | `backend/cmd/seed/` | Creates test users and sample games for local dev |
| HTTP API | `backend/internal/httpapi/` | Gin routes, request handlers, auth middleware |
| WebSocket API | `backend/internal/wsapi/` | HTTP→WebSocket upgrade, authenticates connection |
| Real-time hub | `backend/internal/game/` | Actor-pattern multiplexer; owns all live game state |
| WebSocket connection | `backend/internal/wsconn/` | Low-level read/write loops, ping/pong, framing |
| Wire protocol | `backend/internal/proto/wsmsg/` | Go types for all WS message payloads and envelope |
| Risk game engine | `backend/internal/risk/` | State machine, rules, board definition, card deck |
| Service layer | `backend/internal/service/` | Business logic (users, games, actions, chat) |
| Store layer | `backend/internal/store/` | SQL data-access objects (PostgreSQL via pgx) |
| Database helpers | `backend/internal/db/` | Pool, Querier interface, transaction wrappers |
| Authentication | `backend/internal/auth/` | Argon2id hashing, session tokens, username validation |
| Migrations | `backend/migrations/` | Versioned SQL schema files (V1–V6) |
| API docs | `backend/docs/` | Swagger/OpenAPI output (generated — do not edit) |
| E2E tests | `backend/internal/e2e/` | End-to-end test suite against a live DB |

### Frontend subsystems — `global-conquest/src/`

| Subsystem | Directory | Responsibility |
|---|---|---|
| App root | `src/` | `main.tsx` (Vite entry), `App.tsx` (provider tree) |
| Router & pages | `src/router/` | TanStack Router config, all page components |
| Auth | `src/auth/` | Session context, localStorage persistence, hooks |
| REST API client | `src/api/` | Typed Axios wrappers per domain |
| WebSocket client | `src/realtime/` | Socket hook, context, types, provider |
| Assets | `src/assets/` | Static images (Risk board image, SVGs) |

### Infrastructure — `infra/`

All Terraform. Each `.tf` file corresponds to one AWS concern (see §3).

---

## 3. Important Files

### Backend

| File | Role |
|---|---|
| `backend/cmd/backend/main.go` | **Main entry point.** Initializes hub, DB pool, migrations, all services, HTTP router. |
| `backend/cmd/seed/main.go` | Dev seeding utility — creates test users + sample games. |
| `backend/internal/httpapi/router.go` | **All REST + WS route definitions.** The authoritative list of every HTTP endpoint. |
| `backend/internal/httpapi/handler.go` | HTTP handler implementations (one method per route). |
| `backend/internal/httpapi/auth_middleware.go` | `RequireAuth()` and `RequireAdmin()` Gin middleware. Bearer token validation. |
| `backend/internal/wsapi/handler.go` | WebSocket upgrade handler; authenticates via `?token=` query param. |
| `backend/internal/game/server.go` | **The real-time hub.** Single goroutine, `inbox` channel, all live game/chat state. |
| `backend/internal/proto/wsmsg/messages.go` | Canonical WS message types and `Envelope` struct. |
| `backend/internal/risk/engine.go` | Risk state machine — phases, actions, victory conditions. |
| `backend/internal/risk/board.go` | Classic Risk board — 6 continents, 42 territories, adjacency graph, cards. |
| `backend/internal/service/user.go` | User business logic — create, login, access control, session management. |
| `backend/internal/service/game.go` | Game lifecycle — create, join, fetch, update state. |
| `backend/internal/service/game_action.go` | Thin adapter: applies a player action via the Risk engine, saves game event. |
| `backend/internal/store/user.go` | SQL for `users` and `sessions` tables. |
| `backend/internal/store/game.go` | SQL for `games` table (including `FOR UPDATE` locking). |
| `backend/internal/store/chat.go` | SQL for `chat_messages` (lobby). |
| `backend/internal/store/game_chat.go` | SQL for `game_chat_messages`. |
| `backend/internal/store/game_event.go` | SQL for `game_events`. |
| `backend/internal/db/db.go` | `DB` struct wrapping `pgxpool.Pool`; `WithTx` / `WithTxQ` helpers. |
| `backend/internal/db/querier.go` | `Querier` interface — implemented by both pool and `pgx.Tx`. |
| `backend/internal/db/transaction.go` | `ConfigFromEnv()` — reads all `DB_*` environment variables. |
| `backend/internal/auth/password.go` | Argon2id hash/verify + `ValidateUsername()`. |
| `backend/internal/auth/session_token.go` | `NewSessionToken()` (random bytes) + `HashSessionToken()` (SHA-256). |
| `backend/internal/auth/types.go` | `Role` constants, `User` and `Session` structs. |
| `backend/migrations/V1__init.sql` | `users`, `sessions` tables. |
| `backend/migrations/V2__games.sql` | `games` table with `state jsonb`. |
| `backend/migrations/V3__chat_messages.sql` | `chat_messages` table. |
| `backend/migrations/V4__user_access_control.sql` | Adds `access_status` and `role` columns to `users`. |
| `backend/migrations/V5__game_chat_messages.sql` | `game_chat_messages` table. |
| `backend/migrations/V6__game_events.sql` | `game_events` table. |
| `backend/go.mod` | Module path `global-conquest/backend`, Go 1.24, dependency declarations. |
| `backend/Makefile` | `build`, `dev`, `test`, `e2e`, `db-up/down`, `db-migrate`, `seed-go`, `swagger`. |
| `backend/Dockerfile` | Multi-stage build (golang:1.24-alpine → alpine:3.21), exposes 8080. |
| `backend/docker-compose.yml` | Local dev: Postgres 16 + Flyway + backend service. |

### Frontend

| File | Role |
|---|---|
| `global-conquest/src/main.tsx` | Vite entry point — mounts React into `#root`. |
| `global-conquest/src/App.tsx` | Provider tree: `AuthProvider → SocketProvider → RouterProvider`. |
| `global-conquest/src/router/index.tsx` | TanStack Router tree — all routes with guards. |
| `global-conquest/src/router/views.tsx` | Barrel re-export of all page components. |
| `global-conquest/src/router/pages/AuthPages.tsx` | `LoginPage` and `SignupPage`. |
| `global-conquest/src/router/pages/AppShell.tsx` | Authenticated layout wrapper with nav and `<Outlet>`. |
| `global-conquest/src/router/pages/LobbyPage.tsx` | Game listing and creation; WebSocket-driven presence. |
| `global-conquest/src/router/pages/GamePage.tsx` | In-game board view; drives game actions via WebSocket. |
| `global-conquest/src/router/pages/AdminPage.tsx` | Admin: user list, block/unblock, revoke sessions. |
| `global-conquest/src/auth/AuthProvider.tsx` | Manages auth state, login/logout, token persistence. |
| `global-conquest/src/auth/context.ts` | `AuthContextValue` type definition. |
| `global-conquest/src/auth/storage.ts` | `localStorage` read/write helpers for the session token. |
| `global-conquest/src/auth/useAuth.ts` | Hook to consume `AuthContext`. |
| `global-conquest/src/api/client.ts` | Axios instance; Bearer token interceptor; `toApiError()`. |
| `global-conquest/src/api/auth.ts` | `login()`, `signup()` — POST /auth/login, POST /users/. |
| `global-conquest/src/api/games.ts` | `listGames()`, `createGame()`, `joinGame()`, `getGame()`, `getGameBootstrap()`. |
| `global-conquest/src/api/chat.ts` | Lobby chat REST calls. |
| `global-conquest/src/api/users.ts` | User profile REST calls. |
| `global-conquest/src/realtime/socket.ts` | `useGameSocket()` hook — WS lifecycle, reconnect, message queue, `on()` / `send()`. |
| `global-conquest/src/realtime/types.ts` | `WsEnvelope`, message type constants, `safeParseEnvelope()`. |
| `global-conquest/src/realtime/SocketProvider.tsx` | Context provider for the socket hook. |
| `global-conquest/src/realtime/useSocket.ts` | Hook to consume `SocketContext`. |
| `global-conquest/package.json` | npm dependencies and Vite build scripts. |
| `global-conquest/vite.config.ts` | Vite config — dev proxy, build output. |

### Infrastructure & CI/CD

| File | Role |
|---|---|
| `infra/main.tf` | Terraform provider config; S3+DynamoDB remote state backend. |
| `infra/vpc.tf` | VPC (10.0.0.0/16), subnets, IGW, route tables. |
| `infra/alb.tf` | ALB, HTTP:80 listener, target group :8080, sticky-session cookie. |
| `infra/ecs.tf` | Fargate cluster + task (256 CPU / 512 MB) + service (1 replica). |
| `infra/rds.tf` | RDS PostgreSQL 16 (db.t4g.micro), private subnets, password in SSM. |
| `infra/ecr.tf` | ECR repository for backend Docker images. |
| `infra/s3.tf` | S3 static-website bucket; immutable cache on assets, no-cache on index.html. |
| `infra/iam.tf` | ECS task execution role; GitHub Actions OIDC federation role. |
| `infra/variables.tf` | Input variables (region, account ID, db credentials, app name). |
| `infra/outputs.tf` | Outputs: ALB DNS, S3 URL, ECR URL, RDS endpoint. |
| `infra/terraform.tfvars` | Concrete variable values for the deployment environment. |
| `.github/workflows/deploy.yml` | Build backend image → push to ECR → update ECS service → build frontend → sync to S3. |
| `.github/workflows/infra.yml` | terraform init + plan + apply on changes to `infra/**`. |

---

## 4. Canonical Locations

### HTTP handlers
`backend/internal/httpapi/handler.go` — one method per route.
Route-to-handler mapping is in `backend/internal/httpapi/router.go`.

### Business logic / services
`backend/internal/service/` — one file per domain:
- `user.go` → registration, login, access control
- `game.go` → game lifecycle, state persistence
- `game_action.go` → applying Risk engine actions
- `chat.go` → lobby chat
- `game_chat.go` → in-game chat

### Database access
`backend/internal/store/` — one file per table/domain:
- `user.go` → `users`, `sessions`
- `game.go` → `games`
- `chat.go` → `chat_messages`
- `game_chat.go` → `game_chat_messages`
- `game_event.go` → `game_events`

The `Querier` interface lives in `backend/internal/db/querier.go`; the pool and transaction wrappers are in `backend/internal/db/db.go` and `backend/internal/db/transaction.go`.

### Configuration
- **Backend runtime config:** environment variables read in `backend/internal/db/transaction.go` (`ConfigFromEnv`) and `backend/internal/httpapi/router.go` (CORS, WS origins).
- **Frontend build config:** `global-conquest/vite.config.ts`; runtime env vars (`VITE_API_BASE_URL`, `VITE_WS_URL`) injected at build time in `.github/workflows/deploy.yml`.
- **Infrastructure config:** `infra/variables.tf` (declarations), `infra/terraform.tfvars` (values).
- **Local dev overrides:** `backend/docker-compose.yml`.

### Migrations
`backend/migrations/` — `V1__init.sql` through `V6__game_events.sql`.
Applied automatically at startup by `runMigrations()` in `backend/cmd/backend/main.go`.
Tracked in the `schema_migrations` DB table.
In local dev they can also be run via `make db-migrate` (Flyway in Docker).

### Tests
| Test type | Location | Run command |
|---|---|---|
| Unit — auth | `backend/internal/auth/*_test.go` | `make test` |
| Unit — risk engine | `backend/internal/risk/engine_test.go` | `make test` |
| Unit — services | `backend/internal/service/*_test.go` | `make test` |
| Unit — stores | `backend/internal/store/*_test.go` | `make test` |
| Integration — HTTP | `backend/internal/httpapi/router_integration_test.go` | `make test` |
| Integration — WS | `backend/internal/httpapi/ws_integration_test.go` | `make test` |
| E2E | `backend/internal/e2e/auth_e2e_test.go` | `make e2e` (requires live DB) |

No frontend test files are present in the repository.

### Integrations with external services
All external integrations live in infrastructure config, not application code:
- **AWS RDS** — connection params from env vars; driver in `backend/internal/db/` and `backend/internal/store/`.
- **AWS SSM** — used only at ECS startup to inject `DB_PASSWORD`; configured in `infra/ecs.tf`.
- **AWS S3 / CloudFront** — frontend deploy target; configured in `infra/s3.tf` and `.github/workflows/deploy.yml`.
- **AWS ECR** — Docker image registry; push in `.github/workflows/deploy.yml`.
- **GitHub Actions OIDC** — replaces long-lived AWS keys; trust policy in `infra/iam.tf`.

---

## 5. Key Entry Points

| Entry point | File | Notes |
|---|---|---|
| Backend HTTP server | `backend/cmd/backend/main.go` | Calls `game.NewServer()`, `db.NewPool()`, `runMigrations()`, wires services, `r.Run(":8080")`. |
| DB seeder | `backend/cmd/seed/main.go` | One-shot CLI — run via `make seed-go`. |
| Frontend SPA | `global-conquest/src/main.tsx` | `ReactDOM.createRoot(...).render(...)`. |
| React provider tree | `global-conquest/src/App.tsx` | AuthProvider → SocketProvider → RouterProvider. |
| All HTTP routes | `backend/internal/httpapi/router.go` | Start here when tracing any REST endpoint. |
| WebSocket entry | `backend/internal/wsapi/handler.go` | Gin handler at `GET /ws`; authenticates and creates a `Client`. |
| Real-time hub loop | `backend/internal/game/server.go` | `func (s *Server) Run()` — the single goroutine processing all hub messages. |
| Risk game creation | `backend/internal/risk/engine.go` | `NewClassicGame(players, rng)` — initializes a fresh game state. |

---

## 6. Generated or Sensitive Areas

### Do not edit manually

| Path | Reason |
|---|---|
| `backend/docs/` | Generated by `swaggo` — run `make swagger` to regenerate. Editing by hand will be overwritten. |
| `global-conquest/dist/` | Vite production build output. Regenerated by `npm run build`. |
| `infra/.terraform/` | Terraform provider cache. Managed by `terraform init`. |
| `infra/.terraform.lock.hcl` | Terraform dependency lock. Update only via `terraform init -upgrade`. |
| `backend/go.sum` | Go module checksum database. Updated automatically by `go mod tidy`. |
| `global-conquest/package-lock.json` (or `node_modules/`) | npm lock file / installed packages. Managed by `npm install`. |

### Applied migrations — treat as append-only

| Path | Reason |
|---|---|
| `backend/migrations/V1__init.sql` through `V6__game_events.sql` | Already applied to production. **Never modify existing files.** Add a new `V7__*.sql` for schema changes. |

### Sensitive values — never commit

| Item | Where it lives at runtime |
|---|---|
| `DB_PASSWORD` | AWS SSM Parameter Store → injected into ECS task env |
| AWS credentials | Provided at CI time via GitHub OIDC; never stored in repo |
| `infra/terraform.tfvars` | Contains account IDs and variable values — listed in `.gitignore` |
