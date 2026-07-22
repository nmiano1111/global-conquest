"""Fit a supervised Graph Convolutional Network (GCN) value function --
the "Supervised GCN" milestone in project-docs/bot_player/proposals/
GCN_Strategy_Roadmap_with_References.md, following Jamie Carr's "Using
Graph Convolutional Networks and TD(λ) to Play the Game of Risk"
(arXiv:2009.06355).

Motivation: BoardValueStrategy's flat linear model gave every territory
its own fixed, independent coefficient, so it could never express
"reinforce whoever is currently most threatened" as one general rule --
adding a per-territory enemy-threat feature didn't help either, because
that feature doesn't change based on which candidate is chosen (placing
my own armies doesn't change enemy armies anywhere), so it's an identical
constant across every candidate's score and cancels out of the
comparison. A GCN applies the *same* learned weights to every territory,
but each territory's output differs because it depends on that
territory's own neighbors via message passing -- exactly the capability
the linear model structurally lacked.

Deliberately keeps board_fit.py's exact training objective and data
(regress final Won from cmd/tdtraindata's turn-boundary rows) -- only the
model class changes from linear to GCN, isolating "does a weight-shared,
graph-structured model fix what the flat model couldn't" as the one
variable under test (same "change one variable at a time" discipline as
td_fit.py's Phase 1 validation).

Architecture matches the paper's Figure 1 (verified against the full
text via pypdf -- this environment's PDF-as-image reader needs
poppler-utils, which isn't installed, so pypdf was added instead):
GCN1 (out=60) -> GCN2 (out=30) -> flatten (not pool -- preserves
per-territory identity, matching the paper's "incorporate node specific
knowledge") -> FC2 (out=60); separately FC1 (out=60) processes global
features; concatenate FC2+FC1 -> FC3 (out=30) -> output (1, no
activation, ranking is all that matters same as every other model this
project has fit). ReLU after every layer except the output layer.

Because the classic board is a fixed graph (not a general variable-graph
problem PyTorch Geometric is built for), a graph-convolution layer here
is one dense matrix multiply against a precomputed, shared propagation
matrix P = D^-1/2(A+I)D^-1/2 (Kipf & Welling renormalization) -- avoids
PyTorch Geometric's heavier dependency chain and is trivial to hand-roll
identically in Go afterward (backend/internal/bot/gcnmodel).

Three deliberate divergences from the paper (see the implementation plan
for the full reasoning): (1) node/global features stay player-relative
(IsMine, not a fixed 6-slot owner one-hot) since this project supports
3-6 players, not the paper's fixed 6; (2) output is a single scalar per
perspective, not the paper's one-pass 6-player vector, same reason;
(3) training objective is supervised regression, not TD(λ) -- parked per
this project's own Phase 1 finding that representation, not TD(λ)'s
objective, was the lever that mattered.
"""

from __future__ import annotations

import copy
import json
from collections.abc import Callable
from dataclasses import dataclass
from pathlib import Path

import numpy as np
import torch
from torch import nn

from global_conquest_analytics.td_fit import (
    Episode,
    Standardizer,
    episode_sample_weights,
    fit_standardizer,
    standardize_episodes,
)


@dataclass(frozen=True)
class BoardSchema:
    """A board's static topology, matching backend/internal/tdstate's
    BoardSchema JSON export exactly (order + undirected edge list).
    """

    order: list[str]
    edges: list[tuple[int, int]]


def load_board_schema(path: Path) -> BoardSchema:
    """Load a cmd/tdtraindata-written *.boardschema.json sidecar."""
    data = json.loads(path.read_text(encoding="utf-8"))
    edges = [(int(e[0]), int(e[1])) for e in data["edges"]]
    return BoardSchema(order=list(data["order"]), edges=edges)


def propagation_matrix(schema: BoardSchema) -> np.ndarray:
    """Build the Kipf & Welling renormalized propagation matrix
    P = D^-1/2 (A+I) D^-1/2 for schema's fixed graph -- the same matrix
    both this module's training and backend/internal/bot/gcnmodel's
    inference multiply node features against for one graph-conv layer.
    """
    n = len(schema.order)
    a = np.eye(n)
    for i, j in schema.edges:
        a[i, j] = 1.0
        a[j, i] = 1.0
    degree = a.sum(axis=1)
    d_inv_sqrt = np.diag(1.0 / np.sqrt(degree))
    result: np.ndarray = d_inv_sqrt @ a @ d_inv_sqrt
    return result


def node_feature_dim(feature_names: list[str], schema: BoardSchema) -> int:
    """Count of tdstate.TerritoryFeatures' flattened columns per
    territory (is_mine, army_fraction, continent one-hot,
    is_continent_border, enemy_threat_fraction) -- derived from
    feature_names rather than hardcoded, so a future map/feature-set
    change doesn't silently desync this reshape from tdstate.Flatten()'s
    actual layout. Counts how many "territory_<first territory>_continent_*"
    columns exist to infer the continent one-hot's width -- the only
    piece that varies by board; the rest of the per-territory stride is
    fixed regardless of continent count.
    """
    first = schema.order[0]
    num_continents = sum(1 for n in feature_names if n.startswith(f"territory_{first}_continent_"))
    return 2 + num_continents + 2


@dataclass(frozen=True)
class ReshapedEpisode:
    """One episode's features, reshaped from td_fit.Episode's flat
    per-row vectors into (node matrix, global vector) pairs for GCN
    consumption -- game_id/player_id/won pass through unchanged.
    """

    game_id: str
    player_id: str
    node_features: np.ndarray  # shape (T, num_nodes, node_dim)
    global_features: np.ndarray  # shape (T, global_dim)
    won: bool


def reshape_episodes(
    episodes: list[Episode], feature_names: list[str], schema: BoardSchema
) -> list[ReshapedEpisode]:
    """Reshape every episode's flat Features rows into (node matrix,
    global vector) pairs, using tdstate.Flatten()'s known, fixed layout:
    per-territory blocks (stride node_feature_dim) in schema.order order,
    followed by the global tail.
    """
    num_territories = len(schema.order)
    dim = node_feature_dim(feature_names, schema)
    territory_block_width = num_territories * dim

    reshaped = []
    for ep in episodes:
        node_block = ep.features[:, :territory_block_width]
        global_block = ep.features[:, territory_block_width:]
        node_features = node_block.reshape(ep.features.shape[0], num_territories, dim)
        reshaped.append(
            ReshapedEpisode(
                game_id=ep.game_id,
                player_id=ep.player_id,
                node_features=node_features,
                global_features=global_block,
                won=ep.won,
            )
        )
    return reshaped


class GCNValueNetwork(nn.Module):
    """Matches the paper's Figure 1 architecture exactly (see this
    module's docstring for layer sizes/flow).
    """

    def __init__(
        self,
        node_dim: int,
        global_dim: int,
        num_nodes: int,
        gcn1_out: int = 60,
        gcn2_out: int = 30,
        fc1_out: int = 60,
        fc2_out: int = 60,
        fc3_out: int = 30,
    ) -> None:
        super().__init__()
        self.gcn1 = nn.Linear(node_dim, gcn1_out)
        self.gcn2 = nn.Linear(gcn1_out, gcn2_out)
        self.fc1 = nn.Linear(global_dim, fc1_out)
        self.fc2 = nn.Linear(num_nodes * gcn2_out, fc2_out)
        self.fc3 = nn.Linear(fc1_out + fc2_out, fc3_out)
        self.output = nn.Linear(fc3_out, 1)

    def forward(
        self, node_features: torch.Tensor, global_features: torch.Tensor, p: torch.Tensor
    ) -> torch.Tensor:
        h1 = torch.relu(torch.einsum("ij,bjk->bik", p, self.gcn1(node_features)))
        h2 = torch.relu(torch.einsum("ij,bjk->bik", p, self.gcn2(h1)))
        board_embedding = h2.reshape(h2.size(0), -1)
        fc2_out = torch.relu(self.fc2(board_embedding))
        fc1_out = torch.relu(self.fc1(global_features))
        combined = torch.cat([fc2_out, fc1_out], dim=-1)
        fc3_out = torch.relu(self.fc3(combined))
        result: torch.Tensor = self.output(fc3_out).squeeze(-1)
        return result


@dataclass(frozen=True)
class GCNFit:
    """A fitted GCN value function, ready for export."""

    model: GCNValueNetwork
    standardizer: Standardizer
    schema: BoardSchema
    feature_names: list[str]


def fit_gcn(
    episodes: list[Episode],
    feature_names: list[str],
    schema: BoardSchema,
    epochs: int = 20,
    lr: float = 1e-3,
    seed: int = 0,
) -> GCNFit:
    """Fit a GCNValueNetwork predicting Won from every turn-boundary row
    across episodes, via standard binary cross-entropy, full-batch --
    same standardization/sample-weighting pipeline as
    board_fit.fit_board_value (fit_standardizer/standardize_episodes/
    episode_sample_weights), only the model class differs.
    """
    torch.manual_seed(seed)

    standardizer = fit_standardizer(episodes)
    standardized = standardize_episodes(episodes, standardizer)
    reshaped = reshape_episodes(standardized, feature_names, schema)
    weights = episode_sample_weights(episodes)

    node_features = np.concatenate([ep.node_features for ep in reshaped], axis=0)
    global_features = np.concatenate([ep.global_features for ep in reshaped], axis=0)
    y = np.concatenate(
        [np.full(ep.node_features.shape[0], 1.0 if ep.won else 0.0) for ep in reshaped]
    )

    node_dim = node_features.shape[2]
    global_dim = global_features.shape[1]
    num_nodes = node_features.shape[1]

    model = GCNValueNetwork(node_dim=node_dim, global_dim=global_dim, num_nodes=num_nodes)
    p = torch.tensor(propagation_matrix(schema), dtype=torch.float32)

    node_t = torch.tensor(node_features, dtype=torch.float32)
    global_t = torch.tensor(global_features, dtype=torch.float32)
    y_t = torch.tensor(y, dtype=torch.float32)
    w_t = torch.tensor(weights, dtype=torch.float32)

    optimizer = torch.optim.Adam(model.parameters(), lr=lr)
    loss_fn = nn.BCEWithLogitsLoss(reduction="none")

    for _ in range(epochs):
        optimizer.zero_grad()
        logits = model(node_t, global_t, p)
        loss = (loss_fn(logits, y_t) * w_t).sum() / w_t.sum()
        loss.backward()
        optimizer.step()

    return GCNFit(
        model=model, standardizer=standardizer, schema=schema, feature_names=feature_names
    )


def fit_gcn_td(
    episodes: list[Episode],
    feature_names: list[str],
    schema: BoardSchema,
    epochs: int = 1,
    alpha: float = 1e-3,
    lam: float = 0.8,
    td_error_clip: float = 5.0,
    seed: int = 0,
    target_sync_episodes: int = 1,
    on_progress: Callable[[int, int, int, int], None] | None = None,
) -> GCNFit:
    """Fit a GCNValueNetwork via semi-gradient TD(λ) with eligibility
    traces over its full parameter set -- the GCN generalization of
    td_fit.fit_td_lambda's already-validated linear-model algorithm
    (there, ∇V(x)=x has a closed form; here the same role is played by
    autograd). See this module's own docstring point (3) for why this
    exists: td_fit.py documented that naive "regress every state to the
    game's final outcome" (fit_gcn's own BCEWithLogitsLoss objective)
    produces "erratic, flip-flopping predictions," and that TD(λ)'s
    bootstrapping between temporally close states fixes it -- a finding
    that was validated for a linear model but never re-applied to the
    GCN, which has trained on the same abandoned objective the whole
    time. fit_gcn is left untouched; this is a new, separate objective a
    caller opts into (see cli.py's --objective flag), so both remain
    directly comparable rather than one silently replacing the other.

    Unlike fit_gcn's single vectorized full-batch pass, this must process
    one episode at a time, sequentially within each episode: the
    eligibility trace resets per episode, and each step's bootstrap
    target depends on the *current* weights after every prior update
    within that same episode. This is dramatically more expensive per
    epoch than fit_gcn's batched BCE loop -- epochs defaults to 1, not
    20; time a small run before committing to a large one (see cli.py's
    fit-gcn --objective td help text).

    Value stays in raw, unbounded model-output space (no sigmoid) --
    matching GCNValueNetwork.forward's existing no-activation output and
    fit_td_lambda's own raw-value convention, and consistent with how the
    Go side actually consumes the score (gcnmodel.Model.Score, no
    activation, ranking only). td_error_clip bounds |target - V(s_t)|
    before it's applied, same defense-in-depth against divergence as
    fit_td_lambda.

    Update mechanism is a direct generalization of fit_td_lambda's own
    plain-SGD eligibility-trace update (param += alpha * delta * trace),
    using autograd for ∇_θV(s_t) in place of the linear model's closed-form
    x. alpha defaults to 1e-3, matching fit_td_lambda's own validated
    default. An earlier version of this function instead fed the
    trace-shaped update into the same Adam optimizer fit_gcn trains this
    exact network with successfully (param.grad = -(delta * trace),
    optimizer.step()), reasoning that Adam's adaptive scaling would be
    lower-risk than hand-tuning a raw alpha for a much larger nonlinear
    model than fit_td_lambda's. In practice that was the opposite: it
    reliably collapsed the network to a near-constant output (every state
    scored ~identically) after a few epochs on the real training set,
    while plain SGD at fit_td_lambda's own alpha does not, on the same
    data. Adam's own momentum accumulation stacking with the trace's
    already-temporal accumulation is the most likely cause of that
    collapse; the empirical result (this module's test suite, and a real
    per-territory discrimination check against actual training data) is
    what settled it, not a theoretical argument either way.

    Bootstrap targets come from a separate target_model, a copy of model
    re-synced every target_sync_episodes episodes (default every
    episode, i.e. frozen for exactly one episode at a time rather than
    updated every single step within it) -- the standard DQN-style fix
    for bootstrapping off a constantly-moving target. This alone did not
    prevent the Adam-based collapse above at any sync interval tried (1
    or 50 episodes); it's retained here because the working plain-SGD
    configuration was validated with it in place, not because it was
    independently proven necessary once Adam was removed.

    on_progress, if set, is called after every episode as
    on_progress(epoch, epochs, episode_number, total_episodes) (epoch and
    episode_number are both 1-indexed) -- a purely additive side-channel
    that never influences training, the same pattern as
    cmd/tournament's onProgress/internal/simulation.Config's
    OnTurnBoundary on the Go side. Left unset, it costs nothing extra;
    cli.py's fit-gcn --objective td supplies a throttled, print-based one,
    since a full run here is sequential and can take a long time with no
    other feedback otherwise.
    """
    torch.manual_seed(seed)

    standardizer = fit_standardizer(episodes)
    standardized = standardize_episodes(episodes, standardizer)
    reshaped = reshape_episodes(standardized, feature_names, schema)

    node_dim = reshaped[0].node_features.shape[2]
    global_dim = reshaped[0].global_features.shape[1]
    num_nodes = reshaped[0].node_features.shape[1]

    model = GCNValueNetwork(node_dim=node_dim, global_dim=global_dim, num_nodes=num_nodes)
    target_model = copy.deepcopy(model)
    for target_param in target_model.parameters():
        target_param.requires_grad_(False)
    p = torch.tensor(propagation_matrix(schema), dtype=torch.float32)

    rng = np.random.default_rng(seed)
    episodes_seen = 0
    for epoch_idx in range(epochs):
        order = rng.permutation(len(reshaped))
        for episode_number, idx in enumerate(order, start=1):
            if episodes_seen % target_sync_episodes == 0:
                target_model.load_state_dict(model.state_dict())
            episodes_seen += 1

            ep = reshaped[idx]
            reward = 1.0 if ep.won else 0.0
            t_count = ep.node_features.shape[0]
            traces = {name: torch.zeros_like(param) for name, param in model.named_parameters()}

            for t in range(t_count):
                node_t = torch.tensor(ep.node_features[t : t + 1], dtype=torch.float32)
                global_t = torch.tensor(ep.global_features[t : t + 1], dtype=torch.float32)

                if t < t_count - 1:
                    with torch.no_grad():
                        next_node = torch.tensor(
                            ep.node_features[t + 1 : t + 2], dtype=torch.float32
                        )
                        next_global = torch.tensor(
                            ep.global_features[t + 1 : t + 2], dtype=torch.float32
                        )
                        target = target_model(next_node, next_global, p).item()
                else:
                    target = reward

                model.zero_grad()
                v_t = model(node_t, global_t, p)
                v_t.backward()

                delta = float(np.clip(target - v_t.item(), -td_error_clip, td_error_clip))
                with torch.no_grad():
                    for name, param in model.named_parameters():
                        if param.grad is None:
                            continue
                        traces[name] = lam * traces[name] + param.grad
                        param.data += alpha * delta * traces[name]

            if on_progress is not None:
                on_progress(epoch_idx + 1, epochs, episode_number, len(reshaped))

    return GCNFit(
        model=model, standardizer=standardizer, schema=schema, feature_names=feature_names
    )


def export_gcn(fit: GCNFit, output_path: Path) -> None:
    """Write fit as JSON matching backend/internal/bot/gcnmodel's
    expected shape: each layer's weight/bias (nn.Linear's weight is
    [out_features, in_features]; exported as-is, Go's forward pass
    matches this convention), the standardizer's mean/std, the
    propagation matrix, and board/feature metadata. attack_margin/
    fortify_margin are 0.0 placeholders -- run cmd/bvcalibrate against
    output_path before live use, same as board_fit.py's export.
    """
    output_path.parent.mkdir(parents=True, exist_ok=True)
    sd = fit.model.state_dict()

    def layer(name: str) -> dict[str, list]:
        return {
            "weight": sd[f"{name}.weight"].tolist(),
            "bias": sd[f"{name}.bias"].tolist(),
        }

    payload = {
        "gcn1": layer("gcn1"),
        "gcn2": layer("gcn2"),
        "fc1": layer("fc1"),
        "fc2": layer("fc2"),
        "fc3": layer("fc3"),
        "output": layer("output"),
        "mean": fit.standardizer.mean.tolist(),
        "std": fit.standardizer.std.tolist(),
        "propagation_matrix": propagation_matrix(fit.schema).tolist(),
        "board_order": fit.schema.order,
        "feature_names": fit.feature_names,
        "attack_margin": 0.0,
        "fortify_margin": 0.0,
    }
    output_path.write_text(json.dumps(payload), encoding="utf-8")


__all__ = [
    "BoardSchema",
    "GCNFit",
    "GCNValueNetwork",
    "ReshapedEpisode",
    "export_gcn",
    "fit_gcn",
    "fit_gcn_td",
    "load_board_schema",
    "node_feature_dim",
    "propagation_matrix",
    "reshape_episodes",
]
