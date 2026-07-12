# Bot Game Creation & Live Pacing — Implementation Status

This documents a specific increment built on top of Phase 1 (see
`phase_1_foundation/00_Phase_1_Implementation_Status.md`): letting a user
actually create a mixed human/bot game, and pacing bot turns so they're
legible in live play. It does not add new strategy, ML, simulation
tournaments, or Discord personality — see Scope Exclusions below.

## Creating a mixed human/bot game

`POST /api/games` accepts an additional field:

```json
{ "player_count": 4, "bot_count": 2 }
```

- `bot_count` is optional; omitted or `0` produces the exact same
  human-only game creation behavior as before.
- The creator always occupies one human slot: `bot_count` must satisfy
  `0 <= bot_count <= player_count - 1`, enforced authoritatively in
  `GamesService.CreateClassicGame` regardless of what the frontend sends.
- Remaining human slots (`player_count - bot_count - 1`) fill through the
  existing `POST /api/games/:id/join` flow, unchanged.

Bots are created **immediately** at game-creation time — they are not
"joined" the way humans are, and never authenticate or hold a session.
Each bot gets:

- A synthetic player ID (same textual UUID format Postgres's
  `gen_random_uuid()` produces, generated in Go via `crypto/rand` — see
  `newBotPlayerID` in `backend/internal/service/game.go`). Bots have no row
  in `users` and are never inserted into `game_players` (that table's
  `user_id` has an FK to `users(id)`).
- `Controller: "bot"`, `Strategy: "basic-v1"` (the only strategy that
  exists), and a `Name` — all on `risk.PlayerState`, the same struct
  Phase 1 added `Controller`/`Strategy` to.

**Important consequence:** if bots fill every non-creator slot at creation
(e.g. a 3-player game with `bot_count: 2`), the lobby is full the instant
it's created — nobody will ever call the join endpoint to trigger the
existing "lobby fills up → start the engine" transition. `CreateClassicGame`
detects this and starts the engine immediately, via the same
`startEngineForFullLobby` helper `JoinClassicGame` uses when a human fills
the last slot, so the two paths can never drift apart.

### Lobby occupancy

Bots occupy a slot in the lobby's `player_ids` list immediately, so the
existing "how many players have joined" / "is the lobby full" logic in
`JoinClassicGame` required no changes — bots already count as filled slots
by construction. `GameBootstrap.player_count` (new field) minus
`len(GameBootstrap.players)` gives the number of open human slots, both
during "lobby" status and once a game is running.

## Wrestler names

Bot display names are drawn from a curated pool of 1980s/1990s
professional wrestling ring names: `bot.WrestlerNames` in
`backend/internal/bot/names.go` (40 entries). `bot.AssignBotNames`:

- Shuffles the pool with an injectable RNG (production uses
  `crypto/rand`-backed randomness; tests inject a deterministic sequence).
- Excludes the creator's own username, best-effort (a lookup failure just
  means no exclusion happens — it never blocks game creation).
- Guarantees uniqueness within one game as long as the pool (after
  exclusion) has enough names; if a request ever needed more bot names
  than the pool has, it deterministically appends a roman-numeral suffix
  on repeat cycles (`"Sting II"`) rather than failing.

`GamesService.SetBotNameAssigner` lets tests inject a fake selector instead
of real randomness — the same setter-injection pattern already used for
`SetGameEventStore` etc.

Because bots have no `users` row, the existing username-lookup path
(`userNamesByIDsQ`) would otherwise display them as their raw player ID.
Every place that builds a `userID -> display name` map (`ApplyGameAction`,
`ListGames`, both branches of `GetGameBootstrap`, `JoinClassicGame`'s event
bodies) now overlays the bot's `PlayerState.Name` on top of that map before
using it.

## Live pacing

Bot turns in **live** games are paced so a human opponent can actually
follow what happened, using bounded random delays applied in
`bot.Runner.RunTurn` — never inside a database transaction, and never by
delaying an already-committed broadcast on the frontend. The sequence is
always: submit → commit → broadcast → **pace** → reload → decide again.
**Simulation mode never sleeps.**

`bot.PacingConfig` (see `backend/internal/bot/pacing.go`) holds a bounded
`[min, max]` range per situation, sampled uniformly rather than using one
fixed pause:

| Situation | Default range | Logged category |
|---|---|---|
| Start of a bot's turn (before its first command) | 1200–2200 ms | normal |
| Card turn-in | 1500–2700 ms | significant |
| Reinforcement placement (setup or normal) | 600–1200 ms | minor |
| First attack against a territory | 1000–1800 ms | normal |
| Repeated attack against the same territory | 500–1000 ms | normal |
| Territory capture | 1500–2700 ms | significant |
| Occupation | 750–1500 ms | minor |
| Fortification | 1000–2000 ms | normal |
| End of attack phase (transition only) | 450–900 ms | minor |
| Player elimination or game completion | 2200–3500 ms | dramatic |
| End of turn | none (control passes immediately) | — |

(Bumped ~1.5x from the original first-pass values per user feedback that bot turns felt a little too fast; same relative proportions.)

Classification uses the engine's own typed result
(`risk.AttackResult.Conquered` / `.Eliminated`) and the resulting phase —
already available on `game.GameActionUpdate` from Phase 1 — rather than
diffing JSON or inventing a new domain-event type. "Repeated attack" is
tracked by comparing each attack's target territory to the previous one
within the same `RunTurn` call; it resets on any non-attack action.

The delay itself goes through the same injectable `bot.Sleeper` Phase 1
introduced (`bot.RealSleeper` in production, a fake recorder in tests).

## Frontend

`LobbyPage.tsx`'s create-game form gained a "Computer players" number
input alongside the existing player-count field, with:

- A derived, read-only human-player count (`total - computer`).
- Grammar-correct summary text ("1 computer player" / "2 computer
  players"; "1 additional human player will need to join" / plural).
- The bot-count input's max clamps to `total - 1` and re-clamps live if the
  total player count is lowered below the current bot count.

`GamePage.tsx`'s existing player roster (which already renders during
"lobby" status too, since it reads the same `GET /:id/bootstrap` response
regardless of status — there is no separate "waiting room" view in this
app) now shows a 🤖 badge next to bot players and "Open human slot"
placeholder rows for any unfilled human slots. `MobileGameView.tsx`'s three
duplicate roster renderings get the same 🤖 marker for parity.

**No test framework exists in this frontend** (confirmed: no
jest/vitest config, no `test`/`typecheck` npm scripts) and none was
introduced, per instruction. Frontend correctness here was validated with
`tsc -b --noEmit`, `eslint`, and `vite build`, all clean on every file this
change touched.

## Known limitations / assumptions

- There is still no way to edit bot slots after creation, replace a human
  with a bot (or vice versa) mid-game, or choose a bot's name — all
  explicitly out of scope for this increment.
- The lobby-list view on `LobbyPage.tsx` (the page showing all games) still
  only shows a numeric `current/max` ratio, not a per-slot breakdown — that
  level of detail only exists once you open a specific game via
  `GamePage.tsx`, which already doubles as this app's lobby/waiting-room
  view.
- Pacing ranges are a first pass per the design brief; no admin/env-var
  override exists yet to retune them without a code change.
- `bot_count` validation, name assignment, and pacing are unit-tested with
  fakes; there is no live-DB end-to-end test of a fully bot-filled game
  reaching `in_progress` immediately at creation (the existing e2e suite
  requires a local Postgres this sandbox didn't have running — the new
  service-layer tests use the same fake-store patterns as the rest of
  `internal/service`).

## Scope exclusions confirmed

No additional bot strategy, ML, LLM integration, Discord trash talk,
Monte Carlo search, simulation tournaments, analytics changes, wrestler
personalities/avatars, difficulty levels, or client-side fake pacing were
added.
