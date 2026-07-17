# Learned Board Evaluation — Plan

> Companion doc: `10_Bot_Weight_Tuning.md` (logistic regression over the
> existing hand-picked feature set). This doc's "Comparing the two
> approaches" section covers how the two get evaluated against each other.

## Context

`08_Machine_Learning_Roadmap.md`'s stated progression is "1. Logistic
regression 2. Gradient boosted trees 3. Learned board evaluation" — the
first two steps are refinements of the *same* thing `scored-v1` already
does (a weighted combination of 21 hand-designed features). Learned board
evaluation is a different kind of step: instead of scoring a candidate
move through features a human decided mattered (army advantage, capture
probability, continent completion, ...), a model learns directly from raw
board state what predicts winning, without that hand-designed
intermediate layer. The question this doc's experiment answers: does the
hand-designed feature set in `bot.Weights` already capture what matters,
or is there real signal in the raw state a human wouldn't think to encode?

This is **not** Monte Carlo / forward-search — no tree search, no rollout
policy, no look-ahead beyond one move. It's a different *scoring function*
for the same "evaluate each legal candidate, pick the best" loop
`ScoredStrategy` already runs. Keeping that shape is deliberate: it means
this can be dropped into the exact same `bot.Strategy` interface and
evaluated through the exact same tournament infrastructure as everything
else in this project, with no new comparison machinery needed.

**Explicitly out of scope for this first round**: a graph neural network
respecting the board's actual adjacency structure, an ONNX (or similar)
model-serving runtime in Go, real look-ahead/tree search, self-play
reinforcement learning (policy improvement from the model's own play, not
just supervised fitting on existing self-play games). Look-ahead/RL are
deferred because a much simpler version should show this general
direction has legs first. The GNN is deferred for a *different*,
narrower reason — see the next section — not because it's generically
"fancier than needed."

## Known future constraint: customizable/random maps

The project's actual plan is to eventually support customizable/random
maps, not just `risk.ClassicBoard()`. This changes the state-representation
decision below in a real way, so it's worth stating up front rather than
discovering it mid-implementation.

**The good news, confirmed by checking the actual code**: `internal/risk`
and `internal/bot`'s scoring logic are already fully board-agnostic — a
grep for hardcoded territory/continent name literals outside `board.go`
turns up nothing; combat, adjacency, continent-bonus checks, and every
scored-strategy feature already read generically off
`g.Board.Adjacent`/`g.Board.Continents`/`g.Board.Order` as data, not
baked-in assumptions. The only hardcoding is at the constructor layer —
`NewClassicGame`/`NewClassicAutoStartGame`/`NewClassicRandomTerritoryGame`
(`internal/risk/engine.go:246`) call `ClassicBoard()` internally instead
of accepting a `Board` parameter. Supporting a different board at the
engine/bot-feature level is a small, mechanical change (parameterize the
constructors, thread a `Board` through `internal/simulation.Config` too,
since that layer also currently hardcodes the "Classic" constructors),
not an architectural one.

**The part that isn't free**: this doc's flat, fixed-size state vector
(next section) is keyed to specific territory identity/position (e.g.
"index 3 is always Alaska"). That's a valid design *only* as long as
every game uses the same board — true today, since `ClassicBoard()` is
static and only ownership/turn order are randomized per seed. It stops
being valid the moment a game can hand the model a genuinely different
board (different territory set, different adjacency). At that point a
graph-based encoding stops being a "nicer, more principled" option and
becomes close to mandatory — the classic justification for a GNN
(generalizing across *varying* graph structure, needing permutation
invariance because there's no fixed canonical node ordering) doesn't
apply to one fixed board, but applies directly once boards vary.

**Sequencing implication**: still worth building the flat-vector round
first — it's cheap and still answers the real near-term question (does
raw state beat hand-picked features *at all*, on the one board that
exists today), and doc 10's hand-designed-feature approach, by contrast,
should carry over to new boards with little or no rework for free (its
features are already computed generically off `g.Board`, not positional
indices). But treat the flat-vector model itself as **disposable** once
custom maps ship, not as a foundation to build the GNN version on top
of — budget for a from-scratch graph-based redesign at that point rather
than an incremental migration.

## Key design decision: score the resulting state, not the turn-start state

`ScoredStrategy` scores *candidates* — it needs a value per legal command,
not one value per turn. So the model must evaluate what the board looks
like **after** a candidate move, not just "how good is my position right
now." This is the one point in this doc where a real design choice was
made rather than following an obvious default, so it's worth stating
explicitly: turn-start-state evaluation would be simpler to build but
wouldn't function as a move scorer at all (every candidate on the same
turn would get an identical score) — it was rejected for exactly that
reason.

## Computing the resulting state without touching `internal/risk`

Earlier scoping work on this project (the simulation framework's design)
flagged that Monte Carlo is blocked because `risk.Game` has no exported
clone method and its RNG field is unexported — true for that use case
(actually continuing to *play out* rollouts with independent randomness).
It turns out **not** to block this doc's narrower need, because
"resulting state after one candidate move" only requires *applying* one
move, not continuing a random simulation from it:

- **Deterministic actions** (`reinforce`, `fortify`, `occupy`,
  `trade_cards`, `end_turn`, `end_attack`) have no randomness at all.
  `risk.Game`'s fields are fully exported (`Territories`,
  `Players`, `SetupReserves`, etc. — confirmed in `internal/risk/engine.go`),
  so a caller outside the package can construct a deep-enough manual copy
  (the `Territories` map, `Players` slice, and `SetupReserves` map all
  need explicit copying — a naive `g2 := *g` struct copy would still alias
  the originals through those reference types) and run the *existing*
  `internal/simulation.Dispatch` against the copy to get an exact
  resulting state. No engine change needed — a new small
  `copyGameState(g *risk.Game) *risk.Game` helper, scoped to wherever this
  data-generation code lives, is enough.
- **`attack`** is the one action with real randomness (dice), so no single
  "resulting state" is exact. Rather than mutating a copy and rolling real
  dice (which would need *some* RNG on a copy with a zero-value unexported
  `rng` field — workable, since `ensureRNG()` self-heals a nil RNG, but
  noisy: one dice-roll sample is a poor training signal), reuse the
  **already-built** `bot.ForecastAttack` (`internal/bot/combat_odds.go:43`)
  — the same probability-distribution combat forecaster `scored-v1`'s
  existing `CaptureProbability`/`ExpectedLossCost` features already use.
  Construct an *expected* resulting state (probability-weighted blend of
  "conquered" and "held, both sides at their expected remaining armies")
  instead of one stochastic sample. This is arguably a *better* training
  signal than a real dice roll would be — smoother, and reuses combat math
  already validated elsewhere in the codebase — not just a workaround.

## State representation

Start with a flat, fixed-size vector, not a graph representation. Per
territory (42, `risk.ClassicBoard()`): owning player (one-hot across up to
6 seats, or a simpler "is this seat's territory" boolean plus "is
contested/enemy" — decide from a first pass at feature importance) and
army count (raw or log-scaled — army counts can range widely, worth
checking empirically whether raw or log-scaled trains better). Plus, per
continent (6): a flat "does this player fully own it" boolean. That's
roughly 42 × (1 or 6 + 1) + 6 ≈ 90–260 input dimensions depending on the
owner encoding chosen — still small enough for logistic regression or a
tiny hand-portable network, not a scale that needs a heavy runtime.

**This representation is fixed-board-only** (see "Known future constraint"
above) — it hardcodes a canonical index per `ClassicBoard()` territory and
has no way to represent a different territory set or adjacency graph.
Fine today, since only one board exists; treat any code built against it
as disposable once custom maps are real, not as a base to extend.

A graph-based encoding (a GNN respecting the actual adjacency structure)
is deferred for now for two independent reasons: it needs a real
graph-learning library and Go-side model runtime that aren't justified
until the flat-vector version has shown there's signal worth chasing, and
— the bigger reason — its actual justification (generalizing across
varying graph structure) doesn't fully apply *yet*, only once maps
actually vary. Once they do, this stops being "the fancier option" and
becomes close to the only correct one.

## Training data generation (new Go work — unlike `10_Bot_Weight_Tuning.md`)

Doc 10's logistic-regression approach needed zero new Go code, because
`--trace decision` already records everything needed. This approach needs
new instrumentation: for every legal candidate at every decision point (not
just the chosen one), compute and record its resulting-state encoding —
`Explanation.Alternatives` today only records a runner-up's *score*, not
its resulting board state. This is real new work, not a reuse of existing
trace output:

- A small tool (likely a new `cmd/`, or a flag on `cmd/simulate` gated
  behind an opt-in, since this is meaningfully more expensive per decision
  than normal play — resulting-state computation for *every* legal
  candidate, not just the winner) that runs games and, at each decision,
  emits (resulting-state vector, phase, action type, did-this-player-win)
  rows for every candidate considered, not just the one chosen.
- Same coverage requirements as doc 10 (`auto_start` only — see that doc's
  Coverage section on why `manual` setup mode is out of scope — multiple
  player counts, enough seeds for real sample size) — same reasoning
  applies identically here.

## Model choice, deliberately conservative for round one

Start with **logistic regression on the raw state vector** — not a deep
network. This is the direct, fair comparison point against doc 10: same
training method (logistic regression), same self-play data source,
*different input representation* (hand-designed 21 features vs. raw board
state). That isolates the actual question this experiment is asking —
does representation matter — from "did a bigger model help," which is a
separate question for a later round. (See the earlier conversation this
doc's plan came out of: a deep network was considered and set aside for
this stage specifically because 21-to-~200-dimensional tabular input at
this data scale is exactly the regime where extra model capacity mostly
buys overfitting risk, not accuracy — well-established for tabular data
generally, not specific to this project.)

**Escalation path, only if round one shows promise:** a small
hand-portable MLP (1–2 hidden layers) next — still small enough to
hand-write a forward pass in plain Go (a couple dense-layer matrix-vector
multiplies plus an activation function, no new dependency), avoiding the
model-serving-runtime cost a real deep network would force. Gradient
boosted trees are also worth trying before a network at all, per the
roadmap's own ordering. A GNN or anything needing ONNX/a real runtime
stays out of scope until a simpler version has earned it.

## Go-side integration

Whichever model comes out of this, it still needs to implement
`bot.Strategy.NextCommand` to be evaluable through the existing tooling.
Keep this **out of `internal/bot`'s primary files** while it's
experimental — those are the production types `cmd/backend` registers for
real games. A new package (e.g. `internal/bot/experimental` or a
standalone `internal/learned`, mirroring how `internal/tournament` sits
alongside `internal/simulation` rather than inside it) holds the
resulting-state computation helper, the model-loading code, and a
`Strategy` adapter — registered into `cmd/tournament`'s strategy registry
the same way `10_Bot_Weight_Tuning.md`'s `--weights-variant` flag adds a
custom-weighted `ScoredStrategy`, just via a new flag pointing at this
package's model file instead.

## Comparing the two approaches

Both doc 10's fitted-`Weights` candidate and this doc's learned-evaluator
candidate end up as ordinary `bot.Strategy` implementations, registerable
under any string ID. That means the comparison needs **no new
infrastructure at all** — run them through `cmd/tournament` directly
against each other and against `basic-v1`/baseline `scored-v1`, using the
exact same evaluation methodology already established in
`10_Bot_Weight_Tuning.md` (paired configurations, ≥300–500 games,
coverage matrix, `GameWinRate` over `SeatWinRate`, failure-rate check).
Nothing about the methodology changes based on *how* a candidate's
decisions get made — that was deliberate when the tournament runner and
its `Aggregate` schema were designed (strategy IDs are opaque strings
throughout).

## Tasks

1. `copyGameState` helper — deep copy of the mutable-state fields needed
   to apply a deterministic action to a scratch `*risk.Game` without
   touching the original.
2. Resulting-state encoder — flat vector per the representation above,
   built from a (possibly copied/forecasted) `*risk.Game`.
3. Attack resulting-state via `bot.ForecastAttack`'s probability
   distribution (expected blend, not a stochastic sample).
4. Training-data-generation tool: per-candidate resulting-state rows
   (not just the chosen candidate), labeled with the eventual game
   outcome, at the same coverage as doc 10's data generation.
5. Python-side: fit logistic regression on the raw state vector
   (`analytics/`, reusing the `scikit-learn` dependency doc 10 already
   adds).
6. `internal/bot/experimental` (or `internal/learned`): resulting-state
   computation, model loading, `bot.Strategy` adapter.
7. `cmd/tournament` flag to register the learned strategy, mirroring
   `--weights-variant`.
8. Head-to-head evaluation against doc 10's candidate and both baselines,
   using the shared methodology.

**Trigger for a follow-up doc, not part of this round:** once customizable/
random maps are real, redesign the state representation as a graph-based
encoding (task 2's flat-vector encoder gets replaced, not extended) and
parameterize `internal/risk`'s `NewClassic*Game` constructors plus
`internal/simulation.Config` to accept a `Board` instead of hardcoding
`ClassicBoard()` — the prerequisite for generating self-play data across
varied maps at all. Worth a dedicated doc of its own at that point rather
than folding into this one.

## Verification

Same bar as doc 10: `go build ./... && go vet ./... && go test ./...`
(`-race -count=1`), plus the tournament A/B evidence (raw JSONL +
aggregate, both head-to-head runs) kept as the record of which
representation actually won and by how much.
