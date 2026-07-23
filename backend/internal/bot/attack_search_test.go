package bot

import (
	"context"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
	"github.com/nmiano1111/global-conquest/backend/internal/tdstate"
)

// TestValueStrategyAttackSearchDepthZeroMatchesBlendBaseline confirms
// AttackSearchDepth's zero value (the default) leaves attack() on the
// original, already-validated single-ply attackAfterstateBlend path --
// same scenario/assertions as
// TestValueStrategyAttackPrefersHigherScoringCandidate in
// strategy_value_test.go, just with the new field explicitly set to
// make the "search is opt-in" contract visible in this file too.
func TestValueStrategyAttackSearchDepthZeroMatchesBlendBaseline(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 30}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1}
	g.Territories["Iceland"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["Greenland"] = risk.TerritoryState{Owner: 1, Armies: 10}

	bvs := NewBoardValueStrategy(singleFeatureBoardValue(t, "territory_Kamchatka_is_mine", 10.0))
	bvs.AttackSearchDepth = 0
	cmd, _, err := bvs.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack || cmd.From != "Alaska" {
		t.Fatalf("expected the unchanged blend-based decision (attack from Alaska), got %+v", cmd)
	}
}

// multiFeatureBoardValue is singleFeatureBoardValue generalized to
// several named features at once -- for scenarios that need one first
// move's own 1-ply blend score to genuinely outrank another's (not
// tied at zero), e.g. to test AttackSearchBreadth actually excluding a
// candidate from deeper exploration.
func multiFeatureBoardValue(t *testing.T, weights map[string]float64) *BoardValue {
	t.Helper()
	names := tdstate.FeatureNames(risk.ClassicBoard())
	byName := make(map[string]int, len(names))
	for i, n := range names {
		byName[n] = i
	}
	w := make([]float64, len(names))
	for featureName, weight := range weights {
		idx, ok := byName[featureName]
		if !ok {
			t.Fatalf("feature %q not found in tdstate.FeatureNames", featureName)
		}
		w[idx] = weight
	}
	mean := make([]float64, len(names))
	std := make([]float64, len(names))
	for i := range std {
		std[i] = 1
	}
	return &BoardValue{Weights: w, Intercept: 0, Mean: mean, Std: std}
}

// greenlandSequenceGame builds the scenario TestAttackSequenceSearch*
// share: p0 owns only Alaska (30 armies), adjacent to three
// weakly-defended (1 army) enemy territories -- Northwest Territory,
// Alberta, Kamchatka. Only Northwest Territory sits on a path to
// Greenland (also 1 army): Northwest Territory's own neighbors are
// Alaska, Alberta, Ontario, and Greenland -- Alberta and Kamchatka's
// neighborhoods never reach Greenland within two attacks. A value
// function that rewards only "territory_Greenland_is_mine" therefore
// makes Alaska->Northwest Territory the only first move that can ever
// pay off, and only once the search can see two attacks ahead --
// exactly the property LookaheadDepth's single-greedy-path design could
// never discover, since Northwest Territory doesn't capture anything
// rewarded on its own.
func greenlandSequenceGame(t *testing.T) (*risk.Game, string) {
	t.Helper()
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 30}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 1, Armies: 1}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 1, Armies: 1}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1}
	g.Territories["Greenland"] = risk.TerritoryState{Owner: 1, Armies: 1}
	return g, p0
}

// TestAttackSequenceSearchFindsNonGreedyFirstMove is the core "real
// branching, not a chain of greedy single picks" test: at depth 2, the
// search must choose Alaska->Northwest Territory even though it doesn't
// capture anything the value function rewards by itself -- only
// discoverable by exploring the *sequence* Northwest Territory unlocks
// (Northwest Territory -> Greenland), not by scoring Alaska's three
// immediate neighbors independently and picking the best one (which
// would tie, since none of them captures Greenland in a single move).
func TestAttackSequenceSearchFindsNonGreedyFirstMove(t *testing.T) {
	g, p0 := greenlandSequenceGame(t)

	bvs := NewBoardValueStrategy(singleFeatureBoardValue(t, "territory_Greenland_is_mine", 1.0))
	bvs.AttackSearchDepth = 2
	cmd, expl, err := bvs.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack || cmd.From != "Alaska" || cmd.To != "Northwest Territory" {
		t.Fatalf("expected attack Alaska->Northwest Territory (the only path to the rewarded territory), got %+v", cmd)
	}
	if expl.Score <= 0 {
		t.Errorf("expected a positive leaf score from the discovered sequence, got %v", expl.Score)
	}
}

// TestAttackSequenceSearchDepthLimitsWhatIsFound uses the identical
// scenario as TestAttackSequenceSearchFindsNonGreedyFirstMove but caps
// AttackSearchDepth at 1: Greenland is two attacks away from Alaska, so
// a 1-ply search can never see it, every immediate candidate scores
// identically to doing nothing, and the margin gate (0, via
// singleFeatureBoardValue) correctly ends the attack phase instead of
// taking a pointless capture. Confirms the depth parameter isn't a
// no-op.
func TestAttackSequenceSearchDepthLimitsWhatIsFound(t *testing.T) {
	g, p0 := greenlandSequenceGame(t)

	bvs := NewBoardValueStrategy(singleFeatureBoardValue(t, "territory_Greenland_is_mine", 1.0))
	bvs.AttackSearchDepth = 1
	cmd, _, err := bvs.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionEndAttack {
		t.Fatalf("expected end_attack when the rewarded territory is out of reach at depth 1, got %+v", cmd)
	}
}

// TestAttackSequenceSearchMarginGate confirms the margin gate still
// applies on top of the search's bestScore exactly as it does for the
// blend-based path: a large margin blocks the otherwise-profitable
// Greenland sequence, a tiny one lets it through.
func TestAttackSequenceSearchMarginGate(t *testing.T) {
	t.Run("large margin blocks", func(t *testing.T) {
		g, p0 := greenlandSequenceGame(t)
		bvs := NewBoardValueStrategy(singleFeatureBoardValueWithMargin(t, "territory_Greenland_is_mine", 1.0, 100.0))
		bvs.AttackSearchDepth = 2
		cmd, _, err := bvs.NextCommand(context.Background(), g, p0)
		if err != nil {
			t.Fatalf("NextCommand: %v", err)
		}
		if cmd.Action != ActionEndAttack {
			t.Fatalf("expected end_attack when the sequence's improvement doesn't clear a large margin, got %+v", cmd)
		}
	})

	t.Run("small margin passes", func(t *testing.T) {
		g, p0 := greenlandSequenceGame(t)
		bvs := NewBoardValueStrategy(singleFeatureBoardValueWithMargin(t, "territory_Greenland_is_mine", 1.0, 0.001))
		bvs.AttackSearchDepth = 2
		cmd, _, err := bvs.NextCommand(context.Background(), g, p0)
		if err != nil {
			t.Fatalf("NextCommand: %v", err)
		}
		if cmd.Action != ActionAttack || cmd.From != "Alaska" || cmd.To != "Northwest Territory" {
			t.Fatalf("expected the sequence to still be taken when its improvement clears a small margin, got %+v", cmd)
		}
	})
}

// TestAttackSearchBreadthCanHideTheBestSequence documents the real
// tradeoff AttackSearchBreadth introduces: rewards a big Greenland
// payoff (only reachable via Alaska->Northwest Territory->Greenland,
// same shape as greenlandSequenceGame) alongside small, direct rewards
// for capturing Alberta or Kamchatka outright -- so Northwest
// Territory's own 1-ply blend score (0, it doesn't touch anything
// rewarded by itself) ranks strictly below Alberta/Kamchatka's. With
// AttackSearchBreadth=0 (unlimited) the search still finds the
// Greenland sequence; with AttackSearchBreadth=2, Northwest Territory
// is pruned out of consideration before the search ever looks two
// plies deep, and the strategy settles for the smaller, immediately
// visible reward instead.
func TestAttackSearchBreadthCanHideTheBestSequence(t *testing.T) {
	weights := map[string]float64{
		"territory_Greenland_is_mine": 1.0,
		"territory_Alberta_is_mine":   0.01,
		"territory_Kamchatka_is_mine": 0.01,
	}

	t.Run("unlimited breadth finds the Greenland sequence", func(t *testing.T) {
		g, p0 := greenlandSequenceGame(t)
		bvs := NewBoardValueStrategy(multiFeatureBoardValue(t, weights))
		bvs.AttackSearchDepth = 2
		bvs.AttackSearchBreadth = 0
		cmd, _, err := bvs.NextCommand(context.Background(), g, p0)
		if err != nil {
			t.Fatalf("NextCommand: %v", err)
		}
		if cmd.Action != ActionAttack || cmd.From != "Alaska" || cmd.To != "Northwest Territory" {
			t.Fatalf("expected attack Alaska->Northwest Territory (the Greenland path), got %+v", cmd)
		}
	})

	t.Run("breadth=2 prunes Northwest Territory out and misses it", func(t *testing.T) {
		g, p0 := greenlandSequenceGame(t)
		bvs := NewBoardValueStrategy(multiFeatureBoardValue(t, weights))
		bvs.AttackSearchDepth = 2
		bvs.AttackSearchBreadth = 2
		cmd, _, err := bvs.NextCommand(context.Background(), g, p0)
		if err != nil {
			t.Fatalf("NextCommand: %v", err)
		}
		if cmd.Action != ActionAttack || cmd.To == "Northwest Territory" {
			t.Fatalf("expected the search to settle for the smaller, immediately-visible reward (Alberta or Kamchatka), not Northwest Territory, got %+v", cmd)
		}
	})
}

func TestCandidateAttacksReturnsTopScoringByBreadth(t *testing.T) {
	g, p0 := greenlandSequenceGame(t)
	weights := map[string]float64{
		"territory_Greenland_is_mine": 1.0,
		"territory_Alberta_is_mine":   0.02,
		"territory_Kamchatka_is_mine": 0.01,
	}
	bvs := NewBoardValueStrategy(multiFeatureBoardValue(t, weights))
	pi := playerIndex(g, p0)

	all := bvs.candidateAttacks(g, p0, pi, 0)
	if len(all) != 3 {
		t.Fatalf("expected all 3 legal attacks from Alaska with breadth=0, got %d: %+v", len(all), all)
	}

	top2 := bvs.candidateAttacks(g, p0, pi, 2)
	if len(top2) != 2 {
		t.Fatalf("expected exactly 2 candidates with breadth=2, got %d: %+v", len(top2), top2)
	}
	if top2[0].To != "Alberta" || top2[1].To != "Kamchatka" {
		t.Fatalf("expected [Alberta, Kamchatka] in descending-score order (Northwest Territory scores 0, excluded), got %+v", top2)
	}
}

func TestValueStrategyRiskyDefaultsWhenUnset(t *testing.T) {
	s := &ValueStrategy{}
	if got := s.risky(); got != defaultRisky {
		t.Errorf("risky() with Risky unset = %v, want defaultRisky (%v)", got, defaultRisky)
	}

	s.Risky = 0.5
	if got := s.risky(); got != 0.5 {
		t.Errorf("risky() with Risky=0.5 = %v, want 0.5", got)
	}

	s.Risky = -1
	if got := s.risky(); got != defaultRisky {
		t.Errorf("risky() with Risky negative = %v, want defaultRisky (%v)", got, defaultRisky)
	}
}

func TestApplyTerminalOutcome(t *testing.T) {
	g, _ := newTestGame(t)
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 10}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 3}
	a := risk.AttackAction{From: "Alaska", To: "Kamchatka", SourceArmies: 10, TargetArmies: 3, MaxAttackerDice: 3}

	t.Run("conquered", func(t *testing.T) {
		next := applyTerminalOutcome(g, 0, a, TerminalState{AttackerRemaining: 7, DefenderRemaining: 0}, 3)
		if got := next.Territories["Kamchatka"]; got.Owner != 0 || got.Armies != 3 {
			t.Errorf("Kamchatka after conquest = %+v, want Owner=0 Armies=3", got)
		}
		if got := next.Territories["Alaska"]; got.Owner != 0 || got.Armies != 4 {
			t.Errorf("Alaska after conquest = %+v, want Owner=0 Armies=4 (7 remaining - 3 occupying)", got)
		}
		// original untouched
		if g.Territories["Kamchatka"].Owner != 1 || g.Territories["Alaska"].Armies != 10 {
			t.Errorf("original game state was mutated: %+v / %+v", g.Territories["Kamchatka"], g.Territories["Alaska"])
		}
	})

	t.Run("held", func(t *testing.T) {
		next := applyTerminalOutcome(g, 0, a, TerminalState{AttackerRemaining: 1, DefenderRemaining: 2}, 3)
		if got := next.Territories["Kamchatka"]; got.Owner != 1 || got.Armies != 2 {
			t.Errorf("Kamchatka after held = %+v, want Owner=1 Armies=2", got)
		}
		if got := next.Territories["Alaska"]; got.Owner != 0 || got.Armies != 1 {
			t.Errorf("Alaska after held = %+v, want Owner=0 Armies=1", got)
		}
		if g.Territories["Kamchatka"].Owner != 1 || g.Territories["Kamchatka"].Armies != 3 {
			t.Errorf("original game state was mutated: %+v", g.Territories["Kamchatka"])
		}
	})
}
