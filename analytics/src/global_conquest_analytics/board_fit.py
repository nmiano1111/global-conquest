"""Fit a whole-board linear value function via logistic regression --
Phase 2's chosen training objective (parking TD(λ) after Phase 1 showed
representation, not objective, was the dominant lever -- see
project-docs/bot_player/proposals/GCN_Strategy_Roadmap_with_References.md
and this project's own td_fit.py/Phase 1 validation results).

Reads backend/cmd/tdtraindata's JSONL output via td_fit.load_episodes,
the exact same turn-boundary-per-living-player data Phase 1 used -- only
the training *objective* changes here (a same-data naive logistic
regression, identical to Phase 1's own baseline) rather than TD(λ)'s
eligibility-trace bootstrapping.

backend/internal/bot.BoardValue.Score standardizes a live afterstate
encoding itself using the same mean/std computed here, then takes a dot
product directly against the fitted coefficients -- so coefficients are
exported on the *standardized* scale (not converted back to raw units),
and the standardizer's mean/std travel alongside them in the same JSON
file.

This module does NOT fit BoardValueStrategy's attack_margin/
fortify_margin -- an earlier version computed a single shared margin here
(MARGIN_STD_MULTIPLIER * the fitted score distribution's std), but a live
tournament eval found attack and fortify move the score on completely
different scales (attack changes ownership -- many features move at
once; fortify only reallocates armies between the acting player's own
territories -- at most two per-territory army_fraction coefficients), so
a shared margin calibrated to attack's scale suppressed fortify almost
entirely (12 fortifies/13 turns at margin 0, down to 0 at any margin
>= ~0.1). Margins are fit as a *separate* step, from real observed
per-phase score deltas during actual play, by cmd/bvcalibrate -- see that
tool's module doc comment. export_board_value writes attack_margin=0.0/
fortify_margin=0.0 as placeholders; run cmd/bvcalibrate against the
exported file before using it in a live BoardValueStrategy.
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path

import numpy as np
from sklearn.linear_model import LogisticRegressionCV

from global_conquest_analytics.td_fit import (
    Episode,
    Standardizer,
    fit_standardizer,
    flatten_episodes,
    standardize_episodes,
)


@dataclass(frozen=True)
class BoardValueFit:
    """A fitted whole-board linear value function, ready for export."""

    weights: np.ndarray  # standardized-scale coefficients, shape (D,)
    intercept: float
    standardizer: Standardizer
    feature_names: list[str]


def fit_board_value(episodes: list[Episode], feature_names: list[str]) -> BoardValueFit:
    """Fit a single logistic regression predicting Won from every
    turn-boundary row across episodes (flatten_episodes -- one row per
    living player per completed turn, sample-weighted by
    1/episode-length so long games don't dominate, same as every other
    fit in this project).

    Standardization is fit on episodes here (the caller's training
    split) and travels with the returned weights/intercept so
    backend/internal/bot.BoardValue can standardize a live afterstate
    encoding the same way at decision time, without needing sklearn --
    coefficients are kept on the standardized scale to match.

    l1_ratios=(0.0,) pins pure L2/ridge regularization, scoring=
    "neg_log_loss" and use_legacy_attributes=False pin their upcoming
    defaults -- deliberately unchanged from Phase 1's validated naive
    baseline, per the plan's decision to park TD(λ) rather than adopt a
    new objective for this first live-play test.
    """
    standardizer = fit_standardizer(episodes)
    standardized = standardize_episodes(episodes, standardizer)
    X, y, weights = flatten_episodes(standardized)

    model = LogisticRegressionCV(
        cv=5,
        max_iter=1000,
        l1_ratios=(0.0,),
        scoring="neg_log_loss",
        use_legacy_attributes=False,
    )
    model.fit(X, y, sample_weight=weights)

    return BoardValueFit(
        weights=model.coef_[0],
        intercept=float(model.intercept_[0]),
        standardizer=standardizer,
        feature_names=feature_names,
    )


def export_board_value(fit: BoardValueFit, output_path: Path) -> None:
    """Write fit as JSON matching backend/internal/bot.LoadBoardValue's
    expected shape exactly: weights, intercept, mean, std, attack_margin,
    fortify_margin, feature_names. attack_margin/fortify_margin are
    written as 0.0 placeholders -- see this module's docstring; run
    cmd/bvcalibrate against output_path before live use.
    """
    output_path.parent.mkdir(parents=True, exist_ok=True)
    payload = {
        "weights": fit.weights.tolist(),
        "intercept": fit.intercept,
        "mean": fit.standardizer.mean.tolist(),
        "std": fit.standardizer.std.tolist(),
        "attack_margin": 0.0,
        "fortify_margin": 0.0,
        "feature_names": fit.feature_names,
    }
    output_path.write_text(json.dumps(payload), encoding="utf-8")


__all__ = ["BoardValueFit", "Episode", "fit_board_value", "export_board_value"]
