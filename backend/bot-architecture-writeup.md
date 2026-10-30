# Killbot-v2 and the Goal-Directed Symbolic Layer — Architecture

## Part 1: `killbot-v2`

### Overview

The backend is a Go service that owns all game rules for a real-time multiplayer Risk clone (`internal/risk`); the frontend is a render-only client. Bots are pluggable `Strategy` implementations (`internal/bot`) — each turn, the game server asks the current player's `Strategy` for one `Command` at a time (place reinforcement, attack, occupy, fortify, end phase), applies it through the same transactional path a human's WebSocket action would use, and repeats until the turn ends. A strategy never mutates game state directly; it only returns commands.

`killbot-v2` is the strongest hand-tuned bot in the project and the current live default. It isn't one flat implementation — it's three layers, each wrapping the one below via Go composition (an embedded struct, not inheritance), each overriding only what it needs to fix or extend:

```
BetterPixieStrategy   (base heuristic, ported from an open-source Risk-clone AI)
        ↑ wrapped by
KillbotStrategy        (adds explicit "hunt down a specific rival" behavior)
        ↑ wrapped by
KillbotV2Strategy       (fixes 4 specific bugs/gaps found by testing; current production default)
```

### `BetterPixieStrategy` — the base heuristic

This is priority-tier, imperative decision logic — **not** a scored/ranked system. There is no numeric "value" computed for candidate moves anywhere in this layer; each phase is a sequence of `if`/`else` checks, and whichever check matches first wins.

- **Reinforce** (`betterPixiePlacement`): in order —
  1. If a continent the bot already wants and considers worth pursuing has a weak border (`betterPixieContinentNeedsHelp`), reinforce that border.
  2. Else, if it doesn't currently want any specific continent, fall back to whatever the plain "take a continent" logic picks.
  3. Else, place near the weakest reachable cluster of enemy territory.
  4. Else, fall back to whichever owned territory has the most attackable enemy neighbors.

  Critically, **this always places the entire turn's reinforcement pool on one chosen territory in a single command** — a convention every strategy in the codebase follows, though the engine itself does not require it (see Part 2).

- **Attack**: continent-takeover logic gated on a friendly-vs-enemy army ratio within the continent, using either a raw army-count comparator or a real win-probability comparator depending on tuning; falls back to an "attack as much as possible" hogwild mode once the bot is dominant enough that further caution stops mattering.

- **Occupy**: decides how many armies to move into a just-conquered territory based on enemy-neighbor counts and continent-connectivity heuristics (defend a vulnerable new front vs. keep the stack consolidated).

- **Fortify**: moves armies toward continent borders or territories adjacent to a weak enemy; if nothing qualifies, it ends the turn outright — there's no separate "decline to fortify" action, only `end_turn`.

### `KillbotStrategy` — explicit kill-hunting

Adds a turn-scoped commitment: if a specific rival player looks eliminable (`killTarget`/`dominantPlayerToKill`), the bot commits to chasing them via `risk.Game.KillPlans` (cleared automatically at the start of every turn — this commitment never survives across turns). Once committed, `attack()` single-mindedly walks the cheapest route toward that player's territory instead of considering anything else, and `occupy()` forces maximum armies along that route so the pursuing stack is never diluted.

### `KillbotV2Strategy` — the production layer

Wraps `KillbotStrategy` and fixes four specific defects found by testing, without touching the shared base code other bot variants (Cluster, Quo, Boscoe, Pixie) also depend on:

1. **Locked-in reinforcement redirect** (`guardPlacement`): if the underlying pick has zero attackable neighbors, redirect to the best legal territory that actually has one.
2. **Board-relative hogwild threshold**: replaces an absolute army-count cutoff (tuned for a different, fixed-size map) with one that scales to this project's variable board sizes.
3. **Stalled kill falls through, doesn't end the turn**: if a committed kill route can't fire this turn, `KillbotV2Strategy` still tries the normal attack repertoire instead of giving up on the whole attack phase.
4. **Real win-probability gating**: replaces a raw army-count comparison with actual combat-forecast math (`risk.ForecastAttack`) for deciding whether to commit to and fire a kill route.

`KillbotV2Strategy` also introduces one extensibility point used later by the symbolic layer: an optional `WithAttackBias` hook — a function that can relax the normal win-probability bar for one specific candidate attack. It's `nil` by default and a proven no-op when unset (every pre-existing test still passes unmodified with it in place).

### Decision cycle per turn

`reinforce` (one command places the whole pool, though the engine allows more) → `attack` (repeated commands until the bot ends the phase or runs out of legal attacks) → `occupy` (forced immediately after each conquest) → `fortify` (a single decision) → next player. Tuning constants (minimum attack win probability, kill-commitment thresholds, hogwild army density, etc.) live in `risk.BotTuning` and are overridable per-game.

---

## Part 2: the goal-directed symbolic layer (`goal-killbot-v2`)

### Motivation

Two earlier attempts at adding "strategic" reasoning on top of `killbot-v2` were built, measured, and found wanting — each time root-caused with real instrumentation rather than left as a guess:

- **Pass 1**: a single-decision reactive nudge (relax the attack bar for a near-complete continent, recomputed fresh every call, no memory). No measurable effect across two tournament runs. Reverted.
- **Pass 2** (`goal-killbot-v1`, kept in the codebase as an experimental baseline): a *persistent* goal that survives across turns — a real improvement in shape — but its commit gate required the position to already be almost winnable *before* it would ever commit. Diagnosed via instrumentation: near-complete continents were common (23% of decisions), but only 1.9% of those already had a strong enough army stack to pass the gate. A chicken-and-egg trap — nothing steered reinforcement toward the border until a goal existed to direct it, and a goal couldn't form until the border already looked strong.

This third pass (`goal-killbot-v2`) redesigns the goal mechanism specifically to fix that trap: let a goal commit on the *plausibility* of eventually succeeding, then have it actively work to make itself true.

### The engine-level goal object

```go
type StrategicGoal struct {
    Continent   Continent
    StartTurn   int
    Committed   bool
    Status      GoalStatus // "active" | "securing"
    StatusSince int
}
```

Stored per-player in `Game.StrategicGoals`, and — unlike the kill-hunting commitment above — **deliberately never cleared at a turn boundary**. Its entire purpose is to persist across many turns until the strategy that owns it explicitly completes or abandons it. Which single territory is still missing from the continent is never stored — it's re-derived fresh from current board ownership every time it's read, the same way the kill-hunting commitment only stores a target player, never a precomputed route.

### Desirable, feasible, and executable-now — three different questions

The redesign's central idea is refusing to conflate these:

- **Desirable**: a near-completion fact exists at all (owns every territory in a positive-bonus continent except exactly one). Cheap, existing fact-extraction logic, unchanged from pass 2.
- **Feasible** *(new this pass)*: could the required attacking force plausibly be built within a short, bounded horizon (2 turns)? Computed by finding the minimum attacker army count that would clear a win-probability floor against the missing territory's *current* defenders (a simple monotonic search over the combat-forecast function), then projecting whether current armies plus a conservative estimate of future reinforcement income could plausibly reach that count in time. This is the actual fix — a goal can now commit *before* the position looks winnable, as long as it looks buildable soon.
- **Executable now**: never stored — a purely derived signal (current armies alone already clear the bar), computed fresh whenever something needs it, the same way the missing territory itself is never stored.

A goal commits the moment it's desirable *and* feasible — not once it's already executable.

### Lifecycle

```
(no goal) → Active → Securing → (cleared)
                ↓
           abandoned (feasibility lost, or a turn-count cap elapses)
```

**Active** covers the goal from commitment through capturing the missing territory — whether or not it happens to already be executable this exact turn is not a separate stored state, since the bot behaves the same way either way while Active. **Securing** is a short post-capture consolidation tail (border defense) before the goal clears. The strategy's returned commands carry the transition intent (`StrategicGoal` to commit, `StrategicGoalStatus` to advance, `AbandonStrategicGoal` to clear); the engine applies the actual state change — the bot itself never mutates `Game` directly, consistent with everything else in this codebase.

### Phase integration

`GoalKillbotV2Strategy` wraps `KillbotV2Strategy` the same way `KillbotV2Strategy` wraps `KillbotStrategy` — composition, overriding only what's needed:

- **Reinforce**: while Active and not yet executable, sends only the *top-up* needed to reach the required army count — `min(required − current, this turn's pending pool)` — never the whole pool. This is a deliberate fix over pass 2's all-or-nothing placement, made possible by a fact discovered during this pass: the engine already legally supports placing reinforcements across multiple commands in the same turn (any amount up to what's left, repeated until the pool hits zero); no strategy in the codebase had ever used that before. If the top-up doesn't consume the whole pool, the remainder is left for the bot's own normal placement logic later in the same turn. A bypass skips goal-directed reinforcement (without abandoning the goal) on any turn the bot's own top defensive priority has a genuinely urgent claim elsewhere — but only once a goal is already committed; testing found applying the same bypass during initial discovery blocked nearly every fresh commitment on a false signal (the base heuristic always "wants" exactly one continent early on, even an entirely unclaimed one, which trivially looks "urgent" by the same check).
- **Attack**: reuses the existing `WithAttackBias` hook from `killbot-v2` unchanged — it relaxes the win-probability bar specifically for the goal's missing territory. New this pass: a guard that blocks an unrelated attack the bot would otherwise launch *from* the goal's staging territory, so an accumulating stack isn't spent on an unrelated fight. This is a hard constraint, not a soft bias — there's no numeric attack-priority score anywhere in this codebase's decision logic to blend a bias into, so the only lever available is an outright block.
- **Occupy**: unchanged from pass 2 — forces maximum legal armies onto the missing territory specifically, once conquered.
- **Fortify**: while Active, prefers moves toward the missing territory's border; if none qualify, the same stack-preservation guard as attack (decline the fortify move rather than let it drain the staging territory — lower-risk here since fortify is only ever one decision per turn regardless). While Securing, prefers moves toward the newly-completed continent's external borders instead.

### Instrumentation

Two free, zero-engine-risk mechanisms, both reused from earlier passes: a decision-observer hook computes, for every decision, what a plain `killbot-v2` (no goal bias at all) would have chosen at the exact same, unmutated game state, and reports whether the goal-directed choice actually differed — safe because a strategy's own decision function is contractually read-only. A turn-boundary hook tracks goal lifecycle transitions (committed / entered securing / completed / abandoned) purely by diffing the persisted `StrategicGoal` turn-over-turn.

---

## Part 3: what we found

Measured over 300-game tournaments against a shared field of other hand-tuned bots:

- **The architectural fix works.** 20.3% of games saw at least one commitment (clearing the bar pass 2 missed), and of all commitments, **48.8% went on to actually capture the target** — real, repeated proof the mechanism proactively builds toward and achieves goals a purely-reactive bot wouldn't have.
- **It doesn't win more games.** A properly paired, seat-mirrored comparison against plain `killbot-v2` showed a small, statistically inconclusive *disadvantage* (36.3% vs. 40.0%) — a reversal from pass 2's similarly inconclusive nominal advantage.
- **Root cause of the gap (best current lead, not fully confirmed):** the dominant reason a goal gets abandoned (67% of cases) is the near-completion fact itself disappearing — the bot loses a *different* territory in the same continent to an opponent while its reinforcements are concentrated on the one staging territory. Plausible interpretation: pursuing the goal leaves the rest of the continent thinner than normal play would, making it easier for someone else to knock the whole plan over before it finishes — a real tunnel-vision cost. Confirming that as causal (rather than just correlated) would need a true counterfactual replay harness, deliberately not built in either pass given the engine risk involved.

Both `goal-killbot-v1` and `goal-killbot-v2` are kept in the codebase as independent, unmodified experimental baselines — registered only in the offline tournament tool, never the live game server.
