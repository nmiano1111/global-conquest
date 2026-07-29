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

### 1. Individual/ranked enemy differentiation — tried three ways, all failed a controlled test

**Status: abandoned.** Three separate encodings of "give the value function individual-enemy awareness" were built and evaluated against the killbot matchup (pure model, `AttackSearchDepth=0`, per this doc's own methodology below), and every one of them made things worse, not better, relative to the plain `IsMine` + `StrongestEnemy*` scheme already in `internal/tdstate`. The code has been reverted to that original scheme.

**v1 — live-strength rank.** Replaced `IsMine` with a one-hot over `{mine, rank-1-enemy, ..., rank-5-enemy}`, ranking living rivals by *current total armies*, recomputed every turn. Real retrained result: 5.0% [2.6-9.6] vs. killbot, well below the ~15.9% baseline. Diagnosed a real flaw before even getting the win-rate number: "rank flicker" — two closely-matched rivals swapping slots from routine army fluctuation, relabeling many territories at once mid-game (measured 7-28% flip rate across 5 traced seeds). The user's fix requirement, stated directly: *"We need a way to keep player 'id' completely stable per game. It can't depend on rank."*

**v2 — fixed seat-position identity.** Assigned each enemy a slot by turn-order position relative to the viewer (`(playerIndex - pi) mod numPlayers`), computed once from static game structure — immune to rank flicker by construction, since a slot is never reassigned. Tested at 12 epochs (6.4% [3.4-11.7]) and 18 epochs (2.1% [0.7-6.1], reproducing this project's independently-known "18 epochs overfits" finding rather than showing anything new) — both statistically indistinguishable from v1's flawed 5.0%, i.e. fixing the flicker bug didn't recover the lost win rate. A weight-column-norm check and a synthetic sweep (moving armies between two different enemy seats, holding total strength constant) confirmed the network *had* learned coherent, non-degenerate per-seat identity — ruling out "the network isn't using the signal" as the explanation for the flat result.

**v3 — narrow "weakest enemy" scalars.** Different approach entirely: instead of full per-opponent identity, added just two global scalars, `WeakestEnemyArmyFraction`/`WeakestEnemyTerritoryFraction`, both describing whichever living rival has the fewest total armies (mirroring `internal/bot`'s `killTarget` — the actual signal killbot's elimination-hunting behavior runs on), with no per-territory width increase at all (+2 features vs. the seat scheme's +218). Motivation: killbot's `avg_elims` had been 5-9x higher than the value-function bot across every configuration tested all session, and this was the cheapest, most targeted way to test "does knowing who's weakest help" without the seat scheme's data-starvation risk. Result: catastrophic, not just flat — **0.0% [0.0-2.3]** (seed 0) and **0.6% [0.1-3.4]** (seed 1, an independent training draw confirming this wasn't a fluke), both against a **10.4% [6.6-16.0]** control (the *old* schema retrained on the exact same data) that ruled out "the data changed" as the explanation. Two independent seeds collapsing to near-zero against a clean, passing control is conclusive: this specific feature is actively harmful to training, not merely unhelpful.

**Takeaway**: three structurally different attempts (live rank, stable seat, narrow scalar) at the same underlying idea — give the value function some notion of *which* enemy is weak/strong, not just an aggregate — have now all failed controlled, isolated tests against the same killbot matchup. This looks like a real pattern specific to this training setup (TD(λ), plain SGD, current data volume), not bad luck on any single attempt. Recommendation: stop iterating on this specific direction; if individual-enemy awareness is revisited, it likely needs a fundamentally different mechanism (e.g. surfaced through search/action-selection rather than the value function's input features) or a substantially larger training-data regime, not another encoding variant.

**Original cost estimate** (kept for reference): the biggest item on this list by width — full per-opponent identity changes per-territory feature width (cascading through `node_feature_dim` on both sides) *and* global feature width. The narrow v3 variant showed width alone wasn't the fix; even the cheapest possible version of this idea failed.

### 2. Combat-realism via the existing Attack Handler — tried, reverted, collapsed like §4

**Status: reverted.** The current `EnemyThreatFraction` is a raw army sum — it treats "3 armies next door" as a fixed threat level regardless of how many armies *I* have to respond with, when Risk's actual dice math is highly nonlinear in the army ratio (a 2-army attacker vs. a 1-army defender is a very different proposition than 20 vs. 10, despite an identical 2:1 ratio). This project already had exactly the machinery to compute real combat odds — `ForecastAttack(attackerArmies, defenderArmies) → WinProbability` — built for search, never used as a *feature* until this attempt.

**What was built**: one additive per-territory field, `StrongestAdjacentEnemyWinProbability` — `ForecastAttack(ts.Armies, strongestAdjacentEnemyArmies).WinProbability`, "if I attacked my toughest adjacent neighbor right now, what's my real win probability." `EnemyThreatFraction` was left untouched.

**A real, separate discovery along the way, kept regardless of the feature's fate**: `combat_odds.go`/`attack_handler.go` were relocated from `internal/bot` to `internal/risk` (they were fully self-contained — only imported stdlib `sort` — and `internal/bot` already imports `internal/tdstate` in production code today, so the reverse dependency this feature needed would have been a real, immediate cycle, not the "future risk" the old doc comment claimed). Wiring the new feature into `Encode` — which is called deep inside the Attack Handler search's hot path — surfaced a genuine performance regression (a stress test went from sub-second to a 10+ minute timeout). Root-caused and fixed at the source, independent of whether the tdstate feature survived: `ForecastAttack`'s internal memoization was rewritten from a map to a flat single-allocation slice (map allocation/hashing was the dominant cost, not state-space size — verified empirically, not guessed), and a decision-wide forecast cache already built for the search (`internal/bot`'s `forecastCache`) is now available to any `tdstate.Encode` caller that has one, via `EncodeWithForecast` — except that plumbing was itself reverted alongside the feature (see below), since nothing else needed it once the feature was gone. The relocation and the `ForecastAttack` algorithmic fix stayed; only the tdstate-specific wiring was undone.

**Result**: tested exactly like every other candidate this session — pure model, no search, same killbot matchup, fresh data. **0.0% [0.0-2.2]**, `avg_captures` 37.00 (healthy baseline is ~106-127). Rigorously verified this wasn't a correctness bug in the `ForecastAttack` rewrite before accepting the result: hand-computed values, monotonicity in both directions, probability bounds, and extreme cases all checked out across hundreds of test cases. This is a real result, not an artifact.

**The pattern, now replicated three times**: `StrongestAdjacentEnemyWinProbability` (tactical: real attack odds) joins §4's `ConnectivityFraction` (defensibility) and §1's weakest-enemy scalars (targeting) as a third, structurally unrelated *tactical/action-adjacent* feature that collapsed training to near-zero. Meanwhile the two *positional/descriptive* features tried — §1's seat identity and §3's continent-territory-fraction — never collapsed (mediocre and neutral, respectively, but never catastrophic). Three-for-three on tactical features failing, two-for-two on positional features surviving, across five attempts total, is a real split worth taking seriously, not a coincidence.

**Decision**: paused further value-function feature-expansion work at this point (§5-8 not attempted) to return to the search/action-selection side instead — see `Search_Integration_Roadmap_with_References.md`. If feature-expansion work resumes later, card-set-completeness (§6) and turn-order/initiative (§7) are the next lowest-risk candidates (both positional, matching the surviving category), but expectations should be calibrated by continent-territory-fraction's neutral (not positive) result: even the safe category hasn't yet produced an actual win, only "didn't hurt."

### 3. Continent-completion progress, not just army share — tried, kept out (not proven to help)

**Status: reverted, not adopted.** Built as `ContinentTerritoryFraction[]` (mine-owned-territories-in-continent / total-territories-in-continent, parallel to `ContinentArmyFraction`) and tested in isolation (pure model, no search, same killbot matchup as every other evaluation): **9.4% [5.7-15.2]**, statistically indistinguishable from — and if anything the lowest point estimate among — the repeatedly-reconfirmed clean baseline (15.9%, 10.4%, 12.6% across three separate runs this session). Not harmful, but not shown to help either.

Reverted rather than kept "since it's harmless": every kept-but-unproven feature muddies the baseline for future comparisons (the next feature test would need to account for whether it was measured with or without this one, instead of comparing cleanly against the single well-established reference point), and this project's standard everywhere else has been "did it help," not "did it fail to hurt."

### 4. Structural/board-topology features (chokepoints) — tried, confirmed actively harmful

**Status: reverted after being confirmed as the cause of a real collapse.** Built as `ConnectivityFraction` (`len(board.Adjacent[t])` normalized by the board's own max degree, kept uninverted — higher means more neighbors/more exposed, not "better chokepoint," to avoid an inversion-arithmetic bug and match every other feature in the file being a raw magnitude rather than a pre-editorialized judgment).

First tested bundled with §3's `ContinentTerritoryFraction` (both cheap, static, seemingly orthogonal — a deliberate but, in hindsight, costly exception to one-variable-at-a-time): **0.0% [0.0-2.1]**, `avg_captures` cratered to 44.50 against a healthy ~106-127 baseline range. A pipeline-regression control (old schema, fresh through the same current code) came back healthy (12.6% [8.3-18.6]), ruling out a shared bug. Split-testing then isolated each feature alone: `ContinentTerritoryFraction` alone was healthy (§3's 9.4% above, confirming it wasn't the cause), while **`ConnectivityFraction` alone reproduced the collapse just as badly — 0.0% [0.0-2.1], `avg_captures` 32.55, the lowest of any run this session.** Clean, decisive isolation: this specific feature is the culprit, not an interaction between the two.

No confirmed mechanism for *why* — a "static per-node fingerprint undermining the GCN's shared-weight generalization" theory was floated but doesn't fully hold up, since `Continent` one-hot and `IsContinentBorder` are *already* static per-territory facts sitting in the healthy baseline the whole time. What's confirmed is *that* it breaks training, not the underlying cause.

**Takeaway**: this is now the second feature-expansion attempt (after §1's individual-enemy-differentiation saga) to produce a real, reproducible collapse rather than just a null result — both `ConnectivityFraction` here and the weakest-enemy scalars in §1 were narrow, cheap, seemingly low-risk additions. Width and cost estimates in this doc are not a reliable predictor of training stability; every new feature needs a real isolated evaluation before being trusted, regardless of how safe it looks on paper.

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

1. ~~**Cheapest, most self-contained first**: chokepoint/connectivity (§4) and continent-territory-fraction (§3)~~ — **done**. Bundling them turned out to be the wrong call empirically (see §4's writeup): the bundle collapsed to 0%, and only a split-test after the fact revealed `ConnectivityFraction` alone was the cause while `ContinentTerritoryFraction` was harmless. Both now reverted. Lesson for the remaining phases below: bundle only when truly willing to lose isolatability if it goes wrong, which in practice has meant "don't bundle."
2. ~~**Combat-realism** (§2)~~ — **done**. `ForecastAttack` relocated to `internal/risk` (kept — a genuine, verified architectural + performance fix, independent of the feature) and the new feature tested alone: 0.0% collapse, same signature as §4's. Reverted.
3. **Individual/ranked enemy differentiation** (§1) — already independently attempted and abandoned earlier (three variants, see §1's own writeup) before this phased list was followed strictly.
4. **Paused here.** §3 (continent-territory-fraction) and §1 already done too, out of order — by this point 3 of 5 attempted features had collapsed and the other 2 were neutral-to-mediocre. Rather than continuing to §6/§7 (card-set-completeness, turn-order/initiative) immediately, work paused to return to the search/action-selection side (`Search_Integration_Roadmap_with_References.md`). §6/§7 remain the next candidates if feature-expansion work resumes.
5. **Momentum/trend** (§5) and **territory volatility** (§8) — both need their own design resolution (where does history live, does trend belong in features at all) before they're even scoped enough to build. Parked, not scheduled.

## Evaluation methodology: pure model (depth=0) before search

Every feature added here should be evaluated **without search first** — the plain single-ply `ValueStrategy` (`AttackSearchDepth=0`), not the depth=2/3 search built in `Search_Integration_Roadmap_with_References.md`'s Phase 2. Reasons, not just habit:

- **Isolates the actual variable.** Search depth and feature quality are two separate levers; testing a new feature only with search enabled makes any win-rate change ambiguous between "the value function got better" and "search did more with a differently-shaped signal." This project already spent real effort untangling an analogous confusion once this session (training objective vs. margin calibration, before either was isolated).
- **Far cheaper.** A depth=0 decision costs ~70ms; depth=2 costs 400ms-several seconds; depth=3 games ran into hours. Getting a first read on whether a feature helps at all should not require the search-scale compute budget.
- **The sharpest test of this doc's actual motivating hypothesis.** §1 (ranked enemy differentiation) exists because the killbot dominance was diagnosed as a value-function blind spot, not a search-depth problem — depth=2 and depth=3 both left it untouched. The cleanest possible test of "did adding this fix it" is the *pure* model against the killbot matchup, with no search in the loop to confound attribution. A negative result there (no improvement even with the feature) is real, valuable information before spending more effort on search.
- **Matches this project's own established discipline** — every other lever (TD(λ) vs. supervised, margin percentile, epoch count) was validated in isolation before search was ever added to the picture; feature changes should follow the same order.

Search-enabled evaluation is still a valid, valuable *second* step once a feature is confirmed to help the pure model — features and search could still interact or compound — but it should never be the *first* signal a new feature is judged on.

## Open questions

- ~~Where should `ForecastAttack`/combat-forecast logic actually live...~~ — resolved: `internal/risk`, both `internal/bot` and `internal/tdstate` already depend on it.
- Is momentum/trend information genuinely additive to what TD(λ)'s own bootstrapping already captures, or would it fight the training objective by giving the network a shortcut that doesn't generalize as well?
- For territory volatility (§8): does this belong as real `risk.Game` state (available to live play, not just offline training) or as a training-pipeline-only reconstruction?
- Should features be added and evaluated one at a time (maximally isolatable, slower overall) or in the small bundles proposed above (faster, but a positive result wouldn't say which specific feature drove it)?

## Status

**Paused.** Five feature attempts across §1, §2, §3, §4: three collapsed training to near-zero (individual-enemy differentiation's weakest-enemy variant, chokepoint/connectivity, combat-realism win-probability — all *tactical/action-adjacent* signals), two didn't (seat identity: mediocre; continent-territory-fraction: neutral — both *positional/descriptive* signals, and neither was an actual win). Every collapse was independently verified as a real result, not a bug — including, for combat-realism, a rigorous correctness check of a nontrivial rewrite to shared combat-forecasting code (`internal/risk/combat_odds.go`, kept regardless of the feature's fate) before accepting the training outcome.

Given a 3-collapse, 0-win record, work has paused here rather than continuing to §6/§7 (the two remaining low-risk, positional-category candidates) or attempting §5/§8. Returning to the search/action-selection side instead (`Search_Integration_Roadmap_with_References.md`) — the `EnemyThreatFraction`/`ArmyFraction`/`Continent`-style baseline this project keeps returning to may be closer to a local optimum for this specific training setup (TD(λ), plain SGD, current data volume) than the motivating hypothesis ("the paper's minimal feature set left real signal on the table") assumed, and the tactical-feature collapses in particular suggest the missing piece may be in how the bot *acts* on board judgment, not what it's shown.
