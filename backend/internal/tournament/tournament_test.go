package tournament

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
	"github.com/nmiano1111/global-conquest/backend/internal/simulation"
)

func fullRegistry() bot.StrategyRegistry {
	return bot.StrategyRegistry{
		bot.StrategyBasicV1:  bot.NewBasicStrategy(),
		bot.StrategyScoredV1: bot.NewScoredStrategy(bot.DefaultWeights),
	}
}

// --- aggregator math, exercised directly against hand-built Results (not
// full games) so the arithmetic itself is verified independently of
// whatever a real simulation happens to produce. ---

func completedResult(seed int64, playerCount int, strategies []string, winnerSeat int, turns, commands int) simulation.Result {
	r := simulation.NewResult(seed, playerCount)
	r.Completed = true
	r.Turns = turns
	r.Commands = commands
	r.Duration = time.Duration(turns) * time.Millisecond
	r.Seats = make([]simulation.SeatResult, playerCount)
	for i, sid := range strategies {
		r.Seats[i] = simulation.SeatResult{
			Seat:              i,
			PlayerID:          simulation.SeatPlayerID(i),
			StrategyID:        sid,
			Captures:          i + 1,
			EliminationsMade:  i,
			CombatLossesTaken: i,
			FinalTerritories:  10 - i,
		}
		if i == winnerSeat {
			r.Seats[i].FinishOrder = 1
		} else {
			r.Seats[i].FinishOrder = i + 2
		}
	}
	r.WinnerSeat = winnerSeat
	r.WinnerPlayerID = simulation.SeatPlayerID(winnerSeat)
	r.WinnerStrategy = strategies[winnerSeat]
	return r
}

func failedResult(seed int64, playerCount int, strategies []string, failureType simulation.FailureType) simulation.Result {
	r := simulation.NewResult(seed, playerCount)
	r.Completed = false
	r.Failure = &simulation.Failure{Type: failureType, Seed: seed}
	r.Seats = make([]simulation.SeatResult, playerCount)
	for i, sid := range strategies {
		r.Seats[i] = simulation.SeatResult{Seat: i, PlayerID: simulation.SeatPlayerID(i), StrategyID: sid}
	}
	return r
}

func TestAggregatorBasicMath(t *testing.T) {
	strategies := []string{bot.StrategyBasicV1, bot.StrategyScoredV1}
	agg := newAggregator()

	// Two completed games, scored-v1 (seat 1) wins both.
	agg.absorb(completedResult(1, 2, strategies, 1, 10, 100))
	agg.absorb(completedResult(2, 2, strategies, 1, 20, 200))
	// One failed game.
	agg.absorb(failedResult(3, 2, strategies, simulation.FailureDurationLimitReached))

	result := agg.finalize()

	if result.TotalGames != 3 {
		t.Errorf("TotalGames = %d, want 3", result.TotalGames)
	}
	if result.CompletedGames != 2 {
		t.Errorf("CompletedGames = %d, want 2", result.CompletedGames)
	}
	if result.FailedGames != 1 {
		t.Errorf("FailedGames = %d, want 1", result.FailedGames)
	}
	if result.Failures[simulation.FailureDurationLimitReached] != 1 {
		t.Errorf("Failures[duration_limit_reached] = %d, want 1", result.Failures[simulation.FailureDurationLimitReached])
	}
	if result.AvgTurns != 15 {
		t.Errorf("AvgTurns = %v, want 15 (over completed games only)", result.AvgTurns)
	}
	if result.AvgCommands != 150 {
		t.Errorf("AvgCommands = %v, want 150", result.AvgCommands)
	}

	if len(result.Strategies) != 2 {
		t.Fatalf("Strategies has %d entries, want 2", len(result.Strategies))
	}
	// Sorted by StrategyID: basic-v1 before scored-v1.
	basic, scored := result.Strategies[0], result.Strategies[1]
	if basic.StrategyID != bot.StrategyBasicV1 || scored.StrategyID != bot.StrategyScoredV1 {
		t.Fatalf("Strategies not sorted by ID: got %q, %q", basic.StrategyID, scored.StrategyID)
	}

	// basic-v1 (seat 0): appeared in all 3 games, 2 completed, 0 wins.
	if basic.Appearances != 3 {
		t.Errorf("basic-v1 Appearances = %d, want 3", basic.Appearances)
	}
	if basic.CompletedAppearances != 2 {
		t.Errorf("basic-v1 CompletedAppearances = %d, want 2", basic.CompletedAppearances)
	}
	if basic.Wins != 0 || basic.WinRate != 0 {
		t.Errorf("basic-v1 Wins/WinRate = %d/%v, want 0/0", basic.Wins, basic.WinRate)
	}
	// FinishOrder for seat 0 (loser) in both completed games is 2.
	if basic.AvgFinishOrder != 2 {
		t.Errorf("basic-v1 AvgFinishOrder = %v, want 2", basic.AvgFinishOrder)
	}

	// scored-v1 (seat 1): won both completed games it appeared in.
	if scored.Appearances != 3 || scored.CompletedAppearances != 2 {
		t.Errorf("scored-v1 Appearances/CompletedAppearances = %d/%d, want 3/2", scored.Appearances, scored.CompletedAppearances)
	}
	if scored.Wins != 2 {
		t.Errorf("scored-v1 Wins = %d, want 2", scored.Wins)
	}
	if scored.WinRate != 1 {
		t.Errorf("scored-v1 WinRate = %v, want 1", scored.WinRate)
	}
	if scored.AvgFinishOrder != 1 {
		t.Errorf("scored-v1 AvgFinishOrder = %v, want 1", scored.AvgFinishOrder)
	}
	// Captures for seat 1 is always i+1 = 2 in both completed games.
	if scored.AvgCaptures != 2 {
		t.Errorf("scored-v1 AvgCaptures = %v, want 2", scored.AvgCaptures)
	}
}

func TestAggregatorNoCompletedGamesLeavesAveragesZero(t *testing.T) {
	strategies := []string{bot.StrategyBasicV1, bot.StrategyBasicV1, bot.StrategyBasicV1}
	agg := newAggregator()
	agg.absorb(failedResult(1, 3, strategies, simulation.FailureCommandLimitReached))
	agg.absorb(failedResult(2, 3, strategies, simulation.FailureCommandLimitReached))

	result := agg.finalize()
	if result.CompletedGames != 0 {
		t.Fatalf("CompletedGames = %d, want 0", result.CompletedGames)
	}
	if result.AvgTurns != 0 || result.AvgCommands != 0 || result.AvgGameDuration != 0 {
		t.Errorf("expected zero averages with no completed games, got AvgTurns=%v AvgCommands=%v AvgGameDuration=%v",
			result.AvgTurns, result.AvgCommands, result.AvgGameDuration)
	}
	if len(result.Strategies) != 1 {
		t.Fatalf("Strategies has %d entries, want 1", len(result.Strategies))
	}
	s := result.Strategies[0]
	if s.Appearances != 6 || s.CompletedAppearances != 0 {
		t.Errorf("Appearances/CompletedAppearances = %d/%d, want 6/0", s.Appearances, s.CompletedAppearances)
	}
	if s.WinRate != 0 || s.AvgFinishOrder != 0 {
		t.Errorf("expected zero-valued rate stats with no completed appearances, got WinRate=%v AvgFinishOrder=%v", s.WinRate, s.AvgFinishOrder)
	}
	if result.Failures[simulation.FailureCommandLimitReached] != 2 {
		t.Errorf("Failures[command_limit_reached] = %d, want 2", result.Failures[simulation.FailureCommandLimitReached])
	}
}

// --- Config.Validate ---

func TestConfigValidateRejectsInvalidTournamentFields(t *testing.T) {
	registry := fullRegistry()
	base := Config{
		Strategies:  []string{bot.StrategyBasicV1, bot.StrategyBasicV1, bot.StrategyBasicV1},
		GameMode:    simulation.GameModeAutoStart,
		Limits:      simulation.DefaultLimits(),
		SeedStart:   1,
		Games:       10,
		Parallelism: 2,
	}

	cases := []struct {
		name    string
		mutate  func(c Config) Config
		wantErr bool
	}{
		{"valid base", func(c Config) Config { return c }, false},
		{"zero games", func(c Config) Config { c.Games = 0; return c }, true},
		{"negative games", func(c Config) Config { c.Games = -1; return c }, true},
		{"zero parallelism", func(c Config) Config { c.Parallelism = 0; return c }, true},
		{"unknown strategy", func(c Config) Config {
			c.Strategies = []string{"nope", bot.StrategyBasicV1, bot.StrategyBasicV1}
			return c
		}, true},
		{"too few players", func(c Config) Config { c.Strategies = []string{bot.StrategyBasicV1, bot.StrategyBasicV1}; return c }, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.mutate(base).Validate(registry)
			if tc.wantErr && err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tc.wantErr && !errors.Is(err, simulation.ErrInvalidConfig) {
				t.Errorf("expected errors.Is(err, simulation.ErrInvalidConfig), got %v", err)
			}
		})
	}
}

// --- Run: real games through the actual registry, kept small/fast. ---

func fastConfig(games, parallelism int) Config {
	limits := simulation.DefaultLimits()
	limits.MaxTurns = 300
	return Config{
		Strategies:  []string{bot.StrategyBasicV1, bot.StrategyBasicV1, bot.StrategyBasicV1},
		GameMode:    simulation.GameModeAutoStart,
		Limits:      limits,
		SeedStart:   1,
		Games:       games,
		Parallelism: parallelism,
	}
}

func TestRunRejectsInvalidConfigBeforeAnyGame(t *testing.T) {
	sim := simulation.NewSimulator(fullRegistry())
	cfg := fastConfig(5, 2)
	cfg.Games = 0

	var calls int
	agg, err := Run(context.Background(), sim, fullRegistry(), cfg, func(simulation.Result) { calls++ })
	if err == nil {
		t.Fatal("expected a validation error")
	}
	if calls != 0 {
		t.Errorf("onResult called %d times, want 0 -- no game should run on a config error", calls)
	}
	if agg.TotalGames != 0 {
		t.Errorf("TotalGames = %d, want 0", agg.TotalGames)
	}
}

func TestRunCallsOnResultExactlyGamesTimes(t *testing.T) {
	sim := simulation.NewSimulator(fullRegistry())
	cfg := fastConfig(8, 3)

	var mu sync.Mutex
	seen := make(map[int64]bool)
	agg, err := Run(context.Background(), sim, fullRegistry(), cfg, func(r simulation.Result) {
		mu.Lock()
		defer mu.Unlock()
		seen[r.Seed] = true
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(seen) != cfg.Games {
		t.Fatalf("onResult fired for %d distinct seeds, want %d", len(seen), cfg.Games)
	}
	for i := 0; i < cfg.Games; i++ {
		seed := cfg.SeedStart + int64(i)
		if !seen[seed] {
			t.Errorf("seed %d never reported to onResult", seed)
		}
	}
	if agg.TotalGames != cfg.Games {
		t.Errorf("Aggregate.TotalGames = %d, want %d", agg.TotalGames, cfg.Games)
	}
}

func TestRunIsDeterministicAndParallelismInvariant(t *testing.T) {
	sim := simulation.NewSimulator(fullRegistry())

	aggP1, err := Run(context.Background(), sim, fullRegistry(), fastConfig(6, 1), nil)
	if err != nil {
		t.Fatalf("Run (parallelism=1): %v", err)
	}
	aggP4, err := Run(context.Background(), sim, fullRegistry(), fastConfig(6, 4), nil)
	if err != nil {
		t.Fatalf("Run (parallelism=4): %v", err)
	}
	aggP1Again, err := Run(context.Background(), sim, fullRegistry(), fastConfig(6, 1), nil)
	if err != nil {
		t.Fatalf("Run (parallelism=1, rerun): %v", err)
	}

	for _, pair := range []struct {
		name string
		a, b Aggregate
	}{
		{"parallelism 1 vs 4", aggP1, aggP4},
		{"parallelism 1 vs rerun of parallelism 1", aggP1, aggP1Again},
	} {
		if pair.a.TotalGames != pair.b.TotalGames || pair.a.CompletedGames != pair.b.CompletedGames ||
			pair.a.AvgTurns != pair.b.AvgTurns || pair.a.AvgCommands != pair.b.AvgCommands {
			t.Errorf("%s: aggregates diverged: %+v vs %+v", pair.name, pair.a, pair.b)
		}
		if len(pair.a.Strategies) != len(pair.b.Strategies) {
			t.Fatalf("%s: Strategies length differs: %d vs %d", pair.name, len(pair.a.Strategies), len(pair.b.Strategies))
		}
		for i := range pair.a.Strategies {
			if pair.a.Strategies[i] != pair.b.Strategies[i] {
				t.Errorf("%s: Strategies[%d] diverged: %+v vs %+v", pair.name, i, pair.a.Strategies[i], pair.b.Strategies[i])
			}
		}
	}
}

func TestRunBucketsFailuresWithoutAbortingBatch(t *testing.T) {
	sim := simulation.NewSimulator(fullRegistry())
	cfg := fastConfig(5, 2)
	// A command limit this low guarantees every game hits it before
	// PhaseGameOver -- verifies a failing game is bucketed into
	// FailedGames/Failures rather than stopping the whole tournament.
	cfg.Limits.MaxCommands = 3

	agg, err := Run(context.Background(), sim, fullRegistry(), cfg, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if agg.TotalGames != cfg.Games {
		t.Fatalf("TotalGames = %d, want %d -- a per-game failure must not short-circuit the batch", agg.TotalGames, cfg.Games)
	}
	if agg.FailedGames != cfg.Games {
		t.Fatalf("FailedGames = %d, want %d", agg.FailedGames, cfg.Games)
	}
	if agg.CompletedGames != 0 {
		t.Fatalf("CompletedGames = %d, want 0", agg.CompletedGames)
	}
	if agg.Failures[simulation.FailureCommandLimitReached] != cfg.Games {
		t.Fatalf("Failures[command_limit_reached] = %d, want %d", agg.Failures[simulation.FailureCommandLimitReached], cfg.Games)
	}
}

func TestRunContextCancellationStopsIssuingNewGames(t *testing.T) {
	sim := simulation.NewSimulator(fullRegistry())
	cfg := fastConfig(2000, 1) // parallelism 1: cancellation mid-stream is easy to hit reliably

	ctx, cancel := context.WithCancel(context.Background())
	var count int
	agg, err := Run(ctx, sim, fullRegistry(), cfg, func(simulation.Result) {
		count++
		if count == 2 {
			cancel()
		}
	})
	if err == nil {
		t.Fatal("expected a context-cancellation error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected errors.Is(err, context.Canceled), got %v", err)
	}
	if agg.TotalGames >= cfg.Games {
		t.Fatalf("TotalGames = %d, expected fewer than %d after cancellation", agg.TotalGames, cfg.Games)
	}
}
