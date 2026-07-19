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

// biggestArmyTerritoryWithEnemyNeighbor is biggestArmyTerritory restricted
// to territories bordering at least one enemy -- Lux's
// getPlayersBiggestArmyWithEnemyNeighbor, used by killTarget as a rough
// gauge of pi's strongest currently-attacking-capable stack.
func biggestArmyTerritoryWithEnemyNeighbor(g *risk.Game, pi int) (best risk.Territory, ok bool) {
	bestArmies := -1
	for _, t := range g.Board.Order {
		ts := g.Territories[t]
		if ts.Owner == pi && ts.Armies > bestArmies && enemyNeighborCount(g, t, pi) > 0 {
			best, bestArmies, ok = t, ts.Armies, true
		}
	}
	return best, ok
}

// playerTotalArmies sums pi's armies board-wide.
func playerTotalArmies(g *risk.Game, pi int) int {
	total := 0
	for _, t := range g.Board.Order {
		if g.Territories[t].Owner == pi {
			total += g.Territories[t].Armies
		}
	}
	return total
}

// playerIncome mirrors risk.Game's own unexported reinforcementsFor
// exactly: max(3, territoryCount/3) plus the bonus of every continent pi
// fully owns (no positivity filter -- this is the real per-turn
// reinforcement rule, not an AI heuristic threshold like
// ownsAnyPositiveContinent's). Duplicated here since reinforcementsFor
// isn't exported and dominantPlayerToKill needs every player's income,
// not just the acting one's.
func playerIncome(g *risk.Game, pi int) int {
	count := 0
	for _, t := range g.Board.Order {
		if g.Territories[t].Owner == pi {
			count++
		}
	}
	income := max(3, count/3)
	for _, cont := range continentOrder(g) {
		if ownsContinent(g, pi, cont) {
			income += g.Board.Continents[cont].Bonus
		}
	}
	return income
}

// dominantPlayerToKill reports the player (other than pi) that must be
// stopped: one who holds at least half of all armies, all income, or all
// territories on the board -- Lux's
// SmartAgentBase.placeArmiesToKillDominantPlayer's detection logic,
// factored out so it can be recomputed fresh from both a placement and an
// attack decision (this engine has no equivalent to Lux's turn-scoped
// mustKillPlayer field -- see Lux_Port_Notes.md's Boscoe addendum). If
// multiple players qualify, the highest player index wins -- the same
// last-write-wins result Lux's own unconditional-overwrite loop produces.
// ok is false if no player meets any threshold.
func dominantPlayerToKill(g *risk.Game, pi int) (target int, ok bool) {
	armies := make([]int, len(g.Players))
	territories := make([]int, len(g.Players))
	totalArmies, totalTerritories := 0, len(g.Board.Order)
	for _, t := range g.Board.Order {
		ts := g.Territories[t]
		if ts.Owner < 0 {
			continue
		}
		armies[ts.Owner] += ts.Armies
		territories[ts.Owner]++
		totalArmies += ts.Armies
	}

	income := make([]int, len(g.Players))
	totalIncome := 0
	for i := range g.Players {
		income[i] = playerIncome(g, i)
		totalIncome += income[i]
	}

	for i := range g.Players {
		if i == pi {
			continue
		}
		if float64(armies[i]) >= float64(totalArmies)*0.5 ||
			float64(income[i]) >= float64(totalIncome)*0.5 ||
			float64(territories[i]) >= float64(totalTerritories)*0.5 {
			target, ok = i, true
		}
	}
	return target, ok
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

// playerArmiesAdjoiningContinent sums pi's armies in territories outside
// cont that are adjacent to one of cont's own border territories
// (deduplicated) -- Lux's BoardHelper.getPlayerArmiesAdjoiningContinent,
// used to judge whether pi has enough nearby strength to contest cont
// without owning anything inside it yet.
func playerArmiesAdjoiningContinent(g *risk.Game, pi int, cont risk.Continent) int {
	tc := territoryContinent(g)
	seen := make(map[risk.Territory]bool)
	total := 0
	for _, border := range continentBorders(g, cont) {
		for other := range g.Board.Adjacent[border] {
			if tc[other] == cont || seen[other] {
				continue
			}
			if g.Territories[other].Owner == pi {
				seen[other] = true
				total += g.Territories[other].Armies
			}
		}
	}
	return total
}

// routeNode is one entry in cheapestRouteSearch's priority queue: a
// territory reached at accumulated cost from its predecessor (from -- ""
// for a cont-border starting node), with order recorded purely to keep
// the search deterministic when costs tie.
type routeNode struct {
	t     risk.Territory
	from  risk.Territory
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

// routeSearchResult is cheapestRouteSearch's outcome: the pi-owned
// territory reached at the lowest accumulated enemy-army cost from cont's
// border, that cost, and (if found is reached via at least one
// intermediate hop, rather than being a border territory of cont itself)
// nextHop, the adjacent territory one step closer to cont -- the natural
// next attack target when routing toward cont.
type routeSearchResult struct {
	found   risk.Territory
	cost    int
	nextHop risk.Territory
	hasHop  bool
}

// cheapestRouteSearch runs a Dijkstra search outward from starts, with
// the cost of stepping into a territory equal to its army count unless
// it's owned by pi (a free, terminal node) -- Lux's
// BoardHelper.cheapestRouteFromOwnerToCont. It is the shared search
// cheapestRouteToContinent/cheapestRouteToContinentWithCost/
// cheapestAttackHopToContinent (starts = continentBorders(g, cont)) and
// cheapestAttackHopToPlayer (starts = ownedTerritories(g, target), added
// for KillbotStrategy) all build on -- generalized from a continent-only
// starting set once a second, structurally different caller needed the
// identical algorithm (see Lux_Port_Notes.md's Killbot addendum). ok is
// false if pi owns no territory reachable from starts at all.
func cheapestRouteSearch(g *risk.Game, pi int, starts []risk.Territory) (routeSearchResult, bool) {
	if len(starts) == 0 {
		return routeSearchResult{}, false
	}
	order := orderIndex(g)

	visited := make(map[risk.Territory]bool)
	h := &routeHeap{}
	for _, b := range starts {
		heap.Push(h, routeNode{t: b, from: "", cost: 0, order: order[b]})
	}

	for h.Len() > 0 {
		cur := heap.Pop(h).(routeNode)
		if visited[cur.t] {
			continue
		}
		visited[cur.t] = true
		if g.Territories[cur.t].Owner == pi {
			return routeSearchResult{found: cur.t, cost: cur.cost, nextHop: cur.from, hasHop: cur.from != ""}, true
		}
		for _, neighbor := range g.Board.Order {
			if visited[neighbor] || !g.Board.IsAdjacent(cur.t, neighbor) {
				continue
			}
			cost := cur.cost
			if g.Territories[neighbor].Owner != pi {
				cost += g.Territories[neighbor].Armies
			}
			heap.Push(h, routeNode{t: neighbor, from: cur.t, cost: cost, order: order[neighbor]})
		}
	}
	return routeSearchResult{}, false
}

// cheapestRouteToContinent returns the pi-owned territory reachable from
// cont's border at the lowest weighted cost, used as a placement fallback
// when pi owns nothing in cont itself.
func cheapestRouteToContinent(g *risk.Game, pi int, cont risk.Continent) (risk.Territory, bool) {
	res, ok := cheapestRouteSearch(g, pi, continentBorders(g, cont))
	if !ok {
		return "", false
	}
	return res.found, true
}

// cheapestRouteToContinentWithCost is cheapestRouteToContinent plus the
// route's accumulated cost, used by BoscoeStrategy's placementToKillPlayer
// to compare routes across several of a target player's owned continents
// and place toward the globally cheapest.
func cheapestRouteToContinentWithCost(g *risk.Game, pi int, cont risk.Continent) (t risk.Territory, cost int, ok bool) {
	res, ok := cheapestRouteSearch(g, pi, continentBorders(g, cont))
	if !ok {
		return "", 0, false
	}
	return res.found, res.cost, true
}

// cheapestAttackHopToContinent finds the same cheapest route as
// cheapestRouteToContinent, but returns the (from, to) attack pair one hop
// closer to cont from the found owned territory, plus the route's total
// remaining cost -- Lux's BoardHelper.easyCostFromCountryToContinent,
// reused here via the same global search rather than Lux's own per-origin
// iteration over every owned territory (see Lux_Port_Notes.md's Boscoe
// addendum: a deliberate, principled simplification -- the global search
// always finds a route at least as good as trying each owned territory in
// turn). ok is false if pi owns nothing reachable, or if the found
// territory already borders cont directly (no intermediate hop exists to
// attack into).
func cheapestAttackHopToContinent(g *risk.Game, pi int, cont risk.Continent) (from, to risk.Territory, cost int, ok bool) {
	res, ok := cheapestRouteSearch(g, pi, continentBorders(g, cont))
	if !ok || !res.hasHop {
		return "", "", 0, false
	}
	return res.found, res.nextHop, res.cost, true
}

// ownedTerritories returns pi's owned territories in canonical board
// order -- used as cheapestRouteSearch's starting set for
// cheapestAttackHopToPlayer (unlike the continent-scoped wrappers above,
// which start from continentBorders instead).
func ownedTerritories(g *risk.Game, pi int) []risk.Territory {
	var out []risk.Territory
	for _, t := range g.Board.Order {
		if g.Territories[t].Owner == pi {
			out = append(out, t)
		}
	}
	return out
}

// cheapestAttackHopToPlayer finds the cheapest attack route from pi's own
// territory toward any territory owned by target, returning the (from,
// to) attack pair one hop closer plus the route's cost -- Lux's
// Vulture/Killbot cluster-Hamiltonian-path precomputation
// (placeToKill/attackAlongRoute), replaced by the same clean single-hop
// router cheapestAttackHopToContinent already uses (see
// Lux_Port_Notes.md's Killbot addendum): rather than precomputing a route
// through target's entire connected territory cluster and committing to
// attack all the way through in one turn, this finds the cheapest single
// hop toward any of target's territory; once it lands, subsequent
// NextCommand calls recompute fresh and naturally continue eating into
// target's holdings. Unlike cheapestAttackHopToContinent, hasHop is
// always true whenever a route is found at all: the search starts from
// target's own territories (never pi's, since target != pi), so the
// found pi-owned territory always has a real predecessor. ok is false
// only if pi owns nothing reachable from target's territory.
func cheapestAttackHopToPlayer(g *risk.Game, pi, target int) (from, to risk.Territory, cost int, ok bool) {
	res, ok := cheapestRouteSearch(g, pi, ownedTerritories(g, target))
	if !ok || !res.hasHop {
		return "", "", 0, false
	}
	return res.found, res.nextHop, res.cost, true
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

// nextTradeValue duplicates risk.Game's own unexported nextTradeValue
// exactly (internal/risk/engine.go): the reinforcement value of the next
// card set cashed, given setsTraded sets already traded so far this
// game.
func nextTradeValue(setsTraded int) int {
	setNumber := setsTraded + 1
	if setNumber <= 5 {
		return 2*setNumber + 2
	}
	if setNumber == 6 {
		return 15
	}
	return 15 + (setNumber-6)*5
}

// killTarget identifies which living rival, if any, pi should try to
// eliminate outright -- Lux's Vulture.setToKillPlayer. Among rivals still
// in the game whose total armies+territories a real attack from pi's own
// strongest attacking stack (biggestArmyTerritoryWithEnemyNeighbor)
// plausibly beats, picks the one with the lowest card-adjusted armies
// (discounting the value of the next card set they could cash, since
// they'll effectively be stronger once they do); only returns it if pi's
// own total armies exceed twice that adjusted figure. Recomputed fresh
// from both a placement and an attack decision -- this engine has no
// equivalent to Lux's turn-scoped toKillPlayer field (see
// Lux_Port_Notes.md's Killbot addendum), and Lux's own placement-time
// slack (+ numberOfArmies, since more armies are about to land) is
// dropped for the same reason pixieWantedContinents' was: no
// "about-to-be-placed" figure exists outside the reinforce call itself.
func killTarget(g *risk.Game, pi int) (target int, ok bool) {
	strongest, hasStrongest := biggestArmyTerritoryWithEnemyNeighbor(g, pi)
	if !hasStrongest {
		return 0, false
	}
	strongestArmies := float64(g.Territories[strongest].Armies)
	nextSetValue := float64(nextTradeValue(g.SetsTraded))

	lowest := 0.0
	found := false
	for i, p := range g.Players {
		if i == pi || p.Eliminated {
			continue
		}
		armies := float64(playerTotalArmies(g, i))
		territories := 0
		for _, t := range g.Board.Order {
			if g.Territories[t].Owner == i {
				territories++
			}
		}
		if territories == 0 || strongestArmies <= armies+float64(territories) {
			continue
		}
		adjusted := armies - nextSetValue*float64(len(p.Cards))/3.0
		if !found || adjusted < lowest {
			lowest, target, found = adjusted, i, true
		}
	}
	if !found {
		return 0, false
	}
	if float64(playerTotalArmies(g, pi)) <= lowest*2 {
		return 0, false
	}
	return target, true
}
