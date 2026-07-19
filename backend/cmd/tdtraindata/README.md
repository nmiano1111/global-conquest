# cmd/tdtraindata

Generates whole-board, per-turn-boundary training data for a TD(λ) value
function. This is a different row grain from the now-retired `cmd/traindata`
(one row per legal *candidate* per decision): `tdtraindata` emits one row
per living player's perspective at every *completed turn boundary*, encoded
via `internal/tdstate.Encode`. See
`project-docs/bot_player/proposals/Monte_Carlo_Evaluator_Roadmap_with_References.md`
and the Jamie Carr GCN/TD(λ) paper it cites — the goal is testing whether
TD(λ)'s objective (bootstrap between temporally close turn transitions,
rather than regressing straight from a snapshot to the final game outcome)
fixes the erratic/uninformative-value problem every "regress final `Won`"
attempt this project has tried (logistic regression, then gradient boosted
trees) ran into.

Plays simulated games with `internal/simulation`, buffers one row per living
player at every `OnTurnBoundary` callback, and discards any game that
doesn't complete (no reliable win/loss label exists for it).

## Quick start

```bash
go run ./cmd/tdtraindata \
  --strategies basic-v1,scored-v1,scored-v1 \
  --games 50 \
  --output data/raw/tdtraindata/basic_scored_scored_train.jsonl
```

## Flags

| Flag | Default | Description |
|---|---|---|
| `--strategies` | *(required)* | Comma-separated strategy ID per seat, e.g. `basic-v1,scored-v1,scored-v1` — fixed for every game; player count is this list's length |
| `--games` | *(required)* | Number of games to run |
| `--output` | *(required)* | JSONL destination for the generated turn-boundary rows |
| `--seed-start` | `1` | First seed used; games run with seeds `seed-start..seed-start+games-1` |
| `--parallel` | `runtime.NumCPU()` | Number of games to run concurrently |
| `--game-mode` | `auto_start` | Game construction mode: `auto_start\|manual` |
| `--max-turns` | `0` (use simulation default) | Override the per-game turn safety limit |
| `--max-commands` | `0` (use simulation default) | Override the per-game command safety limit |

## Output

`--output` is a JSONL file, one `trainingRow` per line:

```json
{"GameID":"basic-v1,scored-v1,scored-v1@auto_start@1","Seed":1,"PlayerID":"p2","StrategyID":"scored-v1","Turn":7,"Won":true,"Features":[0.0,1.0,...]}
```

The TD(λ) trainer (`analytics/src/global_conquest_analytics/td_fit.py`)
groups rows by `(GameID, PlayerID)`, sorts by `Turn`, and treats the
resulting sequence as one episode. `Won` is the eventual outcome for
`PlayerID`, repeated on every row in that player's sequence for simplicity.

Two sidecar files are written once per run, alongside `--output` (e.g.
`data.jsonl` → `data.featurenames.json` / `data.boardschema.json`), since
every row from a single run shares the same board and feature layout:

- **`*.featurenames.json`** — the column names matching every row's
  `Features` order (`tdstate.FeatureNames`).
- **`*.boardschema.json`** — the classic board's static topology
  (`tdstate.NewBoardSchema`), used to build an identical graph-propagation
  matrix on both the training and inference side of a GCN.

## Failure handling

Individual game failures (stalemates hitting a safety limit, etc.) are a
normal, expected outcome, not a tool failure — matching `cmd/tournament`'s
own semantics. The final summary line reports how many of `--games`
actually completed:

```
wrote 4213 rows from 48/50 completed games (2 failed) to data.jsonl in 3.2s
```
