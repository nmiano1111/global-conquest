package bot

import (
	"context"
	"sync"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

func explorationOptions() []scoredOption {
	return []scoredOption{
		{Command: Command{Action: ActionAttack, To: "Best"}, Features: []Feature{{"x", 10}}},
		{Command: Command{Action: ActionAttack, To: "Second"}, Features: []Feature{{"x", 8}}},
		{Command: Command{Action: ActionAttack, To: "Third"}, Features: []Feature{{"x", 6}}},
	}
}

func TestScoredStrategySelectBestZeroExplorationRateMatchesPackageLevel(t *testing.T) {
	s := NewScoredStrategy(DefaultWeights) // explorationRate 0, randFloat64/randIntN nil
	options := explorationOptions()

	gotCmd, gotExpl := s.selectBest(options, 3)
	wantCmd, wantExpl := selectBest(options, 3)

	if gotCmd != wantCmd {
		t.Fatalf("command diverged from the package-level selectBest: got %+v, want %+v", gotCmd, wantCmd)
	}
	if gotExpl.Score != wantExpl.Score || gotExpl.Explored != false {
		t.Fatalf("explanation diverged: got %+v, want %+v", gotExpl, wantExpl)
	}
}

func TestScoredStrategySelectBestForcedExplorePicksNonArgmax(t *testing.T) {
	s := &ScoredStrategy{
		weights:         DefaultWeights,
		explorationRate: 0.5,
		randFloat64:     func() float64 { return 0 }, // always < explorationRate -> explore
		randIntN:        func(int) int { return 2 },  // pick index 2 ("Third"), not the argmax at 0
	}
	cmd, expl := s.selectBest(explorationOptions(), 3)

	if cmd.To != "Third" {
		t.Fatalf("expected the forced exploration pick (Third), got %+v", cmd)
	}
	if !expl.Explored {
		t.Fatalf("expected Explanation.Explored to be true, got %+v", expl)
	}
	if expl.Score != 6 {
		t.Fatalf("expected the explanation to reflect the explored candidate's own score (6), got %v", expl.Score)
	}
}

func TestScoredStrategySelectBestExplorationRollFailsUsesArgmax(t *testing.T) {
	s := &ScoredStrategy{
		weights:         DefaultWeights,
		explorationRate: 0.5,
		randFloat64:     func() float64 { return 1.0 }, // always >= explorationRate -> no explore
		randIntN:        func(int) int { t.Fatal("randIntN should never be called when the explore roll fails"); return 0 },
	}
	cmd, expl := s.selectBest(explorationOptions(), 3)

	if cmd.To != "Best" {
		t.Fatalf("expected the normal argmax pick (Best), got %+v", cmd)
	}
	if expl.Explored {
		t.Fatalf("expected Explanation.Explored to be false when the explore roll fails, got %+v", expl)
	}
}

func TestScoredStrategySelectBestExploreLandingOnArgmaxIsNotMarkedExplored(t *testing.T) {
	s := &ScoredStrategy{
		weights:         DefaultWeights,
		explorationRate: 1.0,
		randFloat64:     func() float64 { return 0 }, // always explores
		randIntN:        func(int) int { return 0 },  // but coincidentally picks the actual best
	}
	cmd, expl := s.selectBest(explorationOptions(), 3)

	if cmd.To != "Best" {
		t.Fatalf("expected the coincidental argmax pick (Best), got %+v", cmd)
	}
	if expl.Explored {
		t.Fatalf("expected Explanation.Explored to be false when exploration lands on the actual best option, got %+v", expl)
	}
}

func TestScoredStrategySelectBestRecordCandidatesFalseLeavesAllCandidatesNil(t *testing.T) {
	s := &ScoredStrategy{weights: DefaultWeights} // recordCandidates false (zero value), matches NewScoredStrategy
	_, expl := s.selectBest(explorationOptions(), 3)

	if expl.AllCandidates != nil {
		t.Fatalf("expected AllCandidates to stay nil when recordCandidates is false, got %+v", expl.AllCandidates)
	}
}

func TestScoredStrategySelectBestRecordCandidatesPopulatesEveryOption(t *testing.T) {
	s := &ScoredStrategy{weights: DefaultWeights, recordCandidates: true}
	_, expl := s.selectBest(explorationOptions(), 3)

	if len(expl.AllCandidates) != 3 {
		t.Fatalf("expected 3 recorded candidates, got %d: %+v", len(expl.AllCandidates), expl.AllCandidates)
	}
	for _, c := range expl.AllCandidates {
		want := c.Command.To == "Best"
		if c.Chosen != want {
			t.Errorf("candidate %q: Chosen = %v, want %v", c.Command.To, c.Chosen, want)
		}
		if len(c.Features) == 0 {
			t.Errorf("candidate %q: expected its own Features to be recorded, got none", c.Command.To)
		}
	}
}

func TestScoredStrategySelectBestRecordCandidatesReflectsExploredChoice(t *testing.T) {
	s := &ScoredStrategy{
		weights:          DefaultWeights,
		explorationRate:  1.0,
		randFloat64:      func() float64 { return 0 },
		randIntN:         func(int) int { return 2 }, // explores to "Third", not the argmax
		recordCandidates: true,
	}
	_, expl := s.selectBest(explorationOptions(), 3)

	if len(expl.AllCandidates) != 3 {
		t.Fatalf("expected 3 recorded candidates, got %d", len(expl.AllCandidates))
	}
	for _, c := range expl.AllCandidates {
		want := c.Command.To == "Third"
		if c.Chosen != want {
			t.Errorf("candidate %q: Chosen = %v, want %v (explored pick was Third)", c.Command.To, c.Chosen, want)
		}
	}
}

func TestNewExploringScoredStrategySetsRecordCandidates(t *testing.T) {
	s := NewExploringScoredStrategy(DefaultWeights, 0.15)
	if !s.recordCandidates {
		t.Fatal("expected NewExploringScoredStrategy to always set recordCandidates true")
	}
}

func TestScoredStrategySelectBestSingleOptionNeverExplores(t *testing.T) {
	s := &ScoredStrategy{
		weights:         DefaultWeights,
		explorationRate: 1.0,
		randFloat64:     func() float64 { t.Fatal("randFloat64 should never be called with only one option"); return 0 },
		randIntN:        func(int) int { t.Fatal("randIntN should never be called with only one option"); return 0 },
	}
	options := []scoredOption{{Command: Command{Action: ActionEndAttack}, Features: []Feature{{"end_phase_bias", 0}}}}

	cmd, expl := s.selectBest(options, 3)

	if cmd.Action != ActionEndAttack {
		t.Fatalf("expected the only option to be chosen, got %+v", cmd)
	}
	if expl.Explored {
		t.Fatalf("expected Explanation.Explored to be false with only one option, got %+v", expl)
	}
}

// TestNewExploringScoredStrategyIsSafeForConcurrentUse proves the doc
// comment's claim on ScoredStrategy.randFloat64/randIntN: a single
// NewExploringScoredStrategy instance, reused across many concurrent
// NextCommand calls (exactly how internal/tournament.Run and
// cmd/traindata's worker pools already use a shared Simulator/registry),
// never races -- run with `go test -race` to verify this directly, not
// just by inspection.
func TestNewExploringScoredStrategyIsSafeForConcurrentUse(t *testing.T) {
	strat := NewExploringScoredStrategy(DefaultWeights, 0.5)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			g, p0 := newTestGame(t)
			g.Phase = risk.PhaseAttack
			g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 10}
			g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 0, Armies: 1}
			g.Territories["Alberta"] = risk.TerritoryState{Owner: 0, Armies: 1}
			g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1}

			if _, _, err := strat.NextCommand(context.Background(), g, p0); err != nil {
				t.Errorf("NextCommand: %v", err)
			}
		}()
	}
	wg.Wait()
}
