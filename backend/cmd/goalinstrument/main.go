// Command goalinstrument measures GoalKillbotStrategy's (goal-killbot-v1)
// persistent CompleteContinentGoal funnel: how often a goal gets
// discovered/committed, how it resolves (completed/abandoned/still
// active), and -- the single most important number -- what fraction of
// its committed-goal decisions actually change the chosen command versus
// plain killbot-v2 at the same, unmutated game state. See
// project-docs/bot_player/phase_3_continuous_improvement/
// 12_Symbolic_Strategic_Reasoning_Layer.md for the full design and the
// Stage 1 -> Stage 2 gate this tool's output is measured against.
//
// Needs no new internal/simulation code: goal lifecycle is read straight
// off risk.Game.StrategicGoals via the existing, already-proven
// simulation.Config.OnTurnBoundary hook (the same one cmd/tdtraindata
// already uses), and decision-level changed/unchanged comes from
// GoalKillbotStrategy's own WithDecisionObserver.
//
// Usage:
//
//	go run ./cmd/goalinstrument --games 300 --output funnel.jsonl
//
// goal-killbot-v1's seat is fixed at seat 0 against a shared 5-strategy
// opponent field (see opponentStrategies in extract.go); --strategies is
// not exposed as a flag since the seat-detection logic in runOneGame
// needs to know exactly where goal-killbot-v1 sits.
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
	fs := flag.NewFlagSet("goalinstrument", flag.ExitOnError)
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
		bot.StrategyGoalKillbotV1, bot.StrategyKillbotV2, bot.StrategyBetterPixieV1,
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

	summaries := make(chan gameSummary)
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
				gameID := fmt.Sprintf("goalinstrument@%d", seed)
				summaries <- runOneGame(shared, strategies, cfg, gameID)
			}
		}()
	}
	go func() {
		defer close(summaries)
		wg.Wait()
	}()

	start := time.Now()
	var all []gameSummary
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
