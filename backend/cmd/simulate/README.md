# simulate

Runs one headless, reproducible bot-vs-bot Global Conquest game and reports
the result. No Postgres, no WebSocket, no Discord, no HTTP, no live pacing —
it drives `internal/bot` strategies directly against an in-process
`internal/risk` game. See
[`project-docs/bot_player/phase_2_first_playable_bot/05_Simulation_Framework.md`](../../../project-docs/bot_player/phase_2_first_playable_bot/05_Simulation_Framework.md)
and its companion `05_Simulation_Framework_Tasks.md` for the design behind
this.

## Quick start

Run from the `backend/` directory:

```bash
go run ./cmd/simulate --seed 12345 --strategies basic-v1,scored-v1,scored-v1
```

## Flags

| Flag | Required | Default | Description |
|---|---|---|---|
| `--seed` | yes | — | Any integer. The same seed + strategies + game mode always reproduces the identical game, dice rolls and all. |
| `--strategies` | yes | — | Comma-separated strategy ID per seat, e.g. `basic-v1,scored-v1,scored-v1`. Player count is this list's length (3–6). Available IDs: `basic-v1`, `scored-v1`. |
| `--game-mode` | no | `auto_start` | `auto_start` (armies pre-distributed, starts at Reinforce) or `manual` (starts at SetupReinforce — bots place their own initial armies). |
| `--trace` | no | `summary` | `none`, `summary`, `decision`, or `full` — see [Trace levels](#trace-levels). |
| `--max-turns` | no | 2000 | Override the turn safety limit. |
| `--max-commands` | no | 20000 | Override the command safety limit. |
| `--format` | no | `text` | `text` or `json`. |
| `--output` | no | stdout | Write to this file path instead of stdout. |

## Examples

```bash
# Quick 3-player game, human-readable summary
go run ./cmd/simulate --seed 1 --strategies basic-v1,basic-v1,scored-v1

# 6-player scored-v1 mirror match, full decision trace, saved as JSON
go run ./cmd/simulate --seed 23 --strategies scored-v1,scored-v1,scored-v1,scored-v1,scored-v1,scored-v1 \
  --trace decision --format json --output /tmp/game.json

# Just the final tally, no trace overhead
go run ./cmd/simulate --seed 42 --strategies basic-v1,scored-v1,scored-v1 --trace none
```

## Output

**Text** (default) — a one-line header, the winner (or failure reason),
and a per-seat table:

```
seed 23 · 6 players · 48 turns · 922 commands
winner: seat 5 (scored-v1)

seat  strategy   territories  armies  captures  elims
0     basic-v1   0            0       22        0
1     scored-v1  0            0       14        0
...
```

**JSON** (`--format json`) — the full `simulation.Result` plus whatever
the trace level captured:

```json
{
  "result": { "Seed": 23, "WinnerSeat": 5, "Turns": 48, "Seats": [...], ... },
  "milestones": [ ... ],
  "decisions": [ ... ]
}
```

`milestones` and `decisions` are omitted/empty below the trace level that
populates them (see below).

## Progress

While a game is running, a live status line (spinner, current turn,
command count, elapsed time) prints to stderr — updated a few times a
second, cleared before the final result prints. There's no percent
complete: game length isn't knowable ahead of time (see
[Convergence](#a-note-on-convergence) below). This is suppressed
automatically when stderr isn't a terminal, so redirected output, `2>`
logs, and CI runs never see the spinner's carriage-return noise — only
`--format text`/`json`'s actual result goes to stdout, and that's never
touched by the progress line.

## Trace levels

| Level | Adds |
|---|---|
| `none` | Nothing — final result only. |
| `summary` | Milestones: turn transitions, captures, eliminations, card turn-ins. |
| `decision` | + one entry per bot decision: the chosen command and its full scoring explanation (score, every feature, top alternatives). This is the level that matters for comparing heuristics. |
| `full` | + a state fingerprint and the engine's combat-roll event for each command. Never a full board snapshot — that's deliberately out of scope for now. |

## Exit codes

`0` if the game reached completion. `1` if it didn't — a safety limit was
hit, a strategy or the engine rejected a command, or the flags/config were
invalid. A run that didn't complete still prints whatever partial result
accumulated (seats, captures, etc.) before failing; only a pre-flight
config error (e.g. an unknown strategy ID) skips output entirely, since no
game was ever built.

## A note on convergence

Two evenly-matched `scored-v1` bots can genuinely deadlock at a shared
border, endlessly reinforcing without attacking — this is real emergent
behavior under the current heuristic weights, not a bug. It's fairly
common in larger mirror matches specifically: roughly half of random seeds
for a 6-player all-`scored-v1` game don't converge within a few seconds.
The run will self-terminate at the wall-clock safety limit (30s by
default) and report `did not complete` rather than hang. Seed `23` is
confirmed fast for that particular matchup if you want a reliable quick
demo.
