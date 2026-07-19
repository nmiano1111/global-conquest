package bot

import (
	"context"
	"fmt"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// StrategyClusterV1 is the registry identifier for ClusterStrategy, the
// second Lux Delux-inspired persona (see
// project-docs/bot_player/proposals/Lux_Delux_AI_Research_Notes.md and
// Lux_Port_Notes.md). Where AngryStrategy is purely local and greedy,
// Cluster expands outward from its largest owned landmass via a
// four-stage attack sequence (easy-expand, fill-out, consolidate,
// split-up) plus hogwild/stalemate fallbacks when it's dominant, and is
// the foundation Quo and Boscoe both build on in Lux's own class
// hierarchy.
const StrategyClusterV1 = "cluster-v1"

// ClusterStrategy implements StrategyClusterV1.
type ClusterStrategy struct{}

// NewClusterStrategy creates a ClusterStrategy.
func NewClusterStrategy() *ClusterStrategy { return &ClusterStrategy{} }

// NextCommand dispatches on g.Phase, mirroring AngryStrategy/BasicStrategy's
// shape. It always returns a zero-value Explanation, since Cluster has no
// scoring model to report.
func (c *ClusterStrategy) NextCommand(_ context.Context, g *risk.Game, playerID string) (Command, Explanation, error) {
	cmd, err := c.nextCommand(g, playerID)
	return cmd, Explanation{}, err
}

func (c *ClusterStrategy) nextCommand(g *risk.Game, playerID string) (Command, error) {
	switch g.Phase {
	case risk.PhaseSetupReinforce:
		return c.setupReinforce(g, playerID)
	case risk.PhaseReinforce:
		return c.reinforce(g, playerID)
	case risk.PhaseAttack:
		return c.attack(g, playerID)
	case risk.PhaseOccupy:
		return c.occupy(g, playerID)
	case risk.PhaseFortify:
		return c.fortify(g, playerID)
	default:
		return Command{}, fmt.Errorf("bot: %s has no move for phase %q", StrategyClusterV1, g.Phase)
	}
}

// clusterRoot picks the territory Cluster's cluster-border logic centers
// on: the canonical-order-first territory in pi's most valuable fully
// owned continent, or (if pi owns no positive-bonus continent outright)
// pi's single biggest army -- Lux's Cluster.attackPhase/placeArmies root
// selection. ok is false only if pi owns nothing at all.
func clusterRoot(g *risk.Game, pi int) (risk.Territory, bool) {
	if cont, ok := mostValuablePositiveOwnedContinent(g, pi); ok {
		tc := territoryContinent(g)
		for _, t := range g.Board.Order {
			if tc[t] == cont {
				return t, true
			}
		}
	}
	return biggestArmyTerritory(g, pi)
}

// clusterPlacementTerritory picks where ClusterStrategy places
// reinforcements: the weakest (fewest-armies) territory on clusterRoot's
// cluster border -- Lux's placeArmiesOnClusterBorder, simplified to a
// single placement per call rather than Lux's incremental chunked
// placement across many board.placeArmies calls within one turn (repeated
// NextCommand calls, each re-picking the then-weakest border territory,
// reproduce the same "reinforce the weakest point" effect). Falls back to
// bestByEnemyNeighborCount before any cluster border exists (the very
// first setup placement, or a degenerate single-territory cluster with no
// enemy neighbor at all).
func clusterPlacementTerritory[T any](g *risk.Game, pi int, actions []T, get func(T) risk.Territory) risk.Territory {
	if root, ok := clusterRoot(g, pi); ok {
		if border := clusterBorder(g, root, pi); len(border) > 0 {
			best := border[0]
			bestArmies := g.Territories[best].Armies
			for _, t := range border[1:] {
				if a := g.Territories[t].Armies; a < bestArmies {
					best, bestArmies = t, a
				}
			}
			return best
		}
	}
	return bestByEnemyNeighborCount(g, pi, actions, get)
}

// placeToTakeContinent picks where to place reinforcements when pi owns no
// positive-bonus continent outright -- Lux's placeArmiesToTakeCont, called
// on the continent easiestContinentToTake picks. Two of Lux's three cases
// apply here: pi owns some of the target continent (place on the owned
// territory there with the most in-continent enemy neighbors) or pi owns
// none of it (route toward it via cheapestRouteToContinent). Lux's third
// case -- pi already fully owns the target continent -- can't arise here,
// since easiestContinentToTake only ever considers continents with enemy
// presence left in them.
func placeToTakeContinent(g *risk.Game, pi int, actions []risk.ReinforcementAction) risk.Territory {
	get := func(a risk.ReinforcementAction) risk.Territory { return a.Territory }
	if cont, ok := easiestContinentToTake(g, pi); ok {
		return placeToTakeSpecificContinent(g, pi, actions, cont)
	}
	return bestByEnemyNeighborCount(g, pi, actions, get)
}

// placeToTakeSpecificContinent is placeToTakeContinent's logic generalized
// to an already-chosen target continent, rather than deriving one via
// easiestContinentToTake -- PixieStrategy calls this directly with a
// continent it has already decided it wants (see pixiePlacementTerritory),
// where ClusterStrategy always wants "whichever is easiest."
func placeToTakeSpecificContinent(g *risk.Game, pi int, actions []risk.ReinforcementAction, cont risk.Continent) risk.Territory {
	get := func(a risk.ReinforcementAction) risk.Territory { return a.Territory }

	var owned []risk.Territory
	for _, t := range g.Board.Continents[cont].Territories {
		if g.Territories[t].Owner == pi {
			owned = append(owned, t)
		}
	}
	if len(owned) == 0 {
		if route, ok := cheapestRouteToContinent(g, pi, cont); ok {
			return route
		}
		return bestByEnemyNeighborCount(g, pi, actions, get)
	}

	order := orderIndex(g)
	best := owned[0]
	bestEnemies := -1
	for _, t := range owned {
		enemies := 0
		for _, other := range g.Board.Continents[cont].Territories {
			if other != t && g.Board.IsAdjacent(t, other) && g.Territories[other].Owner != pi {
				enemies++
			}
		}
		if enemies > bestEnemies || (enemies == bestEnemies && order[t] < order[best]) {
			best, bestEnemies = t, enemies
		}
	}
	return best
}

// setupReinforce places the one initial army via clusterPlacementTerritory.
func (c *ClusterStrategy) setupReinforce(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalSetupReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal setup reinforcement for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	best := clusterPlacementTerritory(g, pi, actions, func(a risk.SetupReinforcementAction) risk.Territory { return a.Territory })
	return Command{Action: ActionPlaceInitialArmy, Territory: string(best)}, nil
}

// reinforce only trades cards when a set is mandatory -- Cluster's Lux
// cardsPhase is SmartAgentBase's untouched empty default, same as
// AngryStrategy. Placement dumps every pending reinforcement in one
// command (see clusterPlacementTerritory's doc comment on why per-call
// re-evaluation substitutes for Lux's chunked placement) at
// clusterPlacementTerritory if pi owns a positive continent outright, or
// placeToTakeContinent otherwise.
func (c *ClusterStrategy) reinforce(g *risk.Game, playerID string) (Command, error) {
	if risk.CardTurnInRequired(g, playerID) {
		if sets := risk.LegalCardTurnIns(g, playerID); len(sets) > 0 {
			return Command{Action: ActionTradeCards, CardIndices: sets[0].Indices}, nil
		}
	}

	actions := risk.LegalReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal reinforcement for player %s", playerID)
	}
	pi := playerIndex(g, playerID)

	var best risk.Territory
	if ownsAnyPositiveContinent(g, pi) {
		best = clusterPlacementTerritory(g, pi, actions, func(a risk.ReinforcementAction) risk.Territory { return a.Territory })
	} else {
		best = placeToTakeContinent(g, pi, actions)
	}
	return Command{Action: ActionPlaceReinforcement, Territory: string(best), Armies: g.PendingReinforcements}, nil
}

// attack tries, in order, the first qualifying candidate from Lux's
// four-stage attack sequence (easy-expand, fill-out, consolidate,
// split-up), then -- only if this player is dominant enough to go hogwild
// or the game has stalled into a high-army stalemate -- the same three
// stages broadened to every owned territory as its own root, plus a
// relaxed split-up. Each stage returns the first match in canonical board
// order; repeated NextCommand calls reproduce Lux's while-loops the same
// way AngryStrategy.attack does.
func (c *ClusterStrategy) attack(g *risk.Game, playerID string) (Command, error) {
	pi := playerIndex(g, playerID)
	root, ok := clusterRoot(g, pi)
	if !ok {
		return Command{Action: ActionEndAttack}, nil
	}

	stages := []func(*risk.Game, int, risk.Territory) (Command, bool){
		attackEasyExpand,
		attackFillOut,
		attackConsolidate,
		func(g *risk.Game, pi int, root risk.Territory) (Command, bool) { return attackSplitUp(g, pi, root, 1.2) },
	}
	for _, stage := range stages {
		if cmd, ok := stage(g, pi, root); ok {
			return cmd, nil
		}
	}

	if shouldGoHogWild(g, pi) {
		if cmd, ok := attackAsMuchAsPossible(g, pi); ok {
			return cmd, nil
		}
	}
	return Command{Action: ActionEndAttack}, nil
}

// attackEasyExpand attacks a clusterBorder territory's sole enemy
// neighbor, if it can beat it -- Lux's attackEasyExpand.
func attackEasyExpand(g *risk.Game, pi int, root risk.Territory) (Command, bool) {
	for _, border := range clusterBorder(g, root, pi) {
		if enemyNeighborCount(g, border, pi) != 1 {
			continue
		}
		enemy, _ := weakestEnemyNeighbor(g, border, pi) // the sole enemy neighbor
		src := g.Territories[border]
		if src.Armies > g.Territories[enemy].Armies {
			return Command{Action: ActionAttack, From: string(border), To: string(enemy), AttackerDice: min(3, src.Armies-1)}, true
		}
	}
	return Command{}, false
}

// attackFillOut attacks an enemy "island" -- a non-owned neighbor of a
// clusterBorder territory that has zero non-pi neighbors of its own --
// that the border can beat, killing little pockets fully surrounded by
// pi's territory -- Lux's attackFillOut.
func attackFillOut(g *risk.Game, pi int, root risk.Territory) (Command, bool) {
	for _, border := range clusterBorder(g, root, pi) {
		src := g.Territories[border]
		for _, other := range g.Board.Order {
			if other == border || !g.Board.IsAdjacent(border, other) {
				continue
			}
			if g.Territories[other].Owner == pi || enemyNeighborCount(g, other, pi) != 0 {
				continue
			}
			if src.Armies > g.Territories[other].Armies {
				return Command{Action: ActionAttack, From: string(border), To: string(other), AttackerDice: min(3, src.Armies-1)}, true
			}
		}
	}
	return Command{}, false
}

// attackConsolidate finds an enemy territory bordered by two or more
// clusterBorder territories that each have it as their sole enemy
// neighbor, and whose combined armies beat it, then attacks from the
// first such contributor with more than one army -- Lux's
// attackConsolidate. Lux gates the whole multi-source sequence on the
// combined-armies check once, upfront; this re-validates it on every call
// instead, since state is reloaded fresh each time -- more conservative
// than Lux (it can abandon a partly-executed merge if losses make it no
// longer profitable), not less capable.
func attackConsolidate(g *risk.Game, pi int, root risk.Territory) (Command, bool) {
	border := clusterBorder(g, root, pi)
	for _, b := range border {
		if enemyNeighborCount(g, b, pi) != 1 {
			continue
		}
		enemy, _ := weakestEnemyNeighbor(g, b, pi)

		var contributors []risk.Territory
		combined := 0
		for _, other := range border {
			if enemyNeighborCount(g, other, pi) != 1 {
				continue
			}
			onlyEnemy, _ := weakestEnemyNeighbor(g, other, pi)
			if onlyEnemy == enemy {
				contributors = append(contributors, other)
				combined += g.Territories[other].Armies
			}
		}
		if len(contributors) < 2 || combined <= g.Territories[enemy].Armies {
			continue
		}
		for _, src := range contributors {
			if ts := g.Territories[src]; ts.Armies > 1 {
				return Command{Action: ActionAttack, From: string(src), To: string(enemy), AttackerDice: min(3, ts.Armies-1)}, true
			}
		}
	}
	return Command{}, false
}

// attackSplitUp finds a clusterBorder territory that outnumbers the
// combined armies of all its enemy neighbors by more than ratio, then
// attacks its weakest enemy neighbor -- Lux's attackSplitUp splits armies
// evenly across every enemy neighbor and attacks each in the same
// call; this engine has no way to commit a specific army count to one
// attack (Attack always resolves round-by-round with available dice), so
// this attacks the weakest neighbor first each call instead, a
// deterministic simplification of Lux's even split.
func attackSplitUp(g *risk.Game, pi int, root risk.Territory, ratio float64) (Command, bool) {
	for _, border := range clusterBorder(g, root, pi) {
		src := g.Territories[border]
		if src.Armies <= 1 {
			continue
		}
		var enemies []risk.Territory
		enemyArmies := 0
		for _, other := range g.Board.Order {
			if other == border || !g.Board.IsAdjacent(border, other) {
				continue
			}
			if g.Territories[other].Owner != pi {
				enemies = append(enemies, other)
				enemyArmies += g.Territories[other].Armies
			}
		}
		if len(enemies) == 0 || float64(src.Armies) <= float64(enemyArmies)*ratio {
			continue
		}
		target := enemies[0]
		for _, e := range enemies[1:] {
			if g.Territories[e].Armies < g.Territories[target].Armies {
				target = e
			}
		}
		return Command{Action: ActionAttack, From: string(border), To: string(target), AttackerDice: min(3, src.Armies-1)}, true
	}
	return Command{}, false
}

// shouldGoHogWild reports whether pi's total armies exceed every other
// player's combined (Lux's hogWildCheck) or exceed 1500 (Lux's
// attackStalemate's own threshold) -- either condition broadens the
// attack scan to every owned territory.
func shouldGoHogWild(g *risk.Game, pi int) bool {
	total, own := 0, 0
	for _, t := range g.Board.Order {
		ts := g.Territories[t]
		total += ts.Armies
		if ts.Owner == pi {
			own += ts.Armies
		}
	}
	return own > total-own || own > 1500
}

// attackAsMuchAsPossible broadens easy-expand/fill-out/consolidate, then a
// relaxed (0.01x) split-up, to every territory pi owns as its own root --
// Lux's attackAsMuchAsPossible, which does the same via tripleAttackPack
// and attackSplitUp(0.01) over a full PlayerIterator.
func attackAsMuchAsPossible(g *risk.Game, pi int) (Command, bool) {
	for _, stage := range []func(*risk.Game, int, risk.Territory) (Command, bool){
		attackEasyExpand, attackFillOut, attackConsolidate,
	} {
		for _, t := range g.Board.Order {
			if g.Territories[t].Owner != pi {
				continue
			}
			if cmd, ok := stage(g, pi, t); ok {
				return cmd, true
			}
		}
	}
	for _, t := range g.Board.Order {
		if g.Territories[t].Owner != pi {
			continue
		}
		if cmd, ok := attackSplitUp(g, pi, t, 0.01); ok {
			return cmd, true
		}
	}
	return Command{}, false
}

// occupy compares the weakest enemy neighbor of Occupy.From and Occupy.To
// within clusterRoot's continent, moving the legal minimum if From still
// has the better matchup there, otherwise the legal maximum -- Lux's
// Cluster.moveArmiesIn, substituting the recomputed root's continent for
// Lux's goalCont (see Lux_Port_Notes.md) and dropping the
// obvious/memory-based shortcuts (dead code on this board / architecturally
// unavailable across separate NextCommand calls, respectively).
func (c *ClusterStrategy) occupy(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalOccupations(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal occupation for player %s", playerID)
	}
	pi := playerIndex(g, playerID)

	armies := actions[len(actions)-1].Armies // default: maximum
	if root, ok := clusterRoot(g, pi); ok {
		cont := territoryContinent(g)[root]
		aWeakest, aOk := weakestEnemyNeighborInContinent(g, g.Occupy.From, pi, cont)
		dWeakest, dOk := weakestEnemyNeighborInContinent(g, g.Occupy.To, pi, cont)
		if !dOk || (aOk && g.Territories[aWeakest].Armies < g.Territories[dWeakest].Armies) {
			armies = actions[0].Armies // minimum: From still has the better matchup
		}
	}
	return Command{Action: ActionOccupy, Armies: armies}, nil
}

// fortify uses bestFortifyDestination (shared with AngryStrategy) -- see
// Lux_Port_Notes.md on why Lux's multi-hop fortifyCluster collapses to a
// single best-pair pick under this engine's one-fortify-per-turn limit.
func (c *ClusterStrategy) fortify(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalFortifications(g, playerID)
	pi := playerIndex(g, playerID)
	best, bestScore, ok := bestFortifyDestination(g, pi, actions)
	if !ok || bestScore == 0 {
		return Command{Action: ActionEndTurn}, nil
	}
	return Command{Action: ActionFortify, From: string(best.From), To: string(best.To), Armies: best.MaxArmies}, nil
}
