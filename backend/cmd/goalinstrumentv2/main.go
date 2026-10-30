// Command goalinstrumentv2 measures GoalKillbotV2Strategy's
// (goal-killbot-v2) full desirable/feasible/committed/securing lifecycle
// funnel -- see project-docs/bot_player/phase_3_continuous_improvement/
// 12_Symbolic_Strategic_Reasoning_Layer.md for the full design. Modeled
// directly on cmd/goalinstrument's structure (extended, not replaced --
// goal-killbot-v1's own tool stays as-is for its own baseline
// measurement).
//
// Needs no new internal/simulation code: goal lifecycle is read straight
// off risk.Game.StrategicGoals via the existing simulation.Config.OnTurnBoundary
// hook, and decision-level changed/unchanged comes from
// GoalKillbotV2Strategy's own WithDecisionObserverV2.
//
// Usage:
//
//	go run ./cmd/goalinstrumentv2 --games 300 --output funnel_v2.jsonl
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
	"github.com/nmiano1111/global-conquest/backend/internal/simulation"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("goalinstrumentv2", flag.ExitOnError)
	games := fs.Int("games", 0, "Number of games to run (required)")
	seedStart := fs.Int64("seed-start", 1, "First seed used; games run with seeds seed-start..seed-start+games-1")
	parallel := fs.Int("parallel", runtime.NumCPU(), "Number of games to run concurrently")
	gameMode := fs.String("game-mode", "auto_start", "Game construction mode: auto_start|manual")
	output := fs.String("output", "", "JSONL destination for per-game funnel summaries (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *games <= 0 {
		return fmt.Errorf("--games is required and must be positive")
	}
	if *output == "" {
		return fmt.Errorf("--output is required")
	}

	strategies := []string{
		bot.StrategyGoalKillbotV2, bot.StrategyKillbotV2, bot.StrategyBetterPixieV1,
		bot.StrategyQuoV1, bot.StrategyBoscoeV1, bot.StrategyPixieV1,
	}
	shared := opponentStrategies()

	f, err := os.Create(*output)
	if err != nil {
		return fmt.Errorf("open --output: %w", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)

	seeds := make(chan int64)
	go func() {
		defer close(seeds)
		for i := 0; i < *games; i++ {
			seeds <- *seedStart + int64(i)
		}
	}()

	summaries := make(chan gameSummaryV2)
	var wg sync.WaitGroup
	for i := 0; i < *parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for seed := range seeds {
				cfg := simulation.Config{
					Strategies: strategies,
					GameMode:   simulation.GameMode(*gameMode),
					Trace:      simulation.TraceNone,
					Limits:     simulation.DefaultLimits(),
					Seed:       seed,
				}
				gameID := fmt.Sprintf("goalinstrumentv2@%d", seed)
				summaries <- runOneGame(shared, strategies, cfg, gameID)
			}
		}()
	}
	go func() {
		defer close(summaries)
		wg.Wait()
	}()

	start := time.Now()
	var all []gameSummaryV2
	var writeErr error
	for s := range summaries {
		if werr := enc.Encode(s); werr != nil && writeErr == nil {
			writeErr = werr
		}
		all = append(all, s)
	}
	if writeErr != nil {
		return fmt.Errorf("write --output: %w", writeErr)
	}

	printReport(all, time.Since(start))
	return nil
}
