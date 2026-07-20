package bot

import (
	"context"
	"fmt"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// StrategyPixieV1 is the registry identifier for PixieStrategy, the third
// Lux Delux-inspired persona (see
// project-docs/bot_player/proposals/Lux_Delux_AI_Research_Notes.md and
// Lux_Port_Notes.md). Where ClusterStrategy expands from a single landmass
// with no regard for continent identity, Pixie is continent-economy
// driven: each turn it estimates which continents it can plausibly
// take/hold given the current army balance, commits reinforcement and
// attacks to just those, defends their borders once owned, and always
// tries to grab a card.
const StrategyPixieV1 = "pixie-v1"

// pixieBorderForce is Lux's Pixie.borderForce: an owned continent border
// with this many armies or fewer is considered undefended.
const pixieBorderForce = 20

// PixieStrategy implements StrategyPixieV1.
type PixieStrategy struct{}

// NewPixieStrategy creates a PixieStrategy.
func NewPixieStrategy() *PixieStrategy { return &PixieStrategy{} }

// NextCommand dispatches on g.Phase, mirroring AngryStrategy/ClusterStrategy's
// shape. It always returns a zero-value Explanation, since Pixie has no
// scoring model to report.
func (p *PixieStrategy) NextCommand(_ context.Context, g *risk.Game, playerID string) (Command, Explanation, error) {
	cmd, err := p.nextCommand(g, playerID)
	return cmd, Explanation{}, err
}

func (p *PixieStrategy) nextCommand(g *risk.Game, playerID string) (Command, error) {
	switch g.Phase {
	case risk.PhaseSetupReinforce:
		return p.setupReinforce(g, playerID)
	case risk.PhaseReinforce:
		return p.reinforce(g, playerID)
	case risk.PhaseAttack:
		return p.attack(g, playerID)
	case risk.PhaseOccupy:
		return p.occupy(g, playerID)
	case risk.PhaseFortify:
		return p.fortify(g, playerID)
	default:
		return Command{}, fmt.Errorf("bot: %s has no move for phase %q", StrategyPixieV1, g.Phase)
	}
}

// pixieWantedContinents returns every positive-bonus continent pi can
// plausibly take/hold right now: Lux's Pixie.setupOurConts, using the same
// neededForCont formula (enemy armies in the continent, less pi's own
// armies in and adjoining it) but recomputed fresh from state on every
// call instead of cached once per turn (see Lux_Port_Notes.md's addendum
// on why -- ourConts has no equivalent under this engine's stateless
// architecture), and with Lux's placement-time slack
// (< (1/numContinents)*numberOfArmies, since more armies are about to
// land) tightened to a flat <= 0 (already self-sufficient), since no
// "about to be placed" figure exists outside the reinforce call itself.
func pixieWantedContinents(g *risk.Game, pi int) map[risk.Continent]bool {
	wanted := make(map[risk.Continent]bool)
	for _, cont := range continentOrder(g) {
		info := g.Board.Continents[cont]
		if info.Bonus <= 0 {
			continue
		}
		needed := enemyArmiesInContinent(g, cont, pi) - playerArmiesInContinent(g, pi, cont) - playerArmiesAdjoiningContinent(g, pi, cont)
		if needed <= 0 {
			wanted[cont] = true
		}
	}
	return wanted
}

// continentNeedsHelp reports whether cont is worth sending reinforcements
// to: pi doesn't fully own it yet, or one of its borders has pixieBorderForce
// armies or fewer -- Lux's Pixie.continentNeedsHelp/borderCountryNeedsHelp.
func continentNeedsHelp(g *risk.Game, pi int, cont risk.Continent) bool {
	if !ownsContinent(g, pi, cont) {
		return true
	}
	for _, border := range continentBorders(g, cont) {
		if g.Territories[border].Armies <= pixieBorderForce {
			return true
		}
	}
	return false
}

// weakestOwnedInSlice returns the pi-owned territory in ts (assumed
// already in canonical board order) with the fewest armies. ok is false
// if pi owns none of ts.
func weakestOwnedInSlice(g *risk.Game, pi int, ts []risk.Territory) (best risk.Territory, ok bool) {
	bestArmies := 0
	for _, t := range ts {
		if g.Territories[t].Owner != pi {
			continue
		}
		if a := g.Territories[t].Armies; !ok || a < bestArmies {
			best, bestArmies, ok = t, a, true
		}
	}
	return best, ok
}

// pixiePlacementTerritory picks where PixieStrategy places reinforcements
// (Lux's placeArmies, simplified to a single placement per call the same
// way clusterPlacementTerritory is -- see Lux_Port_Notes.md): the weakest
// owned border of the first wanted continent that continentNeedsHelp, or
// (if that continent is wanted but pi owns nothing in it yet)
// placeToTakeSpecificContinent routes toward it. If no wanted continent
// needs help, spread near enemies (Lux's placeNearEnemies); if nothing is
// wanted at all, placeToTakeContinent (Lux's own fallback when
// setupOurConts finds nothing worth pursuing).
func pixiePlacementTerritory(g *risk.Game, pi int, actions []risk.ReinforcementAction) risk.Territory {
	wanted := pixieWantedContinents(g, pi)
	for _, cont := range continentOrder(g) {
		if !wanted[cont] || !continentNeedsHelp(g, pi, cont) {
			continue
		}
		if t, ok := weakestOwnedInSlice(g, pi, continentBorders(g, cont)); ok {
			return t
		}
		return placeToTakeSpecificContinent(g, pi, actions, cont)
	}
	if len(wanted) == 0 {
		return placeToTakeContinent(g, pi, actions)
	}
	return bestByEnemyNeighborCount(g, pi, actions, func(a risk.ReinforcementAction) risk.Territory { return a.Territory })
}

// setupReinforce places the one initial army via pixiePlacementTerritory.
func (p *PixieStrategy) setupReinforce(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalSetupReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal setup reinforcement for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	// LegalSetupReinforcements and LegalReinforcements both enumerate
	// every owned territory -- the same legal set under different phase
	// types -- so converting lets setupReinforce reuse the same
	// placement logic as reinforce.
	reinforceActions := make([]risk.ReinforcementAction, len(actions))
	for i, a := range actions {
		reinforceActions[i] = risk.ReinforcementAction{Territory: a.Territory}
	}
	best := pixiePlacementTerritory(g, pi, reinforceActions)
	return Command{Action: ActionPlaceInitialArmy, Territory: string(best)}, nil
}

// reinforce only trades cards when a set is mandatory -- Pixie's Lux
// cardsPhase is SmartAgentBase's untouched empty default, same as
// AngryStrategy/ClusterStrategy. Placement dumps every pending
// reinforcement in one command at pixiePlacementTerritory.
func (p *PixieStrategy) reinforce(g *risk.Game, playerID string) (Command, error) {
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
	best := pixiePlacementTerritory(g, pi, actions)
	return Command{Action: ActionPlaceReinforcement, Territory: string(best), Armies: g.PendingReinforcements}, nil
}

// attack tries every wanted continent (deterministic order) for the first
// legal attack into it where an owned neighbor outnumbers the target at
// all (Lux's attackInContinent, outnumberBy=1), then attackForCard, then
// -- only if dominant or stalemated -- the same broadened scan
// ClusterStrategy uses (attackAsMuchAsPossible, shared since Lux's
// SmartAgentBase.attackHogWild/attackStalemate are identical across every
// subclass).
func (p *PixieStrategy) attack(g *risk.Game, playerID string) (Command, error) {
	pi := playerIndex(g, playerID)
	wanted := pixieWantedContinents(g, pi)
	for _, cont := range continentOrder(g) {
		if !wanted[cont] {
			continue
		}
		if cmd, ok := attackInContinent(g, pi, cont); ok {
			return cmd, nil
		}
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

// attackInContinent finds the first enemy-owned territory in cont with an
// owned neighbor that outnumbers it, attacking there -- Lux's
// Pixie.attackInContinent (outnumberBy=1: any edge at all qualifies).
func attackInContinent(g *risk.Game, pi int, cont risk.Continent) (Command, bool) {
	for _, t := range g.Board.Continents[cont].Territories {
		if g.Territories[t].Owner == pi {
			continue
		}
		for _, other := range g.Board.Order {
			if other == t || !g.Board.IsAdjacent(t, other) {
				continue
			}
			src := g.Territories[other]
			if src.Owner != pi {
				continue
			}
			if src.Armies > g.Territories[t].Armies {
				return Command{Action: ActionAttack, From: string(other), To: string(t), AttackerDice: min(3, src.Armies-1)}, true
			}
		}
	}
	return Command{}, false
}

// attackForCard finds the single best-ratio (armies:armies) attack
// anywhere on the board and attacks it, as long as it exceeds a 1:1
// ratio and no conquest has happened yet this turn -- Lux's
// SmartAgentBase.attackForCard(1), shared by Pixie and (later) other
// personas. Unlike the "first qualifying" stages elsewhere, Lux's own
// algorithm scans for the single best matchup, not the first one found,
// so this does too.
func attackForCard(g *risk.Game, pi int) (Command, bool) {
	if g.ConqueredThisTurn {
		return Command{}, false
	}
	var bestFrom, bestTo risk.Territory
	bestRatio := 1.0
	found := false
	for _, from := range g.Board.Order {
		src := g.Territories[from]
		if src.Owner != pi {
			continue
		}
		for _, to := range g.Board.Order {
			if to == from || !g.Board.IsAdjacent(from, to) {
				continue
			}
			dst := g.Territories[to]
			if dst.Owner == pi {
				continue
			}
			if ratio := float64(src.Armies) / float64(dst.Armies); ratio > bestRatio {
				bestRatio, bestFrom, bestTo, found = ratio, from, to, true
			}
		}
	}
	if !found {
		return Command{}, false
	}
	src := g.Territories[bestFrom]
	return Command{Action: ActionAttack, From: string(bestFrom), To: string(bestTo), AttackerDice: min(3, src.Armies-1)}, true
}

// adjacentEnemiesInWantedContinents counts t's neighbors that are both
// enemy-owned and in a continent pi wants (per wanted) -- used by occupy's
// last-resort tie-break, mirroring Lux's own inline neighbor scan.
func adjacentEnemiesInWantedContinents(g *risk.Game, pi int, t risk.Territory, wanted map[risk.Continent]bool) int {
	tc := territoryContinent(g)
	count := 0
	for _, other := range g.Board.Order {
		if other == t || !g.Board.IsAdjacent(t, other) {
			continue
		}
		if g.Territories[other].Owner != pi && wanted[tc[other]] {
			count++
		}
	}
	return count
}

// occupy ports Pixie.moveArmiesIn faithfully: a direct enemyNeighborCount
// comparison first (moving the legal minimum/maximum toward whichever
// side is more contested, or the legal midpoint on a tie where both sides
// have enemies), falling back to continent-membership in
// pixieWantedContinents when neither side borders an enemy at all, and
// falling back further to counting each side's neighbors that are both
// enemy-owned and in a wanted continent. The legal [MinMove, MaxMove]
// midpoint substitutes for Lux's raw armies/2 tie-break, since this
// engine expresses the occupation amount as a legal range rather than a
// share of the attacking territory's own army count.
func (p *PixieStrategy) occupy(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalOccupations(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal occupation for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	minArmies := actions[0].Armies
	maxArmies := actions[len(actions)-1].Armies
	halfArmies := (minArmies + maxArmies) / 2

	from, to := g.Occupy.From, g.Occupy.To
	aEnemies := enemyNeighborCount(g, from, pi)
	dEnemies := enemyNeighborCount(g, to, pi)

	armies := halfArmies
	switch {
	case aEnemies > dEnemies:
		armies = minArmies
	case dEnemies > aEnemies:
		armies = maxArmies
	case aEnemies > 0:
		armies = halfArmies
	default:
		wanted := pixieWantedContinents(g, pi)
		tc := territoryContinent(g)
		aCont, dCont := tc[from], tc[to]
		switch {
		case wanted[aCont] && wanted[dCont]:
			armies = halfArmies
		case wanted[aCont]:
			armies = minArmies
		case wanted[dCont]:
			armies = maxArmies
		default:
			aWanted := adjacentEnemiesInWantedContinents(g, pi, from, wanted)
			dWanted := adjacentEnemiesInWantedContinents(g, pi, to, wanted)
			switch {
			case aWanted > dWanted:
				armies = minArmies
			case dWanted > aWanted:
				armies = maxArmies
			}
		}
	}
	return Command{Action: ActionOccupy, Armies: armies}, nil
}

// fortify uses bestFortifyDestination (shared with AngryStrategy/
// ClusterStrategy) -- see Lux_Port_Notes.md on why this engine's
// one-fortify-per-turn limit erases the difference between Lux's several
// multi-hop fortify algorithms.
func (p *PixieStrategy) fortify(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalFortifications(g, playerID)
	pi := playerIndex(g, playerID)
	best, bestScore, ok := bestFortifyDestination(g, pi, actions)
	if !ok || bestScore == 0 {
		return Command{Action: ActionEndTurn}, nil
	}
	return Command{Action: ActionFortify, From: string(best.From), To: string(best.To), Armies: best.MaxArmies}, nil
}
