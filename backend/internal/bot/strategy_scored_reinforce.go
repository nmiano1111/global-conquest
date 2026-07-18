package bot

import (
	"fmt"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
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
	if cmd, expl, ok := scoredCardTurnIn(g, playerID); ok {
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
	cmd, expl := s.selectBest(options, 3)
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
	cmd, expl := s.selectBest(options, 3)
	return cmd, expl, nil
}

// reinforceSignals holds reinforceFeatures' raw (unweighted) per-candidate
// signal, computed once and shared by two consumers that turn it into a
// score differently: ScoredStrategy.reinforceFeatures (weighted linear
// sum) and GBTStrategy (fed straight into a tree ensemble, no weighting
// at all -- see strategy_gbt.go). Every field is always populated (unlike
// attackSignals, reinforce has no conditional features).
//
// enemy_threat and weakness (threat - ownArmies) are highly correlated
// (0.98, measured across every legal candidate, not just chosen ones --
// see Next_Phase_Bot_ML_Roadmap.md) and an ML fitting pass could never
// assign weakness a non-trivial coefficient as a result. Removing
// enemy_threat entirely to resolve that was tried and reverted: real
// tournament play got dramatically worse (45% of games failing to hit a
// conclusion within the turn limit, vs ~5% before) even though the ML fit
// still couldn't use weakness afterward either -- concrete evidence that
// this feature earns its keep in actual gameplay dynamics the "regress
// final game outcome on one decision's features" ML objective doesn't
// reward, not that it's genuinely redundant. Keep both terms.
type reinforceSignals struct {
	EnemyThreat          float64
	EnemyTerritoryCount  float64
	Weakness             float64
	ContinentValue       float64
	ConcentrationPenalty float64
}

// computeReinforceSignals computes one candidate reinforcement
// territory's raw signal, independent of any Weights value. Shared by
// both reinforce and setup_reinforce.
func computeReinforceSignals(g *risk.Game, pi int, t risk.Territory) reinforceSignals {
	threat := adjacentEnemyArmies(g, t, pi)
	ownArmies := g.Territories[t].Armies
	return reinforceSignals{
		EnemyThreat:          float64(threat),
		EnemyTerritoryCount:  float64(adjacentEnemyTerritoryCount(g, t, pi)),
		Weakness:             float64(threat - ownArmies),
		ContinentValue:       continentReinforceValue(g, pi, t),
		ConcentrationPenalty: float64(ownArmies),
	}
}

// reinforceFeatures scores one candidate reinforcement territory, shared
// by both reinforce and setup_reinforce.
func (s *ScoredStrategy) reinforceFeatures(g *risk.Game, pi int, t risk.Territory) []Feature {
	w := s.weights
	sig := computeReinforceSignals(g, pi, t)

	return []Feature{
		{Name: "enemy_threat", Value: w.ReinforceEnemyThreat * sig.EnemyThreat},
		{Name: "enemy_territory_count", Value: w.ReinforceEnemyTerritoryCount * sig.EnemyTerritoryCount},
		{Name: "weakness", Value: w.ReinforceWeakness * sig.Weakness},
		{Name: "continent_value", Value: w.ReinforceContinentValue * sig.ContinentValue},
		{Name: "concentration_penalty", Value: w.ReinforceConcentrationPenalty * sig.ConcentrationPenalty},
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
