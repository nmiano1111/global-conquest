package bot

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// StrategyBetterPixieV1 is the registry identifier for BetterPixieStrategy
// -- Lux's actual Killbot backer (see Lux_Port_Notes.md's Killbot
// addendum, and strategy_killbot.go), a more sophisticated relative of
// the already-ported plain Pixie: smarter continent-selection math
// (accounts for reinforcements en route, not just armies already
// in/adjoining a continent), a genuinely different two-pass attack
// sequence, an entirely separate "take out a continent some other single
// player owns outright" attack mechanism (takeOutContinentCheck), and
// continent-aware fortify logic.
const StrategyBetterPixieV1 = "betterpixie-v1"

// BetterPixieStrategy implements StrategyBetterPixieV1.
type BetterPixieStrategy struct{}

// NewBetterPixieStrategy creates a BetterPixieStrategy.
func NewBetterPixieStrategy() *BetterPixieStrategy { return &BetterPixieStrategy{} }

// NextCommand dispatches on g.Phase, mirroring PixieStrategy/ClusterStrategy's
// shape. It always returns a zero-value Explanation, since BetterPixie has
// no scoring model to report.
func (bp *BetterPixieStrategy) NextCommand(_ context.Context, g *risk.Game, playerID string) (Command, Explanation, error) {
	cmd, err := bp.nextCommand(g, playerID)
	return cmd, Explanation{}, err
}

func (bp *BetterPixieStrategy) nextCommand(g *risk.Game, playerID string) (Command, error) {
	switch g.Phase {
	case risk.PhaseSetupReinforce:
		return bp.setupReinforce(g, playerID)
	case risk.PhaseReinforce:
		return bp.reinforce(g, playerID)
	case risk.PhaseAttack:
		return bp.attack(g, playerID)
	case risk.PhaseOccupy:
		return bp.occupy(g, playerID)
	case risk.PhaseFortify:
		return bp.fortify(g, playerID)
	default:
		return Command{}, fmt.Errorf("bot: %s has no move for phase %q", StrategyBetterPixieV1, g.Phase)
	}
}

// betterPixieWantedContinents returns every continent BetterPixie will
// commit placement/attack effort to right now -- Lux's
// BetterPixie.setupOurConts, a more sophisticated relative of the
// already-ported pixieWantedContinents: discounts enemy armies by a 1.3x
// loss multiplier, and credits whichever is larger of (a) armies already
// in/adjoining the continent or (b) armies "farther away" en route via
// cheapestRouteToContinentWithCost, discounted by 1.2x the route's cost
// (Lux's own ourArmiesFartherAway term) -- not just (a) alone, the way
// plain Pixie's neededForCont does. When pi owns no positive-bonus
// continent at all, targets only the single cheapest one to take;
// otherwise independently qualifies each continent against a
// numberOfArmies/(numContinents/4.0) budget, Lux's own per-continent share
// of one placement call. numberOfArmies is 0 outside the reinforce phase
// (attack/occupy have no new armies to allocate this call), which is the
// correct value for "how many new armies am I placing right now," not a
// stand-in for Lux's own turn-cached ourConts.
func betterPixieWantedContinents(g *risk.Game, pi int, numberOfArmies int) map[risk.Continent]bool {
	wanted := make(map[risk.Continent]bool)
	needed := make(map[risk.Continent]float64, len(g.Board.Continents))
	for _, cont := range continentOrder(g) {
		n := float64(enemyArmiesInContinent(g, cont, pi)) * 1.3
		nearby := playerArmiesInContinent(g, pi, cont) + playerArmiesAdjoiningContinent(g, pi, cont)
		fartherAway := 0
		if t, cost, ok := cheapestRouteToContinentWithCost(g, pi, cont); ok {
			fartherAway = g.Territories[t].Armies - int(float64(cost)*1.2)
		}
		if fartherAway > nearby {
			n -= float64(fartherAway)
		} else {
			n -= float64(nearby)
		}
		needed[cont] = n
	}

	if !ownsAnyPositiveContinent(g, pi) {
		var target risk.Continent
		lowest := math.MaxFloat64
		found := false
		for _, cont := range continentOrder(g) {
			if g.Board.Continents[cont].Bonus > 0 && needed[cont] < lowest {
				lowest, target, found = needed[cont], cont, true
			}
		}
		if found {
			wanted[target] = true
		}
		return wanted
	}

	numConts := float64(len(g.Board.Continents))
	budget := float64(numberOfArmies) / (numConts / 4.0)
	for _, cont := range continentOrder(g) {
		if g.Board.Continents[cont].Bonus > 0 && needed[cont] < budget {
			wanted[cont] = true
		}
	}
	return wanted
}

// weOwnContsArround reports whether pi fully owns every continent
// bordering t other than t's own -- Lux's Pixie.weOwnContsArround, used
// by BetterPixie's border-safety check (not plain Pixie's own
// continentNeedsHelp) to treat a low-army border as safe when it's
// surrounded by continents pi already owns outright.
func weOwnContsArround(g *risk.Game, pi int, t risk.Territory) bool {
	tc := territoryContinent(g)
	cont := tc[t]
	for other := range g.Board.Adjacent[t] {
		if tc[other] != cont && !ownsContinent(g, pi, tc[other]) {
			return false
		}
	}
	return true
}

// betterPixieContinentNeedsHelp reports whether cont is worth sending
// reinforcements to -- Lux's BetterPixie.continentNeedsHelp/
// borderCountryNeedsHelp: pi doesn't fully own it yet, or one of its
// borders has pixieBorderForce armies or fewer AND isn't surrounded by
// continents pi already owns (weOwnContsArround). The
// weOwnContsArround condition is what distinguishes this from plain
// Pixie's own continentNeedsHelp, which has no such carve-out.
func betterPixieContinentNeedsHelp(g *risk.Game, pi int, cont risk.Continent) bool {
	if !ownsContinent(g, pi, cont) {
		return true
	}
	for _, border := range continentBorders(g, cont) {
		if g.Territories[border].Armies <= pixieBorderForce && !weOwnContsArround(g, pi, border) {
			return true
		}
	}
	return false
}

// clusterArmies sums the armies of every territory in cluster.
func clusterArmies(g *risk.Game, cluster []risk.Territory) int {
	total := 0
	for _, t := range cluster {
		total += g.Territories[t].Armies
	}
	return total
}

// estimatedArmiesNeededToConquer is Lux's
// CountryCluster.estimatedNumberOfArmiesNeededToConquer(): (armies in the
// cluster + territory count) * 1.2.
func estimatedArmiesNeededToConquer(g *risk.Game, cluster []risk.Territory) int {
	return int(float64(clusterArmies(g, cluster)+len(cluster)) * 1.2)
}

// placeNearWeakestEnemyCluster picks BetterPixie's
// placeRemainder/placeNearEnemies(numberOfArmies, minimumToWin=true)
// target: the weakest (fewest total armies) connected enemy cluster
// board-wide pi borders, placing exactly
// estimatedArmiesNeededToConquer(cluster) minus pi's current armies on
// its strongest owned neighbor of that cluster -- Lux's own per-cluster
// loop granularity, one cluster (one command) at a time; a subsequent
// call naturally proceeds to the next-weakest cluster once this one's
// already-placed armies make it no longer the weakest, or once it no
// longer needs reinforcement at all.
func placeNearWeakestEnemyCluster(g *risk.Game, pi int, numberOfArmies int) (risk.Territory, int, bool) {
	components := allNonOwnedComponents(g, pi)
	sort.Slice(components, func(i, j int) bool {
		return clusterArmies(g, components[i]) < clusterArmies(g, components[j])
	})
	order := orderIndex(g)
	for _, cluster := range components {
		var best risk.Territory
		bestArmies, ok := -1, false
		for _, t := range cluster {
			for other := range g.Board.Adjacent[t] {
				if g.Territories[other].Owner != pi {
					continue
				}
				if a := g.Territories[other].Armies; !ok || a > bestArmies ||
					(a == bestArmies && order[other] < order[best]) {
					best, bestArmies, ok = other, a, true
				}
			}
		}
		if !ok {
			continue
		}
		need := estimatedArmiesNeededToConquer(g, cluster) - bestArmies
		if need <= 0 {
			continue
		}
		return best, min(need, numberOfArmies), true
	}
	return "", 0, false
}

// betterPixiePlacement picks one placement decision per call --
// BetterPixie's placeArmies round-robins one army at a time across every
// wanted-and-needy continent; each Go call reproduces exactly one
// iteration of that while-loop (1 army to the first wanted continent in
// continentOrder that betterPixieContinentNeedsHelp), naturally
// re-deriving "does another continent still need help" on the next call
// the same way repeated NextCommand invocations already reproduce every
// other Lux while-loop in this codebase. Falls to
// placeNearWeakestEnemyCluster (a whole cluster's minimum-to-conquer
// amount per call, matching Lux's own per-cluster loop granularity) once
// no wanted continent needs help, and to bestByEnemyNeighborCount's
// full-remainder dump as the final fallback, matching setupOurConts
// finding nothing worth pursuing at all.
func betterPixiePlacement(g *risk.Game, pi int, actions []risk.ReinforcementAction, numberOfArmies int) (risk.Territory, int) {
	wanted := betterPixieWantedContinents(g, pi, numberOfArmies)
	for _, cont := range continentOrder(g) {
		if wanted[cont] && betterPixieContinentNeedsHelp(g, pi, cont) {
			return placeToTakeSpecificContinent(g, pi, actions, cont), 1
		}
	}
	if len(wanted) == 0 {
		return placeToTakeContinent(g, pi, actions), numberOfArmies
	}
	if t, chunk, ok := placeNearWeakestEnemyCluster(g, pi, numberOfArmies); ok {
		return t, chunk
	}
	get := func(a risk.ReinforcementAction) risk.Territory { return a.Territory }
	return bestByEnemyNeighborCount(g, pi, actions, get), numberOfArmies
}

// setupReinforce places the one initial army per call via
// betterPixiePlacement.
func (bp *BetterPixieStrategy) setupReinforce(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalSetupReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal setup reinforcement for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	reinforceActions := setupReinforceActionsAsReinforcements(actions)
	t, _ := betterPixiePlacement(g, pi, reinforceActions, 1)
	return Command{Action: ActionPlaceInitialArmy, Territory: string(t)}, nil
}

// reinforce trades any legal card set whenever one exists, not just
// mandatory ones -- Lux's BetterPixie.cardsPhase calls
// cashCardsIfPossible via super.cardsPhase, so voluntaryCardTurnIn
// (shared with Killbot/Quo/Boscoe, whose own Lux sources have the
// identical override) is the faithful port here, not the mandatory-only
// risk.CardTurnInRequired gate other, non-cashing personas use.
// Placement is one betterPixiePlacement decision per call.
func (bp *BetterPixieStrategy) reinforce(g *risk.Game, playerID string) (Command, error) {
	if cmd, ok := voluntaryCardTurnIn(g, playerID); ok {
		return cmd, nil
	}
	actions := risk.LegalReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal reinforcement for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	t, armies := betterPixiePlacement(g, pi, actions, g.PendingReinforcements)
	return Command{Action: ActionPlaceReinforcement, Territory: string(t), Armies: armies}, nil
}

// allNonOwnedComponents partitions every territory not owned by pi into
// connected components -- Lux's CountryClusterSet.getAllCountriesNotOwnedBy,
// the clustering placeNearWeakestEnemyCluster needs directly. Also the
// basis for occupy's hostile-connectivity check below: Lux's
// getHostileCountries expands its seed list through any enemy-owned
// neighbor board-wide (verified directly against CountryClusterSet.java),
// not just the seed list's own members -- i.e. it's really asking "do
// these seed territories all fall in the same board-wide enemy
// component," which this same partition answers without reclustering
// from scratch.
func allNonOwnedComponents(g *risk.Game, pi int) [][]risk.Territory {
	var ts []risk.Territory
	for _, t := range g.Board.Order {
		if g.Territories[t].Owner != pi {
			ts = append(ts, t)
		}
	}
	return connectedComponents(g, ts)
}

// occupy ports BetterPixie.moveArmiesIn: the initial branch only checks
// for a *zero* raw enemy-neighbor count on either side (unlike plain
// Pixie's own moveArmiesIn, which compares magnitudes at any nonzero
// tie); the continent-scoped fallback replaces Pixie's count comparison
// with a connectivity check -- move everyone in only if both sides'
// wanted-continent enemy neighbors (combined, deduplicated only when
// checking connectivity, not when building each side's own list, to
// match Lux's independent attackerEnemyList/defenderEnemyList) fall
// within a single board-wide non-owned connected component.
func (bp *BetterPixieStrategy) occupy(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalOccupations(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal occupation for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	minArmies, maxArmies := actions[0].Armies, actions[len(actions)-1].Armies
	halfArmies := (minArmies + maxArmies) / 2
	from, to := g.Occupy.From, g.Occupy.To

	aEnemies, dEnemies := enemyNeighborCount(g, from, pi), enemyNeighborCount(g, to, pi)
	switch {
	case aEnemies == 0 && dEnemies != 0:
		return Command{Action: ActionOccupy, Armies: maxArmies}, nil
	case aEnemies != 0 && dEnemies == 0:
		return Command{Action: ActionOccupy, Armies: minArmies}, nil
	case dEnemies == 0:
		return Command{Action: ActionOccupy, Armies: halfArmies}, nil
	}

	wanted := betterPixieWantedContinents(g, pi, 0)
	tc := territoryContinent(g)
	wantedEnemyNeighbors := func(t risk.Territory) []risk.Territory {
		var out []risk.Territory
		for other := range g.Board.Adjacent[t] {
			if g.Territories[other].Owner != pi && wanted[tc[other]] {
				out = append(out, other)
			}
		}
		return out
	}
	aList, dList := wantedEnemyNeighbors(from), wantedEnemyNeighbors(to)

	switch {
	case len(aList) == 0 && len(dList) != 0:
		return Command{Action: ActionOccupy, Armies: maxArmies}, nil
	case len(aList) != 0 && len(dList) == 0:
		return Command{Action: ActionOccupy, Armies: minArmies}, nil
	case len(aList) == 0:
		return Command{Action: ActionOccupy, Armies: halfArmies}, nil
	}

	seen := make(map[risk.Territory]bool)
	var combined []risk.Territory
	for _, t := range append(append([]risk.Territory(nil), aList...), dList...) {
		if !seen[t] {
			seen[t] = true
			combined = append(combined, t)
		}
	}
	componentOf := make(map[risk.Territory]int)
	for ci, comp := range allNonOwnedComponents(g, pi) {
		for _, t := range comp {
			componentOf[t] = ci
		}
	}
	first, allSame := -1, true
	for _, t := range combined {
		if first == -1 {
			first = componentOf[t]
		} else if componentOf[t] != first {
			allSame = false
			break
		}
	}
	if allSame {
		return Command{Action: ActionOccupy, Armies: maxArmies}, nil
	}
	return Command{Action: ActionOccupy, Armies: halfArmies}, nil
}

// betterPixieAttackInContinent tries to take over cont -- Lux's
// BetterPixie.attackInContinent, a genuinely different algorithm from
// plain Pixie's own attackInContinent (reused verbatim as this
// function's own pass 2, below). Gates on a continent-wide
// friendlyCount-vs-enemyCount*1.3 check (with enemyCount padded by 1 per
// enemy-owned territory in cont, matching Lux's own "add a multiple for
// losses and for the # of enemy countries"), then cycles two passes
// until neither makes progress: pass 1 only attacks from territories
// with exactly one enemy neighbor within wanted continents
// (adjacentEnemiesInWantedContinents == 1); pass 2 is plain Pixie's own
// "any owned neighbor outnumbers the target" scan, textually identical
// in the Lux source, so reused directly rather than reimplemented.
func betterPixieAttackInContinent(g *risk.Game, pi int, cont risk.Continent, wanted map[risk.Continent]bool) (Command, bool) {
	enemyCount := float64(enemyArmiesInContinent(g, cont, pi)) * 1.3
	for _, t := range g.Board.Continents[cont].Territories {
		if g.Territories[t].Owner != pi {
			enemyCount++
		}
	}
	friendlyCount := playerArmiesInContinent(g, pi, cont) + playerArmiesAdjoiningContinent(g, pi, cont)
	if enemyCount > float64(friendlyCount) {
		return Command{}, false
	}

	tc := territoryContinent(g)
	for _, t := range g.Board.Continents[cont].Territories {
		src := g.Territories[t]
		if src.Owner != pi || src.Armies <= 1 || adjacentEnemiesInWantedContinents(g, pi, t, wanted) != 1 {
			continue
		}
		for other := range g.Board.Adjacent[t] {
			dst := g.Territories[other]
			if dst.Owner != pi && wanted[tc[other]] && src.Armies > dst.Armies {
				return Command{Action: ActionAttack, From: string(t), To: string(other), AttackerDice: min(3, src.Armies-1)}, true
			}
		}
	}
	return attackInContinent(g, pi, cont)
}

// soleOwnerOfContinent returns the player who owns every territory in
// cont, if exactly one player does.
func soleOwnerOfContinent(g *risk.Game, cont risk.Continent) (pi int, ok bool) {
	territories := g.Board.Continents[cont].Territories
	if len(territories) == 0 {
		return -1, false
	}
	pi = g.Territories[territories[0]].Owner
	if pi < 0 {
		return -1, false
	}
	for _, t := range territories[1:] {
		if g.Territories[t].Owner != pi {
			return -1, false
		}
	}
	return pi, true
}

// takeOutContinentCheck scans every positive-bonus continent some other
// single player owns outright for a border spot where an owned neighbor
// has more than double its armies, attacking there -- BetterPixie's
// takeOutContinentCheck, an entirely new mechanism absent from every
// previously-ported persona. Package-level (not a BetterPixieStrategy
// method) since any future continent-aware persona could reuse it
// identically, matching this file's established package-level
// attack-stage convention. Returns the first qualifying attack across
// continentOrder, matching Lux's own early return after the first
// successful attack.
func takeOutContinentCheck(g *risk.Game, pi int) (Command, bool) {
	for _, cont := range continentOrder(g) {
		if g.Board.Continents[cont].Bonus <= 0 {
			continue
		}
		owner, ok := soleOwnerOfContinent(g, cont)
		if !ok || owner == pi {
			continue
		}
		for _, border := range continentBorders(g, cont) {
			for _, other := range g.Board.Order {
				if other == border || !g.Board.IsAdjacent(border, other) {
					continue
				}
				src := g.Territories[other]
				if src.Owner == pi && src.Armies > g.Territories[border].Armies*2 {
					return Command{Action: ActionAttack, From: string(other), To: string(border), AttackerDice: min(3, src.Armies-1)}, true
				}
			}
		}
	}
	return Command{}, false
}

// attack tries every wanted continent (deterministic order) via
// betterPixieAttackInContinent, then takeOutContinentCheck, then
// attackForCard, then -- only if dominant or stalemated --
// attackAsMuchAsPossible. Lux's BetterPixie.attackPhase order exactly.
func (bp *BetterPixieStrategy) attack(g *risk.Game, playerID string) (Command, error) {
	pi := playerIndex(g, playerID)
	wanted := betterPixieWantedContinents(g, pi, 0)
	for _, cont := range continentOrder(g) {
		if wanted[cont] {
			if cmd, ok := betterPixieAttackInContinent(g, pi, cont, wanted); ok {
				return cmd, nil
			}
		}
	}
	if cmd, ok := takeOutContinentCheck(g, pi); ok {
		return cmd, nil
	}
	if cmd, ok := attackForCard(g, pi); ok {
		return cmd, nil
	}
	if shouldGoHogWild(g, pi) {
		if cmd, ok := attackAsMuchAsPossible(g, pi); ok {
			return cmd, nil
		}
	}
	return Command{Action: ActionEndAttack}, nil
}

// continentBorderDistance returns, for every pi-owned territory in cont,
// its hop distance to the nearest pi-owned border territory of cont
// (distance 0 for the borders themselves) -- a BFS through pi-owned
// territory within cont only, matching Lux's fortifyContinent's own
// constraint that its interior-discovery walk only crosses
// neighbor.getContinent() == cont. Territories pi doesn't own, or that
// aren't in cont, are absent from the result.
func continentBorderDistance(g *risk.Game, pi int, cont risk.Continent) map[risk.Territory]int {
	tc := territoryContinent(g)
	dist := make(map[risk.Territory]int)
	var queue []risk.Territory
	for _, t := range continentBorders(g, cont) {
		if g.Territories[t].Owner == pi {
			dist[t] = 0
			queue = append(queue, t)
		}
	}
	for len(queue) > 0 {
		t := queue[0]
		queue = queue[1:]
		for other := range g.Board.Adjacent[t] {
			if tc[other] != cont || g.Territories[other].Owner != pi {
				continue
			}
			if _, seen := dist[other]; seen {
				continue
			}
			dist[other] = dist[t] + 1
			queue = append(queue, other)
		}
	}
	return dist
}

// betterPixieFortifyDestination picks the single legal (from, to) pair
// that best matches BetterPixie's own fortify criterion, collapsed to
// "rank every legal pair, pick the best" per this engine's one-fortify-
// per-turn rule (see Lux_Port_Notes.md): for a pair within one continent
// pi owns outright, score by how many hops closer to that continent's
// own border the move gets (Lux's fortifyContinent, which drains the
// whole continent toward its borders one BFS layer at a time); every
// other legal pair (different continents, or a same-continent pair in a
// continent pi doesn't own outright) scores by the destination's weakest
// nearby enemy neighbor, most-vulnerable-first (Lux's
// fortifyContinentScraps, which moves armies toward whichever owned
// neighbor itself sits closest to a weak enemy). These two regimes use
// different scales (a small hop count vs. a negated army count) and are
// compared directly against each other via one global best-score scan --
// an approximation, not a faithful priority order, matching every other
// persona's fortify port already accepted in this codebase: Lux's own
// per-continent while-loops can issue many fortifications in one turn,
// which this engine's single-fortify-per-turn rule can never replicate
// regardless of scoring precision.
func betterPixieFortifyDestination(g *risk.Game, pi int, actions []risk.FortificationAction) (best risk.FortificationAction, ok bool) {
	tc := territoryContinent(g)
	order := orderIndex(g)
	bestScore := math.MinInt
	for _, a := range actions {
		var score int
		if fromCont := tc[a.From]; fromCont == tc[a.To] && ownsContinent(g, pi, fromCont) {
			dist := continentBorderDistance(g, pi, fromCont)
			fromDist, fromOK := dist[a.From]
			toDist, toOK := dist[a.To]
			if !fromOK || !toOK {
				continue
			}
			score = fromDist - toDist
		} else if weakest, wOK := weakestEnemyNeighbor(g, a.To, pi); wOK {
			score = -g.Territories[weakest].Armies
		} else {
			score = math.MinInt + 1
		}
		if !ok || score > bestScore || (score == bestScore && order[a.To] < order[best.To]) {
			best, bestScore, ok = a, score, true
		}
	}
	return best, ok
}

// fortify uses betterPixieFortifyDestination, ending the turn without
// fortifying if nothing qualifies.
func (bp *BetterPixieStrategy) fortify(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalFortifications(g, playerID)
	pi := playerIndex(g, playerID)
	best, ok := betterPixieFortifyDestination(g, pi, actions)
	if !ok {
		return Command{Action: ActionEndTurn}, nil
	}
	return Command{Action: ActionFortify, From: string(best.From), To: string(best.To), Armies: best.MaxArmies}, nil
}
