package bot

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/game"
	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// fakeLoader serves a queue of (*risk.Game, status) pairs, one per call,
// repeating the last entry once the queue is exhausted. This lets a test
// script exactly what "fresh state" the runner sees on each reload.
type fakeLoader struct {
	mu     sync.Mutex
	states []loadedState
	calls  int
	err    error
}

type loadedState struct {
	game   *risk.Game
	status string
}

func (f *fakeLoader) LoadGame(_ context.Context, _ string) (*risk.Game, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, "", f.err
	}
	i := f.calls
	if i >= len(f.states) {
		i = len(f.states) - 1
	}
	f.calls++
	return f.states[i].game, f.states[i].status, nil
}

// fakeSubmitter records every submitted command and returns a scripted
// response (or error) per call, defaulting to a generic success reply.
type fakeSubmitter struct {
	mu     sync.Mutex
	inputs []game.GameActionInput
	respFn func(call int, in game.GameActionInput) (game.GameActionUpdate, error)
}

func (f *fakeSubmitter) SubmitGameAction(_ context.Context, in game.GameActionInput) (game.GameActionUpdate, error) {
	f.mu.Lock()
	call := len(f.inputs)
	f.inputs = append(f.inputs, in)
	f.mu.Unlock()
	if f.respFn != nil {
		return f.respFn(call, in)
	}
	return game.GameActionUpdate{Phase: string(risk.PhaseReinforce)}, nil
}

// fakeSleeper records every requested delay without actually sleeping,
// unless armed with a ctx-cancellation check.
type fakeSleeper struct {
	mu            sync.Mutex
	delays        []time.Duration
	respectCancel bool
}

func (f *fakeSleeper) Sleep(ctx context.Context, d time.Duration) error {
	f.mu.Lock()
	f.delays = append(f.delays, d)
	f.mu.Unlock()
	if f.respectCancel && ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

// fakeStrategy always returns the same command, or an error if set.
type fakeStrategy struct {
	cmd Command
	err error
}

func (f *fakeStrategy) NextCommand(_ context.Context, _ *risk.Game, _ string) (Command, Explanation, error) {
	return f.cmd, Explanation{}, f.err
}

func botGame(currentPlayerID string, controller risk.ControllerType) *risk.Game {
	g, err := risk.NewClassicGame([]string{"human", "bot1", "p3"}, nil)
	if err != nil {
		panic(err)
	}
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	for i := range g.Players {
		if g.Players[i].ID == currentPlayerID {
			g.Players[i].Controller = controller
			g.Players[i].Strategy = StrategyBasicV1
			g.CurrentPlayer = i
		}
	}
	return g
}

func TestRunnerNotBotTurnStopsImmediately(t *testing.T) {
	g := botGame("human", risk.ControllerHuman)
	loader := &fakeLoader{states: []loadedState{{game: g, status: "in_progress"}}}
	submitter := &fakeSubmitter{}
	r := NewRunner(loader, submitter, StrategyRegistry{StrategyBasicV1: &fakeStrategy{}}, &fakeSleeper{}, DefaultPacingConfig())

	reason, err := r.RunTurn(context.Background(), "g1", ExecutionSimulation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reason != StopNotBotTurn {
		t.Fatalf("expected StopNotBotTurn, got %s", reason)
	}
	if len(submitter.inputs) != 0 {
		t.Fatalf("expected no commands submitted for a human turn, got %d", len(submitter.inputs))
	}
}

func TestRunnerStopsWhenGameOver(t *testing.T) {
	g := botGame("bot1", risk.ControllerBot)
	g.Phase = risk.PhaseGameOver
	loader := &fakeLoader{states: []loadedState{{game: g, status: "completed"}}}
	submitter := &fakeSubmitter{}
	r := NewRunner(loader, submitter, StrategyRegistry{StrategyBasicV1: &fakeStrategy{}}, &fakeSleeper{}, DefaultPacingConfig())

	reason, err := r.RunTurn(context.Background(), "g1", ExecutionSimulation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reason != StopGameInactive {
		t.Fatalf("expected StopGameInactive (status=completed), got %s", reason)
	}
}

func TestRunnerStopsWhenCommittedActionEndsGame(t *testing.T) {
	g := botGame("bot1", risk.ControllerBot)
	loader := &fakeLoader{states: []loadedState{{game: g, status: "in_progress"}}}
	submitter := &fakeSubmitter{respFn: func(call int, in game.GameActionInput) (game.GameActionUpdate, error) {
		return game.GameActionUpdate{Phase: string(risk.PhaseGameOver)}, nil
	}}
	strat := &fakeStrategy{cmd: Command{Action: ActionEndTurn}}
	r := NewRunner(loader, submitter, StrategyRegistry{StrategyBasicV1: strat}, &fakeSleeper{}, DefaultPacingConfig())

	reason, err := r.RunTurn(context.Background(), "g1", ExecutionSimulation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reason != StopGameOver {
		t.Fatalf("expected StopGameOver after a winning commit, got %s", reason)
	}
	if len(submitter.inputs) != 1 {
		t.Fatalf("expected exactly one command submitted, got %d", len(submitter.inputs))
	}
}

func TestRunnerStopsWhenCurrentPlayerChanges(t *testing.T) {
	g1 := botGame("bot1", risk.ControllerBot)
	g2 := botGame("bot1", risk.ControllerBot)
	// After the committed command, a fresh load shows the human is now current.
	for i := range g2.Players {
		if g2.Players[i].ID == "human" {
			g2.CurrentPlayer = i
		}
	}
	loader := &fakeLoader{states: []loadedState{
		{game: g1, status: "in_progress"},
		{game: g2, status: "in_progress"},
	}}
	submitter := &fakeSubmitter{}
	strat := &fakeStrategy{cmd: Command{Action: ActionEndTurn}}
	r := NewRunner(loader, submitter, StrategyRegistry{StrategyBasicV1: strat}, &fakeSleeper{}, DefaultPacingConfig())

	reason, err := r.RunTurn(context.Background(), "g1", ExecutionSimulation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reason != StopTurnEnded {
		t.Fatalf("expected StopTurnEnded, got %s", reason)
	}
	if len(submitter.inputs) != 1 {
		t.Fatalf("expected exactly one command submitted, got %d", len(submitter.inputs))
	}
	if loader.calls != 2 {
		t.Fatalf("expected state to be reloaded between commands (2 loads), got %d", loader.calls)
	}
}

func TestRunnerUsesNormalCommandPath(t *testing.T) {
	g := botGame("bot1", risk.ControllerBot)
	loader := &fakeLoader{states: []loadedState{{game: g, status: "in_progress"}}}
	submitter := &fakeSubmitter{respFn: func(call int, in game.GameActionInput) (game.GameActionUpdate, error) {
		return game.GameActionUpdate{}, errors.New("stop after one call")
	}}
	strat := &fakeStrategy{cmd: Command{Action: ActionPlaceReinforcement, Territory: "Alaska", Armies: 3}}
	r := NewRunner(loader, submitter, StrategyRegistry{StrategyBasicV1: strat}, &fakeSleeper{}, DefaultPacingConfig())

	_, _ = r.RunTurn(context.Background(), "the-game-id", ExecutionSimulation)

	if len(submitter.inputs) == 0 {
		t.Fatalf("expected the runner to submit through the ActionSubmitter")
	}
	in := submitter.inputs[0]
	if in.GameID != "the-game-id" || in.PlayerUserID != "bot1" || in.Action != ActionPlaceReinforcement || in.Territory != "Alaska" || in.Armies != 3 {
		t.Fatalf("unexpected GameActionInput: %+v", in)
	}
}

func TestRunnerRetriesBoundedOnRejection(t *testing.T) {
	g := botGame("bot1", risk.ControllerBot)
	loader := &fakeLoader{states: []loadedState{{game: g, status: "in_progress"}}}
	submitter := &fakeSubmitter{respFn: func(call int, in game.GameActionInput) (game.GameActionUpdate, error) {
		return game.GameActionUpdate{}, errors.New("always rejected")
	}}
	strat := &fakeStrategy{cmd: Command{Action: ActionEndTurn}}
	r := NewRunner(loader, submitter, StrategyRegistry{StrategyBasicV1: strat}, &fakeSleeper{}, DefaultPacingConfig())

	reason, err := r.RunTurn(context.Background(), "g1", ExecutionSimulation)
	if reason != StopMaxRetriesExceeded {
		t.Fatalf("expected StopMaxRetriesExceeded, got %s (err=%v)", reason, err)
	}
	if len(submitter.inputs) != maxRejectedCommandRetries+1 {
		t.Fatalf("expected exactly %d attempts, got %d", maxRejectedCommandRetries+1, len(submitter.inputs))
	}
}

func TestRunnerStrategyErrorStops(t *testing.T) {
	g := botGame("bot1", risk.ControllerBot)
	loader := &fakeLoader{states: []loadedState{{game: g, status: "in_progress"}}}
	submitter := &fakeSubmitter{}
	strat := &fakeStrategy{err: errors.New("no legal move")}
	r := NewRunner(loader, submitter, StrategyRegistry{StrategyBasicV1: strat}, &fakeSleeper{}, DefaultPacingConfig())

	reason, err := r.RunTurn(context.Background(), "g1", ExecutionSimulation)
	if reason != StopStrategyError || err == nil {
		t.Fatalf("expected StopStrategyError with an error, got reason=%s err=%v", reason, err)
	}
	if len(submitter.inputs) != 0 {
		t.Fatalf("expected no command submitted after a strategy error")
	}
}

func TestRunnerRespondsToCancellation(t *testing.T) {
	g := botGame("bot1", risk.ControllerBot)
	loader := &fakeLoader{states: []loadedState{{game: g, status: "in_progress"}}}
	submitter := &fakeSubmitter{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	strat := &fakeStrategy{cmd: Command{Action: ActionEndTurn}}
	r := NewRunner(loader, submitter, StrategyRegistry{StrategyBasicV1: strat}, &fakeSleeper{}, DefaultPacingConfig())

	reason, err := r.RunTurn(ctx, "g1", ExecutionSimulation)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reason != StopCanceled {
		t.Fatalf("expected StopCanceled, got %s", reason)
	}
}

func TestRunnerLiveModeSleepsSimulationDoesNot(t *testing.T) {
	makeGame := func() *risk.Game { return botGame("bot1", risk.ControllerBot) }

	sleeperLive := &fakeSleeper{}
	loaderLive := &fakeLoader{states: []loadedState{{game: makeGame(), status: "in_progress"}}}
	submitterLive := &fakeSubmitter{respFn: func(call int, in game.GameActionInput) (game.GameActionUpdate, error) {
		return game.GameActionUpdate{}, errors.New("stop after one call")
	}}
	strat := &fakeStrategy{cmd: Command{Action: ActionEndTurn}}
	rLive := NewRunner(loaderLive, submitterLive, StrategyRegistry{StrategyBasicV1: strat}, sleeperLive, DefaultPacingConfig())
	if _, err := rLive.RunTurn(context.Background(), "g1", ExecutionLive); err == nil {
		t.Fatalf("expected the rejected command to bubble up as an error eventually")
	}
	// Exactly one sleep is expected: the turn-start pause before the first
	// command attempt. A rejected command itself must never add a sleep —
	// only a committed one does.
	if len(sleeperLive.delays) != 1 {
		t.Fatalf("expected exactly one (turn-start) sleep despite repeated rejections, got %d", len(sleeperLive.delays))
	}

	sleeperSim := &fakeSleeper{}
	loaderSim := &fakeLoader{states: []loadedState{
		{game: makeGame(), status: "in_progress"},
		{game: func() *risk.Game {
			g := makeGame()
			for i := range g.Players {
				if g.Players[i].ID == "human" {
					g.CurrentPlayer = i
				}
			}
			return g
		}(), status: "in_progress"},
	}}
	submitterSim := &fakeSubmitter{}
	rSim := NewRunner(loaderSim, submitterSim, StrategyRegistry{StrategyBasicV1: strat}, sleeperSim, DefaultPacingConfig())
	if _, err := rSim.RunTurn(context.Background(), "g1", ExecutionSimulation); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sleeperSim.delays) != 0 {
		t.Fatalf("expected simulation mode to never invoke the sleeper, got %d calls", len(sleeperSim.delays))
	}
}

func TestRunnerLiveModeSleepsAfterCommit(t *testing.T) {
	g1 := botGame("bot1", risk.ControllerBot)
	g2 := func() *risk.Game {
		g := botGame("bot1", risk.ControllerBot)
		for i := range g.Players {
			if g.Players[i].ID == "human" {
				g.CurrentPlayer = i
			}
		}
		return g
	}()
	loader := &fakeLoader{states: []loadedState{{game: g1, status: "in_progress"}, {game: g2, status: "in_progress"}}}
	var order []string
	submitter := &fakeSubmitter{respFn: func(call int, in game.GameActionInput) (game.GameActionUpdate, error) {
		order = append(order, "commit")
		return game.GameActionUpdate{Phase: string(risk.PhaseFortify)}, nil
	}}
	sleeper := &recordingOrderSleeper{order: &order}
	strat := &fakeStrategy{cmd: Command{Action: ActionEndTurn}}
	r := NewRunner(loader, submitter, StrategyRegistry{StrategyBasicV1: strat}, sleeper, DefaultPacingConfig())

	reason, err := r.RunTurn(context.Background(), "g1", ExecutionLive)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reason != StopTurnEnded {
		t.Fatalf("expected StopTurnEnded, got %s", reason)
	}
	// order[0] is the turn-start sleep, which precedes the first command by
	// design; the invariant this test actually cares about is that the
	// commit itself is never followed by anything other than a sleep (in
	// particular, never another command submitted before sleeping).
	if len(order) != 3 || order[0] != "sleep" || order[1] != "commit" || order[2] != "sleep" {
		t.Fatalf("expected turn-start sleep, then commit strictly before the post-commit sleep, got %v", order)
	}
}

type recordingOrderSleeper struct {
	order *[]string
}

func (s *recordingOrderSleeper) Sleep(_ context.Context, _ time.Duration) error {
	*s.order = append(*s.order, "sleep")
	return nil
}
