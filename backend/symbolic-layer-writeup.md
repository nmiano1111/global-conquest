# Symbolic Strategic Reasoning Layer for a Risk-like Bot — What We Tried and What Happened

## Background

We have a Go-based multiplayer Risk clone with several hand-tuned heuristic bot strategies. The strongest one, `killbot-v2`, wraps a shared "BetterPixie" heuristic bot (continent-priority placement + army-ratio/win-probability-gated attacking) with a handful of targeted fixes (real win-probability gating instead of raw army-count comparisons, an endgame stalemate breaker, etc.). It's validated at roughly 40-63% win rate against a mixed field of 5-6 other hand-tuned bots, depending on exact field composition.

Earlier the same session, we tried four different tactical tweaks to killbot-v2, all of which failed to beat baseline:

1. Reactive "target whoever's winning" (leader-containment) — regressed win rate badly, twice (once outright, once after a fix that reduced but didn't eliminate the regression).
2. Continent-defense priority — investigated via instrumentation, turned out not to be a real gap (the bot already defends threatened continents about half the time and still loses the fight).
3. Splitting a turn's reinforcements across multiple threatened territories instead of dumping them all on one — regressed (longer games, more timeouts).
4. Fortify-phase consolidation (moving idle armies toward the front) — disconfirmed by measurement (idle armies were *higher* in games the bot won, not lost — the opposite of the hypothesis).

The two that were actually built and regressed (#1 and #3) shared a root cause: both added a new, unconditional "if condition X, always do Y instead" priority tier to the bot's decision cascade. Both ended up diverting the bot from productive play too often — longer games, more command/turn-limit timeouts, lower win rate — even though each individually made intuitive sense.

## The idea: a symbolic reasoning layer

Given the pattern above (every failed idea was a single-step, reactive heuristic with zero lookahead beyond the current turn), we asked whether some form of symbolic AI could help — not to replace the heuristics, but to sit above them: extract explicit strategic facts from the board state (e.g. "this continent is one territory away from being fully owned by a rival"), turn them into goals/biases, and nudge the existing heuristic decision-making rather than overriding it outright.

We deliberately scoped this much narrower than a general "add search/planning" project — no game-tree search, no MCTS, no new learned evaluator (the project has a separate, intentionally-kept-separate ML roadmap for that — GBT scoring → GCN → Monte Carlo with a trained evaluator — this symbolic track exists alongside it, not as a step in it, though the two may merge someday). Just: facts → soft biases → existing heuristics.

The design explicitly built in a lesson from the two regressions above: **soft biases that nudge an existing decision (make an already-legal move more attractive under specific conditions) instead of hard rules that override the existing logic unconditionally.**

## What we built

- A new, independent Go package that reads game state and extracts exactly one kind of fact: "continent X is owned by player P except for exactly one remaining territory T" — split into "an enemy is one territory from completing a continent" and "I am one territory from completing a continent."
- Two rules, both doing the same thing: if the missing territory of a near-complete continent is a legal attack target this turn, relax the bot's normal win-probability threshold for attacking *that specific territory only* (e.g. normally require ~50% win probability to attack, but allow down to 35% for this one high-value, denial/completion-worthy target).
- This is wired in as a small, additive, opt-in hook on the existing bot — nil/disabled by default, and verified to be a complete no-op when disabled (every pre-existing unit test for the base bot still passed completely unmodified after adding the hook).
- A new bot variant that's just the existing bot plus this bias, registered only in the internal tournament/testing tool, never the live game server — kept fully experimental.
- ~16 new unit/integration tests, all passing, plus a full clean build/test of the whole project.
- A long design/roadmap document covering the architecture, the two rules, what was deliberately deferred for a first cut (hard constraints, persistent goals that survive across turns, historical/event-based facts like "this player has been the aggressor against me repeatedly," more rule types), and a short technology comparison (plain Go vs. Datalog vs. Prolog vs. Haskell) for if/when this ever needs to get more sophisticated. Current recommendation there: stay in plain Go until the rule set is much larger and rules start depending on each other in complex ways.

## Measurement

We ran the new bot head-to-head against plain killbot-v2 in the same tournament field (300 simulated games per run, same random seeds, same other opponent bots) — twice:

**Run 1** (initial bias strength — relax the win-probability threshold by 15 percentage points, with a floor of 35%): killbot-v2 won 40.5% of games, the new bot won 37.8%. Slightly *worse*, though the 95% confidence intervals overlapped heavily (roughly ±6 points on each side).

**Run 2** (same seeds and field, bias pushed harder — relax by 25 points, floor of 25% — specifically to check whether the mechanism had *any* measurable effect at all when strengthened): killbot-v2 won 40.3%, the new bot won 41.5%. Slightly *better* this time — but again, the confidence intervals overlap almost entirely.

The tell: killbot-v2's own win rate was essentially identical across both runs (40.5% → 40.3%, a good sanity check that the two runs are apples-to-apples), while the *new* bot's win rate swung by nearly 4 percentage points purely from changing the bias strength. That's a bigger swing than the actual difference between the two bots within either single run — a strong signal we're looking at run-to-run noise, not tuning uncovering a real underlying effect.

## Where we landed

We reverted all the code (deleted the new package and bot variant, reverted the hook on the base bot) but kept the design document, on the reasoning that the *architecture* — the additive/non-destructive hook pattern, the facts → rules → bias pipeline, the "soft bias not hard override" principle — still seems sound and reusable, even though the *specific two rules* (continent-completion only) didn't show any measurable benefit in two tournament runs.

## Open questions we'd like a second opinion on

1. Is "continent completion" too narrow a slice of situations to show up in an aggregate whole-game win-rate metric, even if the individual decisions it changes are genuinely good in isolation? I.e., is this a statistical-power problem (the situation just doesn't come up often enough per game to move the needle) rather than the underlying idea being wrong?
2. Given that four *other*, completely unrelated heuristic tweaks also failed to beat this same baseline earlier in the session, is a ~40-63% win rate (against this specific field of hand-tuned bots, exact number depending on field size) plausibly close to a real ceiling for this general style of architecture ("single-step heuristic + small contextual nudges"), such that getting meaningfully past it would require actual lookahead/search rather than more rules of this same kind?
3. Is there a smarter way to validate a change like this with fewer games or less noise than a blind win-rate tournament, given how much confidence-interval overlap we're seeing even at 300 games per run?
4. Any reaction to the "soft bias, never a hard override" architectural principle itself — adopted specifically to avoid a failure mode we'd already hit twice — as a general design rule for this kind of incremental bot-strategy work?
