package bot

import "github.com/nmiano1111/global-conquest/backend/internal/risk"

// marginalAttackLow and marginalAttackHigh bound the "worth cashing cards
// in for" zone of a would-be attack's win probability: below the low bound
// the attack is hopeless regardless of a few extra armies, above the high
// bound it's already comfortable without them, so only a probability in
// between is a situation where incoming reinforcement armies could
// plausibly tip the balance.
const (
	marginalAttackLow  = 0.25
	marginalAttackHigh = 0.65
)

// scoredCardTurnIn decides both whether to trade in cards this call and,
// if so, which legal set — replacing the placeholder "always trade
// whenever legal" policy every strategy used until now with the doc's
// mandatory/optional/delay distinction: trade immediately when required or
// clearly worthwhile (approaching the forced-trade limit, a border under
// real pressure, or a marginal attack — toward completing a continent or
// eliminating a weak opponent — that the incoming armies could tip into
// favorable); otherwise delay and let reinforce's normal per-territory
// scoring run on whatever base reinforcements the player already has.
//
// A free function, not a ScoredStrategy method -- card-timing policy
// doesn't depend on any Weights value, so both ScoredStrategy and
// ValueStrategy (see strategy_value.go) share this exact decision
// unchanged.
func scoredCardTurnIn(g *risk.Game, playerID string) (Command, Explanation, bool) {
	sets := risk.LegalCardTurnIns(g, playerID)
	if len(sets) == 0 {
		return Command{}, Explanation{}, false
	}
	pi := playerIndex(g, playerID)

	var reason string
	switch {
	case risk.CardTurnInRequired(g, playerID):
		reason = "mandatory"
	case len(g.Players[pi].Cards) >= 4:
		reason = "approaching_card_limit"
	case underPressure(g, pi):
		reason = "under_pressure"
	case hasMarginalContinentCompletionAttack(g, pi):
		reason = "enables_continent"
	case hasMarginalEliminationAttack(g, pi):
		reason = "enables_elimination"
	default:
		return Command{}, Explanation{}, false
	}

	best := bestCardSet(g, pi, sets)
	return Command{Action: ActionTradeCards, CardIndices: best.Indices},
		Explanation{Score: 1, Features: []Feature{{Name: reason, Value: 1}}},
		true
}

// underPressure reports whether any owned territory faces more enemy
// armies than it currently holds — a real defensive emergency worth
// cashing cards in for immediately rather than waiting.
func underPressure(g *risk.Game, pi int) bool {
	for _, t := range g.Board.Order {
		ts := g.Territories[t]
		if ts.Owner != pi {
			continue
		}
		if adjacentEnemyArmies(g, t, pi) > ts.Armies {
			return true
		}
	}
	return false
}

// hasMarginalContinentCompletionAttack reports whether pi is one territory
// away from completing some continent, and the best attack against that
// missing territory is currently marginal.
func hasMarginalContinentCompletionAttack(g *risk.Game, pi int) bool {
	for continent, info := range g.Board.Continents {
		owned, total := continentOwnershipCounts(g, pi, continent)
		if owned != total-1 {
			continue
		}
		for _, t := range info.Territories {
			if g.Territories[t].Owner != pi {
				if bestAttackIsMarginal(g, pi, t) {
					return true
				}
				break
			}
		}
	}
	return false
}

// hasMarginalEliminationAttack reports whether some non-eliminated
// opponent is down to their last territory and the best attack against it
// is currently marginal.
func hasMarginalEliminationAttack(g *risk.Game, pi int) bool {
	for _, p := range g.Players {
		if p.Eliminated {
			continue
		}
		opi := playerIndex(g, p.ID)
		if opi == pi {
			continue
		}
		count := 0
		var last risk.Territory
		for _, t := range g.Board.Order {
			if g.Territories[t].Owner == opi {
				count++
				last = t
				if count > 1 {
					break
				}
			}
		}
		if count == 1 && bestAttackIsMarginal(g, pi, last) {
			return true
		}
	}
	return false
}

// bestAttackIsMarginal reports whether pi's strongest owned territory
// bordering target has a currently marginal (neither hopeless nor already
// comfortable) win probability against it — the situation where incoming
// reinforcement armies could plausibly tip the balance.
func bestAttackIsMarginal(g *risk.Game, pi int, target risk.Territory) bool {
	targetArmies := g.Territories[target].Armies
	best := -1.0
	for other := range g.Board.Adjacent[target] {
		ts := g.Territories[other]
		if ts.Owner != pi || ts.Armies <= 1 {
			continue
		}
		if p := ForecastAttack(ts.Armies, targetArmies).WinProbability; p > best {
			best = p
		}
	}
	if best < 0 {
		return false // no legal attacker borders this target at all
	}
	return best >= marginalAttackLow && best <= marginalAttackHigh
}

// bestCardSet picks the legal set to trade in: prefers one containing a
// card matching a currently-owned territory (the once-per-turn +2
// territory bonus, risk.Game.TerritoryBonusUsed), tie-broken by ascending
// card index for determinism.
func bestCardSet(g *risk.Game, pi int, sets []risk.CardTurnInAction) risk.CardTurnInAction {
	if !g.TerritoryBonusUsed {
		var matching []risk.CardTurnInAction
		for _, s := range sets {
			if setMatchesOwnedTerritory(g, pi, s) {
				matching = append(matching, s)
			}
		}
		if len(matching) > 0 {
			return lowestIndexSet(matching)
		}
	}
	return lowestIndexSet(sets)
}

func setMatchesOwnedTerritory(g *risk.Game, pi int, s risk.CardTurnInAction) bool {
	for _, c := range s.Cards {
		if c.Symbol == risk.Wild {
			continue
		}
		if g.Territories[c.Territory].Owner == pi {
			return true
		}
	}
	return false
}

// lowestIndexSet breaks ties among equally-preferred sets deterministically
// by ascending card index — the same tie-break every strategy has used
// since Phase 1.
func lowestIndexSet(sets []risk.CardTurnInAction) risk.CardTurnInAction {
	best := sets[0]
	for _, s := range sets[1:] {
		if s.Indices[0] < best.Indices[0] ||
			(s.Indices[0] == best.Indices[0] && s.Indices[1] < best.Indices[1]) ||
			(s.Indices[0] == best.Indices[0] && s.Indices[1] == best.Indices[1] && s.Indices[2] < best.Indices[2]) {
			best = s
		}
	}
	return best
}
