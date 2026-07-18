package bot

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
	"github.com/nmiano1111/global-conquest/backend/internal/tdstate"
)

// singleFeatureBoardValue builds a BoardValue that scores purely on one
// named feature (see tdstate.FeatureNames), zero-mean/unit-std
// standardization, weight on every other feature -- enough to
// deterministically test which candidate BoardValueStrategy picks
// without needing a real trained model or hand-listing ~400 weights.
func singleFeatureBoardValue(t *testing.T, featureName string, weight float64) *BoardValue {
	t.Helper()
	names := tdstate.FeatureNames(risk.ClassicBoard())
	idx := -1
	for i, n := range names {
		if n == featureName {
			idx = i
			break
		}
	}
	if idx == -1 {
		t.Fatalf("feature %q not found in tdstate.FeatureNames", featureName)
	}
	weights := make([]float64, len(names))
	mean := make([]float64, len(names))
	std := make([]float64, len(names))
	for i := range std {
		std[i] = 1
	}
	weights[idx] = weight
	return &BoardValue{Weights: weights, Intercept: 0, Mean: mean, Std: std}
}

// singleFeatureBoardValueWithMargin is singleFeatureBoardValue plus an
// explicit margin (applied to both AttackMargin and FortifyMargin), for
// testing the attack/fortify gate's margin requirement specifically.
func singleFeatureBoardValueWithMargin(t *testing.T, featureName string, weight, margin float64) *BoardValue {
	t.Helper()
	bv := singleFeatureBoardValue(t, featureName, weight)
	bv.AttackMargin = margin
	bv.FortifyMargin = margin
	return bv
}

func TestBoardValueStrategyUnhandledPhaseFallsBackToBasic(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseSetupClaim

	bvs := NewBoardValueStrategy(singleFeatureBoardValue(t, "is_my_turn", 1.0))
	basic := NewBasicStrategy()

	_, _, bvErr := bvs.NextCommand(context.Background(), g, p0)
	_, _, basicErr := basic.NextCommand(context.Background(), g, p0)
	if bvErr == nil || basicErr == nil {
		t.Fatalf("expected both strategies to error identically on an unhandled phase, got bvErr=%v basicErr=%v", bvErr, basicErr)
	}
	if bvErr.Error() != basicErr.Error() {
		t.Fatalf("expected the fallback to produce basic-v1's exact error, got bv=%q basic=%q", bvErr, basicErr)
	}
}

func TestBoardValueStrategyAttackNoLegalAttacksEndsAttack(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	// p0 owns nothing (newTestGame's default owner=1 everywhere), so
	// risk.LegalAttacks is empty.
	bvs := NewBoardValueStrategy(singleFeatureBoardValue(t, "my_army_fraction", 1.0))
	cmd, _, err := bvs.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionEndAttack {
		t.Fatalf("expected end_attack with no legal attacks, got %s", cmd.Action)
	}
}

func TestBoardValueStrategyAttackPrefersHigherScoringCandidate(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	// Alaska attacks Kamchatka with a huge advantage (near-certain
	// capture); Iceland attacks Greenland at a disadvantage (near-certain
	// non-capture). Weighting "territory_Kamchatka_is_mine" heavily
	// rewards whichever candidate's afterstate blend is more likely to
	// have captured it.
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 30}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1}
	g.Territories["Iceland"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["Greenland"] = risk.TerritoryState{Owner: 1, Armies: 10}

	bvs := NewBoardValueStrategy(singleFeatureBoardValue(t, "territory_Kamchatka_is_mine", 10.0))
	cmd, expl, err := bvs.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack || cmd.From != "Alaska" {
		t.Fatalf("expected attack from Alaska (much more likely to capture the rewarded territory), got %+v", cmd)
	}
	if expl.Score <= 0 {
		t.Errorf("expected a positive score for the winning candidate, got %v", expl.Score)
	}
}

func TestBoardValueStrategyAttackRequiresBeatingMargin(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	// Same near-certain-win setup as
	// TestBoardValueStrategyAttackPrefersHigherScoringCandidate, but with
	// a margin large enough that even this favorable attack's improvement
	// over the current state doesn't clear it -- the gate must end the
	// attack instead of taking it, confirming Margin is a real
	// requirement and not just documentation.
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 30}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1}
	g.Territories["Iceland"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["Greenland"] = risk.TerritoryState{Owner: 1, Armies: 10}

	bvs := NewBoardValueStrategy(singleFeatureBoardValueWithMargin(t, "territory_Kamchatka_is_mine", 10.0, 100.0))
	cmd, _, err := bvs.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionEndAttack {
		t.Fatalf("expected end_attack when no candidate's improvement clears Margin, got %+v", cmd)
	}
}

func TestBoardValueStrategyAttackActsWhenImprovementClearsSmallMargin(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 30}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1}
	g.Territories["Iceland"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["Greenland"] = risk.TerritoryState{Owner: 1, Armies: 10}

	bvs := NewBoardValueStrategy(singleFeatureBoardValueWithMargin(t, "territory_Kamchatka_is_mine", 10.0, 0.001))
	cmd, _, err := bvs.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack || cmd.From != "Alaska" {
		t.Fatalf("expected the attack to still be taken when its improvement clears a small margin, got %+v", cmd)
	}
}

func TestBoardValueStrategyAttackEndsWhenNoCandidateBeatsCurrentState(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	// Alaska's neighbors are Northwest Territory, Alberta, Kamchatka
	// (board.go) -- all massively defended, so every legal attack has a
	// near-zero win probability and the afterstate blend collapses to
	// almost entirely the "held" branch, which has strictly fewer
	// attacker armies than the current state (expected combat losses).
	// Rewarding "my_army_fraction" means every legal attack should score
	// just *below* the current, unmodified state.
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 1, Armies: 50}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 1, Armies: 50}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 50}

	bvs := NewBoardValueStrategy(singleFeatureBoardValue(t, "my_army_fraction", 10.0))
	cmd, _, err := bvs.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionEndAttack {
		t.Fatalf("expected end_attack when no legal attack beats the current state, got %+v", cmd)
	}
}

func TestBoardValueStrategyReinforceCardTurnInMandatory(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 0
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Players[0].Cards = []risk.Card{
		{Territory: "Alaska", Symbol: risk.Infantry},
		{Territory: "Peru", Symbol: risk.Cavalry},
		{Territory: "Egypt", Symbol: risk.Artillery},
		{Territory: "Ural", Symbol: risk.Infantry},
		{Territory: "Siam", Symbol: risk.Infantry},
	}

	bvs := NewBoardValueStrategy(singleFeatureBoardValue(t, "my_army_fraction", 1.0))
	cmd, _, err := bvs.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionTradeCards {
		t.Fatalf("expected a mandatory card trade-in (4+ matching cards forces it), got %+v", cmd)
	}
}

func TestBoardValueStrategyReinforcePrefersHigherScoringCandidateAndBatches(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 4
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 4}

	// Reward Alaska's own army fraction specifically -- reinforcing Alaska
	// should score higher than reinforcing Argentina under this weighting.
	bvs := NewBoardValueStrategy(singleFeatureBoardValue(t, "territory_Alaska_army_fraction", 10.0))
	cmd, _, err := bvs.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceReinforcement || cmd.Territory != "Alaska" {
		t.Fatalf("expected place_reinforcement at Alaska (its own army fraction is directly rewarded), got %+v", cmd)
	}
	if cmd.Armies != max(1, 4/3) {
		t.Errorf("expected the same batching rule as ScoredStrategy/GBTStrategy, got Armies=%d", cmd.Armies)
	}
}

func TestBoardValueStrategySetupReinforceReturnsLegalPlacement(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseSetupReinforce
	g.SetupReserves[0] = 3
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}

	bvs := NewBoardValueStrategy(singleFeatureBoardValue(t, "my_army_fraction", 1.0))
	cmd, _, err := bvs.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceInitialArmy {
		t.Fatalf("expected place_initial_army, got %+v", cmd)
	}
}

func TestBoardValueStrategyOccupyPrefersHigherScoringCandidate(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseOccupy
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 10}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Occupy = &risk.OccupyState{From: "Alaska", To: "Kamchatka", MinMove: 1, MaxMove: 9}

	// Reward Kamchatka's own army fraction -- moving the max legal armies
	// there should score highest.
	bvs := NewBoardValueStrategy(singleFeatureBoardValue(t, "territory_Kamchatka_army_fraction", 10.0))
	cmd, _, err := bvs.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionOccupy || cmd.Armies != 9 {
		t.Fatalf("expected occupy with the max legal armies (Kamchatka's own army fraction is directly rewarded), got %+v", cmd)
	}
}

func TestBoardValueStrategyFortifyEndsTurnWhenNoLegalMove(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	// p0 owns nothing, so risk.LegalFortifications is empty.
	bvs := NewBoardValueStrategy(singleFeatureBoardValue(t, "my_army_fraction", 1.0))
	cmd, _, err := bvs.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionEndTurn {
		t.Fatalf("expected end_turn with no legal fortification, got %+v", cmd)
	}
}

func TestBoardValueStrategyFortifyPrefersHigherScoringCandidate(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	g.Territories["Madagascar"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["South Africa"] = risk.TerritoryState{Owner: 0, Armies: 1}

	// Reward South Africa's own army fraction -- fortifying it should
	// score higher than the current (unfortified) state.
	bvs := NewBoardValueStrategy(singleFeatureBoardValue(t, "territory_South Africa_army_fraction", 10.0))
	cmd, _, err := bvs.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionFortify {
		t.Fatalf("expected a fortify move (South Africa's own army fraction is directly rewarded), got %+v", cmd)
	}
}

func TestBoardValueScoreStandardizesAndComputesDotProduct(t *testing.T) {
	bv := &BoardValue{
		Weights:   []float64{2.0, -1.0},
		Intercept: 0.5,
		Mean:      []float64{1.0, 0.0},
		Std:       []float64{2.0, 1.0},
	}
	// standardized = [(3-1)/2, (4-0)/1] = [1.0, 4.0]
	// score = 0.5 + 2*1.0 + (-1)*4.0 = 0.5 + 2 - 4 = -1.5
	got := bv.Score([]float64{3.0, 4.0})
	if got != -1.5 {
		t.Errorf("Score(...) = %v, want -1.5", got)
	}
}

func TestBoardValueScoreHandlesZeroStd(t *testing.T) {
	bv := &BoardValue{
		Weights:   []float64{3.0},
		Intercept: 0,
		Mean:      []float64{5.0},
		Std:       []float64{0.0}, // a constant training-set feature
	}
	// std=0 falls back to 1.0 to avoid divide-by-zero: standardized = 5-5=0
	got := bv.Score([]float64{5.0})
	if got != 0 {
		t.Errorf("Score(...) = %v, want 0", got)
	}
}

func TestLoadBoardValueRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "value.json")
	data, _ := json.Marshal(map[string]any{
		"weights":        []float64{1.0, 2.0},
		"intercept":      0.5,
		"mean":           []float64{0.0, 0.0},
		"std":            []float64{1.0, 1.0},
		"attack_margin":  0.75,
		"fortify_margin": 0.05,
		"feature_names":  []string{"a", "b"},
	})
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	bv, err := LoadBoardValue(path)
	if err != nil {
		t.Fatalf("LoadBoardValue: %v", err)
	}
	if len(bv.Weights) != 2 || bv.Intercept != 0.5 || bv.AttackMargin != 0.75 || bv.FortifyMargin != 0.05 {
		t.Errorf("unexpected loaded BoardValue: %+v", bv)
	}
}

func TestLoadBoardValueMissingFile(t *testing.T) {
	if _, err := LoadBoardValue(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

func TestLoadBoardValueRejectsLengthMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "value.json")
	data, _ := json.Marshal(map[string]any{
		"weights": []float64{1.0, 2.0},
		"mean":    []float64{0.0}, // wrong length
		"std":     []float64{1.0, 1.0},
	})
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadBoardValue(path); err == nil {
		t.Fatal("expected an error for mismatched weights/mean/std lengths")
	}
}
