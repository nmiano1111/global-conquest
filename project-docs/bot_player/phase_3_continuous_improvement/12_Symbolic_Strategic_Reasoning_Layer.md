# Symbolic Strategic Reasoning Layer — Design & Roadmap

> Companion docs: `08_Machine_Learning_Roadmap.md` (the separate, learned-evaluator track this is deliberately *not* part of — see "Relationship to the ML roadmap" below), `10_Bot_Weight_Tuning.md`/`11_Learned_Board_Evaluation.md` (the `scored-v1`/GCN scoring-function work this track sits alongside, not on top of).

## 1. Purpose

This is a first, narrow cut of a **symbolic strategic reasoning layer** on top of `killbot-v2`, the live-default bot (`internal/bot/strategy_killbot_v2.go`). It answers a different question than every existing bot heuristic: not "is this specific candidate move tactically good," but "is there a strategically important situation this turn that changes how good a candidate should look" — e.g. "a rival is one territory from completing a continent; capturing it now is worth more risk than the flat odds bar normally allows."

**This augments killbot-v2's existing tactical heuristics; it does not replace them.** Every existing decision path (kill-hunting, hogwild, the `BetterPixieStrategy` backer) is completely unchanged. The symbolic layer only ever adds one narrow, optional consideration on top.

**Why now**: earlier in the same session that produced this doc, four different tactical tweaks to killbot-v2 were tried and all failed or were disconfirmed (see `project_killbot_v2_live_default` session memory for the full account): reactive leader-targeting, continent-defense priority, split defensive reinforcement, fortify consolidation. The two that were actually built both added a **hard priority tier** that unconditionally preempted the existing backer logic under some triggered condition — both caused longer games, more timeouts, and lower win rates. This is the direct reason this design leans so heavily on **soft biases instead of hard overrides** (Section 5) — it's not a stylistic preference, it's a repeat-failure avoidance.

## 2. Architecture

```
risk.Game ──► symbolic.Extract() ──► symbolic.Facts ──► rule funcs ──► AttackBias
                                                                            │
KillbotV2Strategy.attack() ────────────────────────────────────────────────┘
   (existing cascade: kill-hunt → [NEW: biasedAttack] → hogwild → backer.attack())
```

**`internal/bot/symbolic`** — a new package, sibling to `internal/bot/gcnmodel`, depending only on `internal/risk` (confirmed no import cycle: `internal/bot` depends on `internal/bot/symbolic`, never the reverse, matching how `gcnmodel`/`tdstate` are already structured). Pure, read-only fact extraction (`facts.go`) and rule functions (`rules.go`) — no `risk.Game` mutation, no dependency on `internal/bot`.

- `facts.go`: `Extract(g *risk.Game, pi int) Facts` walks every positive-bonus continent and reports any that are exactly one territory from being fully owned by a single player, split into `Facts.EnemyNearCompletion` (rival near-complete) and `Facts.SelfNearCompletion` (pi near-complete). A handful of small board-query helpers (`territoryContinents`, `continentOrder`, `nearCompletion`) deliberately duplicate the *shape* of unexported `internal/bot/geometry.go` equivalents (`territoryContinent`, `continentOrder`) — those are package-private and genuinely inaccessible from a different package regardless of dependency direction, so this is a small, deliberate reimplementation scoped to exactly what `Facts` needs, not a parallel copy of `geometry.go`.
- `rules.go`: `AttackBias{Delta float64; Reason string}` plus two rule functions, `EnemyContinentCompletionBias`/`SelfContinentCompletionBias`, each taking `(Facts, from, to risk.Territory)` and reporting whether `to` is the specific missing territory of a near-complete continent.

**`internal/bot/strategy_killbot_v2.go`** gains one small, additive, optional hook:

```go
type attackBiasFunc func(g *risk.Game, pi int, from, to risk.Territory) float64

type KillbotV2Strategy struct {
    *KillbotStrategy
    bias attackBiasFunc // nil by default
}

func WithAttackBias(b attackBiasFunc) func(*KillbotV2Strategy)
func NewKillbotV2Strategy(opts ...func(*KillbotV2Strategy)) *KillbotV2Strategy
```

mirroring `BetterPixieStrategy`'s existing `WithAttackComparator` functional-option pattern exactly. `bias == nil` (every existing caller) is a true no-op — verified by every pre-existing `strategy_killbot_v2_test.go` test passing completely unmodified. When set, `attack()` calls a new `biasedAttack` method between the existing committed-kill-route check and the hogwild check: it scans every `risk.LegalAttacks` candidate, asks `k.bias` for a relaxation delta on each, and fires the single best-biased candidate if the relaxed bar (`tuning.MinAttackWinProbability - delta`) clears real `risk.ForecastAttack` math — the same direct-`ForecastAttack` pattern the existing kill-hunt check already uses, just against an adjusted bar.

**`internal/bot/strategy_symbolic_killbot.go`** — `SymbolicKillbotStrategy` wraps `*KillbotV2Strategy` (composition, matching the project's established v1→v2 wrapping convention), supplying `WithAttackBias(symbolicAttackBias)` at construction. No `NextCommand` override — inherited unchanged. Registered as `symbolic-killbot-v1` in `cmd/tournament/main.go`'s registry for A/B testing; **not** added to `cmd/backend/main.go`'s live registry (tournament/experimental only until validated).

### Why the bias is a separate hook, not a comparator wrap

`attackComparator` (`geometry.go:36`, `func(attackerArmies, defenderArmies int) bool`) has no territory identity — a wrapping closure can't tell which `(from, to)` pair a given `cmp(a, d)` call corresponds to, so it can't express "relax the bar for *this specific* candidate," only "change the bar for every candidate this call scores." Extending the comparator's signature to carry territory identity would mean editing `geometry.go` and threading the new signature through every call site (`attackEasyExpand`, `attackFillOut`, `attackConsolidate`, `attackInContinent`, `betterPixieAttackInContinent`, `attackAsMuchAsPossible`, `rawArmyComparator`, `winProbabilityComparator`, `BetterPixieStrategy.cmp`) — out of scope for a first cut, and touches shared code this project's convention says not to touch for killbot-v2-specific behavior. The separate `attackBiasFunc` hook sidesteps this entirely by running its own small candidate scan before falling through to the backer.

## 3. Initial Strategic Concepts

Only one fact type ships in v1:

**`ContinentCompletionFact`** — `{Continent, Owner, MissingTerr, Bonus}`. Meaning: `Continent` is a positive-bonus continent where `Owner` holds every territory except `MissingTerr`. Derivation: for each continent, count territories per owner; if some owner holds exactly `len(territories)-1`, the remaining one is `MissingTerr`. Split into `Facts.EnemyNearCompletion` (`Owner != pi`) and `Facts.SelfNearCompletion` (`Owner == pi`).

**Limitations**: no reachability filtering in `Extract` itself — a fact can name a `MissingTerr` pi has no route to this turn. This is intentional, not an oversight: `biasedAttack` only ever evaluates real `risk.LegalAttacks` candidates, so an unreachable missing territory simply never gets asked about; pre-filtering for reachability inside `Extract` would just duplicate that check for no behavioral difference. No notion of "already fully complete, should be broken" — `nearCompletion` requires exactly one territory short, not "own everything and I should take one away." No historical dimension (was this continent just captured, is the near-completion state new or stale) — see Section 9.

## 4. Strategic Goals

There is no explicit `Goal`/`GoalType` type in v1 — the two rules act directly as biases without an intermediate goal-selection step. This is a deliberate scope cut, not an oversight: with only two rules that never compete for the same decision (a continent can't simultaneously be both `Facts.EnemyNearCompletion` and `Facts.SelfNearCompletion`, since `Owner` is either `pi` or not), there is no actual goal-arbitration problem to solve yet. `symbolicAttackBias` (the adapter in `strategy_symbolic_killbot.go`) checks enemy-completion first, then self-completion, as an implicit priority order — fine for two non-competing rules, but this will need a real priority/goal-resolution mechanism once a third rule is added that *can* compete with these two for the same turn. See Section 9's roadmap.

## 5. Hard Constraints vs Soft Biases

**v1 ships zero hard constraints.** Both rules only ever *add* a candidate the backer wouldn't otherwise have taken (relax a threshold); they never remove, block, or override anything else the existing cascade would have done. `AttackBias.Delta` is always `>= 0`.

This is the central lesson from the two hard-override attempts that failed earlier the same session (Section 1): a hard rule that preempts the backer under some triggered condition can fire far more often than intended in real multiplayer chaos, diverting reinforcements/attacks from productive play into a grinding, longer-game pattern. A soft bias structurally can't do that — worst case, it changes *which* attack fires this turn among already-legal options; it can never force an attack that wouldn't otherwise have happened at all if there's nothing to bias, and it never touches reinforcement or fortify decisions at all.

A candidate-filtering hard-constraint mechanism (stripping disallowed `LegalAttacks`/`LegalReinforcements` entries before they reach the comparator/backer, for something like "never attack through the only defensive stack protecting a continent") is designed conceptually here but deliberately not built — see Section 9, roadmap item 2.

## 6. Explainability

`AttackBias.Reason` is a short string (e.g. `"breaks a rival's near-complete continent"`) attached to every fired bias — but it is **not yet surfaced anywhere**. `bot.Explanation` (the struct `Strategy.NextCommand` returns, used by `scored-v1`/`gcn-v1` for their own `Score`/`Features`/`Alternatives`) is still returned zero-valued by the entire killbot-v2 family, including `SymbolicKillbotStrategy` — wiring `AttackBias.Reason` into `Explanation` so it's inspectable via logs/tournament output/a future Discord command or admin UI is a small, clearly-scoped next step (Section 9, roadmap item 8), not done in v1 since nothing currently consumes `Explanation` for this strategy family.

## 7. Testing Strategy

- `internal/bot/symbolic/facts_test.go` — pure, deterministic, hand-built `risk.Game` fixtures (own `newTestGame`/`fixedRNG`, duplicated from `internal/bot`'s test fixture since this package has no `internal/bot` dependency): finds enemy-near-completion, finds self-near-completion, omits when no single owner holds all-but-one.
- `internal/bot/symbolic/rules_test.go` — bias fires only for the exact missing territory, never for a different target, never crosses between `EnemyNearCompletion`/`SelfNearCompletion` fact sets.
- `internal/bot/strategy_symbolic_killbot_test.go` — end-to-end, using the existing `newTestGame` fixture (`strategy_basic_test.go`), matching `strategy_killbot_v2_test.go`'s style:
  - disabled-path parity: no near-complete continents anywhere → identical command from `KillbotV2Strategy` and `SymbolicKillbotStrategy`.
  - enemy-completion: a matchup (`risk.ForecastAttack(5, 4) ≈ 0.4765`) between the relaxed bar (0.35) and the normal bar (0.5) — plain killbot-v2 declines (its backer picks a different, higher-ratio target via `attackForCard`'s pure army-ratio logic instead), symbolic fires the biased attack specifically.
  - self-completion: mirrored.
  - floor: a matchup below even the relaxed bar (`ForecastAttack(5, 6) ≈ 0.2525`) — both decline identically, proving the bias never pushes below its floor.
- Every pre-existing `strategy_killbot_v2_test.go` test passes unmodified — the acceptance criterion for "preserves existing behavior when disabled."
- `go test ./...` full-suite clean at time of writing.

## 8. Measuring Improvement

Tournament validation, run from `backend/`:

```
go run ./cmd/tournament -strategies killbot-v2,symbolic-killbot-v1,boscoe-v1,quo-v1,betterpixie-v1,pixie-v1 -games 300 -seed-start 1 -parallel 12
```

(6 strategies, not 7 — the engine caps player count at 6; `cluster-v1` was dropped from the usual 7-persona field to fit `symbolic-killbot-v1` in alongside `killbot-v2` for a direct comparison.)

Both `killbot-v2` and `symbolic-killbot-v1` in the same field (head-to-head, not a swap) — a clean delta within the same run, since `killbot-v2`'s own win rate shifts with field composition (compare 62.6% in the original 7-persona field vs. ~40% in this smaller 6-persona one) and isn't directly comparable across different lineups.

**Sounding intelligent is not sufficient evidence.** Bar for "worth keeping": `symbolic-killbot-v1`'s `GameWinRate` must exceed `killbot-v2`'s with a favorable (not merely overlapping) 95% Wilson-score CI, `AvgFinishOrder` must not visibly worsen, and completed-game count must not drop — the specific regression signature (`command_limit_reached`/`duration_limit_reached` rising) both earlier hard-override attempts this session produced. If it doesn't clear this bar: revert (drop the `cmd/tournament/main.go` registration and delete the new files) and record the negative result here, exactly as the leader-containment and split-reinforcement attempts were recorded and reverted earlier the same session.

**First validation run (2026-08-26): does not clear the bar.** 300 games, seeds 1-300, 6-seat field `killbot-v2,symbolic-killbot-v1,boscoe-v1,quo-v1,betterpixie-v1,pixie-v1` (`cluster-v1` dropped to fit the engine's 6-player max — the original 7-strategy command errored). 262/300 completed (38 failures: 21 `command_limit_reached`, 17 `duration_limit_reached`).

| strategy | win% | 95% CI | avg finish | avg captures | avg elims |
|---|---|---|---|---|---|
| `killbot-v2` | 40.5% | [34.7–46.5] | 2.75 | 77.30 | 1.95 |
| `symbolic-killbot-v1` | 37.8% | [32.1–43.8] | 2.97 | 68.71 | 1.88 |

`symbolic-killbot-v1` is *lower* than `killbot-v2` in this same run, not higher — the two-rule continent-completion bias did not produce a measurable improvement. CIs overlap heavily, so this reads as noise rather than a confirmed regression, but it plainly does not clear the "must exceed with a favorable CI" bar this section sets. (Note: `killbot-v2`'s 40.5% here isn't directly comparable to the 62.6% figure quoted elsewhere in this doc — that number came from a different 6-seat field including `cluster-v1`; `killbot-v2`'s own win rate *within this run* is the correct baseline for this comparison, not the historical figure.)

Plausible reasons, not yet investigated: the `-0.15` delta / `0.35` floor were first guesses, never tuned; two rules covering only continent-completion may be too narrow a slice of situations to move the needle; or continent-completion opportunities the flat bar was already declining may not have been costing killbot-v2 much to begin with.

**Second validation run (2026-08-26), after retuning `continentCompletionDelta` 0.15→0.25 and `symbolicKillbotMinFloorWinProbability` 0.35→0.25 — still does not clear the bar.** Same 300 games/seeds/field. 258/300 completed (42 failures: 22 `command_limit_reached`, 20 `duration_limit_reached` — essentially unchanged from the first run's 262/38).

| strategy | win% | 95% CI | avg finish | avg captures | avg elims |
|---|---|---|---|---|---|
| `killbot-v2` | 40.3% | [34.5–46.4] | 2.75 | 74.78 | 1.94 |
| `symbolic-killbot-v1` | 41.5% | [35.6–47.6] | 2.91 | 71.92 | 1.95 |

`symbolic-killbot-v1` is nominally higher this time, but the CIs overlap almost entirely — not the "favorable, not merely overlapping" result the bar requires. Taken together with the first run, the pattern across both measurements is informative: `killbot-v2`'s own win rate stayed essentially flat (40.5%→40.3%) both times — a good sanity check that the two runs are comparable — while `symbolic-killbot-v1` swung from 37.8% to 41.5% purely from the constant change, a larger movement than the delta between the two strategies in either single run. That's the signature of noise dominating a genuinely small or absent effect, not a signal being uncovered by better tuning. Per this session's established discipline (don't keep tuning-and-remeasuring indefinitely chasing a point estimate — see the meta-lesson in `project_killbot_v2_live_default` session memory), this was the one planned re-measurement; the two-rule continent-completion bias is not shown to help, and the mechanism is being reverted. The package structure, hook design (`WithAttackBias`), and this document remain in place for a future attempt with different or additional rules.

## 9. Roadmap — Additional Strategic Concepts

Ordered roughly by how directly they build on what v1 already has:

1. **`CriticalBorder`** (designed, cut from v1 — see Section 2's "why a separate hook" and the design plan's own explicit deviation note): raise caution when attacking from a fragile, positive-bonus-continent-defending border. Needs either the `attackComparator` signature extension (out of scope for v1, see above) or a fundamentally different formulation that doesn't need per-source-territory context in the bias signature.
2. Hard-constraint / candidate-filtering infrastructure (Section 5) — e.g. "preserve at least N armies on a critical border," "don't attack through the only defensive stack protecting a continent." Implementation sketch: a function that strips disallowed entries from `risk.LegalAttacks`/`risk.LegalReinforcements` before they reach the comparator/backer, rather than another priority-cascade tier.
3. `major_player_threat`/`vulnerable_player`/`elimination_opportunity` refinements beyond what `killTarget`/`dominantPlayerToKill` (`geometry.go`) already do — e.g. card-value-adjusted elimination priority (a vulnerable player holding many cards is worth more to eliminate than raw army-weakness alone suggests). Note: an earlier, separate attempt this session (`leaderContainmentTarget`, reactive whole-rival targeting) already failed twice — any new attempt here needs to be a genuinely different mechanism, not a repeat.
4. `overextended`/`safe_expansion` — the reinforcement-side equivalent of this doc's attack-side bias. No comparator-equivalent hook exists for `BetterPixieStrategy`'s placement logic (Section 2) — this needs its own integration point, not a reuse of `attackBiasFunc`.
5. Board-topology concepts not yet touched: mobility/connected-component strength, stack concentration, front count, attack corridor preservation.
6. Timing concepts: reinforcement-income trends, card turn-in timing relative to set value escalation.
7. Multi-agent concepts requiring historical data (Section 10 covers why these are hard right now): persistent_aggressor, recently_weakened, rapid_growth, repeatedly_attacked_me, players_fighting_each_other, recent_continent_break, opponent aggression trajectories, retaliation modeling, opponent personality inference, kingmaking risk, leader suppression, second-place survival behavior, temporary non-aggression/emergent diplomacy, bluff-like behavior, endgame/remaining-player-count-conditioned behavior.
8. Explainability wiring (Section 6): thread `AttackBias.Reason` into `bot.Explanation`, then into whatever already consumes `Explanation` for other strategies (tournament raw output, potentially a future Discord command or admin debug view).

## 10. Roadmap — Multi-turn Planning

v1 is `facts → immediate bias`, computed fresh every single decision with no memory of what it decided last turn. The natural next step is:

```
facts → strategic goal → candidate multi-step plans → simulation/evaluation → selected plan → tactical execution
```

Candidate plan types worth modeling eventually: complete continent, eliminate player, break continent, relocate primary stack, consolidate borders, prepare attack corridor. **Combat is stochastic** — `risk.ForecastAttack` already gives exact win-probability/expected-loss math per single attack, but a multi-step plan needs to reason about a *sequence* of such attacks, each conditioned on the (uncertain) outcome of the last. Candidate plans should eventually incorporate combat probabilities, expected losses, contingency paths (what happens if this specific attack in the plan fails), and simulated resulting states rather than a single deterministic projection.

**Persistent strategic intent** (the spec's original ask for this section) is explicitly deferred, not built in v1, for a concrete reason: there is currently **no persistent bot-side state mechanism at all** in this codebase. Every `Strategy` is constructed once and shared/reused concurrently across every game; `bot.Runner.RunTurn` reloads authoritative `*risk.Game` fresh from Postgres on every single command. The one precedent for AI-authored persistent intent, `risk.Game.KillPlans` (engine-tracked, written via the exported `SetKillPlan`, cleared every turn boundary), required real engine surface area to build — a new `risk.Game` field, an exported setter, and a new `Command` field. A "primary goal / secondary goal / goal start turn / commitment inertia" mechanism (the spec's suggested shape) would need the same kind of engine change, mirroring `KillPlans`/`SetKillPlan` exactly. This is appropriately left as a scoped next increment rather than forced into a first cut whose whole point was staying narrow.

## 11. Roadmap — Simulation and Learned Evaluation

This project already has real learned-evaluator infrastructure, complementary to (not competing with) this layer: `internal/tdstate` encodes `*risk.Game` into a fixed-width, player-relative feature vector; `internal/bot/gcnmodel` scores that vector via a trained graph-conv network (`08_Machine_Learning_Roadmap.md`, `11_Learned_Board_Evaluation.md`). The natural eventual architecture once multi-turn planning (Section 10) exists:

```
Symbolic reasoning  → generates strategic goals / candidate plans
Simulation          → produces possible resulting states
Learned evaluator    → scores those states (tdstate + gcnmodel, or a future evaluator)
```

Roles stay distinct: symbolic reasoning answers "what are we trying to accomplish," search/simulation answers "what sequence of actions might accomplish it," and the learned evaluator (or today's simpler heuristics) answers "how good is the resulting state." The symbolic layer should not try to learn everything itself — it stays declarative/explicit, and numeric evaluation stays with whichever scoring mechanism (hand-tuned or learned) is actually being used at the time.

**Relationship to the ML roadmap**: per explicit user direction (session memory, `project_bot_symbolic_ai_track`), this symbolic track is kept **separate** from `08_Machine_Learning_Roadmap.md`'s GBT → GCN → MCTS-with-trained-evaluator progression, with an eye toward merging them later. The overlap is real — the ML roadmap's own phase 3 ("Monte Carlo + trained evaluator") is structurally the same idea as Section 10 above, just with a learned evaluator instead of a hand-written one. A future merge would likely mean building the search/tree-traversal machinery once and swapping in the trained evaluator from ML phase 3 once it exists, rather than maintaining two separate search implementations.

## 12. Roadmap — Prolog

Not introduced now, and nothing in v1's two rules comes close to justifying it. Prolog becomes worth evaluating if: rules become a major subsystem with many derived facts depending on other derived facts; recursive graph reasoning becomes common (e.g. "which territories are on *some* critical path between two continents"); backtracking/query-style reasoning becomes genuinely useful (e.g. "which players satisfy the conditions for elimination, considering all of pi's possible attack sequences"); or the project finds itself accidentally re-implementing an inference engine in Go as rule count grows. Tradeoffs if it ever does: a Go↔Prolog integration boundary, deployment/runtime complexity, converting `risk.Game` into logical facts on every decision, debugging across a language boundary, and combat-probability/numeric evaluation staying outside Prolog regardless (Prolog is a poor fit for that half of the problem either way). **Prolog should never be adopted merely because the system is labeled "symbolic AI"** — the two rules in v1 are already a symbolic layer, expressed in plain Go.

## 13. Roadmap — Datalog

A more plausible fit than Prolog if rule volume grows, given the game has a genuinely bounded, finite set of players/territories/continents/relationships — Datalog's sweet spot. Attractive for: declarative facts and rules with predictable (non-backtracking) evaluation, strong finite relational reasoning, and potentially cleaner semantics for historical/event-based queries (Section 9, item 7) than hand-rolled Go joins over `game_domain_events`-style data once that's wired up. Tradeoffs: still a second runtime/library to integrate and operationally support, less expressive than Prolog (no recursion beyond fixed-point semantics in the simplest form), and numeric optimization stays in Go regardless. **Threshold for reconsidering**: once fact-derivation rules number in the dozens and start depending on *each other* (not just on raw `risk.Game` state) in ways that make the current flat `Extract()` → `rules.go` shape hard to follow — nothing close to that yet with two rules.

## 14. Roadmap — Haskell

Not a symbolic-AI language in the way Prolog is — its case rests on strongly-typed domain modeling and elegant search/planning combinators, not logical inference. Potentially useful if this project ever builds a genuine typed strategy/planning DSL (something like `prefer CompleteContinent unless Overextended thenTry EliminateWeakPlayer otherwise Consolidate`) or a separate functional planning engine for Section 10's multi-turn search — algebraic data types map naturally onto goals/plans/constraints/actions/outcomes, and immutable state transformations suit search-tree manipulation well. Tradeoffs: a second language's full complexity, duplicating/replacing parts of the existing Go bot engine, no native logical-inference advantage over Prolog if that's actually what's needed, real integration overhead, and much harder to justify for anything short of a deliberate DSL/planning-engine project. **Not worth it for the rule layer itself** — that's squarely in native Go's wheelhouse.

## 15. Technology Decision Guide

| | Native Go (current) | Datalog | Prolog | Haskell |
|---|---|---|---|---|
| Best use case | Small, explicit rule set, tight integration with existing Go code | Finite relational/historical fact derivation at scale | Recursive inference, backtracking search over a real knowledge base | Typed strategy/planning DSL, functional search-tree code |
| Numeric reasoning | Native, zero friction | Poor fit, stays in Go | Poor fit, stays in Go/host language | Good, but not the reason to adopt it here |
| Symbolic inference | Manual, explicit | Strong, declarative | Strongest | Encodable via ADTs, not "inference" per se |
| Search/backtracking | Manual | Limited (fixed-point) | Native, strongest | Elegant if hand-built, no native engine |
| Type safety | Go's (adequate) | Weak/dynamic typically | Weak/dynamic typically | Strongest available |
| Integration complexity | None (already the codebase) | New runtime/library | New runtime/library + FFI boundary | New language + FFI boundary |
| Likely trigger for adoption | — | Dozens of interdependent derived facts | Recursive/backtracking reasoning becomes central | A deliberate typed DSL/planning-engine project |

**Recommendation, as of this v1**: native Go, staying that way for the foreseeable next few increments in Section 9's roadmap. Nothing here justifies a different runtime yet — two rules, zero interdependency between them, zero recursion, zero backtracking.

## 16. Failure Modes / Risks

- Brittle expert-system behavior from too many hard rules — mitigated in v1 by shipping zero.
- Rule explosion / contradictory rules as more get added without a real goal-arbitration mechanism (Section 4 flags this as the next real gap once a third, competing rule appears).
- Strategic oscillation across turns — less of a risk in v1 specifically since there's no persistent goal state to oscillate (Section 10), but becomes a real risk once persistence is added without commitment/inertia.
- Double-counting concepts already represented by existing heuristics — actively checked for both v1 rules (verified against `killTarget`/`dominantPlayerToKill` and `betterPixieAttackInContinent` directly, see Section 3 and the design plan); must be re-checked for every future rule.
- Overfitting to the classic board specifically — v1's `Extract` operates generically over `risk.Board.Continents`/`Order`, not hardcoded territory names, matching this project's established board-agnostic convention (`11_Learned_Board_Evaluation.md`'s "Known future constraint" section).
- A rule that sounds smart but reduces win rate — the entire reason Section 8's measurement bar exists and is enforced before keeping anything.
- Coupling between the symbolic and heuristic layers growing too tight — v1's hook is intentionally minimal (`attackBiasFunc` returns a single float, nothing else) specifically to keep this loose.
- Unclear precedence between directives — not yet a problem with two non-competing rules (Section 4); will need explicit resolution once that changes.
- Inability to explain why a rule fired — `AttackBias.Reason` exists but isn't surfaced yet (Section 6); a real gap today, not yet a risk that's bitten anyone since nothing consumes it.

## 17. Long-Term Architecture

```
                        Game State
                            │
                            ▼
                    Feature Extraction
                     /              \
                    /                \
           Symbolic Facts        Numeric Features
             [EXISTS: v1]         [EXISTS: tdstate]
                  │                   │
                  ▼                   │
          Strategic Reasoner          │
          [EXISTS: v1, 2 rules]       │
                  │                   │
                  ▼                   │
          Goals / Constraints         │
          [ROADMAP: Section 4/9]      │
                  │                   │
                  ▼                   │
          Candidate Plans             │
          [ROADMAP: Section 10]       │
                  │                   │
                  └─────────┬─────────┘
                            ▼
                    Simulation / Search
                    [ROADMAP: Section 10/11]
                            │
                      ┌─────┴─────┐
                      │           │
                  Heuristics   GCN / Learned
                  [EXISTS]     Evaluator
                                [EXISTS: gcnmodel]
                      │           │
                      └─────┬─────┘
                            ▼
                       Selected Plan
                     [ROADMAP: Section 10]
                            │
                            ▼
                    Tactical Heuristics
                    [EXISTS: BetterPixieStrategy]
                            │
                            ▼
                         Actions
                        [EXISTS]
```

What exists today: `symbolic.Extract`/`rules.go` (2 rules, attack-side only), `tdstate`+`gcnmodel` (separate, unconnected to the symbolic side yet), and the full existing tactical/action layer. Everything else in this diagram is a documented research direction, not a commitment.

## 18. Second Experiment: Persistent Goals (`goal-killbot-v1`)

### 18.1 Retrospective on the first experiment

Sections 1-17 above describe a single-decision mechanism: two rules (`EnemyContinentCompletionBias`/`SelfContinentCompletionBias`) that relax killbot-v2's attack win-probability threshold for one specific territory, recomputed fresh from board state on every call, with no memory of what the bot decided last turn. Measured twice (Section 8), it produced no measurable win-rate change (37.8%/41.5% vs. killbot-v2's stable 40.5%/40.3% across the same two runs) and was reverted.

An external review argued this never really tested "symbolic strategy" — it tested a reactive, zero-lookahead heuristic wearing a symbolic-sounding name. The distinction worth testing: a **reactive symbolic feature** (`fact → modify current action`) versus **strategic symbolic intent** (`facts → persistent goal → coordinated influence over multiple decisions`). This section covers the second experiment, built to test the latter.

### 18.2 Hypothesis

Does a goal with temporal extent — persists across turns, has an explicit lifecycle, can be abandoned — produce measurably different (and better) behavior than a mechanism recomputed fresh every decision? Scoped to one goal type, `CompleteContinentGoal` (self-completion of a near-complete continent only — the enemy-denial half from the first experiment has no multi-turn plan to attach persistence to, so it's dropped for this pass), integrated across all four turn phases rather than attack alone.

### 18.3 Architecture

```
risk.Game.StrategicGoals (new, persisted engine state)
        │
GoalKillbotStrategy (wraps KillbotV2Strategy)
        │
   ┌────┼────┬─────────┐
   ▼    ▼    ▼         ▼
attack reinforce occupy fortify
(soft   (preemption tiers, narrowly gated)
 bias)
```

**`risk.Game.StrategicGoal`/`StrategicGoals`** (`internal/risk/engine.go`) mirrors the existing `KillPlan`/`KillPlans` mechanism (a turn-scoped commitment already used for kill-hunting) with one deliberate divergence: `KillPlans` is cleared by `startTurn` every single turn (re-established fresh); `StrategicGoals` is **not** — the entire point is that it survives turn boundaries until the committing strategy explicitly clears it via `SetStrategicGoal`/`ClearStrategicGoal`. New `Command` fields (`StrategicGoal`, `AbandonStrategicGoal`) thread through the same path `KillTarget` already uses — which turned out to be **5 layers, not 4**: `internal/bot/command.go` → `internal/game/server.go` → `internal/service/game_action.go` → `internal/service/game.go` → and, critically, `internal/simulation/dispatcher.go`, which has its own independent `KillTarget` handling for simulated/tournament games (a comment there already documents a real prior bug where missing this exact wiring made kill-hunting silently inert in every simulated game — the same fix was applied here for `StrategicGoal`).

**Attack** gets a true soft bias, via the first experiment's `attackBiasFunc`/`WithAttackBias` hook on `KillbotV2Strategy` (re-added unchanged, verified via every pre-existing test passing without modification) — the mechanism itself is identical to Section 2's, just now gated by a persisted commitment instead of a fact recomputed fresh every call.

**Reinforce/occupy/fortify** are **narrowly-gated preemption, not soft bias** — confirmed (again, as this pass's research restated) that `BetterPixieStrategy`'s reinforce/occupy/fortify logic has no comparator/weight hook of any kind, only pure imperative branching. The only zero-shared-edit mechanism available is the same "compute independently, return directly if a strict gate clears, else fall through unchanged" shape the existing kill-hunt tier already uses safely. Every gate here matches on the committed goal's *exact* missing territory or its *exact* adjacent border — never on "a goal is merely active" — specifically to avoid the failure shape of the two earlier, unrelated mechanisms this session that added broad preemption tiers and regressed (see the session's `project_killbot_v2_live_default` memory). This is documented here per Section 5's revised principle: soft preference by default, hard constraints (which these three tiers are, honestly) permitted with strong, specific justification — not silently relabeled as soft.

### 18.4 A real bug found during measurement

The first 300-game tournament run (before the fix below) showed 34-95 `engine_rejected_command` failures per run — a failure type no `killbot-v2`-family strategy had ever produced in any tournament this session. Root cause: `GoalKillbotStrategy.reinforce` checked goal-lifecycle state *before* the mandatory/voluntary card trade-in check `KillbotV2Strategy.reinforce` always does first, so a forced trade-in situation could produce a `place_reinforcement` command the engine correctly rejected (`"must trade cards first"`, or `"invalid reinforcement count"` when `PendingReinforcements` was still 0 pending the trade). Fixed by moving `voluntaryCardTurnIn` to the first line of `GoalKillbotStrategy.reinforce`, exactly mirroring `KillbotV2Strategy.reinforce`'s own structure. Confirmed fixed: the re-run tournament showed zero `engine_rejected_command` failures, only the normal `command_limit_reached`/`duration_limit_reached` pattern every other measurement this session has shown.

### 18.5 Instrumentation funnel and results

Per Section 6a's design (`cmd/goalinstrument`, built for this pass): needed no new `internal/simulation` code at all. `StrategicGoals` is real persisted `*risk.Game` state, so the already-existing `simulation.Config.OnTurnBoundary` hook (used by `cmd/tdtraindata`) observes goal discovery/commitment/completion/abandonment by diffing `g.StrategicGoals[seat]` turn-over-turn. `GoalKillbotStrategy.WithDecisionObserver` additionally calls a plain, bias-free `KillbotV2Strategy.NextCommand` on the same, unmutated game state at every decision and diffs the result — free and safe, since `Strategy.NextCommand` is contractually read-only.

**Result (300 games, seeds 1-300, post-bugfix, 2026-08-26): 222/300 completed.**

| Metric | Result | Stage 1→2 gate |
|---|---|---|
| Games with ≥1 goal committed | 36/222 (16.2%) | needs ≥20% — **missed** |
| Decisions with a goal active | 13,327 / 158,103 total | — |
| Decisions where the goal changed the chosen command | 846 / 13,327 (6.3%) | needs ≥10% — **missed** |
| Changed decisions by phase | attack 400, reinforce 432, fortify 14, occupy 0 | — |

Both Stage 1→2 gate thresholds were missed. In the language of Section 8b's interpretation framework (below): this lands mostly in **Case A** (the goal rarely gets discovered/committed at all — 16.2% of games) with a **Case B** flavor once it does commit (even then, only 6.3% of its own decisions actually change anything versus what killbot-v2 would have done unprompted).

### 18.6 Tournament results (mirrored seats)

Per Section 7's design: two `cmd/tournament --config` batch entries, same seed range (1-300), `goal-killbot-v1`/`killbot-v2` swapped between config indices 0 and 1 — zero new engine code needed, confirmed the turn-order/territory-layout permutation depends only on player count and seed, never on which strategy occupies which seat.

| | seatA | seatB | combined (`pairedstats`) |
|---|---|---|---|
| `goal-killbot-v1` win rate | 40.0% [33.5-46.8] | 39.7% [33.4-46.4] | 39.9% [35.3-44.6] |
| `killbot-v2` win rate | 35.6% [29.4-42.4] | 37.9% [31.6-44.5] | 36.8% [32.3-41.5] |
| Head-to-head finish order (177 paired seeds) | — | — | `goal-killbot-v1` better 92, `killbot-v2` better 85, tied 0 |

`goal-killbot-v1` is nominally ahead in every cut of this data — but the CIs overlap substantially, the head-to-head split is close to even, and per Section 8's own statistical-power table, a ~3-point gap at n≈300-419/arm is *exactly* the size that table says is unpowered to distinguish from noise (need on the order of 1,500+ games/arm for a gap this small). Taken together with 18.5's funnel result — a mechanism that commits in only 16% of games and changes only 6% of its own decisions when it does — there is no plausible causal story connecting "rarely fires, rarely changes anything" to a real multi-point win-rate improvement. The honest read is that this is noise, not signal.

### 18.7 Verdict

**Stage 1→2 gate: not cleared.** Per this document's own stated discipline (Section 8) and this session's established pattern (report the honest result, don't spin a mediocre one), the persistent-goal mechanism as built does not clear the bar set before Stage 2 (the counterfactual branching harness) would even be considered. This maps most cleanly to Case A of the interpretation framework — insufficient coverage/too narrow a strategic concept to matter in aggregate — with a secondary Case B signal (when it does apply, it mostly agrees with what the bot would have done anyway).

**What this does and doesn't establish.** It does not establish that persistent goals are a bad architecture generically, or that symbolic reasoning doesn't help — only that *this specific goal type, this specific viability/inertia/gate design* rarely applies and rarely matters when it does. §18.8 below confirms one specific, quantified reason with real data (not speculation): the viability check's army-count assumption. `completeContinentInertiaTurns = 8` was a first guess, never tuned, and remains untested as a contributing factor. Continent-completion generically may also simply not be a strategically central-enough goal to move a whole-game outcome, regardless of how well the lifecycle machinery around it works — that remains an open, untested possibility too.

**What was kept vs. reverted (decided 2026-08-26).** All of it — the engine fields/methods (§1), the 5-layer command wiring, `internal/bot/symbolic`, `GoalKillbotStrategy` and its tests — is kept in place as **experimental infrastructure**, not reverted. `goal-killbot-v1` remains registered only in `cmd/tournament`'s registry, not `cmd/backend`'s live registry, so it has zero effect on production play. No further tuning was attempted this session despite the concrete, root-caused lead in §18.8 — that fix is documented as a specific, testable next step rather than an open-ended retuning loop, should this pass get picked back up later.

### 18.8 Root-cause investigation: why the commit rate is low

The 16.2% commit rate (§18.5) is below the Stage 1→2 gate's 20% bar by only a few points, which raised the question of whether it was close to clearing for a shallow, fixable reason. A temporary, exported diagnostic hook (`bot.DebugDiscoveryHook`, added to `GoalKillbotStrategy.reinforce`'s discovery branch and removed again once diagnosis was complete) counted, across every reinforce decision with no committed goal, how many times a near-completion fact was found, was viable, and cleared the commit gate. Run via a throwaway `cmd/zzzdiag` tool (also removed) across 300 games:

```
discovery-branch decisions (no committed goal): 181,248
  no fact found:          139,608 (77.0%)
  fact found:               41,640 (23.0%)
    viable | found:              781 (1.9%)
    gate cleared | viable:       781 (100%)
```

**Finding:** near-completion states are not rare — 23.0% of discovery-phase decisions see a continent missing exactly one territory. The bottleneck is entirely in the viability check, `completeContinentGoalViable` (strategy_goal_killbot.go): only 1.9% of found facts pass it. Of those that do, **100%** go on to clear the actual commit gate (`tryCompleteContinentReinforce`) — confirming the gate itself is not an independent filter here, since it uses the identical route and a strictly looser army count (current + pending reinforcements vs. viability's current-only).

**Root cause:** `completeContinentGoalViable` requires the *current* army count on the routed source territory — before any of this turn's pending reinforcements are added — to already forecast ≥35% win probability against the missing territory's defenders. This is a chicken-and-egg gate: most of the time a continent goes near-complete, the border territory isn't already stacked, because no goal existed yet to direct reinforcements there. The mechanism requires the position to look strong *before* it will commit, but committing is exactly what would make the position strong over subsequent turns. This confirms, with real numbers, the hypothesis stated speculatively in §18.7's first draft.

**Not investigated further this session:** whether changing the viability check to use `src.Armies + g.PendingReinforcements` (matching the gate's own army-count basis) would raise the commit rate enough to clear the Stage 1→2 bar, and if so, whether that changes the funnel's downstream numbers (changed-decision rate, tournament win rate) in a way that still holds up under a fresh measurement. The user's explicit decision (§18.7) was to keep the mechanism as experimental infrastructure without attempting this fix — it remains the concrete, specific next step if this pass is revisited.

## 19. Third Experiment: Desirable/Feasible/Executable Separation (`goal-killbot-v2`)

### 19.1 Motivation

§18.8's root cause was structural, not a tuning problem: `goal-killbot-v1` conflated *strategic feasibility* with *immediate tactical executability* — a goal could only commit once the position was already almost won, so the mechanism could confirm opportunities the tactical bot had already stumbled into but never *create* one. This pass redesigns the goal lifecycle around three explicitly separate questions — **desirable** (a near-completion fact exists), **feasible** (the required force could plausibly be built within a short horizon), **executable now** (already winnable with current armies) — and lets a goal commit on feasibility alone, then actively steers reinforcement/attack/occupy/fortify toward making itself executable.

### 19.2 Architecture found

- **Reinforcement is single-shot, whole-pool, per call by convention, not by engine requirement.** `risk.Game.PlaceReinforcement` (`engine.go:586`) only requires `0 < armies <= PendingReinforcements`; the phase advances to attack only once `PendingReinforcements` hits exactly 0. Every existing strategy chooses to always place the full remaining pool in one command. Partial placement across multiple same-turn reinforce calls is legal and, before this pass, unused anywhere in the codebase — it's the mechanism this pass uses to send only a top-up, never the whole pool.
- **No numeric per-territory score exists anywhere in reinforce/occupy/fortify.** `betterPixiePlacement` (`strategy_betterpixie.go:243`) is a strict priority-tier waterfall, not a scored ranking — same shape for occupy and fortify. A literal "baseline score + bias = effective score" blend is only implementable for **attack**, where `risk.ForecastAttack`'s win probability is a real number compared against a real threshold.
- **Attack already had a genuine blended-value mechanism from `goal-killbot-v1`, reused verbatim, unmodified**: `KillbotV2Strategy.bias`/`WithAttackBias`/`biasedAttack` fires the goal's target if `forecast.WinProbability >= tuning.MinAttackWinProbability - bestDelta`. `goalAttackBias` needed zero changes for this pass.
- Fortify and attack are multi-decision-per-turn phases (a bot can attack repeatedly, or decline and repeat next call); fortify is a single decision per turn (`BetterPixieStrategy.fortify` returns `ActionEndTurn`, not a fortify-specific "end," when nothing qualifies — no `ActionEndFortify` constant exists). This shaped how the `PreserveAttackStack` guards were implemented in each phase (§19.4).

### 19.3 Desirable/Feasible/Executable and the goal lifecycle

`StrategicGoal` gained `Status` (`GoalStatusActive` | `GoalStatusSecuring`) and `StatusSince`. **"Executable now" was deliberately NOT made a third stored status** — it's a purely derived, instrumentation-only signal (same re-derive-don't-store convention the missing territory itself already uses), because reinforce/attack behave identically whether or not a goal happens to already be executable; the only behaviorally distinct state is `Securing` (continent fully owned, short consolidation tail before the goal clears via `AdvanceStrategicGoalStatus`/`ClearStrategicGoal`).

`completeContinentFeasible` (`strategy_goal_killbot_v2.go`) replaces `completeContinentGoalViable`: it requires a real one-hop route (`cheapestAttackHopToTerritory`, unchanged), a bounded required-army estimate via `requiredAttackArmies` (a monotonic forward scan over `risk.ForecastAttack`, capped at 60), and a conservative turns-to-build projection using `playerIncome` (assuming *all* future income goes to this one goal — optimistic on that axis, offset by the defense-tier bypass below). A goal commits when this holds within `strategicHorizonTurns = 2` — a small, conservative, first-guess horizon, not tuned.

### 19.4 Phase integration

- **Reinforce**: each call while Active and not yet executable sends only `min(required − current, PendingReinforcements)` to the staging territory — the direct fix for "dump the whole pool speculatively." If `PendingReinforcements` exceeds what's needed, the remainder is left for the backer's own placement later in the same turn (the engine already supported this; no prior strategy used it). A `betterPixieHasUrgentContinentNeed` bypass skips goal-directed reinforcement (without abandoning the goal) on any turn the backer's own top-priority tier has an urgent claim elsewhere — **but only once a goal is already committed**. Testing found applying this same bypass to *fresh discovery* was wrong: before `pi` owns any full continent, `betterPixieWantedContinents` always names exactly one "wanted" continent as its own early-game heuristic pick, and since `pi` owns 0% of it, `betterPixieContinentNeedsHelp` trivially returns true for it regardless of whether it's a real threat — empirically confirmed to block nearly every fresh commitment on a false signal before the fix.
- **Attack**: delegates to the reused `goalAttackBias`-driven mechanism first, then blocks (`PreserveAttackStack`, `ActionEndAttack`) an unrelated attack the backer chose to launch *from* the goal's staging territory while Active — a narrow **hard constraint**, not a soft bias (no numeric attack-priority score exists to blend into), explicitly flagged as forfeiting any other attacks that same call.
- **Occupy**: reused verbatim from `goal-killbot-v1` (force max armies only when the conquered territory is the exact missing territory).
- **Fortify**: while Active, prefers moves adjacent to the missing territory (reusing `bestFortifyDestination`); if none qualify, blocks (`ActionEndTurn`) a backer choice that would move armies *out of* the staging territory — fortify's lower-blast-radius counterpart to the attack guard, since fortify is a single decision per turn regardless. While Securing, prefers moves toward the completed continent's external borders (`continentBorders`, reused unchanged) — `SecureContinentBorder`.

### 19.5 Counterfactual evaluation — honest scoping

No prior counterfactual state-branching harness was ever built in this codebase (`goal-killbot-v1`'s own "Stage 2," a `*risk.Game.Clone` engine method, was explicitly deferred and never implemented). This pass didn't build it either, for the same reason: real engine risk (RNG desync, aliasing) deserving its own dedicated effort. What *is* retained and extended is the decision-level, branch-free comparison `goal-killbot-v1` already proved safe (`WithDecisionObserverV2`'s baseline-vs-actual diff on the same unmutated state) — useful for "does it commit to non-executable goals" and "how often does it change reinforcement," but not for downstream simulated-outcome questions, which only the tournament comparison below can answer.

### 19.6 Instrumentation funnel results

**Result (300 games, seeds 1-300, 2026-08-26, `cmd/goalinstrumentv2`): 217/300 completed.**

| Metric | Result |
|---|---|
| Games with ≥1 goal committed | 44/217 (**20.3%**) — clears `goal-killbot-v1`'s old ≥20% bar |
| Total commitments | 84 |
| Reached Securing (captured the missing territory) | 41/84 (**48.8%**) |
| Fully completed (Securing → cleared) | 30 |
| Abandoned (cleared from Active, never secured) | 39 (**46.4%** of commitments) |
| Still Active / still Securing at game end | 5 / 10 |
| Turns Active, still feasible on fresh re-check | 225/692 (32.5%) |
| Decisions with a goal active | 51,409 / 142,035 total |
| Decisions changed | 1,406 (**2.7%** of goal-active decisions) |
| Changed by phase | attack 467, reinforce 647, fortify 292, occupy 0 |
| Failures | `command_limit_reached` 16, `duration_limit_reached` 67 |
| Avg game length | 92.2 turns |

The commit-rate and reach-Securing numbers are the clearest evidence this pass achieves its stated architectural goal: the mechanism now commits well before being immediately executable (proven directly by `TestGoalKillbotV2CommitsWhenFeasibleButNotExecutable`, using a fixture `goal-killbot-v1`'s own gate would have rejected outright) and then, in very close to half of all commitments, actually builds up to and captures the target. The changed-decision *rate* (2.7%) is lower than `goal-killbot-v1`'s (6.3%) despite far higher absolute volume (1,406 vs. 846 changed decisions) — because many more goal-active decisions now occur where the top-up need is already satisfied and the call correctly delegates to the backer unchanged, diluting the ratio without indicating less real engagement.

**The 46.4% abandonment rate is the number that most needs explaining — see §19.10 for the root-cause investigation.** The short version: it is not primarily a combat-variance/boom-bust artifact as first hypothesized; it is dominated (66.9% of abandonments) by the near-completion fact itself disappearing — pi losing a *second* territory within the goal's own continent to an opponent, while attention and reinforcement were concentrated on the single staging territory.

### 19.7 Tournament results (mirrored seats)

Same methodology as §18.6: two `cmd/tournament --config` batch entries, seeds 1-300, `goal-killbot-v2`/`killbot-v2` swapped between seats.

| | seatA | seatB | combined (`pairedstats`) |
|---|---|---|---|
| `goal-killbot-v2` win rate | 31.4% [25.5-38.0] | 41.3% [34.8-48.1] | 36.3% [31.8-41.1] |
| `killbot-v2` win rate | 41.5% [35.0-48.4] | 38.3% [32.0-45.1] | 40.0% [35.3-44.7] |
| Head-to-head finish order (160 paired seeds) | — | — | `goal-killbot-v2` better 72, `killbot-v2` better 88, tied 0 |

Two things stand out. First, the swing between seats for the *same* strategy (31.4% vs. 41.3%, a 10-point gap from seat/turn-order alone) is itself a reminder of why the mirrored-seat methodology exists — a single-seat read here would have supported opposite conclusions depending on which seat happened to be sampled. Second, unlike `goal-killbot-v1` (nominally *ahead* of `killbot-v2` by ~3 points in §18.6), `goal-killbot-v2` is nominally **behind** by ~3.7 points, with a head-to-head split (45% vs. 55%) that leans the same direction. Per §8's power table, this gap is still within the ~11-12 point band this sample size can't reliably distinguish from noise — so this is not a confident "it's worse" finding — but it is a clear reversal of direction from the previous pass's nominal (also noise-level) lean, and it lines up with the elevated `command_limit_reached`/`duration_limit_reached` failure counts and 92.2-turn average game length seen in §19.6's larger, non-paired sample.

### 19.8 Verdict against the brief's diagnostic questions

1. **Does it commit to goals that are not immediately executable?** Yes — directly proven (§19.6, and the dedicated unit test).
2. **Does it create the conditions to execute them?** Yes, close to half the time (48.8% of commitments reach Securing) — a real, measured success at the core architectural question this pass set out to answer.
3. **How often does it alter reinforcement before execution becomes available?** 647 of the 1,406 changed decisions are reinforce-phase.
4. **How often do committed goals reach executable state?** Not separately instrumented from "reach Securing" this pass (§19.6's named gap) — 48.8% is the closest available number.
5. **How often do executable goals complete?** 30 of 41 that reached Securing went on to fully complete (73.2%) — the remainder were still Securing at game end, not failures.
6. **How often are goals abandoned?** 46.4% of commitments.
7. **Are goals abandoned for sensible reasons?** Now answered with real data (§19.10): 66.9% of abandonments are the near-completion fact itself disappearing — pi losing a *second* territory in the goal's continent to an opponent, not a feasibility miscalculation or a slow build. Whether that's "sensible" is itself the open question §19.10 raises: it may mean the mechanism correctly recognizes a genuinely lost position, or it may mean the mechanism *causes* those losses by concentrating reinforcement on one staging territory and leaving the rest of the continent thinner than the backer would otherwise have kept it.
8. **Does strategic preparation cause harmful opportunity cost?** More clearly suggestive after §19.10: concentrating reinforcement on a single staging territory, while the rest of the goal's continent (and the board generally) gets whatever the backer would have sent it minus that diversion, is a plausible direct mechanism for the observed nominal win-rate disadvantage — not just a correlation.
9. **Does goal persistence improve coherence or create tunnel vision?** §19.10 makes this the most likely candidate explanation available: reinforcement literally tunnels toward one territory while a goal is Active, and the dominant abandonment cause is losing ground elsewhere in the same continent — the shape of the data is consistent with a real tunnel-vision cost, though this pass didn't build a controlled comparison (e.g., "with vs. without the goal-directed reinforcement diversion, holding everything else fixed") to confirm causation directly.
10. **Does it improve counterfactual or tournament outcomes?** No — the properly paired, mirrored-seat comparison shows a nominal disadvantage against `killbot-v2`, not an improvement, though not a statistically confident one at this sample size.

### 19.10 Root-cause investigation: why the abandonment rate is high

The 46.4% abandonment rate (§19.6) raised the same question §18.8 asked of the commit rate: is this close to a threshold for a shallow, fixable reason, or something structural? A temporary, exported diagnostic hook (`bot.DebugAbandonReason`, added to `GoalKillbotV2Strategy.reinforce`'s primary abandonment branch and removed again once diagnosis was complete) categorized every abandonment by cause. Run via a throwaway `cmd/zzzabandondiag` tool (also removed) across 300 games:

```
total abandonments observed: 477

abandonment cause breakdown:
  defender_too_strong              29 (6.1%)
  fact_gone                        319 (66.9%)
  feasible_only_inertia_cap_hit    45 (9.4%)
  too_slow_to_build                84 (17.6%)

"too_slow_to_build" shortfall breakdown (required - current armies, at abandonment):
  small (<=2, likely just lost a fight):   33 (39.3%)
  medium (3-6):                              9 (10.7%)
  large (>6, never really built up):        42 (50.0%)
```

**Finding:** the originally-hypothesized "boom-bust combat variance" story (§19.6's first draft) is not the dominant pattern — `too_slow_to_build` is only 17.6% of abandonments, and even within that bucket the shortfalls split roughly evenly between "just lost a fight" (39.3%) and "never really built up" (50.0%), not overwhelmingly one or the other. The dominant cause by a wide margin is `fact_gone` at 66.9%.

**What `fact_gone` means, precisely:** `GoalKillbotV2Strategy.reinforce` checks `ownsContinent` *before* the feasibility/abandonment check and routes full continent ownership to `GoalStatusSecuring` separately — so `fact_gone` can only mean the near-completion fact (exactly one missing territory) no longer holds *and* the continent isn't fully owned either. The only way that's possible is pi now has **two or more** missing territories in that continent — i.e., pi lost a second territory (or the staging territory itself) to an opponent while the goal was active.

**The open question this raises, not yet answered:** whether the goal-directed reinforcement mechanism is *correctly recognizing* a position that was genuinely lost for reasons unrelated to the goal (normal multiplayer pressure, another player having a strong board position generally), or whether it is *causing* some of these losses itself — by concentrating reinforcement on one staging territory while Active, the goal's own other continent territories (and the rest of the board) get whatever's left over from the backer's normal logic minus that diversion, potentially thinner than they'd otherwise be. This pass's instrumentation cannot distinguish these two stories: doing so would require comparing, at the moment a continent-territory was lost, what the defending army count would have been *without* the goal's reinforcement diversion — a genuine counterfactual, the same class of question §19.5 already scoped out of this pass (would need the deferred `*risk.Game.Clone` branching harness, or at minimum a same-seed A/B replay comparing goal-killbot-v2 against plain killbot-v2 at matched turns). Absent that, the honest conclusion is: this is the single most likely mechanism connecting the architecture's real, proven success (proactive goal creation, §19.6) to its measured, if unconfident, win-rate cost (§19.7) — but it is a strong lead, not a confirmed causal finding.

### 19.11 What was kept vs. reverted

**Decided 2026-08-27: kept as experimental infrastructure, not reverted.** All of it — the engine `Status`/`StatusSince` fields and `AdvanceStrategicGoalStatus` method (§19.1), the 6th `StrategicGoalStatus` command-wiring field across all 5 layers, `GoalKillbotV2Strategy` and its tests — stays in place. `goal-killbot-v2` remains registered only in `cmd/tournament`'s registry, never `cmd/backend`'s live registry, so it has zero effect on production play, identical to `goal-killbot-v1`'s own disposition.

This pass answered its own motivating architectural question affirmatively and with real evidence: **explicit persistent strategic intent can proactively create the conditions for its own goals** — a goal now commits well before being immediately executable, and close to half of all commitments go on to actually capture the target, a capability `goal-killbot-v1`'s executable-now gate structurally could not provide. That success did not translate into a whole-game win-rate improvement — the paired tournament shows a nominal, not-statistically-confident *disadvantage* against plain `killbot-v2` — and §19.10's investigation identified a specific, plausible, evidence-backed cost mechanism (concentrating reinforcement on one staging territory while Active correlates with losing a *different* territory in the same continent, the dominant cause of abandonment) without being able to fully confirm causation absent a counterfactual-branching harness this pass deliberately didn't build (§19.5). No further tuning (e.g., a light border-defense carve-out for the goal continent's non-staging territories) was attempted this session — it remains the concrete, specific, evidence-motivated next step if this pass is revisited, distinct from `goal-killbot-v1`'s own untested lead (§18.7's viability-army-count fix, which this pass already subsumed and fixed via the desirable/feasible/executable redesign).

## 20. Fourth Experiment: A Genuine Director/Executor Split (`director-v1`)

### 20.1 Motivation

Every pass through §19 shared one architectural shape, whatever its other differences: `killbot-v2`'s own decision tree ran first and decided almost everything, and the "symbolic layer" was a thin wrapper that patched its output afterward in a few narrow, phase-specific spots — a relaxed attack bar for one territory, a forced max-army occupy, a filtered fortify candidate, a hard block on an unrelated attack. The user's own assessment, prompted directly by this pattern: this was never really "the symbolic layer thinking strategically," it was a heuristic bot occasionally getting vetoed. This pass builds a genuine alternative: an independent strategic layer (`SymbolicDirectorStrategy`) that only ever decides *whether* to pursue a goal, and a separate, directive-aware heuristic layer (`DirectedPixieStrategy`) that decides *how*, using its own native tactical machinery — director and executor, not patcher and patched.

### 20.2 Scope decisions made before building

Two research passes (parallel `Explore` agents) settled two open questions before design:

1. **Where a directive can reach the executor.** `betterPixieWantedContinents` is consumed directly by reinforce, attack, and occupy — but fortify never consults it at all (confirmed by reading `betterPixieFortifyDestination` in full: it scores purely on continent-border-distance and weakest-enemy-neighbor). A directive therefore reaches 3 of 4 phases through one seam; fortify needed its own explicit mechanism (`Directive.FocusTerritory`).
2. **Historical/replay data access.** Confirmed unreachable from a `bot.Strategy` today without new plumbing (the "live replay" event logs live entirely in Postgres, populated one layer above the engine, read only by a REST endpoint). **Decided out of scope for this pass** — built on current game state only, matching every prior pass, with history remaining an explicitly deferred follow-up.

Also confirmed: every helper function `BetterPixieStrategy`'s phase methods use is a standalone, package-level function, not a method on the type — meaning the fork only required reimplementing the 4 short top-level phase methods (each with exactly one line changed), reusing every other helper unchanged.

### 20.3 Architecture

`Directive{WantedContinents map[risk.Continent]bool, FocusTerritory risk.Territory}` — the zero value is a no-op. `DirectedPixieStrategy` (`strategy_directed_pixie.go`) is a genuine fork of `BetterPixieStrategy` (which stays completely untouched — every "dumb" bot built on it, and both prior goal-killbot passes, are unaffected): reinforce/attack/occupy union `dir.WantedContinents` into whatever `betterPixieWantedContinents` would independently choose (an addition, never a replacement); fortify, which has no `wanted` to union into, prefers a candidate adjacent to `dir.FocusTerritory` when set, falling back to plain `betterPixieFortifyDestination` otherwise. **Verified byte-identical to plain `BetterPixieStrategy` with an empty `Directive`** across all 4 phases (`TestDirectedPixieEmptyDirectiveMatchesPlainBetterPixie`) — the same no-op guarantee every prior pass's hook met.

`SymbolicDirectorStrategy` (`strategy_director.go`) reuses `goal-killbot-v2`'s desirable/feasible/lifecycle machinery unchanged (`completeContinentBestCandidate`, `completeContinentFeasible`, the `Active`→`Securing`→cleared engine lifecycle) but never constructs a tactical `Command` by hand — every decision builds a `Directive` and delegates to a per-tuning `DirectedPixieStrategy` instance (`executorFor`, mirroring `KillbotV2Strategy.backerFor`'s exact per-game-tuning-freshness pattern). This drops the two `PreserveAttackStack` hard-constraint guards `goal-killbot-v2` needed — there's nothing to guard against once the executor's own attack search already prioritizes wanted continents before its unrelated-attack fallbacks.

**A deliberate scope reduction versus `goal-killbot-v2`, made explicit rather than silently dropped:** the reinforce-phase army-amount decision is now **fully delegated** to the executor's own `betterPixieContinentNeedsHelp`/`placeToTakeSpecificContinent` machinery (a fixed 1-army-per-call top-up to any continent whose border has ≤20 armies — `pixieBorderForce`, unrelated to `completeContinentFeasible`'s own computed `required` army count), not `goal-killbot-v2`'s custom "send exactly the computed top-up, then stop" math. This was a deliberate architectural choice — cede the tactical decision to the executor — and its consequences are exactly what §20.5 measures.

### 20.4 Testing

Mirrors `strategy_goal_killbot_v2_test.go`'s shape (commit-on-feasible, Active→Securing transition, securing-duration clear, inertia-cap abandonment, decision-observer Changed reporting) plus the empty-`Directive` parity suite and a fortify `FocusTerritory` steering test. All pass; every pre-existing `TestKillbotV2*`/`TestGoalKillbotV2*` test passes unmodified.

### 20.5 Results (300 games, seeds 1-300, 2026-08-27)

**Funnel (`cmd/goalinstrumentv3`): 196/300 completed.**

| Metric | Result | `goal-killbot-v2` (§19.6) |
|---|---|---|
| Games with ≥1 goal committed | 40/196 (20.4%) | 44/217 (20.3%) |
| Total commitments | 89 | 84 |
| Reached Securing | 42/89 (47.2%) | 41/84 (48.8%) |
| Abandoned | 59/89 (66.3%) | 39/84 (46.4%) |
| Decisions changed (of goal-active) | 23,232/42,417 (54.8%) | 1,406/51,409 (2.7%) |
| Failures | `command_limit_reached` 25, `duration_limit_reached` 79 | 16, 67 |
| Avg game length | 101.9 turns | 92.2 turns |

**Mirrored-seat tournament + `pairedstats`:**

| | `director-v1` | `killbot-v2` |
|---|---|---|
| Win rate (combined) | 10.7% [7.9-14.4] | 61.3% [56.1-66.2] |
| Head-to-head (106 paired seeds) | 27 | 79 |

This gap (10.7% vs. 61.3%) is enormous and **statistically decisive** — the first result across all four passes to clear the ~11-12 point noise floor established in §8's power analysis by a wide margin, not a nominal/inconclusive lean like every prior pass's tournament result.

### 20.6 Separating two confounded effects

A ~50-point gap this large demanded checking what's actually driving it before concluding anything about directing. `betterpixie-v1` — plain, undirected, no kill-hunting, already running in the same tournament field as a baseline — won 14.4% [11.1-18.4] in the same 300 games, essentially matching `director-v1`'s own 10.7% (overlapping CIs). Isolating the directive's own effect directly (`director-v1` vs. `betterpixie-v1`, its own undirected sibling sharing the identical base): win rate nominally lower for `director-v1` (10.7% vs. 14.4%) but head-to-head is nearly even and slightly *favors* `director-v1` (56 vs. 50 of 106 paired seeds) — inconclusive at this sample size, not a confident finding either way.

**Conclusion: the ~50-point gap against `killbot-v2` is overwhelmingly explained by `DirectedPixieStrategy` forking from `BetterPixieStrategy` rather than `KillbotV2Strategy`** — an explicit, approved scope decision (§20.3) that meant `director-v1` carries no kill-hunting logic at all, a capability `killbot-v2`'s own composition chain (`BetterPixieStrategy → KillbotStrategy → KillbotV2Strategy`) adds on top of the identical base this pass forked from. This is a genuine architectural cost of the fork-from-BetterPixie decision, not evidence about whether strategic direction itself helps or hurts — that question is only cleanly answered by the `director-v1` vs. `betterpixie-v1` comparison, which is inconclusive.

### 20.7 The abandonment-cause result — not confounded, and it disproves the hypothesis

Unlike the win-rate comparison, the abandonment-cause breakdown is a **within-goal-mechanism** measurement (what happens once a goal is committed and Active) and isn't confounded by the kill-hunting gap at all — both `goal-killbot-v2` and `director-v1` are being compared against themselves, on the same question: why did commitments fail to secure their continent?

```
                                  goal-killbot-v2 (§19.10)   director-v1 (this pass)
fact_gone                        66.9%                       81.4%
too_slow_to_build                17.6%                       10.2%
feasible_only_inertia_cap_hit     9.4%                        1.7%
defender_too_strong               6.1%                        6.8%
```

**The hypothesis this pass set out to test — that unioning the goal continent into `wanted` (rather than `goal-killbot-v2`'s narrow after-the-fact patches) would let the executor's own multi-continent balancing keep defending the rest of the continent, reducing the dominant `fact_gone` cause — was disproven. `fact_gone` got worse, not better.**

The most plausible mechanism (§20.3's flagged scope reduction, now showing its consequences): `betterPixieContinentNeedsHelp` uses a fixed, generic threshold (`pixieBorderForce = 20`) completely decoupled from `completeContinentFeasible`'s own computed `required` army count. Since the goal continent is by definition not yet fully owned while Active, `betterPixieContinentNeedsHelp` returns true for it unconditionally — and once "wanted," `directedPixiePlacement`'s loop keeps returning to it every reinforcement call, 1 army at a time, for as long as its border stays under 20 armies, a bar with no relationship to the specific tactical requirement the strategic layer already computed. `goal-killbot-v2`'s custom top-up math, by contrast, computed the precise required count and *stopped* once armies met it, explicitly leaving the remainder for the backer. Ceding that decision to the executor's own generic heuristic removed that cap — plausibly *increasing* overcommitment to the goal continent rather than reducing it, which would directly explain a higher rate of losing a second, undefended territory elsewhere in the same continent.

### 20.8 One targeted fix attempted: `Directive.ArmyCap`

§20.7's diagnosis was specific enough to test directly: add a quantitative cap to the directive, so the executor's placement decision respects the strategic layer's own computed `required` army count instead of `betterPixieContinentNeedsHelp`'s generic, unrelated 20-army threshold. Implemented as `Directive.ArmyCap map[risk.Continent]int` — when the continent's staging territory (the same one `placeToTakeSpecificContinent` deterministically picks, confirmed by reading its implementation) already meets the cap, `directedPixiePlacement`'s "needs help" tier skips that continent, even though the generic threshold would still say yes. `SymbolicDirectorStrategy.reinforce` populates it with `completeContinentFeasible`'s own `required` value. Verified directly (`TestDirectedPixiePlacementRespectsArmyCap`): the needs-help tier's own signature (a fixed 1-army placement) stops firing once the cap is met.

**Re-measured (300 games, same methodology): no measurable change.** `fact_gone` stayed at 81.4% (vs. the pre-fix 81.4%, identical to two decimal places), `director-v1` vs. `killbot-v2` moved from 10.7%/61.3% to 9.5%/61.0%, and `director-v1` vs. `betterpixie-v1` moved from 10.7%/14.4% (head-to-head 56-50) to 9.5%/13.1% (head-to-head 58-54) — every number within noise of its pre-fix value.

**Why the fix didn't work, discovered while writing its own test**: `directedPixiePlacement` has a fallback cascade, and the cap only touches its first tier. Once the needs-help tier is capped out, the loop falls through to `placeNearWeakestEnemyCluster` — a *different*, uncapped tier that searches all non-owned board territory for the weakest reachable enemy cluster. With most of the board forming one large non-owned connected component (the common case once one player has expanded at all), this tier independently identifies the goal's own staging territory as the strongest pi-owned candidate bordering that component — for the same underlying reason it was ever a good staging territory in the first place — and redirects reinforcement there anyway, uncapped, via a completely different, unrelated code path. The cap closes one door; the executor's own cascade opens another. A real fix would need to cap reinforcement flow toward the staging territory *across every tier that could reach it*, not just the first — a materially larger, more invasive change than one targeted parameter, and out of scope for this pass's approved fix.

### 20.9 Verdict

**The director/executor split, as an architecture, works exactly as designed** — `SymbolicDirectorStrategy` never constructs a tactical command, `DirectedPixieStrategy` never decides strategy, and the split is real and testable (confirmed by the empty-`Directive` parity guarantee and the `PreserveAttackStack` guards becoming unnecessary). **But this pass's central hypothesis — that letting the executor's own native machinery handle tactical execution would reduce the tunnel-vision cost `goal-killbot-v2` diagnosed — did not hold up, and a targeted, well-reasoned fix (§20.8) did not rescue it.** The reason generalizes past this one pass: `BetterPixieStrategy`'s reinforcement logic is a *cascade* of independent fallback tiers, and a directive that only informs the first tier can be silently overridden by a later one reaching the same territory through unrelated logic — "trust the executor's own judgment" is not a single decision point in this codebase, it's several, and all of them would need directive-awareness for a cap like this to actually hold. The win-rate comparison against `killbot-v2` remains uninformative on its own (dominated by the unrelated, explicit scope decision to fork from `BetterPixieStrategy` rather than `KillbotV2Strategy`, confirmed unchanged by the fix) — the clean, unconfounded read on directing itself (`director-v1` vs. `betterpixie-v1`) stayed inconclusive both before and after the fix, not negative, but the fix's own target metric (`fact_gone`) shows no improvement to point to either.

### 20.10 What was kept vs. reverted

**Decided 2026-08-28: kept as experimental infrastructure, not reverted.** `strategy_directed_pixie.go` (`Directive`, `DirectedPixieStrategy`), `strategy_director.go` (`SymbolicDirectorStrategy`, `DirectorAbandonCause`), and both test files stay in place, alongside `cmd/goalinstrumentv3` and the mirrored-seat measurement scripts. `director-v1` remains registered only in `cmd/tournament`'s registry, never `cmd/backend`'s live registry — identical disposition to every prior pass.

The rationale for keeping rather than reverting, despite two null results in a row (the union hypothesis in §20.7, the targeted `ArmyCap` fix in §20.8): this pass's actual deliverable — the director/executor architecture itself — is a genuine, working answer to the user's explicit request ("I truly want the symbolic layer to be the decision maker, and the bot layer to be the execution layer"), independently verified by the empty-`Directive` parity guarantee and the elimination of `goal-killbot-v2`'s two `PreserveAttackStack` hard-constraint patches (no longer needed once the executor's own search is directive-aware). That the *specific* tactical-delegation choice this pass tested didn't pay off is a separate, well-diagnosed finding, not a flaw in the architecture that hosts it — and the diagnosis itself (BetterPixie's reinforcement logic is a multi-tier cascade, not one decision point; a directive-aware cap needs to reach every tier that could touch the staging territory, not just the first) is exactly the kind of specific, actionable lead worth keeping a record of, matching this session's discipline every prior pass followed.

**Four passes, four kept baselines.** `goal-killbot-v1` (reactive-then-persistent goal, gated on immediate executability), `goal-killbot-v2` (desirable/feasible/executable split, patches `killbot-v2`'s output), `director-v1` (genuine director/executor split, patches nothing) now all coexist as independent, unmodified, tournament-only experimental strategies — none reverted since `goal-killbot-v1`'s own pass 1 precursor. Any future pass should read §18-§20 in full before proposing a fifth: the specific failure points already identified (viability gate too strict; tunnel vision from unmediated reinforcement concentration; a directive needing to reach every fallback tier, not just the first) are the load-bearing findings, not the whole-game win-rate numbers themselves, which have stayed within statistical noise of `killbot-v2` in every comparison except this pass's kill-hunting-confounded one.
