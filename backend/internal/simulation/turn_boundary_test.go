package simulation

import (
	"context"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

func TestRunOneCallsOnTurnBoundaryWithIncreasingTurns(t *testing.T) {
	sim := NewSimulator(fullRegistry())
	cfg := Config{
		Seed:       1,
		Strategies: []string{bot.StrategyBasicV1, bot.StrategyBasicV1, bot.StrategyBasicV1},
		GameMode:   GameModeAutoStart,
		Trace:      TraceNone,
		Limits:     DefaultLimits(),
	}

	var boundaries []TurnBoundary
	cfg.OnTurnBoundary = func(tb TurnBoundary) {
		boundaries = append(boundaries, tb)
	}

	result, _, err := sim.RunOne(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	if len(boundaries) == 0 {
		t.Fatal("expected at least one turn boundary for a multi-turn game")
	}
	for i := 1; i < len(boundaries); i++ {
		if boundaries[i].Turn < boundaries[i-1].Turn {
			t.Fatalf("expected Turn to be non-decreasing across boundaries, got %d then %d", boundaries[i-1].Turn, boundaries[i].Turn)
		}
	}
	last := boundaries[len(boundaries)-1]
	if last.Game.Phase != risk.PhaseGameOver {
		t.Fatalf("expected the final turn boundary's Game.Phase to be game_over, got %s", last.Game.Phase)
	}
	if last.Turn != result.Turns {
		t.Fatalf("expected the final turn boundary's Turn (%d) to match result.Turns (%d)", last.Turn, result.Turns)
	}
	if last.PlayerID == "" {
		t.Error("expected the final turn boundary to report a non-empty PlayerID")
	}
}

func TestRunOneNilOnTurnBoundaryIsSafe(t *testing.T) {
	sim := NewSimulator(fullRegistry())
	cfg := Config{
		Seed:       1,
		Strategies: []string{bot.StrategyBasicV1, bot.StrategyBasicV1, bot.StrategyBasicV1},
		GameMode:   GameModeAutoStart,
		Trace:      TraceNone,
		Limits:     DefaultLimits(),
		// OnTurnBoundary deliberately left nil.
	}
	if _, _, err := sim.RunOne(context.Background(), cfg, nil); err != nil {
		t.Fatalf("RunOne: %v", err)
	}
}

func TestRunOneOnTurnBoundaryDoesNotAffectResult(t *testing.T) {
	sim := NewSimulator(fullRegistry())
	baseCfg := Config{
		Seed:       1,
		Strategies: []string{bot.StrategyBasicV1, bot.StrategyScoredV1, bot.StrategyScoredV1},
		GameMode:   GameModeAutoStart,
		Trace:      TraceNone,
		Limits:     DefaultLimits(),
	}

	withoutCallback, _, err := sim.RunOne(context.Background(), baseCfg, nil)
	if err != nil {
		t.Fatalf("RunOne (no callback): %v", err)
	}

	withCallback := baseCfg
	withCallback.OnTurnBoundary = func(TurnBoundary) {}
	result, _, err := sim.RunOne(context.Background(), withCallback, nil)
	if err != nil {
		t.Fatalf("RunOne (with callback): %v", err)
	}

	if result.Turns != withoutCallback.Turns || result.Commands != withoutCallback.Commands || result.WinnerPlayerID != withoutCallback.WinnerPlayerID {
		t.Fatalf("expected OnTurnBoundary to be a pure observer with no effect on the outcome: got %+v vs %+v", result, withoutCallback)
	}
}
