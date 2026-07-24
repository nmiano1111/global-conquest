# Feature Expansion Roadmap: What the Board Actually Knows That We're Not Telling the Network

> Working document — a living catalog of candidate training-feature additions, not a finished plan. Builds on `GCN_Value_Function_Training_Data.md`'s description of the current feature set; read that first for the baseline this doc proposes to extend. Updated as each candidate is actually scoped, built, and evaluated — like the other roadmap docs in this project, this should stay accurate as a running record, not a stale pre-work snapshot.

## Motivation

The current feature set (`internal/tdstate/encode.go`) is close to a direct port of the source paper's own feature list (Jamie Carr, arXiv:2009.06355) — per-territory ownership/army-fraction/continent/border/threat, plus a handful of global aggregates, one deliberately hand-crafted (`Defence`). That's a reasonable, well-justified starting point, and the paper is explicit about *why* it stayed minimal: the whole thesis is that a GCN can learn positional judgment from low-level, general features via message-passing, so hand-engineering is treated as an admission of the network's limits, not a design choice made freely (see the paper's own quote in `GCN_Value_Function_Training_Data.md`'s `Defence` entry).

That's a defensible research goal for a paper. It's a more questionable choice for this project, whose actual goal is a strong bot, not a demonstration that minimal features suffice. Risk is a game with a lot of structure a human player reasons about explicitly — who's about to be eliminated, whether an attack is actually favorable given real dice odds (not just a raw army ratio), whether a continent is one territory away from completion, whether a hand of cards is already a cashable set — and right now, essentially none of that is handed to the network directly. It either has to be re-derived through the GCN's own 2-hop message-passing and the FC layers' global mixing (which may or may not actually happen in practice, and definitely doesn't happen for anything requiring counting/pathfinding/lookup beyond local aggregation), or it's simply invisible to the value function.

The working hypothesis this doc is built around: the paper's author was optimizing for a specific research question (can a lean architecture learn this game), and in doing so, left real, cheaply-available signal on the table that a bot actually trying to win should just be handed directly.

## What's already been identified (cross-reference, not repeated)

The most-discussed gap so far — no per-territory or per-opponent identity, only a binary `IsMine` and a single "strongest enemy" aggregate — is covered in depth in `GCN_Value_Function_Training_Data.md`'s "current limitation" section and `GCN_Value_Function_Overview.md`'s closing section. It's included in this doc's catalog (below) for completeness and prioritization, but the full reasoning lives there.

## Candidate feature categories

Each candidate below notes: what it captures, why the network can't already get this for free, and roughly how expensive it'd be to add (existing helper to reuse vs. new logic vs. new engine state entirely).

### 1. Individual/ranked enemy differentiation

**What**: replace `IsMine`'s binary mine/not-mine with a ranked category (`mine`, `rank-1-enemy` ... `rank-K-enemy`, ranked fresh each turn by total armies or similar), and expand `StrongestEnemyArmyFraction`/`...TerritoryFraction` into a full per-rank array instead of a single max.

**Why it's missing**: covered at length already — the paper's own "keep it global, not identity-tied" principle, generalized correctly, doesn't actually forbid this (rank is relative, not raw identity), but nobody had built it.

**Cost**: the biggest item on this list. Changes per-territory feature width (cascades through `node_feature_dim` on both sides) *and* global feature width, needs new ranking logic (max players − 1 = 5 ranks), and needs a defined sentinel for "this rank doesn't exist" (fewer players, or that rank's player already eliminated). Full retrain required.

### 2. Combat-realism via the existing Attack Handler

**What**: the current `EnemyThreatFraction` is a raw army sum — it treats "3 armies next door" as a fixed threat level regardless of how many armies *I* have to respond with, when Risk's actual dice math is highly nonlinear in the army ratio (a 2-army attacker vs. a 1-army defender is a very different proposition than 20 vs. 10, despite an identical 2:1 ratio). This project already has exactly the machinery to compute real combat odds — `internal/bot/combat_odds.go`'s `ForecastAttack(attackerArmies, defenderArmies) → WinProbability` and Phase 1's `AttackTerminalStates`/`SelectTerminalState` — built for search, never used as a *feature*.

Candidates:
- Per-territory: `ForecastAttack(myArmies, strongestAdjacentEnemyArmies).WinProbability` — "if I attacked my worst neighbor right now, how would it actually go," instead of a raw army-fraction proxy.
- Per-territory (defensive direction): `ForecastAttack(weakestAdjacentEnemyArmies, myArmies).WinProbability` from the enemy's perspective — "how exposed am I to being attacked, in real terms."

**Why it's missing**: `internal/tdstate` deliberately depends only on `internal/risk`, not `internal/bot` (see `encode.go`'s own package doc comment — avoiding an import cycle, since `internal/bot` will eventually depend on `tdstate`). `ForecastAttack`/`AttackTerminalStates` currently live in `internal/bot`. Using them from the encoder means either moving the combat-forecast machinery to a package both sides can depend on (e.g. into `internal/risk` itself, or a new shared package), or duplicating the (fairly self-contained, already dice-agnostic) forecasting logic into `tdstate` directly — matching the existing precedent (`isContinentBorder`/`enemyThreatFraction`-style helpers are already duplicated rather than imported, for exactly this reason).

**Cost**: moderate. The hard combinatorial work (`roundDistribution`, the recursive/DP forecast) already exists and is already tested — this is a relocation-or-duplication problem, not new algorithm design. Adds 1-2 new per-territory feature slots.

### 3. Continent-completion progress, not just army share

**What**: `ContinentArmyFraction` (already in the encoding) measures the viewer's *army* share within a continent. It does not measure *territory* share within a continent — a player could hold a commanding army fraction in a continent while still not owning one of its territories at all (the actual blocker to the bonus), or vice versa (own every territory narrowly, weak armies). A `ContinentTerritoryFraction[]` (mine-owned-territories-in-continent / total-territories-in-continent), parallel to the existing army-fraction array, would let the network distinguish "I'm one conquest away from a continent bonus" from "I have a lot of armies scattered across a continent I don't control."

**Why it's missing**: seemingly just not built yet — no known reason to think this was deliberately excluded; the paper doesn't discuss this specific distinction (per what's been read so far).

**Cost**: low. Same shape as the existing `ContinentArmyFraction` computation, one more pass over `g.Board.Continents`.

### 4. Structural/board-topology features (chokepoints)

**What**: a territory's number of neighbors is itself real strategic information independent of who currently owns what — a well-known Risk heuristic is that low-connectivity territories (Indonesia guarding Australia, Alaska/Greenland/Iceland as North America/Europe's few connection points) are disproportionately valuable to hold, since they're cheaper to defend (fewer fronts to reinforce). Candidate: `len(g.Board.Adjacent[t])` as a per-territory feature, or specifically "how many of my neighbors are non-mine" as a defensibility signal distinct from `EnemyThreatFraction`'s magnitude-only view.

**Why it's missing**: this is pure, static board topology — never changes within or across games on the same map — so it's easy to overlook as "obviously derivable," but the network never actually sees it as an explicit number; it would have to infer connectivity purely from how often a node's GCN embedding gets refreshed by neighbors, which isn't the same as being told the count directly.

**Cost**: very low — static per the board, computable once and reused (similar to `BoardSchema`), no dependency on live game state at all.

### 5. Momentum / trend features

**What**: everything in the current encoding is a snapshot at one turn boundary — no signal for "am I gaining or losing ground." TD(λ) itself handles temporal credit assignment across the *value* function's own predictions, but that's a different thing from handing the network an explicit velocity signal (e.g. "my army fraction now, minus my army fraction N turns ago" or "territories captured last turn"). Explicit trend features could make the learning problem easier by removing the need to implicitly reconstruct trend from bootstrapped value differences alone.

**Why it's missing**: `Encode(g, pi)` is a pure function of the *current* state — it has no access to history at all. This is a structural gap, not an oversight: `tdstate.Encode` would need either (a) a caller-supplied previous-state snapshot to diff against, or (b) the *training pipeline* (not the encoder) computing a trend feature by comparing consecutive rows of the same episode after the fact.

**Cost**: moderate, and architecturally different from the others — this doesn't fit cleanly into `Encode`'s current pure-function-of-one-state signature. Worth scoping as its own question: does trend belong in the feature vector at all, or is it more honestly TD(λ)'s job already, and adding it would be redundant/fighting the training objective rather than complementing it? (Open question, not resolved here.)

### 6. Card-hand composition (set-completeness, not symbol-blind count)

**What**: `CardFraction` counts hand size only — it can't distinguish a hand that's *already* a cashable set (any 3 matching symbols, or one of each) from a hand of the same size that needs a specific card to complete. A cashable-set-in-hand boolean, or a "how many cards away from a set" feature, is a much more actionable signal than raw count.

**Why it's missing**: `risk.PlayerState.Cards` already carries each card's `Symbol` — the data exists (`risk.LegalCardTurnIns` already computes exactly this kind of set-detection logic for real gameplay), just not surfaced as a training feature.

**Cost**: low — the set-completion check is small, self-contained logic, and (per the same import-direction constraint as item 2) would need to be duplicated into `tdstate` rather than imported from wherever the real check lives, matching this project's existing precedent for that situation.

### 7. Turn-order / initiative

**What**: `IsMyTurn` (already in the encoding) is binary — it doesn't capture *how far away* the viewer's next turn is in a 6-player game, which plausibly matters for how reactive vs. proactive a position should be evaluated (a state right before my turn is worth more to react to than the same state five other players' turns away). A normalized "turns until I move again" feature is a candidate.

**Why it's missing**: not discussed in available source material; likely just not considered given the paper's own framing is per-single-decision-point evaluation, not turn-order-aware.

**Cost**: low — `g.CurrentPlayer` plus seat order already fully determines this.

### 8. Territory volatility (recently conquered) — flagged as the one candidate needing new engine state

**What**: a just-conquered territory is systematically different from a long-held one (minimum-occupy-forced army count, more likely to still be contested, a natural next target for a counter-attack) — human players track this. Currently invisible.

**Why it's missing**: unlike everything else in this list, this genuinely isn't derivable from `risk.Game`'s current state at all — there's no "turns since this territory last changed hands" tracked anywhere (confirmed: no such field exists in the engine today). This would require either adding real state to `risk.Game`/`TerritoryState` (a schema change with real engine-wide implications, not just an encoder change) or reconstructing it from the training pipeline's own turn-boundary history (buildable in `cmd/tdtraindata` without touching the engine, but then only available to training, not to live inference unless the same tracking is added there too).

**Cost**: the second-most expensive item on this list, and the only one that isn't a pure `tdstate`/`gcn_fit.py` change — flagged explicitly rather than scoped further here, since it needs its own design discussion about where that state should actually live.

## Cost model: why "just add a feature" isn't free

Two different kinds of feature have very different costs, worth keeping distinct when prioritizing:

- **Per-territory features** multiply by 42 (the board's node count) into `GCN1`'s input width, and because the network is a *shared-weight* per-node model, adding one per-territory feature is really asking the network to learn one new general rule applicable everywhere, not 42 independent facts — usually the right kind of addition for this architecture, but it does grow `node_feature_dim`, which cascades into `node_feature_dim` derivation on both the Python and Go sides (`gcn_fit.py`'s and `gcnmodel.go`'s identically-named functions, which must stay in lockstep).
- **Global features** are cheaper structurally (one scalar, once), but easier to make redundant with what the GCN layers already capture in aggregate via `FC2`'s board-embedding path.

Either way: **any change to feature width requires regenerating the entire training dataset** (`cmd/tdtraindata` output is tied to one fixed `Flatten()` layout) **and a full retrain from scratch** — nothing here is a hot-swappable inference-time tweak. Per the standing lesson from earlier this project's own history ([[feedback_check_architecture_capacity_before_tuning]]): before assuming a richer feature will actually be *learned* well, it's worth confirming the training data volume is large enough to support the added capacity, not just that the feature is theoretically expressible.

## Proposed phased approach

Matching this project's established "isolate one variable, validate cheaply" discipline — not a big-bang feature dump:

1. **Cheapest, most self-contained first**: chokepoint/connectivity (§4) and continent-territory-fraction (§3) — both low-cost, no cross-package dependency questions, easy to validate in isolation (train once with just these two added, compare against the current baseline on the same two standard tournament matchups).
2. **Combat-realism** (§2) — moderate cost, high plausible value (directly addresses the "army-fraction is a bad proxy for real combat odds" gap), but needs the `ForecastAttack` relocation/duplication question settled first.
3. **Card-set-completeness** (§6) and **turn-order/initiative** (§7) — both low-cost, can likely be bundled with whichever of the above phases is convenient rather than needing their own dedicated pass.
4. **Individual/ranked enemy differentiation** (§1) — highest cost, highest plausible payoff given it's the one directly implicated in the killbot-elimination-hunting gap. Deliberately sequenced after the cheaper wins above, so the training/eval pipeline is already being exercised on smaller changes before committing to the biggest one.
5. **Momentum/trend** (§5) and **territory volatility** (§8) — both need their own design resolution (where does history live, does trend belong in features at all) before they're even scoped enough to build. Parked, not scheduled.

## Evaluation methodology: pure model (depth=0) before search

Every feature added here should be evaluated **without search first** — the plain single-ply `ValueStrategy` (`AttackSearchDepth=0`), not the depth=2/3 search built in `Search_Integration_Roadmap_with_References.md`'s Phase 2. Reasons, not just habit:

- **Isolates the actual variable.** Search depth and feature quality are two separate levers; testing a new feature only with search enabled makes any win-rate change ambiguous between "the value function got better" and "search did more with a differently-shaped signal." This project already spent real effort untangling an analogous confusion once this session (training objective vs. margin calibration, before either was isolated).
- **Far cheaper.** A depth=0 decision costs ~70ms; depth=2 costs 400ms-several seconds; depth=3 games ran into hours. Getting a first read on whether a feature helps at all should not require the search-scale compute budget.
- **The sharpest test of this doc's actual motivating hypothesis.** §1 (ranked enemy differentiation) exists because the killbot dominance was diagnosed as a value-function blind spot, not a search-depth problem — depth=2 and depth=3 both left it untouched. The cleanest possible test of "did adding this fix it" is the *pure* model against the killbot matchup, with no search in the loop to confound attribution. A negative result there (no improvement even with the feature) is real, valuable information before spending more effort on search.
- **Matches this project's own established discipline** — every other lever (TD(λ) vs. supervised, margin percentile, epoch count) was validated in isolation before search was ever added to the picture; feature changes should follow the same order.

Search-enabled evaluation is still a valid, valuable *second* step once a feature is confirmed to help the pure model — features and search could still interact or compound — but it should never be the *first* signal a new feature is judged on.

## Open questions

- Where should `ForecastAttack`/combat-forecast logic actually live so both `internal/bot` (search) and `internal/tdstate` (encoding) can use it without an import cycle — moved into `internal/risk`, a new shared package, or duplicated?
- Is momentum/trend information genuinely additive to what TD(λ)'s own bootstrapping already captures, or would it fight the training objective by giving the network a shortcut that doesn't generalize as well?
- For territory volatility (§8): does this belong as real `risk.Game` state (available to live play, not just offline training) or as a training-pipeline-only reconstruction?
- Should features be added and evaluated one at a time (maximally isolatable, slower overall) or in the small bundles proposed above (faster, but a positive result wouldn't say which specific feature drove it)?

## Status

Not started — this is the catalog and prioritization pass, done before any implementation. Next step (when picked up): scope Phase 1 (§3 + §4) as a concrete, buildable change, following the same plan-mode-first discipline as `Search_Integration_Roadmap_with_References.md`'s phases.
