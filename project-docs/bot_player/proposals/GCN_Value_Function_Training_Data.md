# The GCN Value Function: Training Data

> Part of the GCN value function reference set — see [`GCN_Value_Function_Overview.md`](GCN_Value_Function_Overview.md) for the full pipeline and links to the companion documents (architecture, training).

## Training data generation (`backend/cmd/tdtraindata`)

Games are played entirely by the existing heuristic bot personas (`basic-v1`, `scored-v1`, `angry-v1`, `cluster-v1`, `pixie-v1`, `quo-v1`, `boscoe-v1`, `killbot-v1`, `turtle-v1`) via `internal/simulation.Simulator` — the same headless game engine `cmd/tournament` uses. No human play, no self-play against the GCN itself (yet).

**One row per living player, per completed turn boundary** — not one row per decision, and not one row per game. Every time a player's turn ends (`simulation.Config.OnTurnBoundary`), the tool captures `tdstate.Encode(game, playerIndex).Flatten()` for **every player still alive**, not just the one whose turn just ended — so a single turn produces several rows, one per surviving perspective. A game that doesn't run to completion (hits a safety limit, stalemate, etc.) is discarded entirely: there's no reliable win/loss label for an incomplete game, so none of its rows are kept.

Each row:

```json
{"GameID": "...", "Seed": 1, "PlayerID": "p2", "StrategyID": "scored-v1", "Turn": 7, "Won": true, "Features": [0.0, 1.0, ...]}
```

The capture itself is a closure on `simulation.Config.OnTurnBoundary`, buffered until the game either completes or is discarded (`extract.go`):

```go
cfg.OnTurnBoundary = func(tb simulation.TurnBoundary) {
    for i, p := range tb.Game.Players {
        if p.Eliminated {
            continue
        }
        buffered = append(buffered, pending{
            playerID:   p.ID,
            strategyID: p.Strategy,
            turn:       tb.Turn,
            features:   tdstate.Encode(tb.Game, i).Flatten(),
        })
    }
}

result, _, _ := sim.RunOne(context.Background(), cfg, nil)
if !result.Completed {
    return nil, false // no reliable win/loss label -- discard every buffered row
}
```

`Won` is that specific player's *eventual* outcome, repeated on every row of their sequence (cheap to store, and TD(λ) training only actually needs it at the last row — see `GCN_Value_Function_Training.md`). Two sidecar files are written once per output file, since every row in one run shares the same board/feature layout: `*.featurenames.json` (column names matching `Flatten()`'s order) and `*.boardschema.json` (the board's adjacency, used to build an identical propagation matrix on both the training and inference sides — the one shared source of truth that keeps Python and Go from silently disagreeing on graph structure).

**Avoiding a "policy monoculture"**: a dataset generated from one fixed strategy lineup only ever visits the states that particular lineup's play naturally reaches. `tdtraindata` is deliberately run multiple times with different `--strategies` combinations (rotating personas and player counts — 3p/4p/6p) into separate files, which the training loader concatenates — `make tdtraindata-diverse` runs a ready-made rotation of six such lineups.

## Feature encoding (`internal/tdstate/encode.go`)

**Player-relative, not seat-indexed**: every feature is encoded from one specific player's point of view (`Encode(g, pi)`) as "mine" vs. "not mine," never as a fixed per-seat slot. This is what lets one fixed-width encoding work uniformly across this project's variable 3-6 player games — a fixed one-hot-per-seat scheme would need to change shape depending on player count.

### Per-territory features (one block per territory, `board.Order` order)

```go
type TerritoryFeatures struct {
	IsMine              bool
	ArmyFraction        float64
	Continent           []bool // one-hot, sortedContinents(board) order
	IsContinentBorder   bool
	EnemyThreatFraction float64
}
```

- **`IsMine`** — does the encoding viewer (`pi`) own this territory? The *only* ownership signal in the whole encoding — every non-mine territory looks identical regardless of which rival holds it. (This is the crux of the "individual enemies" open question — see `GCN_Value_Function_Overview.md`.)
- **`ArmyFraction`** — `armies / totalBoardArmies`. Normalized against the *whole board's* total, not the territory's own continent or neighborhood, so any two territories' fractions are directly comparable no matter where they sit.
- **`Continent`** (one-hot) — which of the board's continents this territory belongs to, in a fixed sorted order (`sortedContinents`) shared by every encoding, so "slot 3" always means the same continent across different feature vectors and different games.
- **`IsContinentBorder`** — true if this territory has any neighbor *outside* its own continent (`isContinentBorder`). Strategically this is where continent-bonus races are actually contested: an interior territory (every neighbor same-continent) can never be attacked without first breaching a border territory, so it's structurally lower-priority to reinforce.
- **`EnemyThreatFraction`** — sum of armies across every neighboring territory owned by anyone other than `pi` (vacated/eliminated territories, `Owner < 0`, don't count), divided by total board armies:

  ```go
  func enemyThreatFraction(g *risk.Game, t risk.Territory, pi int, totalArmies int) float64 {
  	threat := 0
  	for neighbor := range g.Board.Adjacent[t] {
  		ts := g.Territories[neighbor]
  		if ts.Owner >= 0 && ts.Owner != pi {
  			threat += ts.Armies
  		}
  	}
  	return float64(threat) / float64(totalArmies)
  }
  ```

  Has a specific origin story worth keeping: a live-play audit found `ValueStrategy`'s reinforce phase locking onto one "favorite" territory per game — up to ~47% of all reinforcements placed that game going to the same spot — because every per-territory `ArmyFraction` coefficient a linear fit ever produced was small but strictly positive (more armies here is always slightly good), with no notion of "this territory is under threat." This feature exists to give the model something to actually prefer a threatened border over a fixed favorite.

### Global features (one block, not per-territory)

- **`MyArmyFraction`** / **`MyTerritoryFraction`** — the viewer's total armies/territories, as board-wide fractions. The most basic "how am I doing overall" signal, and *not* the strict complement of the enemy features below — `1 - MyArmyFraction` is every rival *combined*, whereas the enemy features (next) only ever describe the single strongest one.
- **`MyIncomeFraction`** — an estimate of the viewer's *next* reinforcement income, mirroring the real engine's own (unexported) formula so the network sees roughly what a human player would actually receive:

  ```go
  income := max(3, territoryCount/3)
  for every continent pi fully owns:
      income += continent.Bonus
  bound := totalTerritories/3.0 + sum(every continent's Bonus)  // deliberately generous
  return income / bound
  ```

  `bound` is deliberately generous (the sum of *every* continent's bonus, not just the ones actually reachable) rather than a tight theoretical max — no player can realistically own every continent while rivals still hold territory, so this just keeps the fraction on a stable, comparable scale rather than being an unbounded raw integer.
- **`StrongestEnemyArmyFraction`** / **`StrongestEnemyTerritoryFraction`** — a **max over every living rival**, collapsed to one number each. This is the network's only window onto "the opposition" beyond `EnemyThreatFraction`'s local, ownership-blind view — see the "current limitation" note below for why that's a real limitation, not just a simplification.
- **`ContinentArmyFraction[]`** — *one entry per continent* (not a single scalar): the viewer's share of armies *within that specific continent*, `mine / continentTotal`. Distinct from the board-wide `MyArmyFraction` — a player can hold a commanding overall army fraction while being locally weak in one contested continent, or vice versa, and this is the only feature that lets the network see continent-by-continent strength rather than one global average.
- **`CardFraction`** — `handSize / 5.0` (5 is `risk.CardTurnInRequired`, the mandatory-trade-in threshold). Card count matters strategically because each successive set traded in is worth more reinforcements than the last (the engine's escalating trade-in schedule) — a large hand represents a growing windfall about to land, worth the network being able to see coming.
- **`Defence`** — the one hand-crafted feature ported directly from the source paper (Jamie Carr, arXiv:2009.06355), added after the network failed to learn defensive behavior from low-level features alone. Estimates how thin the viewer's *own* weakest defended front is: for every continent the viewer fully owns, find the minimum army count among that continent's frontier territories, then take a weighted mean across continents (weighted so the very weakest front dominates), normalized by total board armies and capped at 0.2 (uncapped values could reach abnormal highs and confuse training). The paper author's own words, quoted directly in the code: *"the only instance where I had to build in human knowledge... I was sure to keep the feature global and not incorporate information about specific threats."*
- **`Phase[]`** (one-hot) — which of the engine's seven phases (`SetupClaim` through `GameOver`) this state is in. A turn-boundary row is always captured right at a turn's end, so in this dataset specifically the phase tends to cluster around a few values — the full seven-phase enum exists because `Encode` is also designed to score *mid-phase hypothetical afterstates* (exactly what `ValueStrategy`'s search machinery does today), where phase varies far more.
- **`IsMyTurn`** — `pi == g.CurrentPlayer`. Lets the same underlying board state be scored differently depending on whether it's actually the viewer's move or a hypothetical afterstate resulting from someone else's action — relevant background for any future multi-perspective search.

**Flatten order**: per-territory blocks first (in board order), then the global block — this exact layout is what lets both the training and inference sides split the flat vector back into "node matrix" + "global vector" for the GCN (see `GCN_Value_Function_Architecture.md`). `Flatten()` (`internal/tdstate/flatten.go`) is the single source of truth for this ordering; `FeatureNames()` produces matching column labels for every position:

```go
func (f Features) Flatten() []float64 {
	out := make([]float64, 0, f.width())
	for _, t := range f.Territories {
		out = append(out, boolToFloat(t.IsMine), t.ArmyFraction)
		out = append(out, boolsToFloats(t.Continent)...)
		out = append(out, boolToFloat(t.IsContinentBorder), t.EnemyThreatFraction)
	}
	g := f.Global
	out = append(out, g.MyArmyFraction, g.MyTerritoryFraction, g.MyIncomeFraction,
		g.StrongestEnemyArmyFraction, g.StrongestEnemyTerritoryFraction)
	out = append(out, g.ContinentArmyFraction...)
	out = append(out, g.CardFraction, g.Defence)
	out = append(out, boolsToFloats(g.Phase)...)
	out = append(out, boolToFloat(g.IsMyTurn))
	return out
}
```

**A current, load-bearing limitation** (directly relevant to the open question `GCN_Value_Function_Overview.md` exists to inform): the only enemy-related signals anywhere in this encoding are `EnemyThreatFraction` (a per-territory, ownership-blind sum — it doesn't matter which enemy the threat comes from) and `StrongestEnemyArmyFraction`/`StrongestEnemyTerritoryFraction` (a single aggregate max, collapsing every rival into "whichever one is currently strongest"). There is no signal anywhere for how many opponents remain, no per-territory distinction between *which* rival owns a "not mine" territory, and no information about any rival other than the single strongest one. A game with one dominant enemy and two nearly-eliminated stragglers looks identical to a game with three evenly-matched rivals, as long as the strongest one's own numbers happen to coincide — the network has no way to tell those situations apart, because the information was never in its input.
