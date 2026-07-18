package bot

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/bot/gbtmodel"
	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// singleSplitModel builds a tiny one-node model that routes purely on
// featureIndex vs. threshold: <=threshold predicts low (leftValue),
// >threshold predicts high (rightValue) -- enough to deterministically
// test which candidate GBTStrategy picks without needing a real trained
// model.
func singleSplitModel(t *testing.T, featureIndex int, threshold, leftValue, rightValue float64) *gbtmodel.Model {
	t.Helper()
	data := []byte(fmt.Sprintf(`{"tree_info": [{"tree_structure": {
		"split_feature": %d, "threshold": %v, "decision_type": "<=",
		"left_child": {"leaf_value": %v},
		"right_child": {"leaf_value": %v}
	}}]}`, featureIndex, threshold, leftValue, rightValue))
	m, err := gbtmodel.ParseModel(data)
	if err != nil {
		t.Fatalf("ParseModel: %v", err)
	}
	return m
}

// TestGBTStrategyUnhandledPhaseFallsBackToBasic covers setup_claim --
// engine-only and unused in practice (CLAUDE.md), so basic-v1 itself has
// no move for it either; both strategies should fail identically,
// confirming the fallback dispatch actually reaches BasicStrategy rather
// than, say, silently swallowing the phase.
func TestGBTStrategyUnhandledPhaseFallsBackToBasic(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseSetupClaim

	gbt := NewGBTStrategy(map[string]*gbtmodel.Model{})
	basic := NewBasicStrategy()

	_, _, gbtErr := gbt.NextCommand(context.Background(), g, p0)
	_, _, basicErr := basic.NextCommand(context.Background(), g, p0)
	if gbtErr == nil || basicErr == nil {
		t.Fatalf("expected both strategies to error identically on an unhandled phase, got gbtErr=%v basicErr=%v", gbtErr, basicErr)
	}
	if gbtErr.Error() != basicErr.Error() {
		t.Fatalf("expected the fallback to produce basic-v1's exact error, got gbt=%q basic=%q", gbtErr, basicErr)
	}
}

func TestGBTStrategyAttackNoLegalAttacksEndsAttack(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	// p0 owns nothing (newTestGame's default owner=1 everywhere), so
	// risk.LegalAttacks is empty.
	gbt := NewGBTStrategy(map[string]*gbtmodel.Model{
		"attack": singleSplitModel(t, 0, 0.0, -2.0, 2.0),
	})
	cmd, _, err := gbt.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionEndAttack {
		t.Fatalf("expected end_attack with no legal attacks, got %s", cmd.Action)
	}
}

func TestGBTStrategyAttackPrefersHigherPredictedProbability(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	// Alaska attacks Kamchatka with a big army advantage (favorable);
	// Iceland attacks Greenland with a small disadvantage (unfavorable).
	// Feature index 0 is army_advantage -- the model splits purely on it.
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 20}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1}
	g.Territories["Iceland"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["Greenland"] = risk.TerritoryState{Owner: 1, Armies: 3}

	gbt := NewGBTStrategy(map[string]*gbtmodel.Model{
		"attack": singleSplitModel(t, 0, 0.0, -2.0, 2.0),
	})
	cmd, expl, err := gbt.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack || cmd.From != "Alaska" {
		t.Fatalf("expected attack from Alaska (higher army advantage -> higher predicted probability), got %+v", cmd)
	}
	if expl.Score < 0.5 {
		t.Errorf("expected the winning candidate's Score to be the high-probability leaf, got %v", expl.Score)
	}
}

func TestGBTStrategyAttackEndsWhenBestCandidateBelowThreshold(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	// Every legal attack has a negative army advantage -- the model
	// predicts low probability for all of them, so GBTStrategy should end
	// the attack phase rather than take a bad attack anyway. Alaska's
	// neighbors are Northwest Territory, Alberta, and Kamchatka (board.go)
	// -- all three need to be strongly defended, not just Kamchatka, or an
	// easy attack through one of the other two slips in unaccounted for.
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 1, Armies: 10}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 1, Armies: 10}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 10}

	gbt := NewGBTStrategy(map[string]*gbtmodel.Model{
		"attack": singleSplitModel(t, 0, 0.0, -2.0, 2.0),
	})
	cmd, _, err := gbt.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionEndAttack {
		t.Fatalf("expected end_attack when every legal attack is predicted unfavorable, got %+v", cmd)
	}
}

func TestGBTStrategyReinforceCardTurnInMandatory(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 0
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	// 4+ cards forces a trade-in regardless of set composition
	// (scoredCardTurnIn's "approaching_card_limit" case) -- mirrors
	// TestScoredStrategyReinforceCardTurnInMandatory's fixture exactly.
	g.Players[0].Cards = []risk.Card{
		{Territory: "Alaska", Symbol: risk.Infantry},
		{Territory: "Peru", Symbol: risk.Cavalry},
		{Territory: "Egypt", Symbol: risk.Artillery},
		{Territory: "Ural", Symbol: risk.Infantry},
		{Territory: "Siam", Symbol: risk.Infantry},
	}

	gbt := NewGBTStrategy(map[string]*gbtmodel.Model{
		"reinforce": singleSplitModel(t, 2, 0.0, -2.0, 2.0),
	})
	cmd, _, err := gbt.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionTradeCards {
		t.Fatalf("expected a mandatory card trade-in (4+ matching cards forces it), got %+v", cmd)
	}
}

func TestGBTStrategyReinforcePrefersHigherPredictedProbabilityAndBatches(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 4
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 5}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 4}
	// Alaska: weakness = threat(5) - armies(1) = +4 (routes right, high proba).
	// Argentina: weakness = threat(0) - armies(4) = -4 (routes left, low proba).

	gbt := NewGBTStrategy(map[string]*gbtmodel.Model{
		"reinforce": singleSplitModel(t, 2, 0.0, -2.0, 2.0), // feature index 2 = weakness
	})
	cmd, _, err := gbt.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceReinforcement || cmd.Territory != "Alaska" {
		t.Fatalf("expected place_reinforcement at Alaska (higher predicted probability), got %+v", cmd)
	}
	if cmd.Armies != max(1, 4/3) {
		t.Errorf("expected the same batching rule as ScoredStrategy.reinforce, got Armies=%d", cmd.Armies)
	}
}

func TestGBTStrategySetupReinforceReturnsLegalPlacement(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseSetupReinforce
	g.SetupReserves[0] = 3
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}

	gbt := NewGBTStrategy(map[string]*gbtmodel.Model{
		"reinforce": singleSplitModel(t, 2, 0.0, -2.0, 2.0),
	})
	cmd, _, err := gbt.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceInitialArmy {
		t.Fatalf("expected place_initial_army, got %+v", cmd)
	}
}

func TestGBTStrategyOccupyPrefersHigherPredictedProbability(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseOccupy
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 10}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Occupy = &risk.OccupyState{From: "Alaska", To: "Kamchatka", MinMove: 1, MaxMove: 9}

	// feature index 1 = momentum = min(armies, destThreat). destThreat is 0
	// here (Kamchatka's neighbors are all owner 1 by default except Alaska),
	// so momentum is always 0 regardless of armies moved -- use feature
	// index 2 (momentum_surplus = armies) instead, split at 8 (MaxMove-1)
	// so only armies=9 uniquely routes to the high leaf -- every other
	// candidate (1-8) ties at the low leaf, which would otherwise make
	// "prefers higher" untestable via a single strict split.
	gbt := NewGBTStrategy(map[string]*gbtmodel.Model{
		"occupy": singleSplitModel(t, 2, 8.0, -2.0, 2.0),
	})
	cmd, _, err := gbt.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionOccupy || cmd.Armies != 9 {
		t.Fatalf("expected occupy with the max legal armies (momentum_surplus increases with armies moved), got %+v", cmd)
	}
}

func TestGBTStrategyFortifyEndsTurnWhenNoLegalMove(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	// p0 owns nothing, so risk.LegalFortifications is empty.
	gbt := NewGBTStrategy(map[string]*gbtmodel.Model{
		"fortify": singleSplitModel(t, 0, 0.0, -2.0, 2.0),
	})
	cmd, _, err := gbt.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionEndTurn {
		t.Fatalf("expected end_turn with no legal fortification, got %+v", cmd)
	}
}

func TestGBTStrategyFortifyPrefersHigherPredictedProbability(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	g.Territories["Madagascar"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["South Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["East Africa"] = risk.TerritoryState{Owner: 1, Armies: 2}

	gbt := NewGBTStrategy(map[string]*gbtmodel.Model{
		"fortify": singleSplitModel(t, 0, 0.0, -2.0, 2.0), // feature index 0 = destination_threat
	})
	cmd, _, err := gbt.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionFortify {
		t.Fatalf("expected a fortify move (South Africa is threatened by East Africa), got %+v", cmd)
	}
}

func TestLoadGBTModelsLoadsAllFourPhases(t *testing.T) {
	dir := t.TempDir()
	writeMinimalModel(t, dir, "attack.json")
	writeMinimalModel(t, dir, "reinforce.json")
	writeMinimalModel(t, dir, "occupy.json")
	writeMinimalModel(t, dir, "fortify.json")

	models, err := LoadGBTModels(dir)
	if err != nil {
		t.Fatalf("LoadGBTModels: %v", err)
	}
	for _, phase := range []string{"attack", "reinforce", "occupy", "fortify"} {
		if models[phase] == nil {
			t.Errorf("expected a loaded model for phase %q", phase)
		}
	}
}

func TestLoadGBTModelsMissingFile(t *testing.T) {
	dir := t.TempDir()
	writeMinimalModel(t, dir, "attack.json")
	// reinforce.json, occupy.json, fortify.json deliberately absent.

	if _, err := LoadGBTModels(dir); err == nil {
		t.Fatal("expected an error when a phase model file is missing")
	}
}

func writeMinimalModel(t *testing.T, dir, name string) {
	t.Helper()
	data := []byte(`{"tree_info": [{"tree_structure": {"leaf_value": 0.0}}]}`)
	path := dir + "/" + name
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
