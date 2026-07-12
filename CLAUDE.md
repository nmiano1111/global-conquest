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
- `Players []PlayerState` — card hand, eliminated flag, controller (`human`/`bot`, omitempty — see Bot Players below)
- `CurrentPlayer int`, `PendingReinforcements int`
- `Occupy *OccupyState` — non-nil only during occupy phase (post-conquest)
- `Deck, Discard []Card`, `SetsTraded int`

**Phase state machine** (`backend/internal/risk/engine.go`):
```
setup_claim → setup_reinforce → reinforce → attack ──→ occupy → attack
                                                    └──→ fortify → reinforce (next player)
                                                                        └──→ game_over
```
`setup_claim` is engine-only and unused in practice: `JoinClassicGame` always starts games via `NewClassicAutoStartGame` or `NewClassicRandomTerritoryGame`, both of which skip straight to `setup_reinforce` or `reinforce`. Territories are always randomly distributed — no player (human or bot) ever chooses a territory claim.

---

## Bot Players

> Deep reference: `project-docs/bot_player/` (design docs by phase; `phase_1_foundation/00_Phase_1_Implementation_Status.md` and `phase_2_first_playable_bot/00_Bot_Creation_And_Live_Pacing_Status.md` document what's actually built vs. designed).

A player is bot-controlled via `risk.PlayerState.Controller == risk.ControllerBot` (`IsBot()`), with `Strategy` naming which strategy to use — currently only `"basic-v1"` exists, and it plays *legal*, not *strong*, games. `PlayerState.Name` holds a bot's assigned display name (humans leave it empty and use the normal `users` table lookup instead).

- **`bot.Strategy`** picks one `bot.Command` (the same action shape a human WebSocket command uses) from read-only `risk.Legal*` query helpers (`risk/legal_actions.go`) — never duplicates engine legality, never mutates state.
- **`bot.Runner.RunTurn`** drives one bot's one turn, one command at a time, always against freshly reloaded authoritative state, submitting through `game.Server.SubmitGameAction` — the *same* transactional `ApplyGameAction` + broadcast path human commands use, run inside the hub's single goroutine via the inbox channel (never a direct call from another goroutine).
- **`bot.Manager`** keeps an in-memory, single-process registry ensuring at most one runner per game, and chains into the next bot's turn only when a runner reports `StopTurnEnded` — never busy-loops on a human turn.
- **`ExecutionLive` vs `ExecutionSimulation`**: live mode applies bounded random delays per situation (`bot.PacingConfig` — turn start, card turn-in, reinforcement, first/repeat attack, capture, occupation, fortify, elimination/completion) via an injectable `Sleeper`; simulation mode never sleeps. Classification uses the engine's typed `risk.AttackResult` and resulting phase, not JSON diffing.
- Triggered after every committed action (`game.Server`'s `BotTrigger` callback) and on backend startup (`recoverBotGames` in `cmd/backend/main.go`, scanning `in_progress` games) — no in-memory bot plan is ever persisted; resuming just means reloading JSONB state.

**Creating a mixed human/bot game**: `POST /api/games` takes an optional `bot_count` (`0 <= bot_count <= player_count - 1`; the creator always occupies one human slot). Bots are created immediately — no session, no `users` row, no `game_players` row (that table's `user_id` has an FK to `users`) — and occupy a lobby slot right away, so existing "is the lobby full" logic in `JoinClassicGame` needed no changes. If bots fill every non-creator slot at creation, `CreateClassicGame` starts the engine immediately (nobody would ever call join to trigger it) via the same `startEngineForFullLobby` helper `JoinClassicGame` uses. Bot names come from a curated pool of 1980s/1990s wrestlers (`bot.WrestlerNames`, `backend/internal/bot/names.go`), deduplicated within a game and against the creator's username.

---

## Critical Invariants

- **`risk/engine.go` is the sole authority for game rules.** Never replicate rule logic in service or frontend.
- **All game mutations use `SELECT … FOR UPDATE` inside `WithTxQ`.** Using `GetByID` (non-locking) instead of `GetByIDForUpdate` causes race conditions. `[service/game.go:169–170, 305–306]`
- **`game.Server` (`game/server.go`) is single-goroutine.** `s.clients` and `s.chatRooms` are only accessed inside `Run()`. Do not add direct concurrent access. Bot runners submit via `Server.SubmitGameAction`, which posts onto the same inbox channel `Run()` already reads — never call `ApplyGameAction` directly from a bot goroutine, since that would skip the broadcast and violate this invariant.
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
| Bot strategy/runner/manager | `make test` | `backend/internal/bot/` |
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
