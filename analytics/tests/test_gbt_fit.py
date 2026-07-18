"""Unit tests for gbt_fit.py.

Doesn't exercise real cmd/traindata output -- builds a small synthetic
DataFrame directly in the shape phase_matrix expects, focused on the
export shape (one JSON file per phase, parseable, matching
gbtmodel.LoadModel's expected filenames) rather than model quality.
"""

from __future__ import annotations

import json
from pathlib import Path

import numpy as np
import pandas as pd
from global_conquest_analytics.gbt_fit import export_gbt, fit_phase_gbt


def _synthetic_df(phase: str, feature_names: list[str], n: int = 200) -> pd.DataFrame:
    rng = np.random.default_rng(0)
    primary = rng.uniform(0, 10, size=n)
    # Real, learnable signal: higher values on the first feature correlate
    # with winning.
    won = primary > rng.uniform(0, 10, size=n)

    data = {
        "GameID": [f"game-{i % 10}" for i in range(n)],
        "PlayerID": ["p0"] * n,
        "CommandIndex": list(range(n)),
        "Phase": [phase] * n,
        "Won": won,
    }
    data[f"feature_{feature_names[0]}"] = primary
    for name in feature_names[1:]:
        data[f"feature_{name}"] = rng.uniform(0, 5, size=n)
    return pd.DataFrame(data)


def _synthetic_occupy_df(n: int = 200) -> pd.DataFrame:
    return _synthetic_df("occupy", ["momentum_surplus", "defense_coverage", "momentum"], n)


def test_fit_phase_gbt_trains_a_booster() -> None:
    df = _synthetic_occupy_df()
    fit = fit_phase_gbt(df, "occupy", num_boost_round=5)
    dump = fit.booster.dump_model()
    assert len(dump["tree_info"]) == 5


def test_fit_phase_gbt_omits_threshold_for_occupy_and_reinforce() -> None:
    df = _synthetic_occupy_df()
    fit = fit_phase_gbt(df, "occupy", num_boost_round=5)
    assert fit.end_phase_threshold is None


def test_fit_phase_gbt_computes_threshold_for_attack_and_fortify() -> None:
    from global_conquest_analytics.fit import PHASE_FEATURES

    df = _synthetic_df("attack", PHASE_FEATURES["attack"])
    fit = fit_phase_gbt(df, "attack", num_boost_round=5)
    assert fit.end_phase_threshold is not None
    assert 0.0 <= fit.end_phase_threshold <= 1.0


def test_export_gbt_writes_one_json_file_per_phase(tmp_path: Path) -> None:
    df = _synthetic_occupy_df()
    fit = fit_phase_gbt(df, "occupy", num_boost_round=5)

    export_gbt({"occupy": fit}, tmp_path)

    path = tmp_path / "occupy.json"
    assert path.exists()
    dump = json.loads(path.read_text(encoding="utf-8"))
    assert "tree_info" in dump
    assert len(dump["tree_info"]) == 5
    assert "end_phase_threshold" not in dump


def test_export_gbt_embeds_threshold_for_attack(tmp_path: Path) -> None:
    from global_conquest_analytics.fit import PHASE_FEATURES

    df = _synthetic_df("attack", PHASE_FEATURES["attack"])
    fit = fit_phase_gbt(df, "attack", num_boost_round=5)

    export_gbt({"attack": fit}, tmp_path)

    dump = json.loads((tmp_path / "attack.json").read_text(encoding="utf-8"))
    assert dump["end_phase_threshold"] == fit.end_phase_threshold


def test_export_gbt_creates_output_directory(tmp_path: Path) -> None:
    df = _synthetic_occupy_df()
    fit = fit_phase_gbt(df, "occupy", num_boost_round=3)
    nested = tmp_path / "nested" / "dir"

    export_gbt({"occupy": fit}, nested)

    assert (nested / "occupy.json").exists()
