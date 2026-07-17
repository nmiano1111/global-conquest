package bot

import (
	"fmt"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// occupy scores every legal army count to move into the just-conquered
// territory (risk.LegalOccupations enumerates MinMove..MaxMove), balancing
// how well each count covers the source's remaining exposure against how
// much it commits to the newly conquered destination — rather than
// basic-v1's flat "always move the minimum."
func (s *ScoredStrategy) occupy(g *risk.Game, playerID string) (Command, Explanation, error) {
	actions := risk.LegalOccupations(g, playerID)
	if len(actions) == 0 {
		return Command{}, Explanation{}, fmt.Errorf("bot: no legal occupation for player %s", playerID)
	}
	pi := playerIndex(g, playerID)

	sourceArmies := g.Territories[g.Occupy.From].Armies
	sourceThreat := adjacentEnemyArmies(g, g.Occupy.From, pi)
	destThreat := adjacentEnemyArmies(g, g.Occupy.To, pi)

	options := make([]scoredOption, 0, len(actions))
	for _, a := range actions {
		options = append(options, scoredOption{
			Command:  Command{Action: ActionOccupy, Armies: a.Armies},
			Features: s.occupyFeatures(a.Armies, sourceArmies, sourceThreat, destThreat),
		})
	}
	cmd, expl := s.selectBest(options, 3)
	return cmd, expl, nil
}

// occupyFeatures scores one candidate armies-to-move count. Both coverage
// terms are capped at what's actually needed (min(..., threat)) so
// covering the threat twice over earns no extra credit; the small
// unconditional surplus term only exists to break ties in favor of
// pushing more forward once both coverage needs are already met.
func (s *ScoredStrategy) occupyFeatures(armies, sourceArmies, sourceThreat, destThreat int) []Feature {
	w := s.weights
	remaining := sourceArmies - armies
	defenseCoverage := min(remaining, sourceThreat)
	momentum := min(armies, destThreat)

	return []Feature{
		{Name: "defense_coverage", Value: w.OccupyDefenseCoverage * float64(defenseCoverage)},
		{Name: "momentum", Value: w.OccupyMomentum * float64(momentum)},
		{Name: "momentum_surplus", Value: w.OccupyMomentumSurplus * float64(armies)},
	}
}
