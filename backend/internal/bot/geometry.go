package bot

import (
	"container/heap"
	"sort"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// This file provides read-only board-geometry query helpers over *risk.Game
// -- the same category as risk/legal_actions.go, never mutating state. It
// exists to support Lux Delux-inspired bot personas (see
// project-docs/bot_player/proposals/Lux_Delux_AI_Research_Notes.md and
// Lux_Port_Notes.md): AngryStrategy needed only enemyNeighborCount and
// weakestEnemyNeighbor; ClusterStrategy adds continent-ownership, cluster
// border detection, and a weighted shortest-path search. Later personas
// (Pixie, Quo, Killbot, Boscoe) are expected to extend this file further as
// each one needs new queries.

// enemyNeighborCount counts the distinct territories adjacent to t that are
// not owned by pi -- Lux's Country.getNumberEnemyNeighbors(), a territory
// count, not an army sum (contrast strategy_basic.go's adjacentEnemyArmies,
// which sums armies).
func enemyNeighborCount(g *risk.Game, t risk.Territory, pi int) int {
	count := 0
	for _, other := range g.Board.Order {
		if other == t || !g.Board.IsAdjacent(t, other) {
			continue
		}
		if g.Territories[other].Owner != pi {
			count++
		}
	}
	return count
}

// weakestEnemyNeighbor returns the territory adjacent to t not owned by pi
// with the fewest armies, tie-broken by canonical board order. ok is false
// if t has no enemy neighbor.
func weakestEnemyNeighbor(g *risk.Game, t risk.Territory, pi int) (weakest risk.Territory, ok bool) {
	bestArmies := 0
	for _, other := range g.Board.Order {
		if other == t || !g.Board.IsAdjacent(t, other) {
			continue
		}
		ts := g.Territories[other]
		if ts.Owner == pi {
			continue
		}
		if !ok || ts.Armies < bestArmies {
			weakest, bestArmies, ok = other, ts.Armies, true
		}
	}
	return weakest, ok
}

// territoryContinent maps every territory to its continent name.
func territoryContinent(g *risk.Game) map[risk.Territory]risk.Continent {
	m := make(map[risk.Territory]risk.Continent, len(g.Board.Order))
	for cont, info := range g.Board.Continents {
		for _, t := range info.Territories {
			m[t] = cont
		}
	}
	return m
}

// continentOrder returns every continent name in the order its first
// territory appears in g.Board.Order. risk.Board has no canonical
// continent ordering of its own (Continents is a map); this derives one
// deterministically so continent-ranking helpers below tie-break the same
// way on every run, keeping --seed-start reproducible.
func continentOrder(g *risk.Game) []risk.Continent {
	tc := territoryContinent(g)
	seen := make(map[risk.Continent]bool, len(g.Board.Continents))
	order := make([]risk.Continent, 0, len(g.Board.Continents))
	for _, t := range g.Board.Order {
		cont := tc[t]
		if !seen[cont] {
			seen[cont] = true
			order = append(order, cont)
		}
	}
	return order
}

// ownsContinent reports whether pi owns every territory in cont.
func ownsContinent(g *risk.Game, pi int, cont risk.Continent) bool {
	for _, t := range g.Board.Continents[cont].Territories {
		if g.Territories[t].Owner != pi {
			return false
		}
	}
	return true
}

// ownsAnyPositiveContinent reports whether pi fully owns any continent with
// a positive reinforcement bonus (Lux's playerOwnsAnyPositiveContinent).
func ownsAnyPositiveContinent(g *risk.Game, pi int) bool {
	for _, cont := range continentOrder(g) {
		if g.Board.Continents[cont].Bonus > 0 && ownsContinent(g, pi, cont) {
			return true
		}
	}
	return false
}

// mostValuablePositiveOwnedContinent returns the positive-bonus continent
// pi fully owns with the largest bonus, tie-broken by continentOrder (Lux's
// getMostValuablePositiveOwnedCont). ok is false if pi owns no such
// continent.
func mostValuablePositiveOwnedContinent(g *risk.Game, pi int) (best risk.Continent, ok bool) {
	bestBonus := -1
	for _, cont := range continentOrder(g) {
		info := g.Board.Continents[cont]
		if info.Bonus > bestBonus && ownsContinent(g, pi, cont) {
			bestBonus, best, ok = info.Bonus, cont, true
		}
	}
	return best, ok
}

// playerArmiesInContinent sums the armies pi owns within cont.
func playerArmiesInContinent(g *risk.Game, pi int, cont risk.Continent) int {
	total := 0
	for _, t := range g.Board.Continents[cont].Territories {
		if g.Territories[t].Owner == pi {
			total += g.Territories[t].Armies
		}
	}
	return total
}

// enemyArmiesInContinent sums the armies owned by anyone other than pi
// within cont.
func enemyArmiesInContinent(g *risk.Game, cont risk.Continent, pi int) int {
	total := 0
	for _, t := range g.Board.Continents[cont].Territories {
		if g.Territories[t].Owner != pi {
			total += g.Territories[t].Armies
		}
	}
	return total
}

// easiestContinentToTake returns the positive-bonus continent with the
// highest ratio of pi's armies in it to everyone else's, tie-broken by
// continentOrder (Lux's getEasiestContToTake). A continent pi already
// fully owns has zero enemy armies and is skipped, since it isn't a "take"
// target. ok is false if no positive-bonus continent has any enemy
// presence to compare against.
func easiestContinentToTake(g *risk.Game, pi int) (best risk.Continent, ok bool) {
	bestRatio := -1.0
	for _, cont := range continentOrder(g) {
		info := g.Board.Continents[cont]
		if info.Bonus <= 0 {
			continue
		}
		enemy := enemyArmiesInContinent(g, cont, pi)
		if enemy == 0 {
			continue
		}
		ratio := float64(playerArmiesInContinent(g, pi, cont)) / float64(enemy)
		if ratio > bestRatio {
			bestRatio, best, ok = ratio, cont, true
		}
	}
	return best, ok
}

// biggestArmyTerritory returns pi's owned territory with the most armies,
// tie-broken by canonical board order (Lux's getPlayersBiggestArmy). ok is
// false if pi owns nothing.
func biggestArmyTerritory(g *risk.Game, pi int) (best risk.Territory, ok bool) {
	bestArmies := -1
	for _, t := range g.Board.Order {
		ts := g.Territories[t]
		if ts.Owner == pi && ts.Armies > bestArmies {
			best, bestArmies, ok = t, ts.Armies, true
		}
	}
	return best, ok
}

// weakestEnemyNeighborInContinent is weakestEnemyNeighbor restricted to
// neighbors of t that lie within cont.
func weakestEnemyNeighborInContinent(g *risk.Game, t risk.Territory, pi int, cont risk.Continent) (weakest risk.Territory, ok bool) {
	tc := territoryContinent(g)
	bestArmies := 0
	for _, other := range g.Board.Order {
		if other == t || !g.Board.IsAdjacent(t, other) || tc[other] != cont {
			continue
		}
		ts := g.Territories[other]
		if ts.Owner == pi {
			continue
		}
		if !ok || ts.Armies < bestArmies {
			weakest, bestArmies, ok = other, ts.Armies, true
		}
	}
	return weakest, ok
}

// clusterBorder returns the subset of pi's connected owned-territory
// component containing root that borders at least one non-pi territory --
// Lux's ClusterBorderIterator. root must be owned by pi; returns nil
// otherwise. Result is in canonical board order for determinism.
func clusterBorder(g *risk.Game, root risk.Territory, pi int) []risk.Territory {
	if g.Territories[root].Owner != pi {
		return nil
	}

	component := map[risk.Territory]bool{root: true}
	queue := []risk.Territory{root}
	for len(queue) > 0 {
		t := queue[0]
		queue = queue[1:]
		for other := range g.Board.Adjacent[t] {
			if g.Territories[other].Owner == pi && !component[other] {
				component[other] = true
				queue = append(queue, other)
			}
		}
	}

	var borders []risk.Territory
	for _, t := range g.Board.Order {
		if component[t] && enemyNeighborCount(g, t, pi) > 0 {
			borders = append(borders, t)
		}
	}
	return borders
}

// continentBorders returns the territories in cont that have at least one
// neighbor outside cont (Lux's BoardHelper.getContinentBorders), in
// canonical board order.
func continentBorders(g *risk.Game, cont risk.Continent) []risk.Territory {
	tc := territoryContinent(g)
	var out []risk.Territory
	for _, t := range g.Board.Continents[cont].Territories {
		for other := range g.Board.Adjacent[t] {
			if tc[other] != cont {
				out = append(out, t)
				break
			}
		}
	}
	order := orderIndex(g)
	sortTerritoriesByOrder(out, order)
	return out
}

// sortTerritoriesByOrder sorts ts in place by canonical board order.
func sortTerritoriesByOrder(ts []risk.Territory, order map[risk.Territory]int) {
	sort.Slice(ts, func(i, j int) bool { return order[ts[i]] < order[ts[j]] })
}

// routeNode is one entry in cheapestRouteToContinent's priority queue: a
// territory reached at accumulated cost, with order recorded purely to
// keep the search deterministic when costs tie.
type routeNode struct {
	t     risk.Territory
	cost  int
	order int
}

type routeHeap []routeNode

func (h routeHeap) Len() int      { return len(h) }
func (h routeHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h routeHeap) Less(i, j int) bool {
	if h[i].cost != h[j].cost {
		return h[i].cost < h[j].cost
	}
	return h[i].order < h[j].order
}
func (h *routeHeap) Push(x any) { *h = append(*h, x.(routeNode)) }
func (h *routeHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

// cheapestRouteToContinent runs a Dijkstra search outward from cont's
// border territories, with the cost of stepping into a territory equal to
// its army count unless it's owned by pi (a free, terminal node) --
// Lux's BoardHelper.cheapestRouteFromOwnerToCont. It returns the pi-owned
// territory reachable at the lowest accumulated enemy-army cost, used as a
// placement fallback when pi owns nothing in cont itself. ok is false if
// pi owns no territory reachable from cont at all.
func cheapestRouteToContinent(g *risk.Game, pi int, cont risk.Continent) (risk.Territory, bool) {
	borders := continentBorders(g, cont)
	if len(borders) == 0 {
		return "", false
	}
	order := orderIndex(g)

	visited := make(map[risk.Territory]bool)
	h := &routeHeap{}
	for _, b := range borders {
		heap.Push(h, routeNode{t: b, cost: 0, order: order[b]})
	}

	for h.Len() > 0 {
		cur := heap.Pop(h).(routeNode)
		if visited[cur.t] {
			continue
		}
		visited[cur.t] = true
		if g.Territories[cur.t].Owner == pi {
			return cur.t, true
		}
		for _, neighbor := range g.Board.Order {
			if visited[neighbor] || !g.Board.IsAdjacent(cur.t, neighbor) {
				continue
			}
			cost := cur.cost
			if g.Territories[neighbor].Owner != pi {
				cost += g.Territories[neighbor].Armies
			}
			heap.Push(h, routeNode{t: neighbor, cost: cost, order: order[neighbor]})
		}
	}
	return "", false
}

// bestFortifyDestination picks the action from actions whose To has the
// highest enemyNeighborCount, tie-broken by canonical board order (From,
// then To) -- the criterion AngryStrategy.fortify and ClusterStrategy.fortify
// both use, since this engine's one-fortify-per-turn limit collapses any
// multi-hop "walk toward the border" strategy to a single best pick. ok is
// false if actions is empty.
func bestFortifyDestination(g *risk.Game, pi int, actions []risk.FortificationAction) (best risk.FortificationAction, bestScore int, ok bool) {
	if len(actions) == 0 {
		return risk.FortificationAction{}, 0, false
	}
	order := orderIndex(g)
	best = actions[0]
	bestScore = enemyNeighborCount(g, best.To, pi)
	for _, cand := range actions[1:] {
		score := enemyNeighborCount(g, cand.To, pi)
		if score > bestScore ||
			(score == bestScore && (order[cand.From] < order[best.From] ||
				(order[cand.From] == order[best.From] && order[cand.To] < order[best.To]))) {
			best, bestScore = cand, score
		}
	}
	return best, bestScore, true
}
