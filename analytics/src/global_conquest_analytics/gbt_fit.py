"""Fit one gradient-boosted-trees model per game phase, exported for
backend/internal/bot/gbtmodel to run directly at decision time.

Unlike fit.py's logistic regression, a GBT model's decision function
isn't linear -- it can't be reduced to bot.Weights coefficients. A
diagnostic comparison (LogisticRegressionCV vs. LightGBM, same features,
same held-out validation split) found GBT beats logistic regression in
every phase, dramatically for fortify (AUC 0.524 -> 0.724), and recovers
real predictive signal from several features (weakness, expected_loss_cost,
exposure_penalty) every logistic-regression fit crushed to near-zero
coefficients -- see project-docs/bot_player/Next_Phase_Bot_ML_Roadmap.md.

LightGBM was chosen over sklearn's HistGradientBoostingClassifier
specifically for dump_model()'s stable, documented JSON export format,
designed for exactly this kind of cross-language portability -- unlike
sklearn's internal, undocumented tree representation.
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path

import lightgbm as lgb
import numpy as np
import pandas as pd

from global_conquest_analytics.fit import PHASE_FEATURES
from global_conquest_analytics.training_data import phase_matrix

# Matches the diagnostic script's HistGradientBoostingClassifier(random_state=0)
# defaults (num_iterations~100, learning_rate=0.1, num_leaves=31), which
# already showed strong held-out performance without overfitting across
# every phase's actual row count (100k-800k) -- not independently tuned.
DEFAULT_NUM_BOOST_ROUND = 100
DEFAULT_LEARNING_RATE = 0.1
DEFAULT_NUM_LEAVES = 31

# end_phase_threshold's percentile, used only by attack/fortify (see
# GBTPhaseFit's docstring) -- "only skip roughly the worst 10% of legal
# candidates" rather than "only take an above-50%-probability candidate,"
# which a first live tournament eval showed was catastrophically wrong: a
# GBT model's predicted probability is P(this player wins the whole
# 60+-turn game | this one decision's features), not P(this specific move
# is good) -- attack's own predicted probabilities had a median of just
# ~0.34 on real training data (most individual decisions, even perfectly
# reasonable ones, don't move a diffuse whole-game outcome prediction
# anywhere near 0.5), while fortify's median was ~0.58 -- confirming the
# right cutoff isn't just phase-specific but can't be a single hand-picked
# constant shared across phases either, since each phase's predicted
# distribution is shaped completely differently.
END_PHASE_THRESHOLD_PERCENTILE = 10


@dataclass(frozen=True)
class GBTPhaseFit:
    """One phase's fitted booster, plus (for attack/fortify only) the
    end_phase_threshold backend/internal/bot.GBTStrategy compares its best
    real candidate's predicted probability against to decide whether to
    keep attacking/fortifying or end the phase -- see
    END_PHASE_THRESHOLD_PERCENTILE. None for reinforce/occupy, which have
    no "end early" decision to make (a reinforcement or occupation choice
    is always made once the phase is reached).
    """

    booster: lgb.Booster
    end_phase_threshold: float | None


def fit_phase_gbt(
    df: pd.DataFrame,
    phase: str,
    num_boost_round: int = DEFAULT_NUM_BOOST_ROUND,
) -> GBTPhaseFit:
    """Fit one phase's LightGBM binary classifier against df.

    Uses the exact same feature set and sample_weight scheme as
    fit.fit_phase -- the only difference is the model class.
    """
    feature_names = PHASE_FEATURES[phase]
    X, y, weights = phase_matrix(df, phase, feature_names)

    train = lgb.Dataset(X, label=y, weight=weights, feature_name=feature_names)
    params = {
        "objective": "binary",
        "learning_rate": DEFAULT_LEARNING_RATE,
        "num_leaves": DEFAULT_NUM_LEAVES,
        "verbose": -1,
    }
    booster = lgb.train(params, train, num_boost_round=num_boost_round)

    threshold = None
    if phase in ("attack", "fortify"):
        proba = booster.predict(X)
        threshold = float(np.percentile(proba, END_PHASE_THRESHOLD_PERCENTILE))

    return GBTPhaseFit(booster=booster, end_phase_threshold=threshold)


def export_gbt(fits: dict[str, GBTPhaseFit], output_dir: Path) -> None:
    """Write one dump_model() JSON file per phase to output_dir, named
    exactly as backend/internal/bot.LoadGBTModels expects: attack.json,
    reinforce.json, occupy.json, fortify.json. end_phase_threshold (when
    set) is embedded directly into that same JSON object -- self-contained,
    no separate config file to keep in sync.
    """
    output_dir.mkdir(parents=True, exist_ok=True)
    for phase, fit in fits.items():
        dump = fit.booster.dump_model()
        if fit.end_phase_threshold is not None:
            dump["end_phase_threshold"] = fit.end_phase_threshold
        path = output_dir / f"{phase}.json"
        path.write_text(json.dumps(dump), encoding="utf-8")
