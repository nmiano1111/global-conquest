# Porting Lux Delux personas to Go: adaptation notes

Context: `Lux_Delux_AI_Research_Notes.md` recommends reimplementing the six
Lux Delux AIs the GCN/TD(λ) paper trained against (Angry, Pixie, Cluster,
Quo, Killbot, Boscoe) as native `bot.Strategy` implementations, for training
data diversity. `AngryStrategy` (`internal/bot/strategy_angry.go`) is the
first port. Lux's Java source (`SillysoftSDK.zip` at the repo root, package
`com.sillysoft.lux.agent`) doesn't map onto this project's architecture
1:1 — this doc captures the adaptation decisions made once, so the
remaining five ports don't have to re-derive them from the Java source
each time.

## No territory-picking

Per `CLAUDE.md`, territories are always randomly distributed in this
project — no player, human or bot, ever chooses a claim. Every Lux agent's
`pickCountry()`/`pickCountryInContinent()` logic (continent selection for
initial claiming) is inapplicable and gets dropped entirely during a port.

## One command per call, not one phase per call

A Lux `attackPhase()`/`fortifyPhase()` method runs a whole internal loop of
`board.attack()`/`board.fortifyArmies()` calls before returning control to
the game. A Go `Strategy.NextCommand` returns exactly one `Command`; the
bot runner reloads authoritative state and calls again afterward. Lux's
`while (madeAttack) { ... }`-style outer loops don't need to be reproduced
explicitly: returning the single best legal action each call, and getting
re-invoked after the engine resolves it, already reproduces "keep attacking
until nothing qualifies" — see `AngryStrategy.attack`.

## Fortify is one move per turn, total

`risk.LegalFortifications` already enumerates every legal (from, to) pair
reachable through owned territory in a single call, and the engine allows
exactly one fortify action per turn (`Game.HasFortified`). Lux's multi-hop
`fortifyContinent`/`fortifyCluster` loops (which can move armies through
several territories in sequence within one `fortifyPhase()` call) collapse
to: rank the legal pairs by the persona's criterion, pick the best (or end
the turn if nothing qualifies) — the same shape `BasicStrategy.fortify` and
`AngryStrategy.fortify` both already use.

## No strategy-owned mutable state

`bot.StrategyRegistry` instances are shared across every concurrent game a
tournament run plays (`cmd/tournament/main.go` builds one registry;
`internal/simulation` runs games in a goroutine pool against it). Lux's
per-turn instance fields (`goalCont`, `moveInMemory`, `toKillPlayer`,
`ourConts`, `attackRoutes`, etc.) must become values recomputed fresh from
`*risk.Game` on every call, never fields on the `Strategy` struct itself —
matching every existing strategy in this codebase (`BasicStrategy`,
`ScoredStrategy`, `ValueStrategy`, `AngryStrategy` are all stateless
structs; `ValueStrategy.Observer` is the one sanctioned exception, and it's
a caller-supplied side-channel, not turn-scoped memory).

## No strategy-owned randomness

Every existing strategy is fully deterministic given a seed — only the
engine's own `RNG` introduces randomness, via dice rolls. Lux's occasional
random fallback (e.g. `Angry.fortifyPhase`'s "move to a random neighbor
when nothing else qualifies") is replaced with a deterministic tie-break —
canonical board order (`g.Board.Order`), the same convention
`BasicStrategy`/`ValueStrategy`/`AngryStrategy` already use throughout.
This keeps `--seed-start` reproducible: a strategy's own choices must be a
pure function of game state, not an independent source of variance.

## Placement isn't capped per command

`Game.PlaceReinforcement` only requires `armies <= PendingReinforcements` —
a strategy can legally place the whole turn's reinforcements on one
territory in a single command. Some Lux agents (Angry) dump everything on
one country in one `board.placeArmies` call, which ports directly. Others
(`ValueStrategy`, and likely `Pixie`/`Cluster`, which divide armies across
several contested fronts) genuinely need multiple `PlaceReinforcement`
commands across repeated `NextCommand` calls to spread reinforcements —
don't assume every persona wants the single-territory dump; check what the
source actually does with `numberOfArmies` before choosing.

## Naming convention

Registry IDs follow the existing `<name>-v1` pattern (`basic-v1`,
`scored-v1`, `angry-v1`), matching the Lux agent's own name lowercased.
Go type names are `<Name>Strategy` (`AngryStrategy`), constructors
`New<Name>Strategy()`, registry ID constants `Strategy<Name>V1`.

## Base-class inheritance chains

The paper's six personas aren't independent — Lux's class hierarchy means
several build directly on shared machinery, which should be ported in
dependency order rather than each from scratch:

- `Cluster` (extends `SmartAgentBase`) — introduces connected-component
  ("cluster") detection over owned territories and a four-stage attack
  sequence (easy-expand → fill-out → consolidate → split-up).
- `Shaft` (extends `Cluster`) → `Quo` (extends `Shaft`) — adds a
  forward-sweep border-collapse look-ahead and per-turn card cashing.
- `Yakool` (extends `Cluster`) → `Boscoe` (extends `Yakool`) — adds a
  "kill the dominant player" defensive trigger, restrained to a safer
  three-stage attack sequence.
- `Pixie`/`BetterPixie` (extend `SmartAgentBase` directly) — continent-
  economy placement/attack/fortify, independent of the `Cluster` lineage.
- `Vulture` (extends `SmartAgentBase`) → `Killbot` (extends `Vulture`,
  backed by a `BetterPixie` instance for cards/placement/fortify) —
  opportunistic elimination hunting layered on Pixie-style economy.

So the natural build order is roughly: **Cluster → Pixie → Quo → Boscoe →
Killbot**, each reusing the previous ports' shared helpers
(`internal/bot/geometry.go`) rather than re-deriving them.

## Addendum: findings from porting Cluster

- **A cheapest-route-to-continent search is needed starting with Cluster
  itself**, not first at Killbot as guessed above. `Cluster.placeArmies`
  falls through to `SmartAgentBase.placeArmiesToTakeCont` whenever the
  player owns no positive-bonus continent outright, and that method's own
  fallback (owning literally nothing in the target continent) calls
  `BoardHelper.cheapestRouteFromOwnerToCont` — a Dijkstra-style search from
  the continent's borders outward, weighted by enemy army counts, ported as
  `cheapestRouteToContinent` in `geometry.go`.
- **`goalCont` (set only by the initial-claim `pickCountry()` this project
  skips) gets substituted with a freshly recomputed "current root's
  continent"** wherever Lux code reads it (`Cluster.moveArmiesIn`) — the
  same root `attackPhase`/`placeArmies` already derive each call, not a
  stored field. This stays a pure function of state, consistent with "no
  strategy-owned mutable state."
- **`moveInMemory`/`memoryMoveArmiesInTest` don't port at all.** They were
  Lux's own shortcut letting one method (`attackPhase`) tell another
  (`moveArmiesIn`) what it had just decided, within a single call. This
  engine calls attack-selection and occupy-selection as separate,
  independently-reloaded `NextCommand` invocations, so there's no channel
  to carry that intent across — and there doesn't need to be, since Lux's
  own `goalCont`-based comparison (the fallback path once the shortcut
  doesn't apply) already produces the right answer unconditionally.
- **`obviousMoveArmiesInTest` (only fires for a territory with exactly one
  neighbor) is dead code on `risk.ClassicBoard()`** — every territory has
  at least two neighbors (Japan is the minimum). Not worth porting until
  this project supports a map where it could actually fire.
- **Lux's multi-source/multi-target compound attacks (`attackConsolidate`,
  `attackSplitUp`) gate on a combined-armies check computed once, upfront,
  then commit to attacking from every contributing source regardless of
  how the fight goes.** Since this engine reloads fresh state before every
  `NextCommand` call, the natural port re-validates that gate on every
  call instead of trusting a stale precondition — strictly more
  conservative than Lux (it can abandon a partly-executed multi-source
  attack if losses make it no longer profitable), never less capable.
- **A generic `<name>` prefix on package-level attack-stage functions
  (`attackEasyExpand`, `attackFillOut`, `attackConsolidate`, `attackSplitUp`)
  is reused directly by `attackAsMuchAsPossible`'s hogwild/stalemate
  broadening** — they take `(g, pi, root)` rather than being methods on
  `ClusterStrategy`, so any future persona built on top of Cluster's attack
  sequence (Quo, Boscoe) can call them directly instead of duplicating.

## Addendum: findings from porting Pixie

- **`SmartAgentBase` methods Lux's own subclasses inherit verbatim
  (`attackHogWild`/`attackStalemate`/`attackAsMuchAsPossible`) are directly
  reusable across personas once ported once.** `PixieStrategy.attack` calls
  `shouldGoHogWild`/`attackAsMuchAsPossible` straight from
  `strategy_cluster.go` with no duplication — confirms the earlier choice
  to make Cluster's attack-stage helpers package-level functions rather
  than `ClusterStrategy` methods was the right call. `attackForCard` (new
  this phase) is the same kind of shared `SmartAgentBase` method and is
  written the same way, ready for the next persona that inherits it.
- **`ourConts` (Lux: computed once per turn inside `placeArmies`, reused
  by `attackPhase`/`fortifyPhase` via the shared field) has no equivalent
  under "no strategy-owned mutable state."** Substituted with
  `pixieWantedContinents(g, pi)`, a pure function of current state using
  Lux's own `neededForCont` formula (`enemyArmiesInContinent -
  playerArmiesInContinent - playerArmiesAdjoiningContinent`) but with
  Lux's placement-time slack (`< (1/numContinents)*numberOfArmies`, since
  more armies are about to land) tightened to a flat `<= 0` — no
  "about-to-be-placed" figure exists outside the reinforce call itself.
  Called identically from reinforce, attack, and occupy, so a continent's
  "wanted" status can never disagree with itself within one turn the way
  it could if each phase derived it slightly differently.
- **`placeArmiesToTakeCont` is now two functions**: `placeToTakeContinent`
  (derives the target continent itself via `easiestContinentToTake`, what
  `ClusterStrategy` wants) and `placeToTakeSpecificContinent` (takes an
  already-chosen continent, what `PixieStrategy` wants once it's picked a
  specific wanted-but-needy continent to reinforce). Any future persona
  that needs to route reinforcements toward a continent it has already
  decided on, rather than re-deriving "the easiest one," should call the
  latter directly.
- **Lux's `armies/2` (a share of the attacking territory's own army
  count) doesn't translate directly** into this engine's `[MinMove,
  MaxMove]` legal occupation range. `PixieStrategy.occupy` substitutes the
  legal range's midpoint (`(MinMove+MaxMove)/2`) wherever Lux's
  `moveArmiesIn` falls through to `armies/2` — the natural analog given
  the different unit the "how many armies to move" decision is expressed
  in.

## Addendum: findings from porting Quo

- **A Lux subclass that overrides almost nothing (`Quo`/`Shaft` inherit
  `Cluster`'s `placeArmies`/`moveArmiesIn`/`fortifyPhase` untouched) ports
  as almost pure reuse of the parent's existing package-level functions**
  (`clusterRoot`, `clusterPlacementTerritory`, `bestFortifyDestination`) —
  `QuoStrategy` only needed one new attack stage (`sweepForward`) and one
  new card policy (`voluntaryCardTurnIn`). Worth checking, before writing
  a new persona's port, exactly which Lux methods it *doesn't* override —
  that's the reuse budget.
- **`ClusterStrategy.occupy`'s body is now `clusterOccupyDecision`, a
  package-level function** (`ClusterStrategy.occupy` is a thin wrapper),
  extracted the moment a second caller (`QuoStrategy.occupy`) needed the
  identical logic — same rationale as `bestFortifyDestination`'s earlier
  extraction from `AngryStrategy.fortify`.
- **`voluntaryCardTurnIn`** (trade the first legal set whenever one
  exists, no forced-only gate) is shared infrastructure now, not
  Quo-specific — Boscoe's Lux source has the identical
  `cashCardsIfPossible` override in its own `cardsPhase`, so the next
  phase should call this directly rather than re-deriving it.
- **A Lux dry-run pass that never calls `board.attack()` is a pure
  function of board state and ports directly, with no "for real" branch
  needed at all.** `Shaft.sweepForwardBorder`'s worthwhile-check
  (`forReal=false`) only mutates its own local `q`/`seen` bookkeeping —
  `sweepForward` recomputes that same fold-to-a-fixed-point analysis fresh
  from `*risk.Game` each call and, if it converges to a single remaining
  contact point, issues exactly one attack command. No memory needed
  across calls: the next `NextCommand` call re-runs the identical analysis
  against the post-attack board state, which naturally continues the
  sequence (or stops, if the situation no longer supports it) — this
  generalizes past Quo to any future persona with a similar "simulate,
  then possibly act" Lux algorithm, as long as the simulation itself
  never touches the engine.

## Addendum: findings from porting Boscoe

- **`mustKillPlayer` (Lux: set during `placeArmies`, read later the same
  turn during `attackPhase`) substituted the same way as `pixieWantedContinents`**
  substituted for `ourConts`: a pure function (`dominantPlayerToKill`)
  recomputed identically from both the placement and attack decision,
  rather than a turn-scoped field. `Game.reinforcementsFor` (needed for
  the "≥50% of income" leg of the dominance check) is unexported and
  can't be called cross-package, so `playerIncome` duplicates its exact
  formula in `geometry.go` — a second instance of the same "engine has
  the real rule but doesn't export it" situation as `AttackAction`'s own
  40%-attacker-loss forecasting.
- **`easyCostFromCountryToContinent`'s per-origin brute-force loop (Lux:
  try each owned territory via `ArmiesIterator`, first one whose path
  beats its own army count wins) generalizes cleanly onto the *existing*
  continent-border-outward Dijkstra** (`cheapestRouteSearch`, built for
  `cheapestRouteToContinent` back in the Cluster phase) by having it also
  track predecessors. The search already finds the globally cheapest
  route to *any* owned territory; reporting the last hop before reaching
  it gives the same "attack toward the continent" decision Lux's
  per-origin loop produces, generally at least as good a route (Lux's own
  origin order is arbitrary, not cost-ordered) — another instance of the
  "clean global search beats faithfully replicating an arbitrary-order
  per-origin loop" simplification first used for Cluster's own placement
  routing.
- **A discovered gap in two already-shipped strategies got backported
  here rather than left to drift further**: `ClusterStrategy.setupReinforce`
  and `QuoStrategy.reinforce`/`setupReinforce` never called
  `placeToTakeContinent` when the player owned no continent outright,
  despite both being designed to mirror `ClusterStrategy.reinforce`'s own
  branching exactly. Fixed via a new shared `clusterOrTakeContinentPlacement`
  (`strategy_cluster.go`) all three callers (Cluster, Quo, and Boscoe's own
  fallback placement) now use identically. Worth periodically checking
  newer personas' design intent against what older ones actually ended up
  doing, not just trusting an earlier plan's stated intent as
  self-verifying.
- **`attackSplitUp`'s trigger condition (`armies > 1.2 × combined enemy
  armies`) mathematically implies `armies` also exceeds *every individual*
  enemy's own armies** (since `1.2 × sum ≥ 1.2 × max > max ≥ any one
  term`). This means in any persona whose attack sequence includes
  `attackForCard` *after* the restrained cluster stages but *without*
  `attackSplitUp` (Boscoe's own case, deliberately excluding split-up per
  its Lux source), there is no reachable board state where split-up would
  have fired but `attackForCard` doesn't fire first — `attackForCard`'s
  broader "best ratio anywhere on the board" criterion always subsumes
  split-up's narrower "beats this cluster border's combined enemies"
  one. A test set out to prove "Boscoe never uses split-up" behaviorally
  was abandoned for this reason: the exclusion is real and correct (the
  stage list in `strategy_boscoe.go` simply omits `attackSplitUp`), but
  it isn't independently *observable* at runtime once `attackForCard` sits
  downstream of where split-up would have been — verify this kind of
  "stage never reached" design decision by reading the stage list, not by
  hunting for a fixture that can't exist.
- **Test fixtures that leave most of the board at its default owner
  remain a live hazard past dominance checks specifically.** Two
  fixture bugs surfaced while testing `dominantPlayerToKill`-adjacent
  code: (1) a "block every other path with high armies" fixture without
  the blanket applied let a long chain of cheap default-armies(1) hops
  undercut an intended single expensive hop in the route search — apply
  the blanket-high-armies technique (`strategy_boscoe_test.go`,
  `geometry_test.go`) whenever a specific path's cost matters; (2) an
  exact 50/50 split of the remaining board between two non-acting players
  accidentally gave one of them *exactly* 50% territories, crossing
  `dominantPlayerToKill`'s `>=` threshold and silently swapping in the
  kill-branch where a normal-cluster-fallback test was intended — keep
  any such split strictly under 50% per player (e.g. 20/20 of 42, not
  21/21) when a test's premise depends on nobody being dominant.

## Addendum: findings from porting Killbot

- **`Vulture`/`Killbot`'s `backer` field (a whole other `LuxAgent`
  instance placement/occupy/fortify/cards delegate straight to) ports as
  literal delegation to an actual `*PixieStrategy` instance held as a
  struct field**, not a reimplementation — `NewKillbotStrategy` builds
  `&KillbotStrategy{backer: NewPixieStrategy()}` and `occupy` just calls
  `k.backer.occupy(g, playerID)`. This is the closest any port in this
  project has come to a 1:1 structural match with Lux's own object
  composition, and it works because `PixieStrategy` (like every strategy
  here) is a stateless struct — holding an instance costs nothing and
  needs no lifecycle management.
- **`BetterPixie` (Killbot's actual Lux backer) was never separately
  ported** — the Phase-3 decision to scope `BetterPixie` out ("plain
  Pixie is the persona the paper's roster actually names") meant
  Killbot's fallback behavior reuses `PixieStrategy`'s already-shipped
  logic (`pixiePlacementTerritory`, its `occupy` method, `attackInContinent`,
  `attackForCard`) rather than a `BetterPixieStrategy` that doesn't exist.
  If `BetterPixie` is ever ported for its own sake, `KillbotStrategy`'s
  `backer` field is the one place that would need to switch over.
- **`cheapestRouteSearch`'s starting set is now a parameter
  (`starts []risk.Territory`), not always `continentBorders(cont)`** —
  generalized the moment a second, structurally different caller
  (`cheapestAttackHopToPlayer`, searching from a *specific player's*
  owned territories rather than a continent's border) needed the
  identical Dijkstra. The three continent-scoped wrappers now pass
  `continentBorders(g, cont)` explicitly; `cheapestAttackHopToPlayer`
  passes `ownedTerritories(g, target)`. Same "generalize on the second
  caller, not preemptively" pattern as `placeToTakeContinent` →
  `placeToTakeSpecificContinent` and `ClusterStrategy.occupy` →
  `clusterOccupyDecision`.
- **`nextTradeValue` is `risk.Game`'s third unexported-but-needed formula
  duplicated into `geometry.go`** (after `reinforcementsFor` for
  `playerIncome` and — implicitly — the attacker-loss math `AttackAction`
  already exposes via `ForecastAttack`). `killTarget` needs every
  player's next-card-set value to discount their effective armies, and
  the engine only computes it internally during `TradeCards`.
- **This closes out the six-persona Lux Delux rollout** the paper's
  training data used: Angry, Cluster, Pixie, Quo, Boscoe, Killbot are all
  now shipped (`angry-v1` through `killbot-v1`), each documented here with
  its own adaptation decisions. `cmd/tdtraindata`'s hardcoded two-strategy
  registry (`basic-v1`, `scored-v1`) still doesn't draw on this roster —
  wiring the full diversified set into training-data generation, per the
  research notes' original motivation (avoiding self-play "policy
  monoculture"), is the natural next step once this port is confirmed
  stable in live tournament play.
