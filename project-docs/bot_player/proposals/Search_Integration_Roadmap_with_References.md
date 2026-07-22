# Global Conquest – Search Integration Roadmap

> Elaborates milestone 11 ("Search integration") of `GCN_Strategy_Roadmap_with_References.md`, now that milestones 1-10 (schema, encoding, candidate afterstates, supervised GCN, TD(λ) training) are built and validated. A deeper-but-still-single-opponent adversarial-ply lookahead (`LookaheadDepth` on `ValueStrategy`) was built, evaluated, found to make things worse, and removed from the codebase — see "Motivating result" below for the actual numbers and analysis, preserved here even though the code itself is gone. Describes what it actually takes to move from that to the paper's real search-based Attack Handler. **Not started** — this is the plan for later work, written down before starting so the scope is explicit going in.

## Primary References

1. **Using Graph Convolutional Networks and TD(λ) to Play the Game of Risk** (Jamie Carr, 2020) — https://arxiv.org/abs/2009.06355. Section 3.1 (search algorithm), Section 3.3 (Attack Handler). The single source of truth for what "the real thing" looks like; every design decision below either matches it or explains why we're deliberately diverging.

## Where we are today

- `internal/bot/strategy_value.go`'s `ValueStrategy.attack()` enumerates every legal attack via `risk.LegalAttacks`, scores each **independently** via a plain 1-ply `attackAfterstateBlend`, and picks the single best-scoring candidate. Non-attack phases (reinforce/occupy/fortify) never look ahead at all. A deeper, still-single-opponent adversarial-ply generalization (`LookaheadDepth`, `internal/bot/lookahead.go`) was built and evaluated this session, then removed after measuring worse — see "Motivating result" below.
- The GCN value function (`internal/bot/gcnmodel`, trained via `analytics/.../gcn_fit.py`'s `fit_gcn_td`) reached a real, reproducible ~17% win rate this session (12 epochs, TD(λ), `--percentile 0` margin calibration) — no longer the weak link it was when the original shallow lookahead was tested and showed no improvement.
- Non-determinism (dice outcomes) is handled today via `internal/bot/afterstate.go`'s `attackAfterstateBlend`: a probability-weighted blend of two hypothetical afterstates ("conquered," "held"), not an exact simulation of any one dice sequence. This is an intentional simplification carried from the very first `BoardValue` work (see `11_Learned_Board_Evaluation.md`) and is one of the pieces the real Attack Handler replaces.

## Motivating result: the narrow greedy-chained version was tested, and made things worse

This session built `LookaheadDepth` on `ValueStrategy`: alternating "our candidate attack, then the single most relevant opponent's own greedy-best reply" for `N` plies, recursively. It was deliberately narrow in two ways this roadmap exists to remove: **(a)** scoped to exactly one opponent — the contested territory's former owner — never the whole board, and **(b)** at every ply, each side's "move" was *given* by greedy 1-ply selection, never itself explored as a branching choice.

Evaluated at depth=2 against the 12-epoch/no-lookahead baseline (~17% win rate, reproduced three times), with margins properly recalibrated for the depth=2 score distribution via `cmd/bvcalibrate --lookahead-depth` (a first attempt reused the 0-ply-calibrated margins by mistake and showed an even worse, confounded result — recalibrating first was not optional). Real, apples-to-apples result, 200 games per matchup:

| matchup | no lookahead | depth=2 |
|---|---|---|
| `board-value-candidate,boscoe-v1,killbot-v1,quo-v1` | ~17% | **5.9%** [2.5-13.0] |
| `angry-v1,basic-v1,cluster-v1,pixie-v1,scored-v1,board-value-candidate` | ~17% | **10.5%** [6.7-16.2] |

Depth=2 was worse in *both* matchups, not just flat. Three likely reasons, all of which point at the same conclusion — this isn't real search, it's a chain of greedy 1-ply picks wearing a search-shaped hat, and that shape actively hurts once it goes more than one ply deep:

1. **Errors compound instead of cancel.** Each ply commits to a single greedy-best move (`bestOwnReply`) and the next ply reasons on top of that fixed, possibly-wrong choice. Real search backs up the best of many explored branches; this only ever explores one path per ply.
2. **Afterstates are already approximations, chained on top of each other.** Every ply scores the probability-blended "conquered/held" afterstate (`attackBranches`/`blendFeatures`), not a real board state. Three or four plies deep, the leaf being scored is an approximation of an approximation of an approximation — exactly the kind of compounding synthetic-state error the real Attack Handler (below) is built to avoid by working with actual terminal-state probabilities instead.
3. **Single-opponent scope gets less realistic the deeper it goes.** Ply 1 asking "how does the territory's former owner react" is a reasonable simplification. Ply 3 asking "and then the *same single* opponent again, with the rest of the board assumed inert" is an increasingly narrow fiction.

This is the concrete evidence behind this roadmap's premise: the gap isn't "make the existing lookahead deeper," it's the five structural pieces below, each of which addresses one of the three reasons above.

## What the paper actually does (Section 3.1, 3.3)

- **The core loop is search, not greedy scoring.** A heuristically-pruned breadth-first search over the game tree, re-run from scratch after *every single attack* (not once per turn) — because a single attack's dice outcome changes the tree. The GCN value network only ever scores *leaves* of this search; it is never used to greedily rank a flat list of candidates the way `ValueStrategy.attack()` does today.
- **Search is time-budgeted, not depth-budgeted**: a 10-second-per-turn wall-clock budget, anytime-style (return the best move found so far if the clock runs out), not a fixed ply count.
- **Pruning parameters** (the paper's own reported values — treat as a starting point to re-tune empirically against our own board/value-function, not a value to copy verbatim; see "Open design questions" below): `Tp`/`Gp`/`Ga`/`Gf` bound how many territories/continents/attack/fortify options are kept alive at each branching point, keeping the tree tractable within the time budget.
- **The Attack Handler (Section 3.3) is the load-bearing, non-obvious piece.** Rather than blending two hypothetical afterstates by win probability (what `attackAfterstateBlend` does today), it precomputes the *full probability distribution* over an attack's possible **terminal states** — fighting an attack to its actual end (attacker eliminated from the front, defender eliminated, or attacker voluntarily stops), not a single dice round — and commits to exactly *one* terminal state to continue searching from, chosen via a tunable **"Risky"** cumulative-probability threshold (paper value: 0.3 — walk the distribution from most-likely outcome, stop and commit once cumulative probability crosses this threshold). This is what makes a *deterministic* search tractable over a genuinely stochastic game.
- **λ=0.8 for TD(λ)** — we already independently landed on `lam=0.8` for `fit_gcn_td` (matching `td_fit.py`'s own Phase 1 validated value), so no change needed there. The paper's optimizer/epoch choices (Adadelta, lr=0.5, 3 epochs) were for a different training setup than ours (we use plain SGD after finding Adam collapsed the network — see `gcn_fit.py`'s `fit_gcn_td` docstring) and aren't directly relevant to the search work itself.

## Gap analysis: five things we don't have

1. **The Attack Handler itself** — a combat terminal-state probability distribution calculator. `internal/bot/combat.go`'s `ForecastAttack` (expected losses, single win probability) is the closest existing building block but computes a *summary statistic*, not a *distribution over terminal states*. This is new combinatorics/probability code with no existing analog to extend.
2. **True multi-player branching** — considering more than one designated opponent. Both the original shallow lookahead and this session's deeper version deliberately stay single-opponent-scoped. Real search needs to consider (some principled subset of) all rivals' territories, not one.
3. **Heuristic pruning of branching factor** — not needed yet because `bestOwnReply` always greedily collapses each side's options to one before recursing (that's *why* the existing lookahead is only `O(2^depth)`, not combinatorial in candidate count). Real search that considers multiple candidates per side at each node needs actual pruning heuristics (the paper's `Tp`/`Gp`/`Ga`/`Gf`), which is a design-and-empirically-tune problem in itself.
4. **Anytime, time-budgeted search** — nothing in this codebase currently bounds a decision by wall-clock and returns a best-so-far answer. `Strategy.NextCommand`'s existing `ctx context.Context` parameter is actually a good hook for this (`context.WithTimeout` is the idiomatic Go primitive), but the search loop itself (iterative-deepening or similar, checking `ctx.Done()` and returning the best candidate found) doesn't exist.
5. **Searching over move *sequences*, not scoring pre-enumerated single moves** — `ValueStrategy.attack()`'s whole control-flow shape is "enumerate legal attacks, score each in isolation, pick the best." The paper searches over sequences of moves as the fundamental unit. This is a restructuring of the decision loop, not an addition alongside it.

## Proposed phased approach

Each phase should be independently testable and independently valuable — matching how every lever this session (margins, epochs, Turtle's security threshold, lookahead depth) needed its own empirical validation pass, not a single big-bang integration at the end.

1. **Attack Handler**: build the terminal-state probability distribution calculator standalone, alongside (not replacing) `ForecastAttack`. Testable in complete isolation with hand-computed combat scenarios, no dependency on search or the value network at all.
2. **Move-sequence search over our own attacks**: restructure attack-phase decision-making to search over *sequences* of our own candidate attacks (this is the "search over our own attack sequence" option from this session's design discussion — deliberately not chosen for the shallow build, now the natural next foundation), scored via the Attack Handler instead of the simpler conquered/held blend. No multi-player branching yet — still single-opponent-scoped at the leaves, isolating "does sequence search over our own moves help" as one variable.
3. **Multi-player branching**: broaden from one designated opponent to a principled subset of rivals (e.g. every player with a territory adjacent to the contested region) at reply nodes.
4. **Heuristic pruning**: once multiple candidates per side are genuinely explored (not greedily collapsed to one), bound branching with pruning heuristics analogous to the paper's `Tp`/`Gp`/`Ga`/`Gf`.
5. **Time-budgeted anytime search**: wrap the whole tree walk in a `context.WithTimeout`-aware loop returning the best move found if the deadline hits — replacing the fixed-depth cutoff entirely.
6. **Empirical tuning pass**: Risky threshold, pruning cutoffs, time budget vs. depth tradeoffs — re-run through the same `cmd/bvcalibrate` + the two standard tournament matchups used throughout this whole session. Expect this to be its own multi-round effort, not a single run, based on how every other lever this session needed 3-6+ tuning iterations before finding a real sweet spot.

## Reuse map

| Existing | Role in the new design |
|---|---|
| `internal/bot/combat.go`'s `ForecastAttack` | Reference/fallback; the Attack Handler is a new, richer sibling, not a replacement in Phase 1 |
| `internal/bot/afterstate.go`'s `attackAfterstateBlend`/`copyGameState` | The non-Attack-Handler afterstate machinery every other phase (reinforce/occupy/fortify) keeps using unchanged; also the fallback if the Attack Handler is ever unavailable/degenerate. Building the Attack Handler will likely want to split this back into branch-construction and feature-blending pieces the way `internal/bot/lookahead.go` briefly did before it was removed -- that split is fine to redo, just don't resurrect the greedy-chained recursion around it |
| `risk.LegalAttacks` and friends | Legality is still the engine's alone — search must never invent a move these don't return |
| `bot.ValueFunction` interface | The leaf evaluator interface search calls into — no change needed, the GCN already implements it |
| `bot.Runner`'s existing re-invoke-after-every-command loop | Already gives us "re-run after every attack" for free — no new turn-loop infrastructure needed for that specific requirement |
| `Strategy.NextCommand(ctx context.Context, ...)` | Natural hook for the time budget (`context.WithTimeout`) |
| `cmd/bvcalibrate`, the two standard tournament matchups | The evaluation pipeline every phase gets validated through, unchanged |

## Open design questions (decide when each phase actually starts, not now)

- Is the paper's Risky=0.3 threshold, or its Tp/Gp/Ga/Gf pruning values, even a reasonable starting point for our board/value-function, or should these be empirically swept from scratch the way every other hyperparameter this session was (margin percentile, training epochs, `turtleSecurityThreshold`)? Lean toward the latter given this session's own track record.
- How many rivals does "a principled subset" (Phase 3) actually mean — every adjacent player, or some bounded top-K by threat?
- Does move-sequence search (Phase 2) apply only within a single turn's attack phase, or does "sequence" ever need to span the reinforce → attack → fortify boundary within one turn?
- Should search-generated leaf evaluations feed back into training data (closing the loop, generating stronger self-play data than the current heuristic personas) — this connects to the parallel `Monte_Carlo_Evaluator_Roadmap_with_References.md`'s own milestone 10 ("search-generated training targets") and may be worth coordinating rather than treating as fully separate initiatives.

## Success criteria

Same bar as everything else this session: a real, reproducible movement in win rate against the same two standard tournament matchups (`board-value-candidate,boscoe-v1,killbot-v1,quo-v1` and `angry-v1,basic-v1,cluster-v1,pixie-v1,scored-v1,board-value-candidate`), not an offline/theoretical metric. Given how much effort went into finding that the *training objective* and *margin calibration* — not search — were the actual blockers behind the original 0% win rate, treat "search helps" as a hypothesis to validate cheaply per-phase, not an assumed conclusion.
