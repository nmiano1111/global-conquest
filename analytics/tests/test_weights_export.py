"""Unit tests for weights_export.py.

The (phase, feature_name) -> Go field mapping is the highest-risk part of
this whole pipeline: a wrong entry silently corrupts scored-v1's behavior
once loaded via LoadWeights, with no obvious symptom. These tests exist
specifically to catch that class of mistake.
"""

from __future__ import annotations

import json
from pathlib import Path

from global_conquest_analytics.fit import PHASE_FEATURES, PhaseFit
from global_conquest_analytics.weights_export import _WEIGHTS_FIELD, export_weights, weights_json

# The full, real field list from backend/internal/bot/weights.go, hand-copied
# once so this test doesn't need to (and can't) import the Go struct
# directly. Includes EndPhaseBias/FortifyEndTurnBias, which are never
# fitted (see fit.PHASE_FEATURES) but are still real Weights fields.
_REAL_WEIGHTS_FIELDS = {
    "ArmyAdvantage",
    "CaptureProbability",
    "ExpectedLossCost",
    "CompletesContinent",
    "BreaksEnemyContinent",
    "CardOpportunity",
    "EliminatesPlayer",
    "ExposurePenalty",
    "EndPhaseBias",
    "ReinforceEnemyThreat",
    "ReinforceEnemyTerritoryCount",
    "ReinforceWeakness",
    "ReinforceContinentValue",
    "ReinforceConcentrationPenalty",
    "OccupyDefenseCoverage",
    "OccupyMomentum",
    "OccupyMomentumSurplus",
    "FortifyDestinationThreat",
    "FortifyContinentValue",
    "FortifySourceExposureCost",
    "FortifyEndTurnBias",
}


def test_every_phase_feature_has_a_weights_field_mapping() -> None:
    for phase, names in PHASE_FEATURES.items():
        for name in names:
            assert (phase, name) in _WEIGHTS_FIELD, (
                f"{phase}/{name} has no entry in _WEIGHTS_FIELD"
            )


def test_every_mapped_field_is_a_real_weights_field() -> None:
    for (phase, name), field in _WEIGHTS_FIELD.items():
        assert field in _REAL_WEIGHTS_FIELDS, (
            f"{phase}/{name} maps to {field!r}, not a real bot.Weights field"
        )


def test_continent_value_maps_to_different_fields_per_phase() -> None:
    assert _WEIGHTS_FIELD[("reinforce", "continent_value")] == "ReinforceContinentValue"
    assert _WEIGHTS_FIELD[("fortify", "continent_value")] == "FortifyContinentValue"
    assert (
        _WEIGHTS_FIELD[("reinforce", "continent_value")]
        != _WEIGHTS_FIELD[("fortify", "continent_value")]
    )


def _mk_fit(phase: str, coefficients: dict[str, float]) -> PhaseFit:
    return PhaseFit(phase=phase, coefficients=coefficients, n_samples=10, n_positive=5, best_c=1.0)


def test_weights_json_builds_expected_shape() -> None:
    fits = [
        _mk_fit("attack", {"army_advantage": 1.5, "exposure_penalty": -0.8}),
        _mk_fit("reinforce", {"continent_value": 2.0}),
        _mk_fit("fortify", {"continent_value": 3.0}),
    ]
    result = weights_json(fits)
    assert result == {
        "ArmyAdvantage": 1.5,
        "ExposurePenalty": -0.8,
        "ReinforceContinentValue": 2.0,
        "FortifyContinentValue": 3.0,
    }


def test_export_weights_round_trips_through_json(tmp_path: Path) -> None:
    fits = [_mk_fit("occupy", {"momentum": 1.2, "momentum_surplus": 0.05})]
    output_path = tmp_path / "nested" / "weights.json"

    export_weights(fits, output_path)

    loaded = json.loads(output_path.read_text(encoding="utf-8"))
    assert loaded == {"OccupyMomentum": 1.2, "OccupyMomentumSurplus": 0.05}
