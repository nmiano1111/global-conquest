# Phase 1 Implementation Status

This documents what actually shipped for Phase 1 (foundation), as opposed to
the original design docs in this directory, which describe the target
architecture. Where this doc and the design docs disagree, this doc — and
the code — win.

## Important correction vs. the original design docs

Global Conquest does **not** use player-driven territory claiming during
setup. `JoinClassicGame` always starts a game via `NewClassicAutoStartGame`
or `NewClassicRandomTerritoryGame`, both of which skip `setup_claim`
entirely. **Bots never choose territory claims.** The bot's first
meaningful decision is during `setup_reinforce`.

## Player controller model

`risk.PlayerState` (serialized inside `games.state`, not a separate table)
gained two fields:

```go
type PlayerState struct {
    ID         string
    Cards      []Card
    Eliminated bool
    Controller ControllerType `json:"controller,omitempty"`
    Strategy   string         `json:"strategy,omitempty"`
}
```

`ControllerType` is `"human"` or `"bot"` (`risk.ControllerHuman` /
`risk.ControllerBot`); `PlayerState.IsBot()` reports whether a player is
bot-controlled. Both fields are `omitempty` and zero-value-safe: games
serialized before this field existed decode with `Controller == ""`, which
`IsBot()` correctly reports as human. No migration was needed — this is
JSONB state, not a schema change.

There is currently no lobby/REST flow to actually add a bot to a game; this
milestone only adds the underlying model and runtime. Games with bot
players exist today only via direct state construction (tests, or a future
lobby feature).

## Supported strategy: `basic-v1`

The only strategy this milestone ships. It plays **legal, complete games**,
not **strong** games — see Known Limitations below. Implemented in
`backend/internal/bot/strategy_basic.go`.

| Phase | Behavior |
|---|---|
| `setup_reinforce` | Places one army on the owned territory facing the largest total adjacent enemy army count; ties broken by canonical board order. |
| `reinforce` | Card turn-in first (see policy below), then places **all** pending reinforcements on the single owned territory with the largest adjacent enemy threat, preferring one that's weak relative to that threat. |
| `attack` | Only attacks when `source armies >= target armies + 2`. Among eligible attacks, prefers the weakest target, then the largest army advantage, then stable board order. Always uses the maximum legal attacker dice. Ends the attack phase if nothing qualifies. |
| `occupy` | Always moves the minimum legal amount. |
| `fortify` | Prefers moving the maximum legal amount from an interior territory to whichever legal destination faces the largest enemy threat. Ends the turn instead of making a fortification that doesn't face any threat, or if already fortified this turn. |

## Card turn-in policy

Implemented via `risk.LegalCardTurnIns` / `risk.CardTurnInRequired` +
`BasicStrategy.cardTurnIn`:

1. If a valid set exists, it is turned in immediately — whether or not it's
   mandatory (`len(cards) >= 5`).
2. Among multiple legal sets, the one with the lowest card indices
   (ascending) is chosen — fully deterministic.
3. After a turn-in commits, the runner reloads authoritative state before
   deciding the next command (reinforcement placement, or another turn-in
   if still eligible).

No card-hoarding, future-set-value, or opponent-card-count strategy exists
yet — deliberately deferred.

## Legal-action helpers (`backend/internal/risk/legal_actions.go`)

Read-only, deterministic, non-mutating queries the engine already has
enough information to answer without any bot-specific logic:
`LegalSetupReinforcements`, `LegalCardTurnIns`, `CardTurnInRequired`,
`LegalReinforcements`, `LegalAttacks`, `LegalOccupations`,
`LegalFortifications`. They reuse the engine's own `Board.IsAdjacent`,
`isContiguous`, and `isValidSet` rather than re-deriving legality. The
engine still validates every command when it's actually applied — these
helpers only narrow the search space for the strategy.

## Live vs. simulation mode

```go
type ExecutionMode string
const (
    ExecutionLive       ExecutionMode = "live"
    ExecutionSimulation ExecutionMode = "simulation"
)
```

Live mode applies one flat delay (`bot.DefaultLiveDelay`, 750ms) between
committed actions via an injectable `Sleeper`. Simulation mode never
sleeps. Phase 1 deliberately does not implement action-specific pacing
categories (attack vs. capture vs. elimination) — that's a later milestone
per the pacing design doc in `phase_2_first_playable_bot/`.

## How runners start, run, and stop

`bot.Runner.RunTurn(ctx, gameID, mode)` drives **one bot player's one
turn**: load state → verify still `in_progress`, not `game_over`, and the
current player is bot-controlled and the same bot this call started with →
ask the strategy for one command → submit through
`game.Server.SubmitGameAction` (the same transactional
`GamesService.ApplyGameAction` + broadcast path human WebSocket commands
use) → sleep (live mode only) → reload → repeat. It returns a `StopReason`
(`turn_ended`, `not_bot_turn`, `game_over`, `game_inactive`, `load_error`,
`strategy_error`, `max_retries_exceeded`, `canceled`) rather than just an
error, so callers know whether it's worth checking for a follow-on turn.

`bot.Manager` keeps an in-memory registry (single-process only) of active
games and guarantees at most one runner per game. It re-triggers itself
**only** when a runner stops with `turn_ended` — every other reason
(including "not a bot's turn") does not chain, so a human-controlled game
never busy-loops.

**Trigger points**, wired in `cmd/backend/main.go` via
`game.Server.SetBotTrigger`:
- After every committed game action, human or bot (`game.Server.commitGameAction`).
- On backend startup, once per `in_progress` game found (`recoverBotGames`).

The runner's own state check is cheap enough that the startup scan and the
post-commit hook both trigger unconditionally — no separate "is this game's
current player a bot" pre-filter is needed outside the runner.

### Submission path

Bots never call `GamesService.ApplyGameAction` directly. They call
`game.Server.SubmitGameAction`, which posts onto the hub's existing inbox
channel and is processed inside the single hub goroutine (`Run()`) —
preserving the "single-goroutine access to `s.clients`/`s.chatRooms`"
invariant — then commits, broadcasts `game_state_updated` to the game's
chat room exactly like a human action would, and fires the bot trigger.

## Restart recovery

No in-memory bot plan is ever persisted. On startup, `recoverBotGames`
lists every `in_progress` game (paginated, 200 per page) and calls
`Manager.Trigger` for each — the runner's own state load is what actually
resumes play from the persisted phase.

## Known limitations of `basic-v1`

- No continent-completion strategy, elimination targeting, revenge
  behavior, or opponent card-count awareness.
- Reinforcement and setup scoring only look at directly adjacent enemy
  armies, not the wider front.
- Attack/fortify choose exactly one action per phase iteration by design —
  the runner loop is what produces a full sequence of attacks across a
  turn, not multi-step planning inside the strategy.
- No lobby/REST API exists yet to actually add a bot player to a game.
- `Manager`'s registry is in-memory and single-process; a multi-instance
  deployment would need a distributed lock, per the existing "1 ECS task"
  constraint noted in the top-level `CLAUDE.md`.
- Live-mode pacing is a single flat delay, not the action-specific
  categories described in `phase_2_first_playable_bot/06_Bot_UI_Behavior_and_Live_Turn_Pacing.md`.
