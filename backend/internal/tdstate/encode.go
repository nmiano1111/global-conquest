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
	Phase                           []bool    // one-hot, allPhases order
	IsMyTurn                        bool
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
			IsMine:            ts.Owner == pi,
			ArmyFraction:      float64(ts.Armies) / float64(totalArmies),
			Continent:         continentOneHot,
			IsContinentBorder: border,
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
