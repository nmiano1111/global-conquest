# The GCN Value Function: Training

> Part of the GCN value function reference set — see [`GCN_Value_Function_Overview.md`](GCN_Value_Function_Overview.md) for the full pipeline and links to the companion documents (training data, architecture).

## Training methodology

Two distinct training objectives exist in `gcn_fit.py`; only one is currently used for anything shipped.

**`fit_gcn` (supervised, abandoned)**: standard binary cross-entropy, regressing every turn-boundary row directly toward its game's final `Won`/`Loss` outcome, full-batch, Adam. This was the original approach (inherited from the earlier linear `board_fit.py`), and it produces what this project's own `td_fit.py` module docstring calls "erratic, flip-flopping predictions" — because it asks the network to predict a whole game's outcome from any single snapshot, with no notion that states five turns apart should have closely related values. Superseded, but kept in the codebase (not deleted) so the two objectives stay directly comparable rather than one silently replacing the other.

**`fit_gcn_td` (TD(λ), current)**: the objective actually used for every model this project has shipped. An *episode* is one player's full turn-boundary sequence within one completed game. Trained via semi-gradient TD(λ) with eligibility traces:

- At each step `t` in an episode, the **bootstrap target** is the network's own current estimate of the *next* state's value (`V(s_{t+1})`) — except at the episode's last step, where the target is the actual terminal reward (1.0 if that player won, 0.0 otherwise). This is the mechanism that lets the network learn from temporally close transitions instead of only ever being told "predict how the whole game ends" — the fix for the supervised objective's erratic-prediction problem.
- An **eligibility trace** accumulates gradients across the episode (`trace = λ·trace + ∇V(s_t)`, `λ=0.8`), and each step's TD error (`target − V(s_t)`, clipped to ±5.0) is applied to every parameter weighted by that parameter's current trace — a direct generalization of `td_fit.fit_td_lambda`'s already-validated linear-model algorithm, with autograd standing in for the linear case's closed-form gradient.
- Bootstrap targets come from a separate **target network**, a copy of the live model re-synced every `target_sync_episodes` episodes (default: every episode) — the standard DQN-style fix for bootstrapping against a target that would otherwise be moving on every single update.
- Optimizer: **plain SGD** (`param += alpha · delta · trace`, `alpha=1e-3`), not Adam. An earlier version used Adam (reasoning that its adaptive scaling would be safer for a much larger, nonlinear model than the validated linear case) — in practice this reliably collapsed the network to a near-constant output after a few epochs on real training data. Adam's own momentum stacking with the eligibility trace's already-temporal accumulation is the most likely cause; a real per-territory discrimination check against actual training data (not a theoretical argument) is what settled on plain SGD instead.

The per-timestep core of the training loop (`fit_gcn_td`), one episode already selected, iterating its turn-boundary sequence in order:

```python
for t in range(t_count):
    node_t = torch.tensor(ep.node_features[t : t + 1], dtype=torch.float32)
    global_t = torch.tensor(ep.global_features[t : t + 1], dtype=torch.float32)

    if t < t_count - 1:
        with torch.no_grad():
            next_node = torch.tensor(ep.node_features[t + 1 : t + 2], dtype=torch.float32)
            next_global = torch.tensor(ep.global_features[t + 1 : t + 2], dtype=torch.float32)
            target = target_model(next_node, next_global, p).item()  # bootstrap
    else:
        target = reward                                              # terminal step: 1.0 or 0.0

    model.zero_grad()
    v_t = model(node_t, global_t, p)
    v_t.backward()                                                   # populates param.grad = ∇V(s_t)

    delta = float(np.clip(target - v_t.item(), -td_error_clip, td_error_clip))
    with torch.no_grad():
        for name, param in model.named_parameters():
            traces[name] = lam * traces[name] + param.grad           # accumulate eligibility trace
            param.data += alpha * delta * traces[name]                # apply the update directly
```

**Epochs**: the paper's own reported value is 3. This project's own empirical sweep (3 → 6 → 12 → 18 epochs, each independently trained and evaluated via full tournament runs) found **12** to be a reproducible peak — the same win rate recovered across two different tournament matchups, from two independently-trained runs — with 18 showing clear overfitting (win rate dropping in both matchups simultaneously). This is a real, measured difference from the paper's own default, not just an unset parameter.

**Sequential, not batched**: unlike `fit_gcn`'s single vectorized pass over the whole dataset, `fit_gcn_td` must process one episode at a time, one timestep at a time within it — the trace resets per episode, and each step's target depends on the current weights *after* every prior update within that same episode. This makes it dramatically more expensive per epoch than the supervised objective's batched loop.

## Post-training calibration (`cmd/bvcalibrate`)

A trained/exported model isn't immediately playable — `ValueStrategy.attack()`/`fortify()` only act when a candidate's score beats the current state's score by more than a margin (`AttackMargin`/`FortifyMargin`), and attack/fortify candidates score on very different scales (an attack changes ownership across many features at once; a fortify only reallocates armies between two of the acting player's own territories). `cmd/bvcalibrate` runs many headless games with a *zero-margin* wrapper around a candidate model, observes the natural distribution of real score deltas per phase, and sets each margin to a chosen percentile of that phase's positive deltas.

The single highest-leverage fix in this whole project's history came from this step, not the model itself: an earlier median-based calibration (`--percentile 50`) was silently rejecting roughly half of all decisions the model itself scored as genuine improvements. Switching to `--percentile 0` (act on *any* positive-delta decision) took a model stuck at a hard 0% win rate to a real, reproducible ~17%, with no change to the model's weights at all.

The gate this calibrates, at decision time (`ValueStrategy.clearsMargin`, `internal/bot/strategy_value.go`) — deceptively simple, which is exactly why a wrongly-calibrated margin was so easy to get wrong without noticing:

```go
return bestScore > currentScore+margin
```

`bestScore`/`currentScore` come straight from the model's `Score` calls; `margin` is the one number `cmd/bvcalibrate` exists to find.
