# Simulation Framework — Task Breakdown

Implementation task list for `05_Simulation_Framework.md`'s first milestone: a
headless, reproducible bot-vs-bot game simulator. Derived from a full
read-only architecture review of `internal/risk`, `internal/bot`, and
`internal/service` — every constraint below was verified against the actual
source, not assumed.

## Design summary

The simulator drives `bot.Strategy.NextCommand` directly against an
in-process `*risk.Game`, dispatching the returned `Command` straight to the
matching `risk.Game` method — it does **not** reuse `bot.Runner`. Two facts
settle that: `Runner` never returns a strategy's `Explanation` (only logs a
string), and `Runner` retries a rejected command up to 3 times, a policy
built for concurrent production state that would mask strategy bugs in a
single-threaded simulator. The one piece of shared shape is a small,
simulator-owned command dispatcher mirroring production's `ApplyGameAction`
switch — duplicated deliberately, not extracted, the same pattern already
used by `ForecastAttack` (duplicates the engine's combat rules) and
`bestCardSet` (duplicates the card-set-validity rule).

## Scope

**In:** new `internal/simulation` package, direct engine execution, one-game
loop, command dispatcher, seeded RNG, seat/strategy assignment, `Result`/
`Failure` structs, safety limits, all four trace levels, one-game
`cmd/simulate` CLI, determinism tests, no-side-effect tests.

**Out:** tournament runner, parallelism, weight tuning/config, Monte Carlo,
ML pipeline, Postgres persistence, distributed execution, dashboard,
Discord, production game creation, frontend changes, and any change to
`internal/risk`, `internal/bot`, `internal/service`, or `internal/game`
(this milestone is purely additive — no existing file is modified).

Monte Carlo is explicitly blocked today: `risk.Game` has no exported
clone/copy method and its RNG field is unexported, so an external package
cannot clone an in-flight game with an independent RNG stream. That needs a
small, deliberate addition to `internal/risk` itself (e.g. an exported
`Clone(rng RNG) *Game`) — out of scope here, noted so it isn't rediscovered
as a surprise later.

## Tasks

Tasks in Phase A have no dependencies on each other and can be built in any
order. Phase B depends on all of Phase A; Phase C depends on Phase B; Phase D
depends on Phase C.

### Phase A — independent components

- [ ] **Config, Limits, and seat/strategy validation** — `internal/simulation/config.go`
  `Config{Seed int64, Seats []string, GameMode, Trace TraceLevel, Limits}`. Validate every seat's strategy ID resolves in `bot.StrategyRegistry` and player count is 3–6 *before* constructing the game — an unresolvable strategy ID is a config error, never a mid-run `Failure`.

- [ ] **Result, SeatResult, and Failure schemas** — `internal/simulation/result.go`
  `Result{Seed, PlayerCount, Seats []SeatResult, WinnerSeat/PlayerID/Strategy, Turns, Commands, CombatRolls, Captures, Eliminations, CardTurnIns, Duration, Completed, Failure}`. `SeatResult{Seat, PlayerID, StrategyID, Eliminated, FinishOrder, FinalTerritories, FinalArmies, Captures, EliminationsMade, CombatLossesTaken}`. `Failure{Type, Message, Phase, PlayerID, StrategyID, Command, CommandIndex, Turn, Seed}`. Failure types: `invalid_strategy_id`, `strategy_error`, `engine_rejected_command`, `command_limit_reached`, `turn_limit_reached`, `repeated_state_detected`, `context_canceled`, `internal_invariant_violated`. `FinishOrder` and final per-seat territory/army counts have no engine equivalent (`playerTerritoryCount` is unexported) — the simulator computes them.

- [ ] **Deterministic RNG implementing `risk.RNG`** — `internal/simulation/rng.go`
  Seeded `math/rand`-backed implementation of `risk.RNG` (`IntN(n int) int`), constructed once per simulation from `Config.Seed`.
  **Landmine:** `risk.Game.rng` is unexported and tagged `json:"-"` — any JSON round-trip of a live `*Game` silently reverts it to non-deterministic `crypto/rand`. The constructed `*Game` must stay alive in-process for the whole run; never marshal/unmarshal it mid-simulation. Document this in the file comment.

- [ ] **Command-to-engine dispatcher** — `internal/simulation/dispatcher.go`
  Translates a `bot.Command` to the matching `risk.Game` method call (full 8-row mapping below). Two details that must be exactly right:
  1. `bot.Command.DefenderDice` is always the zero value — no strategy sets it. The dispatcher must independently compute `min(2, target's current army count)`, matching production's `ApplyGameAction`, never trust `cmd.DefenderDice`.
  2. `occupy`'s `From`/`To` come from `game.Occupy` (set by the preceding `Attack`), not from the `Command` — the command only carries `Armies` for this action.

  | `Command.Action` | Engine call | Notes |
  |---|---|---|
  | `place_initial_army` | `PlaceInitialArmy` | — |
  | `trade_cards` | `TradeCards` | — |
  | `place_reinforcement` | `PlaceReinforcement` | — |
  | `attack` | `Attack` | DefenderDice computed, not read from Command |
  | `end_attack` | `EndAttackPhase` | — |
  | `occupy` | `OccupyTerritory` | From/To read from `game.Occupy`, not Command |
  | `fortify` | `Fortify` | — |
  | `end_turn` | `EndTurn` | Check `PhaseGameOver` here too |

  No 9th row for `claim_territory` — it isn't a bot command, and `PhaseSetupClaim` is unreachable from either recommended constructor.

- [ ] **Loop-detection state fingerprint** — `internal/simulation/statehash.go`
  Cheap, fixed-size fingerprint over a game's *mutable* state (phase, current player, pending reinforcements, territory ownership/armies, card counts, occupy state, per-turn flags) — not a full JSON marshal of the board every call. **Exclude `TurnNumber` and `SetsTraded`** — both are monotonically increasing, so including them would mean a genuine stuck loop never produces a repeated hash.

- [ ] **Safety limits enforcement** — `internal/simulation/limits.go`
  Enforces `MaxCommands`, `MaxTurns`, plus a cheap first-line check (commands with no current-player or phase change) before falling back to the state-hash check. Maps each tripped limit to its `Failure.Type`. These are diagnostic/pathology signals, distinct from `strategy_error`/`engine_rejected_command`, which always mean a real bug.

- [ ] **Trace recorder (none / summary / decision / full)** — `internal/simulation/recorder.go`
  - `none` — final `Result` only.
  - `summary` — + compact milestone list (turn transitions, captures, eliminations, card turn-ins).
  - `decision` — + one entry per `NextCommand` call: the chosen `Command` and its full `bot.Explanation` (score, every feature, capped alternatives), phase, turn, seat. This is the level that matters most for comparing heuristics — cheap only because the direct loop already holds `Explanation` in scope.
  - `full` — + the `statehash.go` fingerprint per command, plus the engine's own `*risk.DomainEvent` verbatim when `Attack` produces one.

  Never store full `risk.Game` JSON snapshots per command at any level — deliberately deferred. Trace collection must never change the gameplay result (verify: same seed at `none` and `full` → identical `Result`).

### Phase B — integration

- [ ] **Core `RunOne` simulation loop** — `internal/simulation/simulator.go`
  *Blocked by all of Phase A.*
  `Simulator.RunOne(ctx, Config) (Result, error)`: validate config → construct RNG → construct game via `risk.NewClassicAutoStartGame` or `NewClassicRandomTerritoryGame` (never bare `NewClassicGame` — it lands in `PhaseSetupClaim`, which no strategy handles) → loop: read phase/current player → resolve seat's strategy → `NextCommand` → record decision → dispatch → record outcome → check limits/state-hash → repeat until `PhaseGameOver`, a limit trips, or an error occurs. **Zero retries anywhere** — any engine rejection or strategy error is an immediate hard `Failure`, unlike `bot.Runner`'s 3-retry policy. Strategies are stateless and safe to construct once per seat (`ScoredStrategy` holds only immutable `Weights`; `BasicStrategy` is empty).

### Phase C — CLI

- [ ] **`cmd/simulate` CLI** — `cmd/simulate/main.go`
  *Blocked by Phase B.*
  Flags: `--seed` (required), `--strategies` (comma-separated, derives player count), `--trace` (default `summary`), `--max-turns`/`--max-commands`, `--format` (`text`|`json`, default `text`), `--output`. No `--weights` flag — no config-loadable `bot.Weights` parser exists in the repo yet. Runs exactly one game; no tournament/parallelism flags.

### Phase D — validation

- [ ] **Cross-cutting determinism/isolation suite + live smoke test**
  *Blocked by Phase C.*
  - Determinism: same seed+config → byte-identical `Result` (excluding `Duration`) and command trace across repeated runs; different seeds diverge.
  - Correctness: every dispatch-table row exercised at least once; game-over detected after both `OccupyTerritory` and `EndTurn`.
  - Isolation: zero imports of `internal/store`, `net/http`, `database/sql`, or any websocket package anywhere in `internal/simulation`/`cmd/simulate`; `bot.Sleeper` never invoked; parallel `RunOne` calls never share a `*risk.Game` or RNG instance.
  - Failure behavior: an engine rejection fails immediately with `engine_rejected_command` and no retry — assert directly against the absence of anything resembling `bot.Runner`'s `maxRejectedCommandRetries`.
  - Finish with `go build ./... && go vet ./... && go test ./...`, then a live run: `go run ./cmd/simulate --seed 1 --strategies basic-v1,scored-v1,scored-v1 --trace decision --format json`, confirming it reaches `game_over` and the decision trace carries real `Explanation` data for the `scored-v1` seats.
