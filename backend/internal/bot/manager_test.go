package bot

import (
	"context"
	"sync"
	"testing"
	"time"
)

// scriptedRunner returns a queued (reason, err) pair per gameID call and lets
// a test block the first call until released, so it can race a second
// Trigger against a still-running runner.
type scriptedRunner struct {
	mu      sync.Mutex
	calls   int
	results []StopReason
	hold    chan struct{} // if non-nil, the first call blocks until closed
}

func (r *scriptedRunner) RunTurn(_ context.Context, _ string, _ ExecutionMode) (StopReason, error) {
	r.mu.Lock()
	i := r.calls
	r.calls++
	r.mu.Unlock()

	if i == 0 && r.hold != nil {
		<-r.hold
	}

	if i >= len(r.results) {
		return StopNotBotTurn, nil
	}
	return r.results[i], nil
}

func (r *scriptedRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func waitForCallCount(t *testing.T, r *scriptedRunner, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if r.callCount() >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d calls, got %d", want, r.callCount())
}

func TestManagerSuppressesDuplicateTrigger(t *testing.T) {
	hold := make(chan struct{})
	runner := &scriptedRunner{hold: hold}
	m := NewManager(context.Background(), runner, ExecutionSimulation)

	m.Trigger("g1")
	// The first call is now blocked inside RunTurn holding the registry
	// entry; a second Trigger for the same game must be suppressed.
	waitForRunnerStarted(t, runner)
	m.Trigger("g1")
	m.Trigger("g1")

	close(hold)
	waitForCallCount(t, runner, 1)
	// Give any errant second spawn a moment to (not) happen.
	time.Sleep(20 * time.Millisecond)
	if got := runner.callCount(); got != 1 {
		t.Fatalf("expected exactly 1 RunTurn call despite 3 triggers, got %d", got)
	}
}

func waitForRunnerStarted(t *testing.T, r *scriptedRunner) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		started := r.calls > 0
		r.mu.Unlock()
		if started {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("runner never started")
}

func TestManagerRegistryClearsAfterRunnerExits(t *testing.T) {
	runner := &scriptedRunner{results: []StopReason{StopNotBotTurn}}
	m := NewManager(context.Background(), runner, ExecutionSimulation)

	m.Trigger("g1")
	waitForCallCount(t, runner, 1)
	time.Sleep(10 * time.Millisecond) // let Manager.run finish its cleanup

	m.mu.Lock()
	_, stillActive := m.active["g1"]
	m.mu.Unlock()
	if stillActive {
		t.Fatalf("expected registry entry to be removed once the runner exits")
	}

	// A fresh trigger after cleanup must start a new run, not be suppressed.
	m.Trigger("g1")
	waitForCallCount(t, runner, 2)
}

func TestManagerChainsOnlyWhenTurnEnded(t *testing.T) {
	// StopTurnEnded must cause exactly one automatic re-trigger, which then
	// sees StopNotBotTurn and stops chaining (no busy loop).
	runner := &scriptedRunner{results: []StopReason{StopTurnEnded, StopNotBotTurn}}
	m := NewManager(context.Background(), runner, ExecutionSimulation)

	m.Trigger("g1")
	waitForCallCount(t, runner, 2)

	time.Sleep(30 * time.Millisecond) // ensure no further spawns occur
	if got := runner.callCount(); got != 2 {
		t.Fatalf("expected chaining to stop after the non-bot-turn check, got %d calls", got)
	}
}

func TestManagerDoesNotChainOnGameOverOrError(t *testing.T) {
	for _, reason := range []StopReason{StopGameOver, StopGameInactive, StopStrategyError, StopMaxRetriesExceeded, StopCanceled} {
		runner := &scriptedRunner{results: []StopReason{reason}}
		m := NewManager(context.Background(), runner, ExecutionSimulation)
		m.Trigger("g1")
		waitForCallCount(t, runner, 1)
		time.Sleep(20 * time.Millisecond)
		if got := runner.callCount(); got != 1 {
			t.Fatalf("reason=%s: expected no chaining, got %d calls", reason, got)
		}
	}
}

func TestManagerHumanOnlyGameDoesNotChain(t *testing.T) {
	runner := &scriptedRunner{results: []StopReason{StopNotBotTurn}}
	m := NewManager(context.Background(), runner, ExecutionSimulation)
	m.Trigger("g1")
	waitForCallCount(t, runner, 1)
	time.Sleep(20 * time.Millisecond)
	if got := runner.callCount(); got != 1 {
		t.Fatalf("expected a human-only game to result in exactly one no-op call, got %d", got)
	}
}
