# The GCN Value Function: Architecture

> Part of the GCN value function reference set — see [`GCN_Value_Function_Overview.md`](GCN_Value_Function_Overview.md) for the full pipeline and links to the companion documents (training data, training).

## Graph Convolutional Networks, briefly

A regular fully-connected layer treats its input as an unstructured vector — nothing about the input's *position* matters to how the layer processes it. A **graph convolutional network** instead treats the input as a set of nodes connected by a fixed graph, and every layer mixes each node's features with its neighbors' features before applying a learned transform. The same weights are applied at every node (weight sharing), but each node's *output* differs because it depends on that node's own local neighborhood.

Concretely, one graph-conv layer (the Kipf & Welling formulation this project uses) is:

```
H' = ReLU(P · (H · W + b))
```

where `H` is the current per-node feature matrix, `W`/`b` are a normal linear layer's learned weight/bias (shared across every node), and `P` is a fixed **propagation matrix** built once from the graph's adjacency:

```
P = D^(-1/2) (A + I) D^(-1/2)
```

`A` is the adjacency matrix (1 where two nodes are connected), `I` adds a self-loop (so a node's own features survive the aggregation, not just its neighbors'), and `D` is the diagonal degree matrix, used to renormalize so high-degree nodes don't dominate. Multiplying by `P` is exactly "each node's new representation is a normalized average of itself and its immediate neighbors' current representations" — then the shared `W`/`b`/`ReLU` transforms that mixed representation.

This project builds `P` once, straight from the board's static adjacency (`gcn_fit.py`):

```python
def propagation_matrix(schema: BoardSchema) -> np.ndarray:
    n = len(schema.order)
    a = np.eye(n)                       # I: self-loops
    for i, j in schema.edges:
        a[i, j] = 1.0                   # A: adjacency (undirected)
        a[j, i] = 1.0
    degree = a.sum(axis=1)
    d_inv_sqrt = np.diag(1.0 / np.sqrt(degree))
    return d_inv_sqrt @ a @ d_inv_sqrt  # D^-1/2 (A+I) D^-1/2
```

**Why this matters for Risk specifically**: the board's own doc comment explains what motivated switching from a flat linear model to this. A flat linear model gives every territory its own independent coefficient — it can express "Alaska matters this much," but it structurally cannot express a *rule* like "reinforce whichever territory is currently most threatened," because that's a relationship between a territory and its neighbors, not a fixed per-territory constant. A graph-conv layer's shared weights, combined with the propagation step, let the network apply the *same* learned rule everywhere, with each territory's output shaped by its own actual local situation.

**Receptive field**: stacking `N` graph-conv layers lets a node's representation depend on information up to `N` hops away (each layer mixes in one more hop of neighbors). This project's network uses exactly two graph-conv layers, so **a node's representation depends only on itself and territories within 2 hops** — anything happening 3+ hops away only reaches a node indirectly, through whatever the flatten + fully-connected layers do afterward (see "Network architecture" below). This is a real, concrete limit on what the graph-conv layers alone can "see" — relevant context for the individual-enemies question (`GCN_Value_Function_Overview.md`).

**A fixed graph, not a general one**: PyTorch Geometric and similar libraries are built for graphs that change shape between examples. The classic Risk board never does — same 42 territories, same adjacency, every game — so a graph-conv layer here is just a dense matrix multiply against one precomputed `P`, which is simple enough to hand-roll identically in Go for inference without any tensor-library dependency at serving time.

## Network architecture

`GCNValueNetwork` (`gcn_fit.py`) and `gcnmodel.Model` (`gcnmodel.go`) are two independent implementations of the *same* forward pass — one for training (PyTorch, autograd), one for serving (hand-written Go, no dependencies). They must stay byte-for-byte equivalent; the export/import JSON format is the contract between them.

Layer by layer, given a state's per-territory node matrix and global feature vector:

```
node features ──▶ GCN1 (node_dim → 60) ──▶ propagate(P) ──▶ ReLU ──┐
                                                                     │
                   GCN2 (60 → 30) ──▶ propagate(P) ──▶ ReLU ────────┘
                                                                     │
                                                              flatten (NOT pooled)
                                                                     │
                                                        FC2 (num_nodes×30 → 60) ──▶ ReLU ──┐
                                                                                             │
global features ──▶ FC1 (global_dim → 60) ──▶ ReLU ─────────────────────────────────────────┤
                                                                                             │
                                                                          concat ──▶ FC3 (120 → 30) ──▶ ReLU
                                                                                                            │
                                                                                          output (30 → 1, no activation)
```

A few choices worth understanding, not just the shape:

- **Flatten, not pool.** After the two graph-conv layers, most GCN architectures would pool (e.g. average) across nodes into one fixed-size vector, discarding *which* node contributed what. This network instead concatenates every territory's final embedding in a fixed order, preserving per-territory identity all the way into `FC2` — matching the paper's own description of wanting to "incorporate node specific knowledge." This is also why the network's input size is architecture-specific to one exact board (42 territories) rather than generalizing to any graph.
- **No activation on the output.** The final layer produces one raw scalar with no sigmoid/clamp. Every value function in this project is used purely for *ranking* candidates against each other, never as a calibrated probability, so an unbounded score is fine — and matches how TD(λ) training treats the target (`GCN_Value_Function_Training.md`).
- **Standardization before the forward pass.** Every input feature is z-score standardized (`(x - mean) / std`, with `std` from training data) before hitting `GCN1`/`FC1`. The paper's own reasoning: unstandardized feature magnitudes ("some input values can reach values close to 5") destabilize training with large gradient updates.
- **The propagation matrix is precomputed, not learned.** `P` is fixed by board topology alone (see above) and computed once (`propagation_matrix` in `gcn_fit.py`, mirrored by the `propagation_matrix` field exported into the model JSON) — never updated during training, unlike the per-layer weights.

Default layer widths (all currently unmodified from the paper): GCN1=60, GCN2=30, FC1=60, FC2=60, FC3=30.

Both implementations side by side — same computation, same variable names where possible, so the correspondence is easy to check directly:

```python
# gcn_fit.py -- GCNValueNetwork.forward (training, PyTorch/autograd)
def forward(self, node_features, global_features, p):
    h1 = torch.relu(torch.einsum("ij,bjk->bik", p, self.gcn1(node_features)))
    h2 = torch.relu(torch.einsum("ij,bjk->bik", p, self.gcn2(h1)))
    board_embedding = h2.reshape(h2.size(0), -1)          # flatten, not pool
    fc2_out = torch.relu(self.fc2(board_embedding))
    fc1_out = torch.relu(self.fc1(global_features))
    combined = torch.cat([fc2_out, fc1_out], dim=-1)
    fc3_out = torch.relu(self.fc3(combined))
    return self.output(fc3_out).squeeze(-1)                # no activation
```

```go
// gcnmodel.go -- Model.Score (inference, hand-rolled, no tensor library)
h1 := applyLayerPerNode(m.gcn1, nodeFeatures)
h1 = propagate(m.propagationMatrix, h1)
reluMatrix(h1)

h2 := applyLayerPerNode(m.gcn2, h1)
h2 = propagate(m.propagationMatrix, h2)
reluMatrix(h2)

boardEmbedding := flattenMatrix(h2)     // flatten, not pool
fc2Out := applyLayer(m.fc2, boardEmbedding)
reluVector(fc2Out)

fc1Out := applyLayer(m.fc1, globalFeatures)
reluVector(fc1Out)

combined := make([]float64, 0, len(fc2Out)+len(fc1Out))
combined = append(combined, fc2Out...)
combined = append(combined, fc1Out...)
fc3Out := applyLayer(m.fc3, combined)
reluVector(fc3Out)

out := applyLayer(m.output, fc3Out)     // no activation
return out[0]
```
