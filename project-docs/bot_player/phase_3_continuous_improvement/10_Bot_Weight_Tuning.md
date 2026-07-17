# Bot Weight Fitting — Logistic Regression

> Companion doc: `11_Learned_Board_Evaluation.md` — a second, independent
> approach evaluated head-to-head against this one via the same tournament
> infrastructure. Read that doc's "Comparing the two approaches" section
> for how the two fit together.

## Context

Every phase `scored-v1` plays (`attack`, `reinforce`/`setup_reinforce`,
`occupy`, `fortify`, card timing) is already on the candidate-scoring
pipeline — that migration is complete (`internal/bot/strategy_scored*.go`).
What's never happened is validating or tuning the actual numbers: `bot.Weights`
(`internal/bot/weights.go`) is 21 hand-picked `float64` coefficients,
assigned once as `bot.DefaultWeights` and never empirically tested against
an alternative. Every `scored-v1` vs `basic-v1` win rate quoted so far in
this project came from small, ad-hoc tournament runs (tens of games) done
while building the tooling itself — not enough to trust as a real result.

This doc originally proposed hand-tuning those 21 numbers by manual
coordinate ascent (pick one weight, change it, run a paired A/B, repeat).
That's abandoned as the primary method: it's slow, doesn't scale past a
couple of variables at a time, and — the decisive point — the data needed
to *fit* the weights statistically instead of guessing them is nearly free
to produce with what's already built. Manual tuning is kept only as a
cheap fallback for a single targeted hypothesis (see "Manual fallback"
below), not the main path.

**Explicitly out of scope**: Monte Carlo / forward-search move evaluation
(a different, blocked initiative — see `11_Learned_Board_Evaluation.md`'s
notes on why it turns out *not* to be a hard blocker for that doc's
purposes, but is still out of scope here), Discord personality
(`09_Personality_and_Discord.md`), any change to `internal/risk` engine
rules, any frontend change.

**A quiet advantage worth noting**: this project plans to eventually
support customizable/random maps, not just `risk.ClassicBoard()` (see
`11_Learned_Board_Evaluation.md`'s "Known future constraint" section for
the full reasoning). The 21 features fitted here are already computed
generically off `g.Board.Adjacent`/`g.Board.Continents`/`g.Board.Order`
(confirmed — no hardcoded territory/continent names anywhere in
`internal/bot`'s scoring code), not positional indices tied to one
specific board layout. A fitted `Weights` result from this doc should
carry over to a new map with little or no rework, unlike doc 11's
first-round state representation, which is fixed-board-only by
construction and gets thrown away once maps vary.

## The approach

`ScoredStrategy` already computes, for every legal candidate command, a
named breakdown of weighted feature contributions
(`bot.Explanation.Features`, `internal/bot/scoring.go:13`) — the same
structure `--trace decision` already serializes to JSON. Two things turn
that into a labeled training set:

1. **Recovering the raw signal.** `Feature.Value` is the *weighted*
   contribution (`weight × raw_signal`), not the raw signal itself — but
   the weight is a known constant (`bot.DefaultWeights`'s current value
   for that named feature), so `raw_signal = Value / weight` recovers it
   exactly (every weight in `DefaultWeights` is non-zero except the two
   `EndPhaseBias`/`FortifyEndTurnBias` flat terms, which aren't
   per-signal features to begin with and can be dropped from the
   regression input).
2. **Labeling with the outcome.** Each decision entry already carries
   `PlayerID`; cross-referencing that against the game's final
   `Result.WinnerPlayerID` gives a binary win/loss label for every
   decision made in a completed game.

The result: for every legal candidate a `scored-v1` bot considered, a
(21-dimensional raw feature vector, did-this-player-win label) pair.
Logistic regression over that directly refits the 21 weights as
statistically-fitted coefficients instead of hand-picked constants — same
shape, same deployment (`bot.Weights`), same `ScoredStrategy` architecture,
zero new Go runtime dependency.

## A real gap: where the training data actually comes from

`cmd/tournament` is **not** the source for this. `internal/tournament.Run`
deliberately forces every game's `Trace` to `simulation.TraceNone` (see its
doc comment) — a tournament only needs `Result`s, and keeping per-decision
traces for hundreds of games would be pure waste for its actual job
(aggregate win-rate comparison). Its raw JSONL output (`--raw-output`) has
no `Explanation` data in it at all.

`cmd/simulate --trace decision --format json` *does* produce exactly the
needed per-decision `Explanation` data — but only for one game at a time,
sequentially, no built-in bulk collection. Generating a real training set
means running it across many seeds externally (a shell loop, `xargs -P` /
GNU parallel for concurrency, or a small Python driver script in
`analytics/`) and parsing each resulting JSON file's `decisions` array
plus its `result.WinnerPlayerID`. No Go changes are needed for this half —
it's a data-collection script, not new application code.

## Tasks

**Shared infrastructure (needed regardless of where the candidate weights
come from):**

1. `LoadWeights(path string) (bot.Weights, error)` — small JSON-unmarshal
   helper, `internal/bot/weights.go` or a new `weights_io.go`. Base value
   is `DefaultWeights`, so a variant file only needs to override the
   fields it changes. (`bot.Weights`' fields are all exported `float64`s
   with no JSON tags, so `encoding/json` already round-trips it correctly
   with zero struct changes.)
2. `cmd/tournament`: `--weights-variant <strategy-id>=<path>` flag
   (repeatable), loading each file and registering
   `bot.NewScoredStrategy(w)` under the given ID alongside the two
   built-ins, for evaluating a candidate against baseline. No changes
   needed in `internal/tournament`/`internal/simulation`.

**Logistic-regression-specific:**

3. Data-generation script: loop `cmd/simulate --trace decision --format json`
   across a wide seed range (and several player counts — same coverage
   reasoning as the evaluation matrix below; `auto_start` only, see the
   game-mode note under Coverage), parse each
   run's `decisions[].Explanation.Features` (inverted to raw signal) and
   `result.WinnerPlayerID`, write a flat table (pandas DataFrame /
   Parquet, matching `analytics/`'s existing raw/processed convention) of
   (21 raw features, phase, win label) rows.
4. Add `scikit-learn` to `analytics/pyproject.toml` (not currently a
   dependency — only `pandas` is). Fit one logistic regression per phase
   (attack/reinforce/occupy/fortify features aren't all active on the same
   candidates, so a single global model would be fitting mostly-zero
   columns for the phases that don't apply — four smaller, cleaner fits
   instead) or one combined model if the phase-separation turns out not to
   matter empirically; either is fine, decide from the data.
5. Export fitted coefficients as a `bot.Weights`-shaped JSON file, load via
   task 1's helper, evaluate via task 2's flag and the evaluation
   methodology below.

**Manual fallback (cheap, single-hypothesis only):** if a specific,
concrete behavioral problem is worth targeting directly rather than
waiting on a full regression pass — e.g. the mirror-match convergence
issue below — hand-edit one or two weights, skip straight to the
evaluation methodology. Don't use this for general-purpose tuning; it's
what logistic regression is replacing as the default method.

## Evaluation methodology

*(Unchanged from this doc's original version — applies identically to a
logistic-regression-fitted candidate, a hand-edited one, or eventually a
promoted result from `11_Learned_Board_Evaluation.md`; this section is the
shared contract every candidate weight set is judged against.)*

**Open question to settle first: does seat position bias the result?**
`Config.Strategies` fixes which seat index plays which strategy for every
game in one tournament run, but `risk.NewClassicAutoStartGame`/
`NewClassicRandomTerritoryGame` reshuffle both turn order and territory
distribution internally per seed (`SeatPlayerID(i)` is assigned *before*
that shuffle). Confirm empirically rather than assuming: run
`basic-v1,basic-v1,basic-v1` (a true mirror) for a few hundred games and
check each seat's `GameWinRate` is close to 1/3. **Do this once, before
trusting any A/B result.**

**Matchup shape.** `risk` requires 3–6 players, so a pure 1v1 isn't
possible. Run each candidate through *two* configurations and require it
to win in both:

- `candidate,baseline,baseline` (candidate outnumbered 1-vs-2)
- `candidate,candidate,baseline` (candidate favored 2-vs-1)

Comparing `GameWinRate` (not `SeatWinRate`) across both cancels out most of
the seat-count asymmetry a single configuration would introduce.

**Sample size.** Target **at least 300–500 completed games per
configuration** — the standard error of a win-rate estimate at `n=200` is
still ~3.5 percentage points, enough to swallow a real but modest
improvement. Same spirit as `analytics/`'s existing "Sample Size Warning."

**Coverage.** At least two player counts — a candidate shouldn't be
accidentally overfit to one narrow setup (this matters even more for a
regression-fitted candidate than a hand-tuned one, since the training data
itself needs this same coverage to avoid learning a setup-specific
pattern). **`--game-mode manual` is out of scope** for this dataset and
every evaluation below — the user is likely removing `manual` setup mode
from the game entirely (poor gameplay experience; `auto_start` works
better and is the only mode real players ever use). Generate and evaluate
against `auto_start` only; don't spend compute covering a mode that won't
ship.

**Failure/convergence rate matters as much as win rate.** Check
`Aggregate.FailedGames`/`Failures` alongside `GameWinRate`. The mirror-match
convergence problem (`cmd/simulate/README.md`'s "A note on convergence" —
roughly half of random seeds for a 6-player all-`scored-v1` mirror don't
converge within a few seconds today) is a concrete, valuable thing for
either method to fix, independent of win-rate.

## Promotion criteria

A candidate replaces `bot.DefaultWeights` only when, across the full
coverage matrix (≥300–500 games per configuration, `auto_start` only,
multiple player counts):

1. `GameWinRate` beats baseline in **both** paired configurations.
2. `FailedGames`/stalemate rate is no worse than baseline's.
3. No regression in the existing hand-verified behavior tests in
   `internal/bot/strategy_scored*_test.go`.

## Verification

Each promoted change: `go build ./... && go vet ./... && go test ./...`
(matching `make test`'s `-race -count=1`), plus the tournament-based A/B
evidence itself (raw JSONL + aggregate) kept as the record of *why* the
change was made.
