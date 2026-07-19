package bot

import "github.com/nmiano1111/global-conquest/backend/internal/risk"

// fortify scores every legal (from, to) fortification move plus a
// synthetic "end turn without fortifying" candidate — ending the phase is
// never special-cased, exactly like attack's end_attack sentinel. When
// HasFortified is already true or no legal move exists,
// risk.LegalFortifications returns nothing and only the sentinel remains,
// which is what already-fortified/no-op turns fall through to.
func (s *ScoredStrategy) fortify(g *risk.Game, playerID string) (Command, Explanation, error) {
	actions := risk.LegalFortifications(g, playerID)
	pi := playerIndex(g, playerID)

	options := make([]scoredOption, 0, len(actions)+1)
	for _, a := range actions {
		options = append(options, scoredOption{
			Command: Command{
				Action: ActionFortify,
				From:   string(a.From),
				To:     string(a.To),
				Armies: a.MaxArmies,
			},
			Features: s.fortifyFeatures(g, pi, a),
		})
	}
	options = append(options, scoredOption{
		Command:  Command{Action: ActionEndTurn},
		Features: []Feature{{Name: "end_turn_bias", Value: s.weights.FortifyEndTurnBias}},
	})

	cmd, expl := selectBest(options, 3)
	return cmd, expl, nil
}

// fortifySignals holds fortifyFeatures' raw (unweighted) per-candidate
// signal, computed once by computeFortifySignals and turned into a score
// by ScoredStrategy.fortifyFeatures (weighted linear sum).
type fortifySignals struct {
	DestinationThreat  float64
	ContinentValue     float64
	SourceExposureCost float64
}

// computeFortifySignals computes one candidate fortification move's raw
// signal, independent of any Weights value (reusing continentReinforceValue,
// the same helper reinforce uses).
func computeFortifySignals(g *risk.Game, pi int, a risk.FortificationAction) fortifySignals {
	return fortifySignals{
		DestinationThreat:  float64(adjacentEnemyArmies(g, a.To, pi)),
		ContinentValue:     continentReinforceValue(g, pi, a.To),
		SourceExposureCost: float64(adjacentEnemyArmies(g, a.From, pi)),
	}
}

// fortifyFeatures scores one candidate fortification move: reward
// reinforcing a threatened or continent-valuable destination, penalize
// weakening a source that's itself under threat.
func (s *ScoredStrategy) fortifyFeatures(g *risk.Game, pi int, a risk.FortificationAction) []Feature {
	w := s.weights
	sig := computeFortifySignals(g, pi, a)

	return []Feature{
		{Name: "destination_threat", Value: w.FortifyDestinationThreat * sig.DestinationThreat},
		{Name: "continent_value", Value: w.FortifyContinentValue * sig.ContinentValue},
		{Name: "source_exposure_cost", Value: w.FortifySourceExposureCost * sig.SourceExposureCost},
	}
}
