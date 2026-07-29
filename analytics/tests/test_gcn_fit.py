"""Unit tests for gcn_fit.py.

Doesn't exercise real cmd/tdtraindata output -- builds a tiny synthetic
3-territory/1-continent board schema and matching synthetic episodes
directly (same style as test_td_fit.py/test_board_fit.py), focused on the
reshape/propagation-matrix mechanics and the export shape rather than
model quality on real games.
"""

from __future__ import annotations

import json
from pathlib import Path

import numpy as np
import pytest
import torch
from global_conquest_analytics.gcn_fit import (
    BoardSchema,
    export_gcn,
    fit_gcn,
    fit_gcn_td,
    load_board_schema,
    node_feature_dim,
    propagation_matrix,
    reshape_episodes,
)
from global_conquest_analytics.td_fit import Episode, standardize_episodes

# A 3-node path graph A-B-C, 1 continent, matching tdstate's per-territory
# stride (is_mine, army_fraction, continent one-hot x1, is_continent_border,
# enemy_threat_fraction = 5 dims) + 2 global dims = 17 total columns.
_SCHEMA = BoardSchema(order=["A", "B", "C"], edges=[(0, 1), (1, 2)])
_FEATURE_NAMES = []
for _t in _SCHEMA.order:
    _FEATURE_NAMES += [
        f"territory_{_t}_is_mine",
        f"territory_{_t}_army_fraction",
        f"territory_{_t}_continent_c1",
        f"territory_{_t}_is_continent_border",
        f"territory_{_t}_enemy_threat_fraction",
    ]
_FEATURE_NAMES += ["global1", "global2"]


def test_propagation_matrix_matches_hand_computed_values() -> None:
    p = propagation_matrix(_SCHEMA)
    # A-I: A has itself + B = degree 2; B has itself + A + C = degree 3;
    # C has itself + B = degree 2.
    assert p.shape == (3, 3)
    d = np.array([2.0, 3.0, 2.0])
    want_ab = 1.0 / np.sqrt(d[0] * d[1])
    assert p[0, 1] == pytest.approx(want_ab)
    assert p[1, 0] == pytest.approx(want_ab)
    want_aa = 1.0 / d[0]
    assert p[0, 0] == pytest.approx(want_aa)
    assert p[0, 2] == 0.0  # A and C aren't adjacent


def test_node_feature_dim_counts_continent_columns() -> None:
    assert node_feature_dim(_FEATURE_NAMES, _SCHEMA) == 5


def test_reshape_episodes_splits_node_and_global_blocks() -> None:
    features = np.arange(2 * 17, dtype=np.float64).reshape(2, 17)
    ep = Episode(game_id="g1", player_id="p0", features=features, won=True)

    reshaped = reshape_episodes([ep], _FEATURE_NAMES, _SCHEMA)

    assert len(reshaped) == 1
    r = reshaped[0]
    assert r.node_features.shape == (2, 3, 5)
    assert r.global_features.shape == (2, 2)
    # Row 0's flat vector is [0..16]; territory C (index 2) starts at
    # column 10, so its 5-dim block is [10, 11, 12, 13, 14].
    assert r.node_features[0, 2].tolist() == [10.0, 11.0, 12.0, 13.0, 14.0]
    # The last 2 columns [15, 16] are the global block.
    assert r.global_features[0].tolist() == [15.0, 16.0]
    assert r.game_id == "g1"
    assert r.won is True


def _synthetic_episodes(n_per_class: int = 15) -> list[Episode]:
    rng = np.random.default_rng(0)
    episodes = []
    for i in range(n_per_class):
        won_features = rng.uniform(0.5, 1.0, size=(3, 17))
        episodes.append(Episode(game_id=f"g{i}", player_id="p0", features=won_features, won=True))
        lost_features = rng.uniform(0.0, 0.5, size=(3, 17))
        episodes.append(
            Episode(game_id=f"g{i}", player_id="p1", features=lost_features, won=False)
        )
    return episodes


def test_fit_gcn_produces_finite_weights_and_predictions() -> None:
    episodes = _synthetic_episodes()
    fit = fit_gcn(episodes, _FEATURE_NAMES, _SCHEMA, epochs=5)

    for param in fit.model.parameters():
        assert np.isfinite(param.detach().numpy()).all()


def test_fit_gcn_td_discriminates_winning_from_losing_trajectories() -> None:
    # Same idea as test_td_fit.py's own
    # test_fit_td_lambda_discriminates_winning_from_losing_trajectories,
    # generalized to the GCN's node/global feature layout: every feature
    # climbs steadily toward 1.0 in the winning episode, decays steadily
    # toward 0.0 in the losing one.
    won = Episode(
        game_id="g1",
        player_id="p0",
        features=np.tile(np.array([[0.2], [0.5], [0.8], [1.0]]), (1, 17)),
        won=True,
    )
    lost = Episode(
        game_id="g1",
        player_id="p1",
        features=np.tile(np.array([[0.8], [0.5], [0.2], [0.0]]), (1, 17)),
        won=False,
    )

    fit = fit_gcn_td([won, lost], _FEATURE_NAMES, _SCHEMA, epochs=200, alpha=0.05, seed=0)

    standardized = standardize_episodes([won, lost], fit.standardizer)
    reshaped = reshape_episodes(standardized, _FEATURE_NAMES, _SCHEMA)
    p = torch.tensor(propagation_matrix(_SCHEMA), dtype=torch.float32)

    def final_score(index: int) -> float:
        ep = reshaped[index]
        node_t = torch.tensor(ep.node_features[-1:], dtype=torch.float32)
        global_t = torch.tensor(ep.global_features[-1:], dtype=torch.float32)
        with torch.no_grad():
            return float(fit.model(node_t, global_t, p).item())

    won_final = final_score(0)
    lost_final = final_score(1)

    assert won_final > lost_final


def test_fit_gcn_td_produces_finite_weights() -> None:
    episodes = _synthetic_episodes()
    fit = fit_gcn_td(episodes, _FEATURE_NAMES, _SCHEMA, epochs=1)

    for param in fit.model.parameters():
        assert np.isfinite(param.detach().numpy()).all()


def test_export_gcn_writes_expected_shape(tmp_path: Path) -> None:
    episodes = _synthetic_episodes()
    fit = fit_gcn(episodes, _FEATURE_NAMES, _SCHEMA, epochs=3)
    output_path = tmp_path / "gcn.json"

    export_gcn(fit, output_path)

    payload = json.loads(output_path.read_text(encoding="utf-8"))
    for layer_name in ("gcn1", "gcn2", "fc1", "fc2", "fc3", "output"):
        assert "weight" in payload[layer_name]
        assert "bias" in payload[layer_name]
    assert len(payload["mean"]) == 17
    assert len(payload["std"]) == 17
    assert len(payload["propagation_matrix"]) == 3
    assert len(payload["propagation_matrix"][0]) == 3
    assert payload["board_order"] == ["A", "B", "C"]
    assert payload["feature_names"] == _FEATURE_NAMES
    assert payload["attack_margin"] == 0.0
    assert payload["fortify_margin"] == 0.0


def test_export_gcn_creates_output_directory(tmp_path: Path) -> None:
    episodes = _synthetic_episodes()
    fit = fit_gcn(episodes, _FEATURE_NAMES, _SCHEMA, epochs=2)
    nested = tmp_path / "nested" / "dir" / "gcn.json"

    export_gcn(fit, nested)

    assert nested.exists()


def test_load_board_schema_round_trips(tmp_path: Path) -> None:
    path = tmp_path / "board.json"
    path.write_text(
        json.dumps({"order": ["A", "B", "C"], "edges": [[0, 1], [1, 2]]}), encoding="utf-8"
    )

    schema = load_board_schema(path)

    assert schema.order == ["A", "B", "C"]
    assert schema.edges == [(0, 1), (1, 2)]
