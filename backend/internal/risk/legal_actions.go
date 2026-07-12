package risk

// This file provides read-only legal-action queries for callers (bots, in
// this milestone) that need to enumerate what a player could legally do
// without duplicating the engine's own legality rules. Every helper here is
// deterministic for a fixed *Game and never mutates state; the engine still
// re-validates any command derived from these results when it is applied.

// SetupReinforcementAction is a legal single-army placement during
// setup_reinforce: the engine's PlaceInitialArmy always places exactly one
// army per call.
type SetupReinforcementAction struct {
	Territory Territory
}

// LegalSetupReinforcements returns the player's owned territories, in
// canonical board order, while the player still has setup reserves left.
func LegalSetupReinforcements(g *Game, playerID string) []SetupReinforcementAction {
	if g.Phase != PhaseSetupReinforce {
		return nil
	}
	pi := indexOfPlayer(g, playerID)
	if pi < 0 || g.SetupReserves[pi] <= 0 {
		return nil
	}
	var out []SetupReinforcementAction
	for _, t := range g.Board.Order {
		if g.Territories[t].Owner == pi {
			out = append(out, SetupReinforcementAction{Territory: t})
		}
	}
	return out
}

// CardTurnInAction is a candidate set of three cards forming a valid trade,
// identified by their index into the player's current hand.
type CardTurnInAction struct {
	Indices [3]int
	Cards   [3]Card
}

// LegalCardTurnIns enumerates every combination of three cards in the
// player's hand that forms a valid set, in ascending index order.
func LegalCardTurnIns(g *Game, playerID string) []CardTurnInAction {
	pi := indexOfPlayer(g, playerID)
	if pi < 0 {
		return nil
	}
	cards := g.Players[pi].Cards
	var out []CardTurnInAction
	for i := 0; i < len(cards); i++ {
		for j := i + 1; j < len(cards); j++ {
			for k := j + 1; k < len(cards); k++ {
				set := [3]Card{cards[i], cards[j], cards[k]}
				if isValidSet(set[:]) {
					out = append(out, CardTurnInAction{Indices: [3]int{i, j, k}, Cards: set})
				}
			}
		}
	}
	return out
}

// CardTurnInRequired reports whether the player must trade in a set before
// any further reinforcement can be placed, mirroring the same five-card
// threshold PlaceReinforcement enforces.
func CardTurnInRequired(g *Game, playerID string) bool {
	pi := indexOfPlayer(g, playerID)
	if pi < 0 {
		return false
	}
	return len(g.Players[pi].Cards) >= 5
}

// ReinforcementAction is a legal destination for placing reinforcement
// armies during the reinforce phase.
type ReinforcementAction struct {
	Territory Territory
}

// LegalReinforcements returns the player's owned territories, in canonical
// board order, while reinforcements are pending and no mandatory card trade
// is blocking placement.
func LegalReinforcements(g *Game, playerID string) []ReinforcementAction {
	if g.Phase != PhaseReinforce || g.PendingReinforcements <= 0 {
		return nil
	}
	pi := indexOfPlayer(g, playerID)
	if pi < 0 || len(g.Players[pi].Cards) >= 5 {
		return nil
	}
	var out []ReinforcementAction
	for _, t := range g.Board.Order {
		if g.Territories[t].Owner == pi {
			out = append(out, ReinforcementAction{Territory: t})
		}
	}
	return out
}

// AttackAction is a legal attack: an owned source territory with more than
// one army against an adjacent enemy-owned territory.
type AttackAction struct {
	From, To        Territory
	SourceArmies    int
	TargetArmies    int
	MaxAttackerDice int
}

// LegalAttacks enumerates every legal (from, to) attack pair, in canonical
// board order for both source and target.
func LegalAttacks(g *Game, playerID string) []AttackAction {
	if g.Phase != PhaseAttack {
		return nil
	}
	pi := indexOfPlayer(g, playerID)
	if pi < 0 {
		return nil
	}
	var out []AttackAction
	for _, from := range g.Board.Order {
		src := g.Territories[from]
		if src.Owner != pi || src.Armies <= 1 {
			continue
		}
		maxDice := min(3, src.Armies-1)
		for _, to := range g.Board.Order {
			if !g.Board.IsAdjacent(from, to) {
				continue
			}
			dst := g.Territories[to]
			if dst.Owner == pi || dst.Owner < 0 {
				continue
			}
			out = append(out, AttackAction{
				From:            from,
				To:              to,
				SourceArmies:    src.Armies,
				TargetArmies:    dst.Armies,
				MaxAttackerDice: maxDice,
			})
		}
	}
	return out
}

// OccupationAction is a legal army count to move into a just-conquered
// territory, bounded by the engine's OccupyState.
type OccupationAction struct {
	Armies int
}

// LegalOccupations returns every legal army count from OccupyState.MinMove
// through MaxMove, ascending.
func LegalOccupations(g *Game, playerID string) []OccupationAction {
	if g.Phase != PhaseOccupy || g.Occupy == nil {
		return nil
	}
	pi := indexOfPlayer(g, playerID)
	if pi < 0 {
		return nil
	}
	from := g.Territories[g.Occupy.From]
	to := g.Territories[g.Occupy.To]
	if from.Owner != pi || to.Owner != pi {
		return nil
	}
	var out []OccupationAction
	for a := g.Occupy.MinMove; a <= g.Occupy.MaxMove; a++ {
		out = append(out, OccupationAction{Armies: a})
	}
	return out
}

// FortificationAction is a legal single fortification move: both
// territories owned by the player and connected through owned territory.
type FortificationAction struct {
	From, To  Territory
	MaxArmies int
}

// LegalFortifications enumerates every legal (from, to) fortification pair,
// in canonical board order for both source and destination. It returns
// nothing once the player has already fortified this turn.
func LegalFortifications(g *Game, playerID string) []FortificationAction {
	if g.Phase != PhaseFortify || g.HasFortified {
		return nil
	}
	pi := indexOfPlayer(g, playerID)
	if pi < 0 {
		return nil
	}
	var out []FortificationAction
	for _, from := range g.Board.Order {
		src := g.Territories[from]
		if src.Owner != pi || src.Armies <= 1 {
			continue
		}
		for _, to := range g.Board.Order {
			if to == from {
				continue
			}
			dst := g.Territories[to]
			if dst.Owner != pi {
				continue
			}
			if !g.isContiguous(from, to, pi) {
				continue
			}
			out = append(out, FortificationAction{From: from, To: to, MaxArmies: src.Armies - 1})
		}
	}
	return out
}

func indexOfPlayer(g *Game, playerID string) int {
	for i, p := range g.Players {
		if p.ID == playerID {
			return i
		}
	}
	return -1
}
