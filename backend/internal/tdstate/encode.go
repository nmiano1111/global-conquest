// Package tdstate encodes a risk.Game into a fixed-width, player-relative
// feature vector for a TD(λ)-trained whole-board value function --
// player-relative (ownership expressed as mine/enemy, not an absolute
// seat index) so the same fixed-width encoding works uniformly across
// our variable 3-6 player games, unlike a fixed one-hot-per-seat scheme.
//
// Depends only on internal/risk, not internal/bot -- internal/simulation
// (which already depends on internal/bot) uses this package to capture
// turn-boundary training data, and a future internal/bot Strategy would
// import this package too (bot -> tdstate, never the reverse, avoiding an
// import cycle).
package tdstate

import (
	"slices"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// TerritoryFeatures is one territory's contribution to a Features vector,
// from the encoding viewer's perspective.
type TerritoryFeatures struct {
	IsMine            bool
	ArmyFraction      float64
	Continent         []bool // one-hot, sortedContinents(board) order
	IsContinentBorder bool
	// EnemyThreatFraction is the sum of armies in territories adjacent to
	// this one, owned by anyone other than pi, as a fraction of the
	// board's total armies. Added after a live-play audit found
	// ValueStrategy's reinforce phase locking onto one "favorite"
	// territory per game (a single territory receiving up to ~47% of all
	// reinforcements placed that game) and never reinforcing embattled
	// border territories instead: every per-territory ArmyFraction
	// coefficient a fit ever produced was small but strictly positive
	// (more armies here is always slightly good, with no notion of "this
	// territory needs it"), so the model had no way to prefer a
	// threatened territory over its fixed favorite. This mirrors
	// internal/bot's local adjacentEnemyArmies signal (used by
	// ScoredStrategy's reinforceSignals) -- not imported (this package
	// must not depend on internal/bot, see the package doc comment),
	// recomputed here at the whole-board level instead.
	EnemyThreatFraction float64
}

// GlobalFeatures holds the non-per-territory signals in a Features
// vector, from the encoding viewer's perspective.
type GlobalFeatures struct {
	MyArmyFraction                  float64
	MyTerritoryFraction             float64
	MyIncomeFraction                float64
	StrongestEnemyArmyFraction      float64
	StrongestEnemyTerritoryFraction float64
	ContinentArmyFraction           []float64 // sortedContinents(board) order
	CardFraction                    float64   // len(cards) / cardFractionCap
	// Defence estimates how thin pi's weakest defended front is, per
	// continent pi fully owns -- ported from Jamie Carr's "Using Graph
	// Convolutional Networks and TD(λ) to Play the Game of Risk"
	// (arXiv:2009.06355), the one hand-crafted feature that paper's
	// author found necessary to add after the network failed to learn
	// defensive behavior from low-level features alone: "the only
	// instance where I had to build in human knowledge... I was sure to
	// keep the feature global and not incorporate information about
	// specific threats" -- unlike EnemyThreatFraction, this never looks
	// at enemy army counts, only at how weak pi's own frontier is.
	Defence  float64
	Phase    []bool // one-hot, allPhases order
	IsMyTurn bool
}

// Features is one player's encoded view of a game state.
type Features struct {
	Territories []TerritoryFeatures // g.Board.Order order
	Global      GlobalFeatures
}

// cardFractionCap bounds CardFraction the same way every other Features
// field is bounded to roughly [0,1] -- 5 is the mandatory-trade-in
// threshold (risk.CardTurnInRequired), the natural cap for "how many
// cards is a lot."
const cardFractionCap = 5.0

// defenceCap bounds Defence the same way cardFractionCap bounds
// CardFraction -- the paper found uncapped values could reach abnormal
// highs early/late game and confuse training, capping at 0.2 before
// standardizing.
const defenceCap = 0.2

// allPhases is every risk.Phase this encoder can one-hot, in a fixed
// (not data-derived) canonical order -- the full enum, not just the ones
// a turn boundary can observe, since Encode is also meant for mid-phase
// afterstates (future work).
var allPhases = []risk.Phase{
	risk.PhaseSetupClaim,
	risk.PhaseSetupReinforce,
	risk.PhaseReinforce,
	risk.PhaseAttack,
	risk.PhaseOccupy,
	risk.PhaseFortify,
	risk.PhaseGameOver,
}

// sortedContinents returns board's continents in a fixed, deterministic
// order -- map iteration order in Go is randomized, so every one-hot
// slice (TerritoryFeatures.Continent, GlobalFeatures.ContinentArmyFraction)
// needs a stable order to be a meaningful, comparable feature position
// across encodings.
func sortedContinents(board risk.Board) []risk.Continent {
	continents := make([]risk.Continent, 0, len(board.Continents))
	for c := range board.Continents {
		continents = append(continents, c)
	}
	slices.Sort(continents)
	return continents
}

// isContinentBorder reports whether t has a neighbor outside continent c
// -- duplicated from internal/bot's identical helper (strategy_scored_reinforce.go)
// rather than imported, since internal/bot will eventually depend on this
// package (a future TDValueStrategy), and the reverse dependency would be
// a cycle.
func isContinentBorder(board risk.Board, t risk.Territory, c risk.Continent) bool {
	inContinent := make(map[risk.Territory]struct{}, len(board.Continents[c].Territories))
	for _, ct := range board.Continents[c].Territories {
		inContinent[ct] = struct{}{}
	}
	for other := range board.Adjacent[t] {
		if _, in := inContinent[other]; !in {
			return true
		}
	}
	return false
}

// enemyThreatFraction sums the armies in territories adjacent to t owned
// by anyone other than pi (unowned/eliminated-vacated territories don't
// contribute, matching risk.TerritoryState's Owner < 0 convention used
// elsewhere in this package), as a fraction of totalArmies.
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

// isFrontier reports whether t, owned by pi, has a neighbor owned by a
// different, non-eliminated-vacated player.
func isFrontier(g *risk.Game, t risk.Territory, pi int) bool {
	if g.Territories[t].Owner != pi {
		return false
	}
	for neighbor := range g.Board.Adjacent[t] {
		ts := g.Territories[neighbor]
		if ts.Owner >= 0 && ts.Owner != pi {
			return true
		}
	}
	return false
}

// computeDefence implements GlobalFeatures.Defence: for each continent pi
// fully owns, finds the minimum army count among pi's own territories in
// that continent that are a frontier (isFrontier) -- the weakest link
// defending that continent's front, if any front exists at all (a fully
// owned continent with no adjacent enemy anywhere contributes nothing).
// Collects one such minimum per continent, sorted smallest to largest,
// and takes a weighted mean (weights n, n-1, ..., 1, so the very weakest
// fronts dominate the average), divided by total board armies, capped at
// defenceCap.
//
// This is an interpretation of the paper's own description (which
// additionally traces a connected-owned-territory search per border
// territory) rather than a byte-exact reproduction -- it captures the
// same intent (how vulnerable is pi's weakest owned front) without that
// extra graph-search machinery, which doesn't change the core signal for
// a continent that's already fully owned (every territory in it is
// "connected" to every other by definition of owning the whole thing).
func computeDefence(g *risk.Game, pi int, continents []risk.Continent, totalArmies int) float64 {
	var weakestPerContinent []int
	for _, c := range continents {
		info := g.Board.Continents[c]
		owned := true
		for _, t := range info.Territories {
			if g.Territories[t].Owner != pi {
				owned = false
				break
			}
		}
		if !owned {
			continue
		}
		minArmies := -1
		for _, t := range info.Territories {
			if !isFrontier(g, t, pi) {
				continue
			}
			armies := g.Territories[t].Armies
			if minArmies == -1 || armies < minArmies {
				minArmies = armies
			}
		}
		if minArmies != -1 {
			weakestPerContinent = append(weakestPerContinent, minArmies)
		}
	}
	if len(weakestPerContinent) == 0 {
		return 0
	}
	slices.Sort(weakestPerContinent)

	n := len(weakestPerContinent)
	var weightedSum, weightSum float64
	for i, armies := range weakestPerContinent {
		weight := float64(n - i)
		weightedSum += weight * float64(armies)
		weightSum += weight
	}
	defence := (weightedSum / weightSum) / float64(totalArmies)
	return min(defence, defenceCap)
}

// Encode builds pi's perspective of g's current state.
func Encode(g *risk.Game, pi int) Features {
	continents := sortedContinents(g.Board)

	totalArmies := 0
	for _, ts := range g.Territories {
		totalArmies += ts.Armies
	}
	if totalArmies == 0 {
		totalArmies = 1 // avoid divide-by-zero; only possible before any armies are placed
	}

	territoryOf := make(map[risk.Continent][]risk.Territory, len(continents))
	for _, c := range continents {
		territoryOf[c] = g.Board.Continents[c].Territories
	}

	territories := make([]TerritoryFeatures, len(g.Board.Order))
	for i, t := range g.Board.Order {
		ts := g.Territories[t]
		continentOneHot := make([]bool, len(continents))
		var border bool
		for ci, c := range continents {
			if slices.Contains(territoryOf[c], t) {
				continentOneHot[ci] = true
				border = isContinentBorder(g.Board, t, c)
			}
		}
		territories[i] = TerritoryFeatures{
			IsMine:              ts.Owner == pi,
			ArmyFraction:        float64(ts.Armies) / float64(totalArmies),
			Continent:           continentOneHot,
			IsContinentBorder:   border,
			EnemyThreatFraction: enemyThreatFraction(g, t, pi, totalArmies),
		}
	}

	global := encodeGlobal(g, pi, continents, totalArmies)

	return Features{Territories: territories, Global: global}
}

func encodeGlobal(g *risk.Game, pi int, continents []risk.Continent, totalArmies int) GlobalFeatures {
	armiesOf := make(map[int]int)
	territoriesOf := make(map[int]int)
	for _, ts := range g.Territories {
		if ts.Owner < 0 {
			continue
		}
		armiesOf[ts.Owner] += ts.Armies
		territoriesOf[ts.Owner]++
	}
	totalTerritories := len(g.Board.Order)

	var strongestEnemyArmies, strongestEnemyTerritories int
	for i, p := range g.Players {
		if i == pi || p.Eliminated {
			continue
		}
		if armiesOf[i] > strongestEnemyArmies {
			strongestEnemyArmies = armiesOf[i]
		}
		if territoriesOf[i] > strongestEnemyTerritories {
			strongestEnemyTerritories = territoriesOf[i]
		}
	}

	continentArmyFraction := make([]float64, len(continents))
	for ci, c := range continents {
		info := g.Board.Continents[c]
		var continentTotal, mine int
		for _, t := range info.Territories {
			ts := g.Territories[t]
			continentTotal += ts.Armies
			if ts.Owner == pi {
				mine += ts.Armies
			}
		}
		if continentTotal > 0 {
			continentArmyFraction[ci] = float64(mine) / float64(continentTotal)
		}
	}

	phaseOneHot := make([]bool, len(allPhases))
	for i, p := range allPhases {
		phaseOneHot[i] = g.Phase == p
	}

	cardCount := 0
	if pi >= 0 && pi < len(g.Players) {
		cardCount = len(g.Players[pi].Cards)
	}

	return GlobalFeatures{
		MyArmyFraction:                  float64(armiesOf[pi]) / float64(totalArmies),
		MyTerritoryFraction:             float64(territoriesOf[pi]) / float64(totalTerritories),
		MyIncomeFraction:                incomeFraction(g, pi),
		StrongestEnemyArmyFraction:      float64(strongestEnemyArmies) / float64(totalArmies),
		StrongestEnemyTerritoryFraction: float64(strongestEnemyTerritories) / float64(totalTerritories),
		ContinentArmyFraction:           continentArmyFraction,
		CardFraction:                    float64(cardCount) / cardFractionCap,
		Defence:                         computeDefence(g, pi, continents, totalArmies),
		Phase:                           phaseOneHot,
		IsMyTurn:                        pi == g.CurrentPlayer,
	}
}

// incomeFraction estimates pi's next reinforcement income as a fraction
// of a generous upper bound, mirroring internal/risk's own (unexported)
// reinforcementsFor: max(3, territory count / 3) plus the bonus for every
// continent pi fully owns. The bound (totalTerritories/3 + sum of every
// continent's bonus) is deliberately generous -- it's never the true max
// (no player can own every continent while others still hold
// territories), just a stable denominator so the fraction stays roughly
// comparable across encodings rather than being an unbounded raw count.
func incomeFraction(g *risk.Game, pi int) float64 {
	territoryCount := 0
	for _, ts := range g.Territories {
		if ts.Owner == pi {
			territoryCount++
		}
	}
	income := max(3, territoryCount/3)

	maxBonus := 0
	for _, c := range g.Board.Continents {
		owned := true
		for _, t := range c.Territories {
			if g.Territories[t].Owner != pi {
				owned = false
				break
			}
		}
		if owned {
			income += c.Bonus
		}
		maxBonus += c.Bonus
	}

	bound := float64(len(g.Board.Order))/3.0 + float64(maxBonus)
	if bound == 0 {
		return 0
	}
	return float64(income) / bound
}
