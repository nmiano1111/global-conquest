package bot

import (
	"context"
	"testing"

	"backend/internal/risk"
)

// TestFortifyFeaturesRewardsDestinationThreatAndContinentValue checks the
// named feature values directly, hand-derived: with only Madagascar and
// East Africa owned (South Africa left at its default enemy owner), East
// Africa's other neighbors (Egypt, North Africa, Congo, South Africa,
// Middle East) are all enemy -> destination_threat = 1.0*5 = 5.0. Africa
// (bonus 3) has Madagascar+East Africa owned = 2 of 6, missing 4; East
// Africa is a frontier (several unowned Africa neighbors) -> value = 3/5
// = 0.6 -> continent_value = 2.0*0.6 = 1.2. Madagascar's only other
// neighbor (South Africa) is enemy -> source_exposure_cost = -1.0*1 = -1.0.
func TestFortifyFeaturesRewardsDestinationThreatAndContinentValue(t *testing.T) {
	g, _ := newTestGame(t)
	g.Territories["Madagascar"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["East Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}

	strat := NewScoredStrategy(DefaultWeights)
	features := strat.fortifyFeatures(g, 0, risk.FortificationAction{From: "Madagascar", To: "East Africa", MaxArmies: 4})

	want := map[string]float64{
		"destination_threat":   5.0,
		"continent_value":      1.2,
		"source_exposure_cost": -1.0,
	}
	if len(features) != len(want) {
		t.Fatalf("expected %d features, got %d: %+v", len(want), len(features), features)
	}
	for _, f := range features {
		wantV, ok := want[f.Name]
		if !ok {
			t.Fatalf("unexpected feature %q", f.Name)
		}
		if !almostEqual(f.Value, wantV) {
			t.Errorf("feature %s = %v, want %v", f.Name, f.Value, wantV)
		}
	}
}

// TestScoredStrategyFortifyMovesTowardBorder reuses the same fixture as
// basic-v1's equivalent test. Hand-derived total scores per candidate
// destination (destination_threat + continent_value, Africa bonus 3,
// Madagascar+South Africa+East Africa owned = 3/6):
//   - Madagascar (interior, no unowned Africa neighbor): 0
//   - South Africa (borders unowned Congo): 1.0*1 + 2.0*(3/4) = 2.5
//   - East Africa (borders unowned Egypt/North Africa/Congo): 1.0*4 + 2.0*(3/4) = 5.5
// East Africa is the clear best destination; between the two legal
// sources reaching it, Madagascar (source_exposure_cost 0, since its only
// other neighbor is owned) beats South Africa (cost -1.0, Congo).
func TestScoredStrategyFortifyMovesTowardBorder(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	g.Territories["Madagascar"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["South Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["East Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}

	strat := NewScoredStrategy(DefaultWeights)
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionFortify {
		t.Fatalf("expected fortify, got %s", cmd.Action)
	}
	if cmd.From != "Madagascar" || cmd.To != "East Africa" {
		t.Fatalf("expected Madagascar -> East Africa, got %s -> %s", cmd.From, cmd.To)
	}
	if cmd.Armies != 4 {
		t.Fatalf("expected max legal fortify amount (armies-1=4), got %d", cmd.Armies)
	}
}

func TestScoredStrategyFortifyAlreadyFortifiedEndsTurn(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	g.HasFortified = true
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 0, Armies: 1}

	strat := NewScoredStrategy(DefaultWeights)
	cmd, expl, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionEndTurn {
		t.Fatalf("expected end_turn after already fortifying, got %s", cmd.Action)
	}
	if expl.Score != DefaultWeights.FortifyEndTurnBias {
		t.Fatalf("expected explanation score to equal FortifyEndTurnBias, got %v", expl.Score)
	}
}
