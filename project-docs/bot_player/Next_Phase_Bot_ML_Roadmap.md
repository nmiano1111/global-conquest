# Next Phase: Empirical Improvement of Bot Quality

## Primary Recommendation

Complete **Approach A (logistic regression over the existing heuristic
features)** end-to-end before investing heavily in **Approach B (raw
board-state evaluation).**

Current pipeline:

``` text
self-play
→ cmd/traindata
→ JSONL
→ Python training
→ weights.json
→ --weights-variant
→ tournament evaluation
```

The first milestone is proving the end-to-end learning and evaluation
loop, not maximizing ML sophistication.

## Why Approach A First

This validates:

-   Current feature engineering
-   Training labels
-   Training-data generation
-   JSON weight export/import
-   Tournament evaluation
-   Whether fitted coefficients outperform hand-tuned ones

Even a negative result is valuable because it validates (or invalidates)
the overall methodology.

## Statistical Concern

The current dataset contains only **chosen actions**.

This introduces policy-induced selection bias because rejected
candidates are absent.

Treat Approach A as:

> Recalibrating the existing heuristic policy from self-play.

Do not present it as learning an optimal move evaluator.

## Data Splitting

Never split by decision row.

Split by complete games:

-   Training games
-   Validation games
-   Test games

Reserve separate seed ranges for final tournament evaluation.

## Model Organization

Prefer one model per decision family initially:

-   Reinforcement
-   Attack
-   Occupy
-   Fortify

This maps naturally back onto the existing weight structure.

## Training Weights

Long games produce more decisions.

Consider weighting each decision approximately by:

    1 / decisions_for_that_player_in_that_game

so that each player-game trajectory contributes similar influence.

## Regularization

Use regularized logistic regression.

Inspect:

-   coefficient magnitude
-   coefficient signs
-   feature correlations
-   coefficient stability

Do not deploy weights solely because validation loss improved.

Tournament performance is the real success metric.

## Tournament Evaluation

Use:

-   Balanced seat rotation
-   Unseen seeds
-   Multiple player counts
-   Multiple opponent mixes
-   Confidence intervals
-   Stall rate
-   Average game length

Evaluate more than just candidate vs default.

## Recommendation for Approach B

Eventually record **all candidate actions**, not only the chosen action.

Each candidate should include:

-   Decision ID
-   Candidate command
-   Resulting state
-   Heuristic score
-   Chosen/not chosen
-   Context

This enables ranking models and stronger supervision.

This should not block completing Approach A.

## Intermediate Experiment

Before training a learned board evaluator:

1.  Capture all candidate moves.
2.  Build resulting-state representations.
3.  Train a model to reproduce the existing heuristic ranking.
4.  Validate the board representation pipeline.

## Model Progression

1.  Logistic regression
2.  Gradient-boosted trees
3.  Raw board-state evaluation
4.  Candidate-level ranking
5.  Rollout/value targets
6.  Graph neural networks (once customizable boards exist)

Skip a plain MLP unless it demonstrates a clear advantage.

## Board Representation

Retain topology-aware information:

-   Territory ID
-   Owner
-   Army count
-   Continent
-   Adjacency
-   Border status

Flatten this representation for current models while keeping the source
representation flexible for future GNN work.

## Dataset Validation

Validate:

-   Stable feature columns
-   Zero-filled conditional features
-   No divide-by-zero recovery issues
-   Supported phases only
-   No basic-v1 rows
-   No excluded decision types
-   Unique decision IDs
-   Game IDs
-   Player IDs
-   Class balance
-   Feature variance
-   Feature correlations

## Deliverables

1.  Dataset validation report
2.  Game-level train/validation/test split
3.  Per-phase feature matrices
4.  Regularized logistic regression models
5.  Coefficient report
6.  Exported weights.json
7.  Tournament configuration
8.  Tournament comparison report

## Decision After First Results

If tournament performance improves:

-   Gather more data
-   Tune hyperparameters
-   Improve candidate logging

If tournament performance does not improve:

Investigate:

-   Noisy labels
-   Selection bias
-   Feature recovery
-   Game weighting
-   Phase mixing
-   Coefficient scaling

If offline metrics improve but tournaments do not:

Treat that as evidence that predicting eventual winners is not
equivalent to ranking moves. Candidate-level supervision then becomes
the next priority.

## Instructions for Claude

Review the implementation of:

-   cmd/traindata
-   Tournament evaluation
-   Weight loading
-   Simulation traces

Compare the implementation to this roadmap.

Identify:

1.  Already completed recommendations
2.  Items to complete before training the first model
3.  Future enhancements

Produce a prioritized implementation plan.

Do not implement changes yet.
