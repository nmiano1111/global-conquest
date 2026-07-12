# Global Conquest Bot UI Behavior and Human-Like Turn Pacing

## Purpose

This document defines how computer-controlled players should behave in live Global Conquest matches so that their turns feel legible, intentional, and socially compatible with human play.

The goal is not to make the bot pretend to be human or hide that it is automated. The goal is to avoid a poor user experience where a bot completes an entire turn in milliseconds, causes the board to jump through many states instantly, and gives human players no practical chance to understand what happened.

This behavior applies only to interactive live games. It must be disabled for simulations, automated tournaments, analytics generation, tests, and other headless workloads.

## Core Principle

A bot-controlled player should use the same authoritative engine commands as a human player, but command submission should be paced by a separate presentation-oriented turn runner.

The engine remains responsible for legality, dice, losses, state transitions, captures, occupation, fortification, and game completion.

The live bot runner is responsible for delays between visible actions, pausing after significant events, keeping intermediate states observable, and stopping safely if the game changes unexpectedly.

The live pacing layer must never alter rules or bypass engine validation.

## Goals

1. Make each major decision understandable to spectators and opponents.
2. Allow the frontend to render intermediate authoritative states.
3. Make combat sequences visually followable.
4. Avoid unnecessary delay during trivial actions.
5. Preserve fast, deterministic simulation execution.
6. Keep UI timing outside gameplay transactions.
7. Support configurable pacing profiles.
8. Avoid blocking the WebSocket hub or mutation processing.
9. Resume safely after restarts.
10. Keep bot players clearly identified as bots.

## Non-Goals

- Deceiving users into believing a bot is human
- Simulating mouse movement or browser input
- Adding sleeps to the game engine
- Holding transactions open during pacing delays
- Delaying simulations
- Letting an LLM control timing
- Making animation timing part of correctness

## Execution Modes

### Live Interactive Mode

Used in normal games involving human players.

- Submit commands one at a time.
- Broadcast every committed state normally.
- Wait between visible actions.
- Pause longer after important outcomes.
- Stop if the bot is no longer the current player.

### Simulation Mode

Used for bot tournaments, analytics, tests, and balance work.

- No artificial delays.
- No transient UI indicators.
- No Discord chatter unless explicitly enabled.
- Execute as quickly as engine and persistence constraints allow.

The mode should be explicit:

```go
type BotExecutionMode string

const (
    BotExecutionLive       BotExecutionMode = "live"
    BotExecutionSimulation BotExecutionMode = "simulation"
)
```

Do not infer mode from whether humans happen to be present.

## Recommended Architecture

```text
Bot Manager
    ↓
Strategy selects next legal command
    ↓
Live Turn Runner or Simulation Runner
    ↓
Normal application command path
    ↓
Authoritative engine
    ↓
Database transaction commits
    ↓
WebSocket game_state_updated broadcast
    ↓
Optional pacing delay
    ↓
Next decision
```

The strategy should not sleep. The engine should not sleep. The repository should not sleep. Only the live runner applies pacing.

## Bot Turn Runner

A bot turn should be a sequence of independently committed commands.

```go
type BotTurnRunner interface {
    RunTurn(
        ctx context.Context,
        gameID uuid.UUID,
        botPlayerID uuid.UUID,
        mode BotExecutionMode,
    ) error
}
```

The live runner should:

1. Load current state.
2. Confirm the bot is still current player.
3. Ask the strategy for one next command.
4. Submit through the normal application path.
5. Wait for commit success.
6. Classify the visible significance of the result.
7. Apply a configured delay.
8. Reload authoritative state.
9. Repeat until the turn ends or context is canceled.

Do not calculate a whole turn against one stale snapshot.

## Pacing Categories

```go
type ActionPace string

const (
    PaceImmediate   ActionPace = "immediate"
    PaceMinor       ActionPace = "minor"
    PaceNormal      ActionPace = "normal"
    PaceSignificant ActionPace = "significant"
    PaceDramatic    ActionPace = "dramatic"
)
```

Suggested live defaults:

| Pace | Delay | Typical use |
|---|---:|---|
| Immediate | 0–150 ms | No meaningful visible change |
| Minor | 300–700 ms | Small reinforcement placement |
| Normal | 700–1,400 ms | Attack or fortification decision |
| Significant | 1,200–2,200 ms | Capture or card turn-in |
| Dramatic | 2,000–3,500 ms | Elimination or winning move |

Use bounded ranges rather than fixed durations. Small random variation avoids a mechanical cadence without materially slowing play.

## Suggested Turn Cadence

### Turn Start

Broadcast the new turn immediately, then wait 800–1,800 ms before the first action.

### Card Turn-In

Commit the turn-in and pause 1,200–2,200 ms so the reinforcement change is visible.

### Reinforcement

Prefer one command per meaningful placement rather than one command per army.

- Between placements: 400–900 ms
- Before attack phase: 700–1,400 ms

### Attack

- First roll against a target: 900–1,300 ms
- Repeated roll against same target: 450–850 ms
- Territory capture: 1,200–2,000 ms
- Player elimination: 2,000–3,500 ms

Repeated combat should accelerate enough to avoid tedious turns while remaining followable.

### Occupation

Because occupation is a distinct phase:

- Before occupation command: 600–1,200 ms
- After occupation result: 500–1,000 ms

### Fortification

Pause 900–1,600 ms after the fortification commits.

### End Turn

Do not delay making the next player current after the final command commits. Any dramatic pause belongs before the final transition, not inside the transaction.

## Frontend Behavior

The frontend remains render-only and continues replacing local state with each full authoritative snapshot.

Optional transient activity messages may improve UX:

```json
{
  "type": "bot_activity",
  "payload": {
    "game_id": "...",
    "player_id": "...",
    "activity": "thinking"
  }
}
```

Possible values:

- `thinking`
- `reinforcing`
- `choosing_attack`
- `attacking`
- `occupying`
- `fortifying`

These are best-effort presentation messages. They must not be persisted in `games.state` or required for correctness.

## Human-Like Does Not Mean Artificially Slow

The goal is legibility, not theater.

A typical live bot turn should usually take several seconds to perhaps tens of seconds depending on combat length. Obvious decisions should remain quick. Long pauses should be reserved for major events.

## Pacing Configuration

```go
type LivePacingConfig struct {
    TurnStartMin time.Duration
    TurnStartMax time.Duration

    MinorMin time.Duration
    MinorMax time.Duration

    NormalMin time.Duration
    NormalMax time.Duration

    SignificantMin time.Duration
    SignificantMax time.Duration

    DramaticMin time.Duration
    DramaticMax time.Duration

    RepeatAttackMultiplier float64
}
```

Possible presets:

- Fast
- Standard
- Cinematic
- Instant

Do not treat simulation mode as merely “fast live mode.” Simulation may bypass presentation paths entirely.

## Cancellation and State Drift

Before every bot action:

- Reload authoritative state.
- Verify the game is active.
- Verify the bot is current player.
- Verify the phase.
- Recompute legal actions.
- Recompute strategy.

Cancel the runner when:

- The process shuts down.
- The game ends.
- The bot is no longer current player.
- An administrator takes over.
- The game is paused.
- The bot is removed or converted to human.
- The runner context expires.

Never submit a cached action after a delay without revalidation.

## Transaction Rules

Never hold a transaction open during a delay.

Correct:

```text
load state
→ choose one action
→ open transaction
→ lock row
→ revalidate/apply
→ commit
→ broadcast
→ sleep
```

Incorrect:

```text
open transaction
→ lock row
→ sleep
→ apply
→ commit
```

## WebSocket Considerations

Pacing reduces the chance of flooding a slow client with rapid snapshots, but pacing is not a reliability mechanism.

Clients must still recover through bootstrap or reconnect because delivery remains at-most-once.

## Strategy and UI Separation

Strategy should return a command and semantic reason, not a duration.

```go
type Decision struct {
    Command risk.Command
    Reason  DecisionReason
}
```

The live runner maps reasons and committed outcomes to pacing categories.

Examples:

- Routine reinforcement
- Attack continuation
- Territory capture
- Player elimination
- End turn

## Significant Event Detection

Some delays depend on the committed result rather than the submitted command.

Use typed domain events or application results to detect:

- Territory capture
- Player elimination
- Continent acquisition
- Card turn-in
- Game completion

Do not infer these by diffing arbitrary JSON when semantic events already exist.

## Failure Handling

### Strategy Error

Log and stop unless the error is explicitly transient. Do not automatically end the turn unless that is a deliberate fallback policy.

### Engine Rejection

Reload state, recompute legal actions, and retry with a strict bound. Repeated illegal decisions are a bug and must not loop forever.

### Database Failure

Stop submitting actions, use bounded backoff, and resume only when authoritative state can be loaded.

### Process Restart

On startup, scan active games. If the current player is a bot, reconstruct a runner from current authoritative state. Do not rely on an in-memory turn plan.

## Simulation Runner

The simulation runner should reuse strategy, legal-action generation, engine commands, evaluation, and event collection while bypassing:

- Sleeps
- UI activity events
- Discord messages
- Human-facing logging
- WebSocket broadcasts where unnecessary

## Testing Strategy

Inject sleep behavior:

```go
type Sleeper interface {
    Sleep(ctx context.Context, duration time.Duration) error
}
```

Use a fake sleeper in tests.

Unit tests should cover:

- Pace classification
- Repeat-attack acceleration
- Capture and elimination escalation
- Zero delay in simulation mode
- Config bounds
- Cancellation during delay
- Strategy independence from pacing

Integration tests should cover:

1. Commands are submitted one at a time.
2. State commits before each delay.
3. No transaction remains open during delay.
4. State reload occurs between commands.
5. Runner stops when current player changes.
6. Runner resumes from authoritative state after restart.
7. Simulation mode performs no sleeps.
8. Existing broadcasts still occur.
9. Stale decisions are recomputed.
10. Game completion stops the runner.

Behavioral tests should assert semantic ordering rather than exact milliseconds.

## Observability

Track:

- Bot turn start and completion
- Commands submitted
- Total live turn duration
- Strategy decision duration
- Artificial pacing duration
- Engine/application duration
- Illegal-command retries
- Runner cancellations
- Restart recovery

Keep `decision_time`, `execution_time`, and `presentation_delay` distinct.

## Suggested Implementation Phases

### Phase 1

- Explicit live and simulation modes
- Injectable sleeper
- Configurable pacing ranges
- One-command-at-a-time live runner
- State reload before every action
- No delays inside transactions
- Basic pacing for reinforcement, combat, capture, occupation, fortify, and turn start
- Fake-time tests

### Phase 2

- Bot activity WebSocket messages
- Adaptive repeated-combat pacing
- Restart recovery
- Structured timing metrics
- Speed presets

### Phase 3

- Pause, resume, and takeover controls
- Cinematic spectator mode
- Discord typing/status integration
- More sophisticated event-aware pacing

## Claude Implementation Prompt

> Add a live bot turn runner that executes bot decisions through the existing authoritative command path one command at a time. The runner must support explicit `live` and `simulation` modes. In live mode, apply configurable bounded delays between committed visible actions; in simulation mode, apply no artificial delays. Never sleep inside a database transaction. Reload authoritative state and recompute the next decision after every committed action. Introduce an injectable sleeper or clock so tests do not depend on wall-clock delays. Keep strategy, rules, and pacing separate. Add focused tests for cancellation, state drift, action pacing categories, transaction boundaries, and zero-delay simulation behavior. Do not add frontend activity indicators, Discord behavior, or new strategy heuristics in this milestone.

## Acceptance Criteria

- A live bot does not complete an entire turn instantaneously.
- Humans can follow reinforcements, attacks, captures, occupation, and fortification.
- The engine remains the rule authority.
- No transaction is held open during delays.
- Simulation mode has no artificial waits.
- Bot turns can be canceled safely.
- State is reloaded between actions.
- Tests use fake time.
- Existing human gameplay behavior remains unchanged.
