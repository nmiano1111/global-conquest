"""Fit one regularized logistic regression per game phase.

scored-v1 (backend/internal/bot/strategy_scored*.go) picks the
highest-scoring legal candidate each decision -- a pure ranking rule, not
a calibrated probability. Logistic regression fits
logit(p) = w . x + b; since the sigmoid is monotonic, ranking candidates
by the raw linear score w . x gives the identical order to ranking by
fitted probability. So the fitted coefficients can be used directly as
bot.Weights values (see weights_export.py) -- no probability calibration
or rescaling needed.
"""

from __future__ import annotations

from dataclasses import dataclass

import pandas as pd
from sklearn.linear_model import LogisticRegressionCV
from sklearn.preprocessing import StandardScaler

from global_conquest_analytics.training_data import phase_matrix

# Mirrors backend/cmd/traindata/extract.go's phaseFeatures table exactly --
# hand-verified against it, not derived, for the same reason that file
# gives: Go field names don't mechanically match these feature-name
# strings (e.g. ReinforceEnemyThreat -> "enemy_threat"), and there's no
# cross-language way to share the table, so a clearly-labeled parallel
# definition is the only real option here (same deliberate-duplication
# precedent as internal/simulation's dispatcher mirroring internal/risk's
# action shape instead of importing it).
#
# setup_reinforce is excluded entirely: out of scope for training data per
# 10_Bot_Weight_Tuning.md's Coverage section (manual game mode).
#
# A prior version of this table dropped "exposure_penalty" (attack) and
# "enemy_threat" (reinforce) entirely, diagnosed as collinear with other
# features (0.65 and 0.98 correlation respectively) and crushed to
# near-zero coefficients by ridge regression on chosen-only training data.
# That fix backfired badly in practice: dropping a feature from fitting
# means weights_export.py leaves it unset, so LoadWeights falls back to
# bot.DefaultWeights' hand-tuned value for it -- but that hand-tuned
# magnitude isn't on the same scale as this pipeline's fitted,
# standardized-then-rescaled coefficients, and mixing the two within one
# phase's score let the untouched weight dominate the ranking (0% win
# rate, 43% of games failing to complete in tournament eval). The real fix
# is on the data side instead -- see cmd/traindata's rowsFromEntries,
# which now emits one row per legal candidate per decision (not just the
# chosen one), addressing the collinearity's likely root cause (chosen-only
# rows selection-biased toward whatever combination of correlated features
# jointly produced the highest score) without dropping any feature here.
PHASE_FEATURES: dict[str, list[str]] = {
    "attack": [
        "army_advantage",
        "capture_probability",
        "expected_loss_cost",
        "completes_continent",
        "breaks_enemy_continent",
        "card_opportunity",
        "eliminates_player",
        "exposure_penalty",
    ],
    "reinforce": [
        "enemy_threat",
        "enemy_territory_count",
        "weakness",
        "continent_value",
        "concentration_penalty",
    ],
    "occupy": [
        "defense_coverage",
        "momentum",
        "momentum_surplus",
    ],
    "fortify": [
        "destination_threat",
        "continent_value",
        "source_exposure_cost",
    ],
}


@dataclass(frozen=True)
class PhaseFit:
    """One phase's fitted model, reduced to what weights_export.py needs."""

    phase: str
    coefficients: dict[str, float]  # feature_name -> fitted coefficient
    n_samples: int
    n_positive: int
    best_c: float


def fit_phase(df: pd.DataFrame, phase: str) -> PhaseFit:
    """Fit one phase's LogisticRegressionCV against df.

    Features are standardized (mean 0, unit variance) before fitting --
    raw feature scales vary wildly (capture_probability is 0-1;
    enemy_threat/weakness can be tens of armies), and L2 regularization is
    scale-sensitive: an unstandardized large-magnitude feature needs only
    a small coefficient to make the same contribution to the score, so
    regularization shrinks it more than a small-scale feature of equal
    true importance, independent of which one actually matters. Fitted
    coefficients are converted back to raw-signal units before returning
    (divide by that feature's stddev) -- since standardization is just
    x_std = (x_raw - mean) / std, w_std . x_std = (w_std / std) . x_raw
    - (a constant absorbed into the dropped intercept), so
    coefficient / std is exactly the equivalent raw-scale weight
    bot.Weights needs, not an approximation.

    l1_ratios=(0.0,) pins pure L2 (ridge) regularization explicitly rather
    than relying on a default that's mid-deprecation in the installed
    scikit-learn version; scoring='neg_log_loss' and
    use_legacy_attributes=False are likewise pinned to their upcoming
    defaults to avoid relying on soon-to-change behavior. cv=5 lets
    LogisticRegressionCV pick the regularization strength itself rather
    than us hand-guessing it.
    """
    feature_names = PHASE_FEATURES[phase]
    X, y, weights = phase_matrix(df, phase, feature_names)

    scaler = StandardScaler()
    X_scaled = scaler.fit_transform(X)

    model = LogisticRegressionCV(
        cv=5,
        max_iter=1000,
        l1_ratios=(0.0,),
        scoring="neg_log_loss",
        use_legacy_attributes=False,
    )
    model.fit(X_scaled, y, sample_weight=weights)

    raw_scale_coefficients = model.coef_[0] / scaler.scale_
    coefficients = dict(zip(feature_names, raw_scale_coefficients, strict=True))
    return PhaseFit(
        phase=phase,
        coefficients=coefficients,
        n_samples=len(y),
        n_positive=int(y.sum()),
        best_c=float(model.C_),
    )
