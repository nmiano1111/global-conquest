package simulation

import (
	"testing"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// generousLimits returns Limits with every field set high enough not to
// interfere with a test focused on a single limit, which the caller then
// tightens.
func generousLimits() Limits {
	return Limits{
		MaxCommands:                1_000_000,
		MaxTurns:                   1_000_000,
		MaxCommandsWithoutProgress: 1_000_000,
		MaxRepeatedStates:          1_000_000,
		MaxDuration:                time.Hour,
	}
}

func limitFixture(t *testing.T) *risk.Game {
	t.Helper()
	g, err := risk.NewClassicAutoStartGame([]string{"p0", "p1", "p2", "p3"}, NewDeterministicRNG(1))
	if err != nil {
		t.Fatalf("build fixture: %v", err)
	}
	return g
}

func TestLimitTrackerMaxDuration(t *testing.T) {
	limits := generousLimits()
	limits.MaxDuration = 20 * time.Millisecond
	tracker := NewLimitTracker(limits)
	g := limitFixture(t)

	if breach := tracker.Observe(g); breach != nil {
		t.Fatalf("expected no breach immediately after construction, got %+v", breach)
	}

	time.Sleep(30 * time.Millisecond)

	breach := tracker.Observe(g)
	if breach == nil {
		t.Fatalf("expected a breach once MaxDuration elapsed")
	}
	if breach.Type != FailureDurationLimitReached {
		t.Fatalf("expected FailureDurationLimitReached, got %s", breach.Type)
	}
}

// TestLimitTrackerMaxCommands: MaxCommands=3 means 3 commands may be
// dispatched total -- the breach fires on the 3rd Observe call (the one
// that reaches the cap), not the 4th, so a caller that stops immediately
// on breach (as Simulator.RunOne does) ends with exactly 3 commands
// recorded, never 4.
func TestLimitTrackerMaxCommands(t *testing.T) {
	limits := generousLimits()
	limits.MaxCommands = 3
	tracker := NewLimitTracker(limits)
	g := limitFixture(t)

	for i := 1; i <= 2; i++ {
		if breach := tracker.Observe(g); breach != nil {
			t.Fatalf("call %d: expected no breach yet, got %+v", i, breach)
		}
	}
	breach := tracker.Observe(g)
	if breach == nil {
		t.Fatalf("expected a breach on the 3rd call")
	}
	if breach.Type != FailureCommandLimitReached {
		t.Fatalf("expected FailureCommandLimitReached, got %s", breach.Type)
	}
}

func TestLimitTrackerMaxTurns(t *testing.T) {
	limits := generousLimits()
	limits.MaxTurns = 5
	tracker := NewLimitTracker(limits)
	g := limitFixture(t)

	g.TurnNumber = 5
	if breach := tracker.Observe(g); breach != nil {
		t.Fatalf("expected no breach at exactly MaxTurns, got %+v", breach)
	}
	g.TurnNumber = 6
	breach := tracker.Observe(g)
	if breach == nil {
		t.Fatalf("expected a breach past MaxTurns")
	}
	if breach.Type != FailureTurnLimitReached {
		t.Fatalf("expected FailureTurnLimitReached, got %s", breach.Type)
	}
}

func TestLimitTrackerNoProgressDetection(t *testing.T) {
	limits := generousLimits()
	limits.MaxCommandsWithoutProgress = 3
	limits.MaxRepeatedStates = 1_000_000 // ensure the fingerprint check can't fire first
	tracker := NewLimitTracker(limits)
	g := limitFixture(t)

	// First observation always establishes the baseline, never a breach.
	if breach := tracker.Observe(g); breach != nil {
		t.Fatalf("expected no breach on the first observation, got %+v", breach)
	}
	// Two more with no CurrentPlayer/Phase change: still under threshold.
	for i := 0; i < 2; i++ {
		if breach := tracker.Observe(g); breach != nil {
			t.Fatalf("unexpected breach before threshold: %+v", breach)
		}
	}
	// The 3rd consecutive no-change observation trips it.
	breach := tracker.Observe(g)
	if breach == nil {
		t.Fatalf("expected a breach once no-progress threshold is reached")
	}
	if breach.Type != FailureRepeatedStateDetected {
		t.Fatalf("expected FailureRepeatedStateDetected, got %s", breach.Type)
	}
}

func TestLimitTrackerNoProgressResetsOnPhaseChange(t *testing.T) {
	limits := generousLimits()
	limits.MaxCommandsWithoutProgress = 3
	tracker := NewLimitTracker(limits)
	g := limitFixture(t)

	// Two no-change observations (below threshold).
	tracker.Observe(g)
	tracker.Observe(g)

	// Change phase -- this must reset the counter rather than let it
	// carry over toward the threshold.
	g.Phase = risk.PhaseFortify
	if breach := tracker.Observe(g); breach != nil {
		t.Fatalf("expected the phase change itself to reset progress tracking, got %+v", breach)
	}

	// Two more no-change observations from the new baseline: still
	// under threshold (noProgressCount reaches 1, then 2) if the reset
	// actually zeroed the counter rather than letting it carry over.
	if breach := tracker.Observe(g); breach != nil {
		t.Fatalf("expected no breach 1 no-change call after reset, got %+v", breach)
	}
	if breach := tracker.Observe(g); breach != nil {
		t.Fatalf("expected no breach 2 no-change calls after reset, got %+v", breach)
	}
	// The 3rd no-change call after the reset reaches the threshold of 3.
	if breach := tracker.Observe(g); breach == nil {
		t.Fatalf("expected the breach to fire on the 3rd no-change call after reset")
	}
}

func TestLimitTrackerRepeatedStateDetection(t *testing.T) {
	limits := generousLimits()
	limits.MaxRepeatedStates = 3
	limits.MaxCommandsWithoutProgress = 100 // keep the cheaper check from firing first
	tracker := NewLimitTracker(limits)
	g := limitFixture(t)

	var last *LimitBreach
	for i := 0; i < 3; i++ {
		last = tracker.Observe(g)
	}
	if last == nil {
		t.Fatalf("expected a breach once the identical state recurred MaxRepeatedStates times")
	}
	if last.Type != FailureRepeatedStateDetected {
		t.Fatalf("expected FailureRepeatedStateDetected, got %s", last.Type)
	}
}

func TestLimitTrackerDoesNotFalseTriggerOnNormalProgress(t *testing.T) {
	tracker := NewLimitTracker(DefaultLimits())
	g := limitFixture(t)

	phases := []risk.Phase{risk.PhaseReinforce, risk.PhaseAttack, risk.PhaseOccupy, risk.PhaseFortify}
	for i := 0; i < 40; i++ {
		g.Phase = phases[i%len(phases)]
		g.CurrentPlayer = i % len(g.Players)
		ts := g.Territories["Alaska"]
		ts.Armies = i + 1
		g.Territories["Alaska"] = ts

		if breach := tracker.Observe(g); breach != nil {
			t.Fatalf("call %d: unexpected breach during normal, changing progress: %+v", i, breach)
		}
	}
}
