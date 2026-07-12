package bot

import (
	"context"
	"testing"
	"time"

	"backend/internal/game"
	"backend/internal/risk"
)

func TestPacingCaptureIsSignificant(t *testing.T) {
	cfg := DefaultPacingConfig()
	cmd := Command{Action: ActionAttack, To: "Kamchatka"}
	update := game.GameActionUpdate{Phase: string(risk.PhaseOccupy), Result: risk.AttackResult{Conquered: true}}
	d := cfg.classifyAction(cmd, update, false)
	if d.category != PaceSignificant {
		t.Fatalf("expected PaceSignificant for a capture, got %s", d.category)
	}
	if d.min != cfg.CaptureMin || d.max != cfg.CaptureMax {
		t.Fatalf("expected capture range, got [%v, %v]", d.min, d.max)
	}
}

func TestPacingEliminationIsDramatic(t *testing.T) {
	cfg := DefaultPacingConfig()
	cmd := Command{Action: ActionAttack, To: "Kamchatka"}
	update := game.GameActionUpdate{
		Phase:  string(risk.PhaseOccupy),
		Result: risk.AttackResult{Conquered: true, Eliminated: "victim-id"},
	}
	d := cfg.classifyAction(cmd, update, false)
	if d.category != PaceDramatic {
		t.Fatalf("expected PaceDramatic for an elimination, got %s", d.category)
	}
	if d.min != cfg.DramaticMin || d.max != cfg.DramaticMax {
		t.Fatalf("expected dramatic range, got [%v, %v]", d.min, d.max)
	}
}

func TestPacingGameCompletionIsDramatic(t *testing.T) {
	cfg := DefaultPacingConfig()
	cmd := Command{Action: ActionEndTurn}
	update := game.GameActionUpdate{Phase: string(risk.PhaseGameOver)}
	d := cfg.classifyAction(cmd, update, false)
	if d.category != PaceDramatic {
		t.Fatalf("expected PaceDramatic for game completion, got %s", d.category)
	}
	if d.min != cfg.DramaticMin || d.max != cfg.DramaticMax {
		t.Fatalf("expected dramatic range, got [%v, %v]", d.min, d.max)
	}
}

func TestPacingRepeatAttackShorterThanFirstAttack(t *testing.T) {
	cfg := DefaultPacingConfig()
	cmd := Command{Action: ActionAttack, To: "Kamchatka"}
	update := game.GameActionUpdate{Phase: string(risk.PhaseAttack), Result: risk.AttackResult{Conquered: false}}

	first := cfg.classifyAction(cmd, update, false)
	repeat := cfg.classifyAction(cmd, update, true)

	if first.min != cfg.FirstAttackMin || first.max != cfg.FirstAttackMax {
		t.Fatalf("expected first-attack range, got [%v, %v]", first.min, first.max)
	}
	if repeat.min != cfg.RepeatAttackMin || repeat.max != cfg.RepeatAttackMax {
		t.Fatalf("expected repeat-attack range, got [%v, %v]", repeat.min, repeat.max)
	}
	if repeat.max > first.max {
		t.Fatalf("expected repeated-attack pacing to be no longer than first-attack pacing: repeat.max=%v first.max=%v", repeat.max, first.max)
	}
}

func TestPacingReinforcementIsMinor(t *testing.T) {
	cfg := DefaultPacingConfig()
	d := cfg.classifyAction(Command{Action: ActionPlaceReinforcement}, game.GameActionUpdate{Phase: string(risk.PhaseReinforce)}, false)
	if d.category != PaceMinor || d.min != cfg.ReinforcementMin || d.max != cfg.ReinforcementMax {
		t.Fatalf("unexpected reinforcement pacing: %+v", d)
	}
}

func TestPacingCardTurnInIsSignificant(t *testing.T) {
	cfg := DefaultPacingConfig()
	d := cfg.classifyAction(Command{Action: ActionTradeCards}, game.GameActionUpdate{Phase: string(risk.PhaseReinforce)}, false)
	if d.category != PaceSignificant || d.min != cfg.CardTurnInMin || d.max != cfg.CardTurnInMax {
		t.Fatalf("unexpected card turn-in pacing: %+v", d)
	}
}

func TestRandomDurationWithinBounds(t *testing.T) {
	min, max := 100*time.Millisecond, 200*time.Millisecond
	for i := 0; i < 50; i++ {
		d := randomDuration(min, max)
		if d < min || d > max {
			t.Fatalf("randomDuration out of bounds: %v not in [%v, %v]", d, min, max)
		}
	}
}

// cancelingSleeper cancels its own context the first time Sleep is called,
// simulating cancellation arriving while the runner is "sleeping", then
// returns ctx.Err() like RealSleeper would.
type cancelingSleeper struct {
	cancel func()
	calls  int
}

func (s *cancelingSleeper) Sleep(ctx context.Context, _ time.Duration) error {
	s.calls++
	s.cancel()
	return ctx.Err()
}

func TestRunnerCancellationDuringSleepStopsPromptly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	g := botGame("bot1", risk.ControllerBot)
	loader := &fakeLoader{states: []loadedState{{game: g, status: "in_progress"}}}
	submitter := &fakeSubmitter{}
	strat := &fakeStrategy{cmd: Command{Action: ActionEndTurn}}
	sleeper := &cancelingSleeper{cancel: cancel}
	r := NewRunner(loader, submitter, StrategyRegistry{StrategyBasicV1: strat}, sleeper, DefaultPacingConfig())

	reason, err := r.RunTurn(ctx, "g1", ExecutionLive)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reason != StopCanceled {
		t.Fatalf("expected StopCanceled once cancellation arrives during sleep, got %s", reason)
	}
	if sleeper.calls == 0 {
		t.Fatalf("expected the sleeper to have been invoked at least once")
	}
	// The runner must not have gone on to submit a command after the
	// canceled sleep (this is the turn-start sleep, before any command).
	if len(submitter.inputs) != 0 {
		t.Fatalf("expected no command submitted after cancellation, got %d", len(submitter.inputs))
	}
}
