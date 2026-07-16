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
| `--strategies` | yes | — | Comma-separated strategy ID per seat, e.g. `basic-v1,scored-v1,scored-v1`. Fixed for every game in the tournament. Available IDs: `basic-v1`, `scored-v1`. |
| `--games` | yes | — | How many games to run. |
| `--seed-start` | no | `1` | Seeds used are `seed-start .. seed-start+games-1`. Same `seed-start` + `games` + `strategies` always reproduces the same batch of games. |
| `--parallel` | no | number of CPUs | How many games run concurrently. |
| `--game-mode` | no | `auto_start` | `auto_start` or `random_territory` — same as `cmd/simulate`. |
| `--max-turns` | no | 2000 | Override the per-game turn safety limit. |
| `--max-commands` | no | 20000 | Override the per-game command safety limit. |
| `--format` | no | `text` | Aggregate output format: `text` or `json`. |
| `--output` | no | stdout | Write the aggregate summary to this file instead of stdout. |
| `--raw-output` | no | (none) | If set, path to write one JSON-encoded `simulation.Result` per line (JSONL) as each game completes. Omitted = aggregate only, no raw dump. |

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

strategy   appearances  completed  wins  seat win%  game win%  avg finish  avg captures  avg elims
basic-v1   98           98         12    12.2%      12.2%      2.60        88.45         0.20
scored-v1  196          196        86    43.9%      87.8%      1.71        123.67        0.90
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

## Progress

While games run, a live progress bar on stderr ([schollz/progressbar](https://github.com/schollz/progressbar))
shows percent complete, games/total, throughput, ETA, and a running failure
count — updated as results arrive, finalized before the aggregate prints.
Suppressed automatically when stderr isn't a terminal, same as
`cmd/simulate`'s spinner.

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
