"""Load backend/cmd/traindata's JSONL output into fittable per-phase matrices.

Each row is one bot decision: a GameID (safe to group/join across separate
cmd/traindata invocations -- see that tool's own README), a Phase, whether
the deciding player Won the game, and a Features object with one raw
(unweighted) signal per named feature for that phase.
"""

from __future__ import annotations

from pathlib import Path

import pandas as pd


def load_training_rows(paths: list[Path]) -> pd.DataFrame:
    """Load and concatenate one or more cmd/traindata JSONL files.

    Features (a nested object per row) is flattened into its own columns
    via pd.json_normalize, matching the loading pattern already documented
    in cmd/traindata's own README.
    """
    frames = []
    for path in paths:
        df = pd.read_json(path, lines=True)
        # pandas-stubs' json_normalize signature doesn't account for a
        # Series-of-dicts input, though it works fine at runtime (same
        # pattern documented in cmd/traindata's own README).
        features = pd.json_normalize(df["Features"]).add_prefix(  # type: ignore[arg-type]
            "feature_"
        )
        frames.append(pd.concat([df.drop(columns=["Features"]), features], axis=1))
    return pd.concat(frames, ignore_index=True)


def sample_weights(df: pd.DataFrame) -> pd.Series:
    """1 / (candidates_in_this_decision * decisions_for_that_player_in_that_game), per row.

    Per Next_Phase_Bot_ML_Roadmap.md's weighting recommendation: a long
    game contributes many more decision-rows than a short one, so without
    this a handful of unusually long games would dominate the fit. Grouped
    by (GameID, PlayerID) -- GameID (not Seed alone) is the safe,
    collision-free game identity across separate cmd/traindata invocations
    that may share overlapping seed ranges under different configs.

    Since cmd/traindata now emits one row per legal candidate per decision
    (not just the chosen one -- see rowsFromEntries), a decision with many
    legal candidates would otherwise dominate a decision with few just by
    contributing more rows. CommandIndex identifies which rows share one
    decision (assigned once per executed command, not per candidate), so
    the weight is split two ways: evenly across a decision's own
    candidate-rows first, then evenly across a (game, player)'s decisions,
    same as before -- every (game, player) still sums to equal total
    weight, and within that budget every *decision* (not every row) still
    gets equal weight regardless of how many candidates it had.
    """
    decision_key = ["GameID", "PlayerID", "CommandIndex"]
    candidates_per_decision = df.groupby(decision_key)["PlayerID"].transform("size")
    decisions_per_game_player = df.groupby(["GameID", "PlayerID"])["CommandIndex"].transform(
        "nunique"
    )
    return 1.0 / (candidates_per_decision * decisions_per_game_player)


def phase_matrix(
    df: pd.DataFrame, phase: str, feature_names: list[str]
) -> tuple[pd.DataFrame, pd.Series, pd.Series]:
    """Filter df to one phase and return (X, y, weights).

    X's columns are feature_names in that fixed order (the same order
    fit.PHASE_FEATURES declares them in), y is Won, weights is
    sample_weights(df) restricted to the same rows -- computed from the
    full df's game/player grouping, not recomputed per phase, since a
    player's total decision count in a game spans every phase, not just
    this one.
    """
    weights = sample_weights(df)
    mask = df["Phase"] == phase
    columns = [f"feature_{name}" for name in feature_names]
    X = df.loc[mask, columns].rename(columns=lambda c: c.removeprefix("feature_"))
    y = df.loc[mask, "Won"]
    return X, y, weights.loc[mask]
