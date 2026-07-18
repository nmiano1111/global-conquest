"""Fit a whole-board value function via TD(λ), and a same-data naive
baseline for comparison -- Phase 1 of the TD(λ) plan (see
project-docs/bot_player/proposals/Monte_Carlo_Evaluator_Roadmap_with_References.md
and the Jamie Carr GCN/TD(λ) paper it cites).

Reads backend/cmd/tdtraindata's JSONL output: one row per living player's
perspective at every completed turn boundary (a genuinely different grain
from cmd/traindata's per-candidate rows -- see that tool's own module
docstring). Grouped by (GameID, PlayerID) and sorted by Turn, each group
is one episode: a sequence of whole-board feature vectors ending in a
terminal reward (1.0 if PlayerID won, 0.0 otherwise).

Both models here are deliberately linear (V(x) = w . x + b) -- the point
of this phase is isolating whether the *objective* (TD(λ)'s bootstrapping
between temporally close states) fixes the erratic/uninformative-value
problem every "regress final Won directly" attempt this project has tried
hit, independent of model capacity. A bigger model (MLP, GCN) is a
separate, later question -- see 11_Learned_Board_Evaluation.md and
GCN_Strategy_Roadmap_with_References.md's own reasoning for the same
"isolate one variable" discipline.
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path

import numpy as np
import pandas as pd


@dataclass(frozen=True)
class Episode:
    """One (GameID, PlayerID)'s turn-boundary sequence, ordered by Turn."""

    game_id: str
    player_id: str
    features: np.ndarray  # shape (T, D), turn order
    won: bool


def load_episodes(paths: list[Path]) -> list[Episode]:
    """Load and concatenate one or more cmd/tdtraindata JSONL files into
    per-(GameID, PlayerID) episodes, each sorted by Turn.
    """
    frames = []
    for path in paths:
        df = pd.read_json(path, lines=True)
        frames.append(df)
    df = pd.concat(frames, ignore_index=True)
    df = df.sort_values("Turn")

    episodes = []
    for (game_id, player_id), group in df.groupby(["GameID", "PlayerID"], sort=False):
        features = np.array(group["Features"].tolist(), dtype=np.float64)
        won = bool(group["Won"].iloc[0])
        episodes.append(
            Episode(game_id=str(game_id), player_id=str(player_id), features=features, won=won)
        )
    return episodes


def load_feature_names(path: Path) -> list[str]:
    """Load the sidecar feature-name file cmd/tdtraindata writes alongside
    its JSONL output (see that tool's featureNamesPath/writeFeatureNames).
    """
    names: list[str] = json.loads(path.read_text(encoding="utf-8"))
    return names


def _augment(x: np.ndarray) -> np.ndarray:
    """Append a constant 1.0 feature -- folds the value function's bias
    term into the weight vector itself (w[-1]) rather than tracking a
    separate scalar, since TD(λ)'s eligibility-trace update treats every
    weight identically regardless of what it multiplies.
    """
    return np.concatenate([x, [1.0]])


@dataclass(frozen=True)
class Standardizer:
    """Feature standardization (z-score), fit once on training episodes
    and reused for any later episodes (val/test/live) -- never refit per
    call, matching the same train/apply split every other fit in this
    project already follows.
    """

    mean: np.ndarray
    std: np.ndarray

    def apply(self, features: np.ndarray) -> np.ndarray:
        result: np.ndarray = (features - self.mean) / self.std
        return result


def fit_standardizer(episodes: list[Episode]) -> Standardizer:
    """Compute per-feature mean/std across every row in episodes. Carr's
    own paper standardizes features for the identical reason this project
    needed it: large/unbounded input magnitudes destabilize training --
    "some input values can reach values close to 5... this could cause
    large gradient updates preventing the network from converging."
    """
    all_features = np.concatenate([ep.features for ep in episodes], axis=0)
    mean = all_features.mean(axis=0)
    std = all_features.std(axis=0)
    # A constant feature (e.g. always 0) would divide by zero; leaving std
    # at 1.0 there just means that feature stays at 0 post-standardization.
    std[std == 0] = 1.0
    return Standardizer(mean=mean, std=std)


def standardize_episodes(episodes: list[Episode], standardizer: Standardizer) -> list[Episode]:
    """Apply an already-fit Standardizer to episodes, returning new
    Episode objects (features replaced, game_id/player_id/won unchanged).
    """
    return [
        Episode(
            game_id=ep.game_id,
            player_id=ep.player_id,
            features=standardizer.apply(ep.features),
            won=ep.won,
        )
        for ep in episodes
    ]


def fit_td_lambda(
    episodes: list[Episode],
    lam: float = 0.8,
    alpha: float = 0.001,
    epochs: int = 3,
    seed: int = 0,
    td_error_clip: float = 5.0,
) -> np.ndarray:
    """Fit a linear value function via online TD(λ) with accumulating
    eligibility traces (Sutton & Barto's standard backward-view
    algorithm) -- gradient-based, but ∇V(x) = x for a linear V, so no
    autodiff framework is needed.

    For each episode, the eligibility trace resets to zero; at each step
    t, the bootstrap target is V(x_{t+1}) using the *current* weights
    (semi-gradient) for every non-terminal step, and the episode's
    terminal reward (1.0 if the player won, else 0.0) for the last step --
    this is the mechanism that lets the value function learn from
    temporally close transitions instead of only ever seeing "predict the
    final outcome from any point," which is what caused the erratic,
    flip-flopping predictions every prior fitting attempt in this project
    (and Jamie Carr's own TD(1) experiment) ran into.

    Callers should standardize episodes first (fit_standardizer +
    standardize_episodes on the *training* split only, then apply that
    same Standardizer to any other split) -- required, not optional: this
    project's feature vectors are ~400-dimensional with many mutually
    correlated features (the same collinearity documented throughout this
    project's fitting attempts), and linear TD(λ)'s stability in that
    regime is much more fragile than the per-parameter learning rate
    alone suggests. Concretely: even *with* standardized features,
    alpha=0.01 diverged to weights on the order of 1e33 within a couple of
    epochs on this project's real data; alpha=0.001 (the default here) was
    the first value that stayed bounded end to end. Carr's own reported
    alpha=0.5 was tuned for a deep network's very different (much
    smaller, non-linear) parameterization -- not a transferable value for
    a several-hundred-dimensional linear model.

    td_error_clip bounds |delta| before it's applied to the eligibility
    trace -- a standard TD/RL stabilization technique (e.g. DQN's reward
    clipping), kept on by default as defense in depth: it doesn't replace
    picking a stable alpha (a badly-diverging run will still produce a
    useless, saturated-at-the-clip-bound value function), but it turns a
    silent NaN blowup into a bounded, inspectable failure instead.
    """
    if not episodes:
        raise ValueError("fit_td_lambda: no episodes to train on")

    dim = episodes[0].features.shape[1] + 1  # +1 for the augmented bias term
    w = np.zeros(dim)
    rng = np.random.default_rng(seed)

    for _ in range(epochs):
        order = rng.permutation(len(episodes))
        for idx in order:
            ep = episodes[idx]
            reward = 1.0 if ep.won else 0.0
            t_count = ep.features.shape[0]
            e = np.zeros(dim)
            for t in range(t_count):
                x_t = _augment(ep.features[t])
                v_t = w @ x_t
                if t < t_count - 1:
                    x_next = _augment(ep.features[t + 1])
                    target = w @ x_next
                else:
                    target = reward
                delta = np.clip(target - v_t, -td_error_clip, td_error_clip)
                e = lam * e + x_t
                w = w + alpha * delta * e

    return w


def predict_td(w: np.ndarray, features: np.ndarray) -> np.ndarray:
    """Predict V(x) for every row in features (shape (N, D)) using a
    TD(λ)-fitted weight vector from fit_td_lambda.
    """
    augmented = np.concatenate([features, np.ones((features.shape[0], 1))], axis=1)
    result: np.ndarray = augmented @ w
    return result


def episode_sample_weights(episodes: list[Episode]) -> np.ndarray:
    """1 / (turn-boundary rows in that episode), broadcast per row -- same
    "don't let long games dominate" rationale as training_data's own
    sample_weights, recomputed here since this row grain (one per living
    player per turn boundary) differs from cmd/traindata's per-candidate
    rows.
    """
    return np.concatenate(
        [np.full(ep.features.shape[0], 1.0 / ep.features.shape[0]) for ep in episodes]
    )


def flatten_episodes(episodes: list[Episode]) -> tuple[np.ndarray, np.ndarray, np.ndarray]:
    """Stack every episode's rows into (X, y, weights) for a naive,
    non-sequential baseline fit -- X is (N, D), y is Won broadcast across
    every row of its episode, weights is episode_sample_weights.
    """
    X = np.concatenate([ep.features for ep in episodes], axis=0)
    y = np.concatenate([np.full(ep.features.shape[0], 1.0 if ep.won else 0.0) for ep in episodes])
    weights = episode_sample_weights(episodes)
    return X, y, weights
