"""Unit tests for td_fit.py.

Doesn't exercise real cmd/tdtraindata output -- builds small synthetic
episodes directly, focused on TD(λ)'s core mechanics (bootstrapping
between temporally close states, per-episode eligibility-trace reset)
and the data-loading/reshaping helpers, not model quality on real games.
"""

from __future__ import annotations

import json
from pathlib import Path

import numpy as np
import pytest
from global_conquest_analytics.td_fit import (
    Episode,
    episode_sample_weights,
    fit_standardizer,
    fit_td_lambda,
    flatten_episodes,
    load_episodes,
    load_feature_names,
    predict_td,
    standardize_episodes,
)


def test_fit_td_lambda_discriminates_winning_from_losing_trajectories() -> None:
    # A single scalar feature ("progress") that's consistently higher in
    # the winning episode's states than the losing one's -- TD(λ) should
    # learn a positive weight and predict higher value for higher
    # progress, and specifically end up predicting closer to 1 for the
    # winning episode's final state than the losing one's.
    won = Episode(
        game_id="g1", player_id="p0",
        features=np.array([[0.6], [0.8], [1.0]]), won=True,
    )
    lost = Episode(
        game_id="g1", player_id="p1",
        features=np.array([[0.0], [0.2], [0.4]]), won=False,
    )

    w = fit_td_lambda([won, lost], lam=0.8, alpha=0.3, epochs=200, seed=0)

    won_final = predict_td(w, won.features[-1:])[0]
    lost_final = predict_td(w, lost.features[-1:])[0]

    assert won_final > lost_final
    assert won_final > 0.5
    assert lost_final < 0.5


def test_fit_td_lambda_raises_on_empty_episodes() -> None:
    with pytest.raises(ValueError, match="no episodes"):
        fit_td_lambda([])


def test_fit_td_lambda_td_error_clip_prevents_blowup() -> None:
    # A deliberately unstable configuration (large alpha, long episode,
    # unstandardized large-magnitude features) that diverges to NaN
    # without clipping -- td_error_clip should keep it bounded instead.
    rng = np.random.default_rng(0)
    features = rng.uniform(0, 50, size=(200, 20))
    episodes = [Episode(game_id="g1", player_id="p0", features=features, won=True)]

    w = fit_td_lambda(episodes, lam=0.9, alpha=0.5, epochs=5, seed=0, td_error_clip=5.0)
    assert np.isfinite(w).all()


def test_fit_standardizer_and_standardize_episodes() -> None:
    episodes = [
        Episode(
            game_id="g1", player_id="p0",
            features=np.array([[0.0, 10.0], [2.0, 10.0]]), won=True,
        ),
        Episode(game_id="g1", player_id="p1", features=np.array([[4.0, 10.0]]), won=False),
    ]
    standardizer = fit_standardizer(episodes)

    # feature 0: values [0, 2, 4] -> mean=2, std=sqrt(8/3); feature 1 is
    # constant (10.0 everywhere) -- std forced to 1.0 to avoid div-by-zero,
    # so it standardizes to 0, not NaN.
    assert standardizer.mean[1] == pytest.approx(10.0)
    assert standardizer.std[1] == pytest.approx(1.0)

    standardized = standardize_episodes(episodes, standardizer)
    assert np.allclose(standardized[0].features[:, 1], 0.0)
    # game_id/player_id/won pass through unchanged.
    assert standardized[0].game_id == "g1"
    assert standardized[0].won is True
    assert standardized[1].won is False


def test_predict_td_matches_hand_computed_value() -> None:
    # w = [2.0, -1.0, bias=0.5]; features = [1.0, 1.0] -> 2*1 - 1*1 + 0.5 = 1.5
    w = np.array([2.0, -1.0, 0.5])
    features = np.array([[1.0, 1.0]])
    got = predict_td(w, features)
    assert got[0] == pytest.approx(1.5)


def test_load_episodes_groups_and_sorts_by_turn(tmp_path: Path) -> None:
    path = tmp_path / "data.jsonl"
    # Deliberately out of Turn order within the group.
    rows = [
        {"GameID": "g1", "PlayerID": "p0", "Turn": 2, "Won": True, "Features": [0.2]},
        {"GameID": "g1", "PlayerID": "p0", "Turn": 0, "Won": True, "Features": [0.0]},
        {"GameID": "g1", "PlayerID": "p0", "Turn": 1, "Won": True, "Features": [0.1]},
        {"GameID": "g1", "PlayerID": "p1", "Turn": 0, "Won": False, "Features": [0.5]},
    ]
    path.write_text("\n".join(json.dumps(r) for r in rows), encoding="utf-8")

    episodes = load_episodes([path])
    assert len(episodes) == 2

    p0 = next(e for e in episodes if e.player_id == "p0")
    assert p0.features.tolist() == [[0.0], [0.1], [0.2]]
    assert p0.won is True

    p1 = next(e for e in episodes if e.player_id == "p1")
    assert p1.features.tolist() == [[0.5]]
    assert p1.won is False


def test_load_feature_names(tmp_path: Path) -> None:
    path = tmp_path / "data.featurenames.json"
    path.write_text(json.dumps(["a", "b", "c"]), encoding="utf-8")
    assert load_feature_names(path) == ["a", "b", "c"]


def test_episode_sample_weights_sums_to_one_per_episode() -> None:
    episodes = [
        Episode(game_id="g1", player_id="p0", features=np.zeros((3, 2)), won=True),
        Episode(game_id="g1", player_id="p1", features=np.zeros((5, 2)), won=False),
    ]
    weights = episode_sample_weights(episodes)
    assert weights.shape == (8,)
    assert weights[:3].sum() == pytest.approx(1.0)
    assert weights[3:].sum() == pytest.approx(1.0)


def test_flatten_episodes_shapes_and_labels() -> None:
    episodes = [
        Episode(game_id="g1", player_id="p0", features=np.array([[1.0], [2.0]]), won=True),
        Episode(game_id="g1", player_id="p1", features=np.array([[3.0]]), won=False),
    ]
    X, y, weights = flatten_episodes(episodes)
    assert X.shape == (3, 1)
    assert y.tolist() == [1.0, 1.0, 0.0]
    assert weights.shape == (3,)
