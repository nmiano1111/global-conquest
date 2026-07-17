// Command traindata generates a logistic-regression training set from
// headless self-play: runs many games via internal/simulation.Simulator
// at simulation.TraceDecision, and for each decision a scored strategy
// made, recovers the raw (unweighted) value of every named feature (see
// extract.go) plus whether that player went on to win the game.
//
// Deliberately not cmd/tournament: internal/tournament.Run forces every
// game's Trace to simulation.TraceNone, since a tournament only needs
// final Results, not per-decision Explanation data. This tool calls
// internal/simulation.Simulator.RunOne directly, in-process, at
// TraceDecision instead -- see project-docs/bot_player/phase_3_continuous_improvement/10_Bot_Weight_Tuning.md.
//
// Usage:
//
//	go run ./cmd/traindata --strategies basic-v1,scored-v1,scored-v1 --games 500 --output data.jsonl
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
	"github.com/nmiano1111/global-conquest/backend/internal/simulation"
)

func main() {
	completed, err := run(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if !completed {
		os.Exit(1)
	}
}

func run(args []string) (completed bool, err error) {
	fs := flag.NewFlagSet("traindata", flag.ExitOnError)
	strategies := fs.String("strategies", "", "Comma-separated strategy ID per seat, e.g. basic-v1,scored-v1,scored-v1 -- fixed for every game. At least one seat should be a scored strategy; basic-v1 produces no feature data.")
	games := fs.Int("games", 0, "Number of games to run (required)")
	seedStart := fs.Int64("seed-start", 1, "First seed used; games run with seeds seed-start..seed-start+games-1")
	parallel := fs.Int("parallel", runtime.NumCPU(), "Number of games to run concurrently")
	gameMode := fs.String("game-mode", "auto_start", "Game construction mode: auto_start|manual")
	maxTurns := fs.Int("max-turns", 0, "Override the default turn safety limit per game (0 = use the default)")
	maxCommands := fs.Int("max-commands", 0, "Override the default command safety limit per game (0 = use the default)")
	output := fs.String("output", "", "JSONL destination for the generated training rows (required)")
	if err := fs.Parse(args); err != nil {
		return false, err
	}

	if strings.TrimSpace(*strategies) == "" {
		return false, fmt.Errorf("--strategies is required")
	}
	if *games <= 0 {
		return false, fmt.Errorf("--games is required and must be positive")
	}
	if strings.TrimSpace(*output) == "" {
		return false, fmt.Errorf("--output is required")
	}

	limits := simulation.DefaultLimits()
	if *maxTurns > 0 {
		limits.MaxTurns = *maxTurns
	}
	if *maxCommands > 0 {
		limits.MaxCommands = *maxCommands
	}

	baseCfg := simulation.Config{
		Strategies: strings.Split(*strategies, ","),
		GameMode:   simulation.GameMode(*gameMode),
		Trace:      simulation.TraceDecision,
		Limits:     limits,
	}

	registry := bot.StrategyRegistry{
		bot.StrategyBasicV1:  bot.NewBasicStrategy(),
		bot.StrategyScoredV1: bot.NewScoredStrategy(bot.DefaultWeights),
	}
	if err := baseCfg.Validate(registry); err != nil {
		return false, err
	}
	sim := simulation.NewSimulator(registry)

	raw, err := newRawWriter(*output)
	if err != nil {
		return false, fmt.Errorf("open --output: %w", err)
	}
	defer raw.close()

	progress := newProgressReporter(*games)
	defer progress.done()

	type gameOutcome struct {
		rows      []trainingRow
		completed bool
	}

	seeds := make(chan int64)
	go func() {
		defer close(seeds)
		for i := 0; i < *games; i++ {
			seeds <- *seedStart + int64(i)
		}
	}()

	outcomes := make(chan gameOutcome)
	var wg sync.WaitGroup
	for i := 0; i < *parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for seed := range seeds {
				cfg := baseCfg
				cfg.Seed = seed
				result, recorder, _ := sim.RunOne(context.Background(), cfg, nil)
				winner := ""
				if result.Completed {
					winner = result.WinnerPlayerID
				}
				id := computeGameID(baseCfg.Strategies, string(baseCfg.GameMode), seed)
				rows := rowsFromEntries(seed, id, recorder.Entries(), winner)
				outcomes <- gameOutcome{rows: rows, completed: result.Completed}
			}
		}()
	}
	go func() {
		defer close(outcomes)
		wg.Wait()
	}()

	start := time.Now()
	var totalRows, completedGames, failedGames int
	var writeErr error
	for o := range outcomes {
		if o.completed {
			completedGames++
		} else {
			failedGames++
		}
		for _, row := range o.rows {
			if werr := raw.write(row); werr != nil && writeErr == nil {
				writeErr = werr
			}
		}
		totalRows += len(o.rows)
		progress.update(completedGames + failedGames)
	}
	elapsed := time.Since(start)

	if writeErr != nil {
		return false, fmt.Errorf("write --output: %w", writeErr)
	}

	fmt.Printf("wrote %d rows from %d/%d completed games (%d failed) to %s in %s\n",
		totalRows, completedGames, *games, failedGames, *output, elapsed.Round(10*time.Millisecond))

	// Individual game failures (stalemates hitting a safety limit, etc.)
	// are a normal, expected outcome, not a tool failure -- matching
	// cmd/tournament's own semantics, where FailedGames never affects its
	// exit code either. Only a config error or a write failure (both
	// already returned above) means this run didn't complete.
	return true, nil
}
