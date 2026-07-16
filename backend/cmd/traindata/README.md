# traindata

Generates a logistic-regression training set from headless self-play: runs
many games and, for every decision a scored strategy made, recovers the
raw (unweighted) value of each named feature plus whether that player went
on to win the game. See
[`project-docs/bot_player/phase_3_continuous_improvement/10_Bot_Weight_Tuning.md`](../../../project-docs/bot_player/phase_3_continuous_improvement/10_Bot_Weight_Tuning.md)
for the plan this feeds into.

Not [`cmd/tournament`](../tournament/README.md): a tournament deliberately
never records per-decision trace data (every game runs at
`simulation.TraceNone` — see that package's README). This binary calls
`internal/simulation.Simulator.RunOne` directly at `simulation.TraceDecision`
instead, so its raw output has feature data in it that `cmd/tournament`'s
never does.

## Quick start

Run from the `backend/` directory:

```bash
go run ./cmd/traindata --strategies basic-v1,scored-v1,scored-v1 --games 500 --output data.jsonl
```

## Flags

| Flag | Required | Default | Description |
|---|---|---|---|
| `--strategies` | yes | — | Comma-separated strategy ID per seat, e.g. `basic-v1,scored-v1,scored-v1`. At least one seat should be a scored strategy — `basic-v1` never produces feature data (its `Explanation` is always empty), so an all-`basic-v1` matchup silently produces zero rows. |
| `--games` | yes | — | How many games to run. |
| `--seed-start` | no | `1` | Seeds used are `seed-start .. seed-start+games-1`. |
| `--parallel` | no | number of CPUs | How many games run concurrently. |
| `--game-mode` | no | `auto_start` | `auto_start` or `manual` — same as `cmd/simulate`/`cmd/tournament`. Run this tool again with `--game-mode manual` (and a different `--output`) to also cover `setup_reinforce` decisions, which `auto_start` games never enter. |
| `--max-turns` | no | 2000 | Override the per-game turn safety limit. |
| `--max-commands` | no | 20000 | Override the per-game command safety limit. |
| `--output` | **yes** | — | JSONL destination for the generated rows. |

## What counts as a row

One row per decision a scored strategy actually made — not one row per
legal candidate it considered. `bot.Explanation.Alternatives` only records
a runner-up's score, not its feature breakdown, so getting genuine
per-candidate rows isn't possible without extending that type; this is the
buildable interpretation with what's already recorded by
`--trace decision`.

A decision produces a row only if it belongs to a completed game (no
reliable win/loss label otherwise) and at least one of its phase's known
features was present on the chosen candidate — this is what naturally
excludes `basic-v1`'s empty `Explanation`, card-trade-in decisions, and
"end this phase"/"end turn" decisions, with no special-casing needed.

**This tool assumes the games were played under `bot.DefaultWeights`** —
the feature-to-weight mapping used to recover raw signal from
`Explanation.Features`' weighted values is hardcoded against today's
baseline, not an arbitrary candidate (see `extract.go`).

## Output

One compact JSON object per line:

```json
{"Seed":42,"Phase":"attack","StrategyID":"scored-v1","PlayerID":"p1","Seat":1,"Turn":12,"CommandIndex":87,"Won":true,"Features":{"army_advantage":3,"capture_probability":0.62,"expected_loss_cost":-1.1,"completes_continent":0,"breaks_enemy_continent":0,"card_opportunity":0,"eliminates_player":0,"exposure_penalty":-2}}
```

`Features` always has one key per that `Phase`'s full known feature set —
a feature the engine didn't append to this particular candidate (e.g.
`completes_continent` when the move doesn't complete one) is present with
value `0`, never a missing key.

Loading it in pandas — `Features` needs flattening into its own columns:

```python
import pandas as pd
df = pd.read_json("data.jsonl", lines=True)
features = pd.json_normalize(df["Features"]).add_prefix("feature_")
df = pd.concat([df.drop(columns=["Features"]), features], axis=1)
```

## Progress

A live progress bar on stderr tracks games completed (not rows written)
against the requested total — same [mpb](https://github.com/vbauerster/mpb)
approach as `cmd/tournament`. Suppressed automatically when stderr isn't a
terminal.

## Failure handling

A game hitting a safety limit (stalemate, etc.) doesn't stop the run — no
rows are produced from that game (no reliable win/loss label), and
generation continues. The final printed line reports how many of the
requested games actually completed; a low completion rate is worth
investigating (tighter `--max-turns`, an unstable matchup) since it
directly limits how much data ends up in the output file.
