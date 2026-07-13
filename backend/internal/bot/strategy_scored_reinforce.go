package bot

import (
	"fmt"

	"backend/internal/risk"
)

// reinforce decides card timing first (see strategy_scored_cards.go), then
// scores every legal reinforcement territory and places a capped batch —
// max(1, pending/3) — at the top scorer rather than dumping the whole pool
// at once. Since every call re-scores from scratch against the freshly
// updated armies/threat, a territory that's still the worst-off keeps
// winning and keeps getting batches (effectively concentrating there),
// while a comparably-threatened second border gets its share once the
// first stops being the clear top pick — see ReinforceConcentrationPenalty
// and ReinforceWeakness.
func (s *ScoredStrategy) reinforce(g *risk.Game, playerID string) (Command, Explanation, error) {
	if cmd, expl, ok := s.scoredCardTurnIn(g, playerID); ok {
		return cmd, expl, nil
	}

	actions := risk.LegalReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, Explanation{}, fmt.Errorf("bot: no legal reinforcement for player %s", playerID)
	}
	pi := playerIndex(g, playerID)

	options := make([]scoredOption, 0, len(actions))
	for _, a := range actions {
		options = append(options, scoredOption{
			Command:  Command{Action: ActionPlaceReinforcement, Territory: string(a.Territory)},
			Features: s.reinforceFeatures(g, pi, a.Territory),
		})
	}
	cmd, expl := selectBest(options, 3)
	cmd.Armies = min(g.PendingReinforcements, max(1, g.PendingReinforcements/3))
	return cmd, expl, nil
}

// setupReinforce uses the same feature set as reinforce, but places
// exactly one army per call (risk.PlaceInitialArmy's only legal amount),
// so there's no batching decision to make.
func (s *ScoredStrategy) setupReinforce(g *risk.Game, playerID string) (Command, Explanation, error) {
	actions := risk.LegalSetupReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, Explanation{}, fmt.Errorf("bot: no legal setup reinforcement for player %s", playerID)
	}
	pi := playerIndex(g, playerID)

	options := make([]scoredOption, 0, len(actions))
	for _, a := range actions {
		options = append(options, scoredOption{
			Command:  Command{Action: ActionPlaceInitialArmy, Territory: string(a.Territory)},
			Features: s.reinforceFeatures(g, pi, a.Territory),
		})
	}
	cmd, expl := selectBest(options, 3)
	return cmd, expl, nil
}

// reinforceFeatures scores one candidate reinforcement territory, shared
// by both reinforce and setup_reinforce.
func (s *ScoredStrategy) reinforceFeatures(g *risk.Game, pi int, t risk.Territory) []Feature {
	w := s.weights
	threat := adjacentEnemyArmies(g, t, pi)
	ownArmies := g.Territories[t].Armies

	return []Feature{
		{Name: "enemy_threat", Value: w.ReinforceEnemyThreat * float64(threat)},
		{Name: "enemy_territory_count", Value: w.ReinforceEnemyTerritoryCount * float64(adjacentEnemyTerritoryCount(g, t, pi))},
		{Name: "weakness", Value: w.ReinforceWeakness * float64(threat-ownArmies)},
		{Name: "continent_value", Value: w.ReinforceContinentValue * continentReinforceValue(g, pi, t)},
		{Name: "concentration_penalty", Value: w.ReinforceConcentrationPenalty * float64(ownArmies)},
	}
}

// adjacentEnemyTerritoryCount counts the distinct territories adjacent to t
// owned by someone other than pi — a separate signal from
// adjacentEnemyArmies's summed total: a border facing 3 weak enemies is a
// bigger multi-front risk than 1 strong one with the same total armies.
func adjacentEnemyTerritoryCount(g *risk.Game, t risk.Territory, pi int) int {
	count := 0
	for other := range g.Board.Adjacent[t] {
		ts := g.Territories[other]
		if ts.Owner != pi && ts.Owner >= 0 {
			count++
		}
	}
	return count
}

// continentReinforceValue scores how valuable it is to add armies to t for
// continent purposes: the full continent bonus if t borders outside a
// continent pi already fully owns (defense), or a completion-scaled value
// if t is a frontier of a continent pi is close to finishing (push).
func continentReinforceValue(g *risk.Game, pi int, t risk.Territory) float64 {
	var value float64
	for _, c := range continentsContaining(g, t) {
		owned, total := continentOwnershipCounts(g, pi, c)
		if owned == total {
			if isContinentBorder(g, t, c) {
				value += float64(g.Board.Continents[c].Bonus)
			}
			continue
		}
		if isContinentFrontier(g, pi, t, c) {
			missing := total - owned
			value += float64(g.Board.Continents[c].Bonus) / float64(missing+1)
		}
	}
	return value
}

// isContinentBorder reports whether t has a neighbor outside continent c —
// i.e. whether t is one of the territories an enemy could actually invade
// c through, as opposed to an interior territory with no external exposure.
func isContinentBorder(g *risk.Game, t risk.Territory, c risk.Continent) bool {
	inContinent := make(map[risk.Territory]struct{}, len(g.Board.Continents[c].Territories))
	for _, ct := range g.Board.Continents[c].Territories {
		inContinent[ct] = struct{}{}
	}
	for other := range g.Board.Adjacent[t] {
		if _, in := inContinent[other]; !in {
			return true
		}
	}
	return false
}

// isContinentFrontier reports whether t is adjacent to a territory of
// continent c that pi does not yet own — i.e. whether reinforcing t would
// help pi push toward completing c.
func isContinentFrontier(g *risk.Game, pi int, t risk.Territory, c risk.Continent) bool {
	for _, ct := range g.Board.Continents[c].Territories {
		if g.Territories[ct].Owner == pi {
			continue
		}
		if g.Board.IsAdjacent(t, ct) {
			return true
		}
	}
	return false
}
