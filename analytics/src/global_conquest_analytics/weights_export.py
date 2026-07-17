"""Export fitted PhaseFits as a backend/internal/bot.Weights-shaped JSON file.

bot.Weights' fields are plain exported float64s with no JSON tags, so the
Go side's LoadWeights (internal/bot/weights_io.go) already round-trips
whatever JSON object we write here via encoding/json's default
reflection-based unmarshal, unmarshaled onto a copy of bot.DefaultWeights
-- meaning this file only needs to write the fields it actually fitted;
everything else (including EndPhaseBias/FortifyEndTurnBias, which are
never fitted -- see fit.PHASE_FEATURES) is filled in by LoadWeights
automatically.
"""

from __future__ import annotations

import json
from pathlib import Path

from global_conquest_analytics.fit import PhaseFit

# feature_name (per phase) -> bot.Weights Go field name. Reverse of
# fit.PHASE_FEATURES, hand-verified against internal/bot/weights.go's
# actual field names. "continent_value" maps to a *different* Go field
# depending on phase (ReinforceContinentValue vs FortifyContinentValue),
# so this is keyed by (phase, name), never name alone -- the Go side
# documents the exact same gotcha in cmd/traindata/extract.go.
_WEIGHTS_FIELD: dict[tuple[str, str], str] = {
    ("attack", "army_advantage"): "ArmyAdvantage",
    ("attack", "capture_probability"): "CaptureProbability",
    ("attack", "expected_loss_cost"): "ExpectedLossCost",
    ("attack", "completes_continent"): "CompletesContinent",
    ("attack", "breaks_enemy_continent"): "BreaksEnemyContinent",
    ("attack", "card_opportunity"): "CardOpportunity",
    ("attack", "eliminates_player"): "EliminatesPlayer",
    ("attack", "exposure_penalty"): "ExposurePenalty",
    ("reinforce", "enemy_threat"): "ReinforceEnemyThreat",
    ("reinforce", "enemy_territory_count"): "ReinforceEnemyTerritoryCount",
    ("reinforce", "weakness"): "ReinforceWeakness",
    ("reinforce", "continent_value"): "ReinforceContinentValue",
    ("reinforce", "concentration_penalty"): "ReinforceConcentrationPenalty",
    ("occupy", "defense_coverage"): "OccupyDefenseCoverage",
    ("occupy", "momentum"): "OccupyMomentum",
    ("occupy", "momentum_surplus"): "OccupyMomentumSurplus",
    ("fortify", "destination_threat"): "FortifyDestinationThreat",
    ("fortify", "continent_value"): "FortifyContinentValue",
    ("fortify", "source_exposure_cost"): "FortifySourceExposureCost",
}


def weights_json(fits: list[PhaseFit]) -> dict[str, float]:
    """Build the bot.Weights-shaped dict from a list of PhaseFits.

    Intercepts are deliberately not included -- a per-decision constant
    offset never affects which candidate ranks highest within one phase's
    own decision, so it carries no information bot.Weights can use.
    """
    result: dict[str, float] = {}
    for pf in fits:
        for name, coefficient in pf.coefficients.items():
            field = _WEIGHTS_FIELD[(pf.phase, name)]
            result[field] = coefficient
    return result


def export_weights(fits: list[PhaseFit], output_path: Path) -> None:
    """Write weights_json(fits) to output_path, creating parent dirs."""
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(json.dumps(weights_json(fits), indent=2), encoding="utf-8")
