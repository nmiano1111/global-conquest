"""Unit tests for board_fit.py.

Doesn't exercise real cmd/tdtraindata output -- builds small synthetic
episodes directly (same style as test_td_fit.py), focused on the fit's
learnable-signal sanity check and the export shape (one JSON file,
matching backend/internal/bot.LoadBoardValue's expected fields) rather
than model quality on real games.
"""

from __future__ import annotations

import json
from pathlib import Path

import numpy as np
from global_conquest_analytics.board_fit import export_board_value, fit_board_value
from global_conquest_analytics.td_fit import Episode


def _synthetic_episodes(n_per_class: int = 30) -> list[Episode]:
    # Two features: the first is a real, learnable signal (higher values
    # correlate with winning); the second is pure noise. Multiple rows
    # per episode (turn boundaries) so flatten_episodes/sample-weighting
    # is actually exercised, not just a single-row-per-episode case.
    rng = np.random.default_rng(0)
    episodes = []
    for i in range(n_per_class):
        won_features = np.column_stack(
            [rng.uniform(5.0, 10.0, size=4), rng.uniform(-1.0, 1.0, size=4)]
        )
        episodes.append(Episode(game_id=f"g{i}", player_id="p0", features=won_features, won=True))
        lost_features = np.column_stack(
            [rng.uniform(0.0, 5.0, size=4), rng.uniform(-1.0, 1.0, size=4)]
        )
        episodes.append(
            Episode(game_id=f"g{i}", player_id="p1", features=lost_features, won=False)
        )
    return episodes


def test_fit_board_value_learns_a_positive_weight_on_the_real_signal() -> None:
    episodes = _synthetic_episodes()
    fit = fit_board_value(episodes, feature_names=["signal", "noise"])

    assert fit.weights.shape == (2,)
    assert fit.weights[0] > 0
    assert abs(fit.weights[0]) > abs(fit.weights[1])


def test_fit_board_value_standardizer_matches_training_data() -> None:
    episodes = _synthetic_episodes()
    fit = fit_board_value(episodes, feature_names=["signal", "noise"])

    all_features = np.concatenate([ep.features for ep in episodes], axis=0)
    assert np.allclose(fit.standardizer.mean, all_features.mean(axis=0))
    assert np.allclose(fit.standardizer.std, all_features.std(axis=0))


def test_export_board_value_writes_expected_shape(tmp_path: Path) -> None:
    episodes = _synthetic_episodes()
    fit = fit_board_value(episodes, feature_names=["signal", "noise"])
    output_path = tmp_path / "value.json"

    export_board_value(fit, output_path)

    payload = json.loads(output_path.read_text(encoding="utf-8"))
    assert set(payload.keys()) == {
        "weights",
        "intercept",
        "mean",
        "std",
        "attack_margin",
        "fortify_margin",
        "feature_names",
    }
    assert len(payload["weights"]) == 2
    assert len(payload["mean"]) == 2
    assert len(payload["std"]) == 2
    assert payload["feature_names"] == ["signal", "noise"]
    assert isinstance(payload["intercept"], float)
    # Margins are placeholders here -- fit separately by cmd/bvcalibrate
    # from real observed per-phase score deltas, not by this module.
    assert payload["attack_margin"] == 0.0
    assert payload["fortify_margin"] == 0.0


def test_export_board_value_creates_output_directory(tmp_path: Path) -> None:
    episodes = _synthetic_episodes()
    fit = fit_board_value(episodes, feature_names=["signal", "noise"])
    nested = tmp_path / "nested" / "dir" / "value.json"

    export_board_value(fit, nested)

    assert nested.exists()
