package main

import (
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
	"github.com/nmiano1111/global-conquest/backend/internal/simulation"
)

func testRegistry() bot.StrategyRegistry {
	return bot.StrategyRegistry{
		bot.StrategyBasicV1:  bot.NewBasicStrategy(),
		bot.StrategyScoredV1: bot.NewScoredStrategy(bot.DefaultWeights),
	}
}

func TestComputeGameIDDeterministicAndCollisionFree(t *testing.T) {
	a := computeGameID([]string{"basic-v1", "scored-v1"}, "auto_start", 1)
	aAgain := computeGameID([]string{"basic-v1", "scored-v1"}, "auto_start", 1)
	if a != aAgain {
		t.Errorf("expected the same config+seed to always produce the same GameID, got %q vs %q", a, aAgain)
	}

	differentStrategies := computeGameID([]string{"scored-v1", "scored-v1"}, "auto_start", 1)
	differentMode := computeGameID([]string{"basic-v1", "scored-v1"}, "manual", 1)
	differentSeed := computeGameID([]string{"basic-v1", "scored-v1"}, "auto_start", 2)

	for _, other := range []string{differentStrategies, differentMode, differentSeed} {
		if a == other {
			t.Errorf("expected a different strategies/game-mode/seed to produce a different GameID, both were %q", a)
		}
	}
}

func TestRunOneGameSkipsIncompleteGames(t *testing.T) {
	sim := simulation.NewSimulator(testRegistry())
	limits := simulation.DefaultLimits()
	limits.MaxCommands = 1 // guarantees the game can't complete
	cfg := simulation.Config{
		Seed:       1,
		Strategies: []string{bot.StrategyBasicV1, bot.StrategyBasicV1, bot.StrategyBasicV1},
		GameMode:   simulation.GameModeAutoStart,
		Trace:      simulation.TraceNone,
		Limits:     limits,
	}

	rows, ok := runOneGame(sim, cfg, "test-game")
	if ok {
		t.Fatal("expected ok=false for a game that hit a safety limit before completing")
	}
	if rows != nil {
		t.Fatalf("expected no rows for an incomplete game, got %d", len(rows))
	}
}

func TestRunOneGameEmitsRowsPerLivingPlayerPerBoundary(t *testing.T) {
	sim := simulation.NewSimulator(testRegistry())
	cfg := simulation.Config{
		Seed:       1,
		Strategies: []string{bot.StrategyBasicV1, bot.StrategyBasicV1, bot.StrategyBasicV1},
		GameMode:   simulation.GameModeAutoStart,
		Trace:      simulation.TraceNone,
		Limits:     simulation.DefaultLimits(),
	}

	rows, ok := runOneGame(sim, cfg, "test-game")
	if !ok {
		t.Fatal("expected this game to complete")
	}
	if len(rows) == 0 {
		t.Fatal("expected at least one row for a completed multi-turn game")
	}

	byPlayer := make(map[string]int)
	var sawWinner bool
	maxTurn := make(map[string]int)
	for _, r := range rows {
		if r.GameID != "test-game" {
			t.Errorf("expected every row's GameID to be %q, got %q", "test-game", r.GameID)
		}
		if r.Seed != 1 {
			t.Errorf("expected every row's Seed to be 1, got %d", r.Seed)
		}
		if len(r.Features) == 0 {
			t.Error("expected every row to carry a non-empty Features vector")
		}
		byPlayer[r.PlayerID]++
		if r.Turn > maxTurn[r.PlayerID] {
			maxTurn[r.PlayerID] = r.Turn
		}
		if r.Won {
			sawWinner = true
		}
	}
	if !sawWinner {
		t.Error("expected at least one row marked Won for a completed game")
	}
	if len(byPlayer) < 2 {
		t.Errorf("expected rows from at least 2 distinct players in a 3p game, got %d", len(byPlayer))
	}
}

func TestFeatureNamesPath(t *testing.T) {
	cases := map[string]string{
		"data.jsonl":        "data.featurenames.json",
		"dir/data.jsonl":    "dir/data.featurenames.json",
		"no_extension_here": "no_extension_here.featurenames.json",
	}
	for input, want := range cases {
		if got := featureNamesPath(input); got != want {
			t.Errorf("featureNamesPath(%q) = %q, want %q", input, got, want)
		}
	}
}
