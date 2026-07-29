# The GCN Value Function: Overview

> Reference documentation for the whole-board GCN value function, split into four focused documents so each stays readable on its own:
>
> - **This file** — what the system is for, the end-to-end pipeline, and the open question these docs exist to inform.
> - [`GCN_Value_Function_Training_Data.md`](GCN_Value_Function_Training_Data.md) — how training data is generated and exactly what's in it (`cmd/tdtraindata`, `internal/tdstate/encode.go`).
> - [`GCN_Value_Function_Architecture.md`](GCN_Value_Function_Architecture.md) — a graph convolutional network primer, then this project's exact network architecture (`gcn_fit.py`, `gcnmodel.go`).
> - [`GCN_Value_Function_Training.md`](GCN_Value_Function_Training.md) — how the network is actually trained (TD(λ)) and calibrated for play (`gcn_fit.fit_gcn_td`, `cmd/bvcalibrate`).
>
> Grounded directly in the current code throughout, not summarized from memory. See `GCN_Strategy_Roadmap_with_References.md` and `Search_Integration_Roadmap_with_References.md` for the roadmaps this system feeds into.

## What this system is for

A single learned function, `Score(state) → float`, estimates how good a board position is for one player. Every search-based bot strategy (`ValueStrategy`) calls it to rank candidate moves — reinforcements, attacks, occupations, fortifications — by scoring the resulting board state and picking the best-scoring one. The function is trained offline from simulated self-play, exported as a flat JSON file of weights, and loaded by a small hand-written Go inference engine at play time — there's no live training, no PyTorch dependency on the serving side, and no database involved.

The pipeline, end to end:

```
internal/simulation (bot-vs-bot games)
  → cmd/tdtraindata (captures one row per living player per turn boundary)
  → analytics: td_fit.py / gcn_fit.py (loads rows, trains a GCN)
  → gcn_fit.export_gcn (writes weights + metadata as JSON)
  → cmd/bvcalibrate (calibrates action-margin thresholds)
  → internal/bot/gcnmodel (loads the JSON, runs inference in Go)
```

Each pipeline stage is covered in its own companion document — see the links above.

## Where this leaves the "individual enemies" question

Every enemy-related signal in the current feature encoding (see `GCN_Value_Function_Training_Data.md`'s "current limitation" section) collapses every rival into either a per-territory, ownership-blind threat sum, or a single aggregate "strongest enemy" scalar. This is why the model currently can't learn Killbot-style opportunistic elimination hunting, independent of how deep any search built on top of it goes: the leaf evaluator has no signal for "this specific rival is nearly eliminated," only "the strongest rival has this much." Closing that gap means extending the feature schema to carry some form of *relative, rank-based* per-opponent information (not raw player identity), which touches the per-territory feature width, the global feature width, `node_feature_dim`'s derivation on both sides, the exported JSON schema, and requires a full retrain, not an incremental tweak.

This turned out to be one instance of a broader pattern — see `Feature_Expansion_Roadmap_with_References.md` for a fuller catalog of game knowledge the current feature set leaves untapped (combat-realism via the existing Attack Handler, continent-completion progress, board chokepoints, card-set completeness, turn-order, momentum, and this enemy-differentiation question), along with a proposed phased approach to closing them.
