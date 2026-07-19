# cmd/bvcalibrate

Fits `ValueStrategy`'s `AttackMargin`/`FortifyMargin` empirically against
any `bot.ValueFunction` — a linear `BoardValue` (from `board_fit.py`) or a
GCN `gcnmodel.Model` (from `gcn_fit.py`).

`attack()` and `fortify()` only act on a candidate whose afterstate score
beats the current state's score by more than a margin. A first live
tournament eval found attack and fortify move the score on completely
different scales — attack changes ownership (many features at once);
fortify only reallocates armies between the acting player's own
territories (at most two per-territory `army_fraction` coefficients) — see
`internal/bot/linear_value.go`'s `BoardValue.AttackMargin`/`FortifyMargin`
doc comment. A single shared margin calibrated to attack's scale suppressed
fortify almost entirely, hence a dedicated calibration pass per phase.

It runs many headless games with a *zero-margin* wrapper around the input
weights file (so `ValueStrategy` acts on any positive-delta candidate,
letting its `Observer` see each phase's natural, unfiltered score-delta
distribution), then writes a copy of the input file with
`attack_margin`/`fortify_margin` set to a chosen percentile of each phase's
*positive* observed deltas. The same reasoning and tool work identically
regardless of model class.

## Quick start

```bash
go run ./cmd/bvcalibrate --input value.json --output calibrated.json --games 200
go run ./cmd/bvcalibrate --input gcn.json --model-type gcn --output calibrated.json --games 200
```

## Flags

| Flag | Default | Description |
|---|---|---|
| `--input` | *(required)* | Path to a `board_fit.py`/`gcn_fit.py`-exported weights JSON file |
| `--output` | *(required)* | Destination for the calibrated weights JSON |
| `--model-type` | `linear` | Value function model type to calibrate: `linear\|gcn` |
| `--strategies` | `basic-v1,scored-v1,board-value-candidate` | Comma-separated strategy ID per seat — exactly one seat must be `board-value-candidate` |
| `--games` | `200` | Number of calibration games to run |
| `--percentile` | `50` | Percentile (0–100) of each phase's positive score-delta distribution to use as its margin |
| `--seed-start` | `1` | First seed used |
| `--parallel` | `runtime.NumCPU()` | Number of games to run concurrently |
| `--game-mode` | `auto_start` | Game construction mode: `auto_start\|manual` |
| `--max-turns` | `0` (use simulation default) | Override the per-game turn safety limit |

## Output

Console output reports each phase's observed sample size and chosen margin:

```
attack: n=1842 observed decisions, margin=0.0231 (p50 of positive deltas)
fortify: n=513 observed decisions, margin=0.0042 (p50 of positive deltas)
Calibrated margins written -> calibrated.json
```

`--output` is a copy of `--input` with only `attack_margin`/`fortify_margin`
overwritten — every other field (`weights`, `intercept`, `mean`, `std`,
`feature_names`, or a GCN model's own fields) passes through unchanged, so
the result is a drop-in replacement for the original file wherever it's
loaded (e.g. `cmd/tournament --board-value-variant`/`--gcn-variant`).

Choosing `--percentile 0` calibrates to the smallest observed positive
delta (act on almost any improvement); higher percentiles make the
strategy pickier about which candidates clear the bar. Only *positive*
deltas are used for the percentile — a delta of zero or less already
correctly ends the phase under any non-negative margin and isn't
informative about how large a genuine improvement looks for that phase.
