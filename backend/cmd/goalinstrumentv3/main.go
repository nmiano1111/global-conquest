// Command goalinstrumentv3 measures SymbolicDirectorStrategy's
// (director-v1) full desirable/feasible/committed/securing lifecycle
// funnel, plus an abandonment-cause breakdown (DirectorAbandonCause) --
// see project-docs/bot_player/phase_3_continuous_improvement/
// 12_Symbolic_Strategic_Reasoning_Layer.md for the full design. Modeled
// directly on cmd/goalinstrumentv2's structure (extended, not replaced --
// goal-killbot-v2's own tool stays as-is for its own baseline
// measurement).
//
// The abandonment-cause breakdown is this tool's most important
// addition over v2: it answers directly whether unioning the goal
// continent into DirectedPixieStrategy's own `wanted` set (rather than
// goal-killbot-v2's narrow, after-the-fact phase patches) reduces the
// dominant fact_gone abandonment cause goal-killbot-v2's investigation
// found (66.9% of abandonments -- pi losing a SECOND territory in the
// goal's continent while reinforcement concentrated on one staging
// territory).
//
// Usage:
//
//	go run ./cmd/goalinstrumentv3 --games 300 --output funnel_v3.jsonl
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
	fs := flag.NewFlagSet("goalinstrumentv3", flag.ExitOnError)
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
		bot.StrategyDirectorV1, bot.StrategyKillbotV2, bot.StrategyBetterPixieV1,
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

	summaries := make(chan gameSummaryV3)
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
				gameID := fmt.Sprintf("goalinstrumentv3@%d", seed)
				summaries <- runOneGame(shared, strategies, cfg, gameID)
			}
		}()
	}
	go func() {
		defer close(summaries)
		wg.Wait()
	}()

	start := time.Now()
	var all []gameSummaryV3
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
