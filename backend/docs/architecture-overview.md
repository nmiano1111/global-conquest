# Global Conquest Backend Architecture Overview

## Project Components

1. **Application entrypoint**
- [`cmd/backend/main.go`](/Users/nmiano/prj/global-conquest/backend/cmd/backend/main.go): bootstraps everything.
- Starts the in-memory game server goroutine.
- Loads DB config from env, opens pgx pool, wires store + service, then starts Gin on `:8080`.

2. **HTTP API layer**
- [`internal/httpapi/router.go`](/Users/nmiano/prj/global-conquest/backend/internal/httpapi/router.go): Gin router and route registration.
- REST endpoints:
  - `GET /api/ping`
  - `POST /api/users/`
  - `GET /api/users/:username`
- WebSocket endpoint:
  - `GET /ws`
- Swagger endpoint:
  - `GET /swagger/*any`

- [`internal/httpapi/handler.go`](/Users/nmiano/prj/global-conquest/backend/internal/httpapi/handler.go): request validation and HTTP responses for user operations.

3. **Service layer**
- [`internal/service/user.go`](/Users/nmiano/prj/global-conquest/backend/internal/service/user.go)
- `UsersService` orchestrates user use-cases and wraps operations in DB transactions (`WithTx`).

4. **Persistence layer**
- [`internal/store/user.go`](/Users/nmiano/prj/global-conquest/backend/internal/store/user.go)
- `UsersStore` interface + Postgres implementation (`PostgresUsersStore`).
- SQL for creating and fetching users.

5. **DB infrastructure**
- [`internal/db/db.go`](/Users/nmiano/prj/global-conquest/backend/internal/db/db.go): env-driven DB config and pgx pool creation.
- [`internal/db/transaction.go`](/Users/nmiano/prj/global-conquest/backend/internal/db/transaction.go): transaction helper (`WithTx`).

6. **Realtime game subsystem (in-memory)**
- [`internal/game/server.go`](/Users/nmiano/prj/global-conquest/backend/internal/game/server.go)
- Single-threaded event-loop via `inbox chan any`.
- Tracks connected clients and active games in memory.
- Handles messages like `create_game`, `join_game`, `leave_game`, `list_games`, `ping`.

7. **WebSocket transport**
- [`internal/wsapi/handler.go`](/Users/nmiano/prj/global-conquest/backend/internal/wsapi/handler.go): Gin handler upgrading HTTP to WS and bridging messages into game server inbox.
- [`internal/wsconn/conn.go`](/Users/nmiano/prj/global-conquest/backend/internal/wsconn/conn.go): WS connection abstraction with read loop, send channel, ping loop.
- [`internal/proto/wsmsg/messages.go`](/Users/nmiano/prj/global-conquest/backend/internal/proto/wsmsg/messages.go): envelope and message/payload types.

8. **Auth utilities (currently not wired into HTTP routes)**
- [`internal/auth/password.go`](/Users/nmiano/prj/global-conquest/backend/internal/auth/password.go): Argon2id hashing + verification.
- [`internal/auth/session_token.go`](/Users/nmiano/prj/global-conquest/backend/internal/auth/session_token.go): random session tokens + SHA-256 hashing.
- [`internal/auth/types.go`](/Users/nmiano/prj/global-conquest/backend/internal/auth/types.go), [`internal/auth/errors.go`](/Users/nmiano/prj/global-conquest/backend/internal/auth/errors.go).
- Unit tests exist for password/session primitives.

9. **Schema and ops**
- [`migrations/V1__init.sql`](/Users/nmiano/prj/global-conquest/backend/migrations/V1__init.sql): `users` and `sessions` tables.
- [`docker-compose.yml`](/Users/nmiano/prj/global-conquest/backend/docker-compose.yml): Postgres + Flyway + app service orchestration.
- [`Makefile`](/Users/nmiano/prj/global-conquest/backend/Makefile): build/run/swagger/db helper targets.
- Swagger artifacts in [`docs/swagger.yaml`](/Users/nmiano/prj/global-conquest/backend/docs/swagger.yaml).

## Architecture Diagram

```mermaid
flowchart LR
  C1[HTTP Client] -->|REST /api/users| GIN[Gin Router + Handlers]
  C2[WS Client] -->|GET /ws (upgrade)| WSAPI[wsapi GinHandler]
  C3[Docs Client] -->|/swagger/*any| SWAG[Swagger UI]

  GIN --> USVC[UsersService]
  USVC --> TX[DB.WithTx]
  TX --> USTORE[PostgresUsersStore]
  USTORE --> PG[(PostgreSQL)]

  WSAPI --> WSCONN[wsconn.Conn]
  WSCONN -->|Incoming Envelopes| HUB[Game Server Inbox/Event Loop]
  HUB -->|State updates| MEM[(In-memory clients/games)]
  HUB -->|Outbound Envelopes| WSCONN
  WSCONN --> C2

  MIG[Flyway Migrations] --> PG
```

## Current architectural notes

- Game state is **ephemeral** (in-memory only), so restarts drop active games.
- Auth/session primitives and `sessions` table exist, but login/session APIs are not yet integrated.
- `docker-compose.yml` references a `Dockerfile`; I do not see one in this repo root currently.
