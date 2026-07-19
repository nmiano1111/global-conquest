# tournament

Runs many headless, reproducible bot-vs-bot Global Conquest games in
parallel for one fixed strategy matchup, sweeping a range of seeds, and
reports an aggregated summary — win rate per strategy, average turns/
commands, failure counts. Optionally dumps every individual game's raw
result as JSONL for external analysis (e.g. the `analytics/` Python
project's pandas-based tooling).

Built on [`cmd/simulate`](../simulate/README.md)'s
`internal/simulation.Simulator.RunOne` — see that README for what a single
game run looks like, strategy IDs, and game modes. This binary runs many of
those in parallel and never keeps a decision/milestone trace per game (see
[Trace level](#trace-level) below).

## Quick start

Run from the `backend/` directory:

```bash
go run ./cmd/tournament --strategies basic-v1,scored-v1,scored-v1 --games 100
```

## Flags

| Flag | Required | Default | Description |
|---|---|---|---|
| `--strategies` | yes | — | Comma-separated strategy ID per seat, e.g. `basic-v1,scored-v1,scored-v1`. Fixed for every game in the tournament. Available IDs: `basic-v1`, `scored-v1`, `angry-v1`, `cluster-v1`, `pixie-v1` (Lux Delux-inspired personas — see [`project-docs/bot_player/proposals/Lux_Delux_AI_Research_Notes.md`](../../../project-docs/bot_player/proposals/Lux_Delux_AI_Research_Notes.md)). |
| `--games` | yes | — | How many games to run. |
| `--seed-start` | no | `1` | Seeds used are `seed-start .. seed-start+games-1`. Same `seed-start` + `games` + `strategies` always reproduces the same batch of games. |
| `--parallel` | no | number of CPUs | How many games run concurrently. |
| `--game-mode` | no | `auto_start` | `auto_start` or `manual` — same as `cmd/simulate`. |
| `--max-turns` | no | 2000 | Override the per-game turn safety limit. |
| `--max-commands` | no | 20000 | Override the per-game command safety limit. |
| `--format` | no | `text` | Aggregate output format: `text` or `json`. |
| `--output` | no | stdout | Write the aggregate summary to this file instead of stdout. |
| `--raw-output` | no | (none) | If set, path to write one JSON-encoded `simulation.Result` per line (JSONL) as each game completes. Omitted = aggregate only, no raw dump. |
| `--config` | no | (none) | Path to a JSON file running several tournaments concurrently instead of one — see [Batch mode](#batch-mode---config) below. Mutually exclusive with every flag above except `--format`/`--output`, which apply to the combined batch output instead. |
| `--weights-variant` | no | (none) | `<strategy-id>=<path>`, repeatable — register a custom-weighted `scored-v1` variant loaded from a JSON file (see [`internal/bot.LoadWeights`](../../internal/bot/weights_io.go)) under its own strategy ID, usable anywhere a strategy ID appears (`--strategies`, or a `--config` entry's `strategies`). Compatible with both modes above, not mutually exclusive with either. |

## Examples

```bash
# 100-game sweep, aggregate to stdout only
go run ./cmd/tournament --strategies basic-v1,scored-v1,scored-v1 --games 100

# Same sweep, also dump every raw result for analysis in pandas
go run ./cmd/tournament --strategies basic-v1,scored-v1,scored-v1 --games 100 \
  --raw-output /tmp/tournament.jsonl

# Aggregate as JSON, saved to a file
go run ./cmd/tournament --strategies scored-v1,scored-v1,scored-v1 --games 50 \
  --format json --output /tmp/aggregate.json

# Reproduce a specific 20-game batch (same seed-start + games + strategies
# always plays the same 20 games)
go run ./cmd/tournament --strategies basic-v1,basic-v1,scored-v1 --games 20 --seed-start 500
```

## Output

**Text** (default) — a header line, a `failures:` breakdown (only printed
when at least one game didn't complete), and a per-strategy table:

```
tournament: 100 games (98 completed, 2 failed) · seeds 1-100 · avg 79.5 turns, 1861.3 commands · 39.8s elapsed
failures: duration_limit_reached: 2

strategy   appearances  completed  wins  seat win%  game win%  95% ci         avg finish  avg captures  avg elims
basic-v1   98           98         12    12.2%      12.2%      [7.1-20.2]     2.60        88.45         0.20
scored-v1  196          196        86    43.9%      87.8%      [79.8-92.9]    1.71        123.67        0.90
```

A strategy's `appearances` counts every seat that used it across every game
run — a mirror matchup like `scored-v1,scored-v1,basic-v1` gives
`scored-v1` 2x the samples per game, since each seat is an independent
sample of that strategy playing from that seat. This means there are
**two different, easily-confused win rates** once a strategy occupies more
than one seat:

- **`seat win%`** — `wins / completed appearances`: given a seat is
  playing this strategy, how often does *that seat* win. A strategy
  occupying `k` of `n` seats can never exceed `1/k` here even if it wins
  every game, because its own seats are competing against each other too.
- **`game win%`** — `wins / completed games`: what fraction of games did
  *any* seat playing this strategy win, regardless of which one. This is
  the number that answers "is this strategy actually better" — in the
  example above, `scored-v1` occupies 2 of 3 seats and won 87.8% of all
  games, even though no single one of its seats won more than 43.9% of
  the time.
- **`95% ci`** — a 95% Wilson score confidence interval around `game
  win%`, over the tournament's completed game count. Answers "how much of
  this could plausibly be sampling noise" without doing the arithmetic by
  hand: `[79.8-92.9]` for `scored-v1` above doesn't come close to
  overlapping 50%, a real signal even before running it out to a much
  larger sample. Wilson rather than a simpler normal approximation
  because Wald-style intervals can extend outside `[0%, 100%]` and have
  poor coverage at small sample sizes or win rates near 0/100% — exactly
  the regime a "did this candidate actually beat baseline" check often
  starts in.

Both `avg *` columns are computed over `completed` games only: a
stalemate/limit-hit game has no winner and no meaningful finish order for
anyone, so including it would misread a systemic matchup property as a
strategy weakness — see `failures` instead.

**JSON** (`--format json`) — the `Config` that produced the run, paired
with the full `Aggregate`:

```json
{
  "config": { "Strategies": [...], "SeedStart": 1, "Games": 100, ... },
  "aggregate": { "TotalGames": 100, "Strategies": [...], ... }
}
```

**Raw output** (`--raw-output <path>`) — one compact JSON object per line
(JSONL), written as each game completes. Each line is a full
`simulation.Result` — the exact same shape `cmd/simulate --format json`
emits under its `"result"` key, so both binaries produce field-compatible
output. Order isn't seed order (games finish whenever they finish); every
line carries its own `Seed`, so that doesn't matter for analysis:

```json
{"Seed":3,"PlayerCount":3,"Seats":[{"Seat":0,"PlayerID":"p0","StrategyID":"basic-v1","Eliminated":true,"FinishOrder":3,...}],"WinnerSeat":1,"WinnerStrategy":"scored-v1","Turns":50,"Commands":1134,...}
```

Loading it in pandas:

```python
import pandas as pd
df = pd.read_json("/tmp/tournament.jsonl", lines=True)
```

## Custom weights (`--weights-variant`)

Evaluates a hand-edited `bot.Weights` candidate against baseline. A
per-candidate logistic-regression fitting pipeline that used to feed this
flag was tried and retired (see
[`project-docs/bot_player/phase_3_continuous_improvement/10_Bot_Weight_Tuning.md`](../../../project-docs/bot_player/phase_3_continuous_improvement/10_Bot_Weight_Tuning.md)
for that history) — `--weights-variant` itself stays as a general-purpose
way to A/B test any `Weights`-shaped JSON file against
`bot.DefaultWeights`, however it was produced.

```bash
go run ./cmd/tournament \
  --strategies basic-v1,scored-v1,scored-v1-candidate \
  --games 500 \
  --weights-variant scored-v1-candidate=candidate.json
```

`candidate.json` only needs to specify the fields it's actually changing —
[`internal/bot.LoadWeights`](../../internal/bot/weights_io.go) fills in
everything else from today's `bot.DefaultWeights`:

```json
{ "ArmyAdvantage": 1.8, "ExposurePenalty": -1.1 }
```

The registered ID (`scored-v1-candidate` above) is then usable anywhere a
strategy ID appears — a direct `--strategies` list, or any entry's
`strategies` in a `--config` batch file, so you can compare several
candidates against baseline and against each other in one run. Repeatable
for multiple candidates at once (`--weights-variant a=1.json --weights-variant b=2.json`);
an ID that collides with a built-in (`basic-v1`/`scored-v1`) or another
`--weights-variant` is rejected up front, before any game runs.

## Batch mode (`--config`)

Run several tournaments concurrently from one process instead of one
invocation per comparison — the natural way to run "baseline vs candidate
A" and "baseline vs candidate B" side by side, each with its own live
progress bar, and get a combined comparison table at the end instead of
mentally diffing separate outputs.

```bash
go run ./cmd/tournament --config batch.json
```

```json
{
  "tournaments": [
    {
      "name": "baseline-vs-candidate-a",
      "strategies": ["basic-v1", "scored-v1", "scored-v1"],
      "games": 500,
      "raw_output": "results/baseline-vs-candidate-a.jsonl"
    },
    {
      "name": "baseline-vs-candidate-b",
      "strategies": ["basic-v1", "scored-v1", "scored-v1"],
      "games": 500,
      "seed_start": 1000,
      "raw_output": "results/baseline-vs-candidate-b.jsonl"
    }
  ]
}
```

Each entry mirrors the direct flags above by name: `name` (required,
unique — labels its progress bar and output section), `strategies`,
`games` (required), `seed_start` (default `1`), `parallel` (default
number of CPUs), `game_mode` (default `auto_start`), `max_turns`,
`max_commands`, `raw_output`. Every entry is validated before any
tournament starts — one bad entry fails the whole batch up front, no
partial runs. Tournaments run **fully concurrently**, each with its own
`parallel` budget (no shared global cap — size each entry's `parallel`
with the others in mind if running many at once).

Output: each tournament's own aggregate (labeled with its `name`),
followed by a combined comparison table — one row per `(tournament,
strategy)` pair, so entries comparing different strategy sets against a
shared baseline are still easy to scan together:

```
=== baseline-vs-candidate-a ===
tournament: 500 games (498 completed, 2 failed) · seeds 1-500 · avg 74.2 turns, 1701.9 commands · 3m12s elapsed
...

=== baseline-vs-candidate-b ===
...

tournament                 strategy     appearances  completed  wins  seat win%  game win%  95% ci        avg finish  avg captures  avg elims
baseline-vs-candidate-a    basic-v1     498          498        61    12.2%      12.2%      [9.7-15.4]    2.60        88.45         0.20
baseline-vs-candidate-a    scored-v1    996          996        437   43.9%      87.8%      [84.6-90.3]   1.71        123.67        0.90
baseline-vs-candidate-b    basic-v1     499          499        58    11.6%      11.6%      [9.1-14.7]    2.58        90.12         0.19
baseline-vs-candidate-b    scored-v1    998          998        441   44.2%      88.4%      [85.3-90.9]   1.73        125.40        0.91
```

`--raw-output` (the single-tournament flag) doesn't apply in batch mode —
each entry specifies its own `raw_output` path instead.

## Progress

While games run, one live progress bar per tournament renders on stderr
([mpb](https://github.com/vbauerster/mpb)) — a single-tournament run gets
one bar, a `--config` batch run gets one per entry, each on its own line,
updating concurrently as that tournament's own results arrive. Suppressed
automatically when stderr isn't a terminal, same as `cmd/simulate`'s
spinner.

## Color

The terminal aggregate table ([fatih/color](https://github.com/fatih/color))
highlights the best strategy by win rate in green and the worst in dim
gray, with the header/failure lines colored for scannability. Color is
applied *after* the table is fully aligned by `tabwriter`, one whole
rendered line at a time, so it never disturbs column alignment. Disabled
automatically for `--output <file>` (a file shouldn't carry escape codes),
when stdout isn't a live terminal (piped/redirected), or when `NO_COLOR`
is set / `TERM=dumb`. `--format json` and `--raw-output` are never
colored.

## Trace level

Every game in a tournament always runs at `simulation.TraceNone` — this is
not configurable via a flag. A tournament only consumes `Result`s;
retaining a decision/milestone trace for every game in a batch of hundreds
or thousands would be pure waste. For inspecting *why* a specific bot made
a specific decision, reproduce that one game with `cmd/simulate` using its
exact seed and strategies at `--trace decision` or `--trace full`.

## Failure handling

A single game hitting a safety limit (stalemate, runaway strategy bug,
etc.) never aborts the tournament — it's counted in `failed`/`failures`
and the batch continues. Only a config-validation error (e.g. an unknown
strategy ID, `--games 0`) stops the run entirely, before any game starts.

## Exit codes

`0` if every requested game ran to either completion or a clean per-game
failure. `1` if the run was cut short (e.g. `Ctrl+C`) or the flags/config
were invalid — a pre-flight config error skips output entirely, since no
game ever ran.
