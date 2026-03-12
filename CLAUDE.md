# Global Conquest — CLAUDE.md

> Deep reference: `.claude/architecture/` (canonical-patterns.md, repository-map.md, system-overview.md).

---

## Architecture

**Global Conquest** is a real-time multiplayer Risk adaptation. The Go backend (`backend/internal/risk/engine.go`) owns all game logic and state; the React frontend is a render-only client — it sends player intent over WebSocket and replaces state wholesale on each `game_state_updated` broadcast.

---

## Game Model

**`risk.Game`** is serialized as JSONB in `games.state`. Key fields:
- `Phase Phase` — drives all action legality
- `Territories map[Territory]TerritoryState` — owner (player index) + army count
- `Players []PlayerState` — card hand, eliminated flag
- `CurrentPlayer int`, `PendingReinforcements int`
- `Occupy *OccupyState` — non-nil only during occupy phase (post-conquest)
- `Deck, Discard []Card`, `SetsTraded int`

**Phase state machine** (`backend/internal/risk/engine.go`):
```
setup_claim → setup_reinforce → reinforce → attack ──→ occupy → attack
                                                    └──→ fortify → reinforce (next player)
                                                                        └──→ game_over
```

---

## Critical Invariants

- **`risk/engine.go` is the sole authority for game rules.** Never replicate rule logic in service or frontend.
- **All game mutations use `SELECT … FOR UPDATE` inside `WithTxQ`.** Using `GetByID` (non-locking) instead of `GetByIDForUpdate` causes race conditions. `[service/game.go:169–170, 305–306]`
- **`game.Server` (`game/server.go`) is single-goroutine.** `s.clients` and `s.chatRooms` are only accessed inside `Run()`. Do not add direct concurrent access.
- **`game_state_updated` is a full snapshot, not a diff.** Clients replace state entirely on each message (`GamePage.tsx:162–184`).
- **A client must send `game_chat_join` before the hub delivers game state updates or accepts game actions.** `[game/server.go:364–366]`
- **Migrations are append-only.** `V1__`–`V6__` are in production. Add `V7__*.sql` for any schema change; never modify existing files.
- **Sessions are 32-byte CSPRNG values stored as SHA-256 hashes.** Every REST request and WebSocket upgrade requires a DB lookup — no JWT. `[auth/session_token.go]`
- **WebSocket delivery is at-most-once.** Send channel is buffered (cap=16); full buffer drops the message silently. `[wsconn/conn.go:85–92]`

---

## Networking

**REST** (`/api/*`, `httpapi/router.go`): auth, user profiles, game listing/creation/joining, lobby chat history, admin, bootstrap (`GET /games/:id/bootstrap`).

**WebSocket** (`/ws?token=<session_token>`, `wsapi/handler.go`): all real-time gameplay, chat, typing indicators. Unauthenticated clients connect as `anon` and receive broadcasts but cannot submit game actions.

**ALB** uses `lb_cookie` sticky sessions (1-day TTL). Deployment runs 1 ECS task — horizontal scaling requires replacing the in-memory hub with distributed pub/sub.

Envelope shape — Go: `proto/wsmsg/messages.go:36–42`, TypeScript: `realtime/types.ts:3–9`:
```json
{ "type": "string", "id": "uuid", "correlation_id": "uuid", "game_id": "uuid", "payload": {} }
```

Frontend reconnects with exponential backoff: `250ms × 2^attempt`, capped at 8s (`realtime/socket.ts:135–147`). No server-push resync — `GamePage.tsx` re-fetches via `GET /games/:id/bootstrap` on mount.

---

## Development

```bash
# Backend
cd backend
docker compose up -d           # Postgres on :5432
go run ./cmd/backend/main.go   # :8080; migrations run automatically
go run ./cmd/seed/main.go      # seed test users (password: "password")
```

Makefile targets: `make build`, `make dev`, `make test`, `make e2e`, `make db-up`, `make db-down`, `make swagger`.

```bash
# Frontend
cd global-conquest
npm install
npm run dev    # Vite on :5173; /api and /ws proxied to :8080 (vite.config.ts)
```

**Dev env defaults:** `DB_HOST=localhost`, `DB_PORT=5432`, `DB_USER/PASSWORD/NAME=globalconq`, `DB_SSL_MODE=disable`.

**Tests:**

| Scope | Command | Location |
|---|---|---|
| Unit (auth, engine, services, stores) | `make test` | `backend/internal/*/` |
| HTTP + WebSocket integration | `make test` | `backend/internal/httpapi/` |
| End-to-end (requires live DB) | `make e2e` | `backend/internal/e2e/` |

No frontend tests.

---

## References

| File | Contents |
|---|---|
| `.claude/architecture/canonical-patterns.md` | System-level action flows with line references |
| `backend/.claude/architecture/canonical-patterns.md` | Backend implementation patterns (handlers, services, stores, tests) |
| `.claude/architecture/repository-map.md` | File-by-file navigation guide |
| `.claude/architecture/system-overview.md` | Full tech stack and data model |
| `MISSING_FUNCTIONALITY.md` | Known gaps: setup-phase UI, card trading UI, game-over screen, elimination events |
