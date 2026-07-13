package bot

import (
	"context"
	"testing"

	"backend/internal/risk"
)

func TestScoredStrategyNonAttackPhaseFallsBackToBasic(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	g.Territories["Madagascar"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["South Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["East Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}

	scored := NewScoredStrategy(DefaultWeights)
	basic := NewBasicStrategy()

	scoredCmd, _, err := scored.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("scored NextCommand: %v", err)
	}
	basicCmd, _, err := basic.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("basic NextCommand: %v", err)
	}
	if scoredCmd != basicCmd {
		t.Fatalf("expected non-attack phases to fall back to basic-v1 identically, got scored=%+v basic=%+v", scoredCmd, basicCmd)
	}
}

func TestScoredStrategyAttackNoLegalAttacksEndsAttack(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	// Player 0 owns nothing bordering an enemy with enough armies to
	// attack from (newTestGame's default owner=1 everywhere means p0 owns
	// no territory at all here), so risk.LegalAttacks is empty and only
	// the end_attack sentinel exists.
	scored := NewScoredStrategy(DefaultWeights)
	cmd, expl, err := scored.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionEndAttack {
		t.Fatalf("expected end_attack with no legal attacks, got %s", cmd.Action)
	}
	if expl.Score != DefaultWeights.EndPhaseBias {
		t.Fatalf("expected explanation score to equal EndPhaseBias, got %v", expl.Score)
	}
}

func TestScoredStrategyAttackPrefersHigherCaptureProbability(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	// Alaska's neighbors are Northwest Territory, Alberta, Kamchatka; own
	// the first two so only Kamchatka remains attackable. Give Alaska a
	// commanding army advantage so scoring clearly favors attacking over
	// ending the phase.
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 10}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1}

	strat := NewScoredStrategy(DefaultWeights)
	cmd, expl, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack || cmd.From != "Alaska" || cmd.To != "Kamchatka" {
		t.Fatalf("expected Alaska -> Kamchatka, got %+v", cmd)
	}
	foundCapture := false
	for _, f := range expl.Features {
		if f.Name == "capture_probability" {
			foundCapture = true
			if f.Value <= 0 {
				t.Errorf("expected a positive capture_probability contribution, got %v", f.Value)
			}
		}
	}
	if !foundCapture {
		t.Fatalf("expected capture_probability to be part of the winning explanation, got %+v", expl.Features)
	}
}

// TestScoredStrategyAttackAllowsLowAdvantageWhenCompletingContinent gives
// player 0 three of Australia's four territories (missing only Eastern
// Australia) with only a 1-army advantage over the target -- basic-v1's
// flat 2-army rule would refuse this outright. This asserts the attack is
// chosen and that completes_continent is part of why, without claiming
// it's the sole deciding feature (capture_probability alone already clears
// end_attack's zero baseline here).
func TestScoredStrategyAttackAllowsLowAdvantageWhenCompletingContinent(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	g.Territories["New Guinea"] = risk.TerritoryState{Owner: 0, Armies: 1} // can't attack (armies <= 1)
	g.Territories["Indonesia"] = risk.TerritoryState{Owner: 0, Armies: 1}  // can't attack (armies <= 1)
	g.Territories["Western Australia"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["Eastern Australia"] = risk.TerritoryState{Owner: 1, Armies: 1}

	strat := NewScoredStrategy(DefaultWeights)
	cmd, expl, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack || cmd.From != "Western Australia" || cmd.To != "Eastern Australia" {
		t.Fatalf("expected Western Australia -> Eastern Australia, got %+v", cmd)
	}
	found := false
	for _, f := range expl.Features {
		if f.Name == "completes_continent" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected completes_continent in the winning explanation, got %+v", expl.Features)
	}
}

// TestScoredStrategyAttackPrefersEliminatingOverIdenticalNonEliminatingAttack
// builds two attacks with identical SourceArmies/TargetArmies (so identical
// army_advantage, capture_probability, expected_loss_cost, card_opportunity)
// and identical exposure (each source's only enemy neighbor is the target
// itself, at the same army count) -- the only difference is that Kamchatka
// is player 2's sole territory (an elimination) while Great Britain belongs
// to player 1, who owns most of the rest of the board (not an elimination).
// Everything else being equal, eliminates_player must be what tips it.
func TestScoredStrategyAttackPrefersEliminatingOverIdenticalNonEliminatingAttack(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack

	// Alaska -> Kamchatka: isolate Kamchatka as the only attackable
	// neighbor, and make it player 2's only territory anywhere.
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 2, Armies: 3}

	// Iceland -> Great Britain: isolate Great Britain as the only
	// attackable neighbor, at the exact same army counts. Great Britain
	// stays owned by player 1, the default "everyone else" owner, which
	// owns most of the rest of the board -- not an elimination.
	g.Territories["Iceland"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Greenland"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Scandinavia"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Great Britain"] = risk.TerritoryState{Owner: 1, Armies: 3}

	strat := NewScoredStrategy(DefaultWeights)
	cmd, expl, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack || cmd.From != "Alaska" || cmd.To != "Kamchatka" {
		t.Fatalf("expected the eliminating attack on Kamchatka to win over the identical-stats non-eliminating attack, got %+v", cmd)
	}
	found := false
	for _, f := range expl.Features {
		if f.Name == "eliminates_player" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected eliminates_player in the winning explanation, got %+v", expl.Features)
	}
}
