// Command tournament runs many headless, reproducible bot-vs-bot Global
// Conquest games in parallel for a fixed strategy matchup across a sweep of
// seeds, printing an aggregated summary and optionally dumping every raw
// per-game result as JSONL. Built on internal/tournament, which itself
// builds on internal/simulation.Simulator.RunOne -- see cmd/simulate for
// running exactly one game.
//
// Usage:
//
//	go run ./cmd/tournament --strategies basic-v1,scored-v1,scored-v1 --games 100
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
	"github.com/nmiano1111/global-conquest/backend/internal/simulation"
	"github.com/nmiano1111/global-conquest/backend/internal/tournament"
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
	fs := flag.NewFlagSet("tournament", flag.ExitOnError)
	strategies := fs.String("strategies", "", "Comma-separated strategy ID per seat, e.g. basic-v1,scored-v1,scored-v1 -- fixed for every game (player count is this list's length)")
	games := fs.Int("games", 0, "Number of games to run (required)")
	seedStart := fs.Int64("seed-start", 1, "First seed used; games run with seeds seed-start..seed-start+games-1")
	parallel := fs.Int("parallel", runtime.NumCPU(), "Number of games to run concurrently")
	gameMode := fs.String("game-mode", "auto_start", "Game construction mode: auto_start|random_territory")
	maxTurns := fs.Int("max-turns", 0, "Override the default turn safety limit per game (0 = use the default)")
	maxCommands := fs.Int("max-commands", 0, "Override the default command safety limit per game (0 = use the default)")
	format := fs.String("format", "text", "Aggregate output format: text|json")
	output := fs.String("output", "", "Write the aggregate summary to this file instead of stdout")
	rawOutput := fs.String("raw-output", "", "If set, write one JSON-encoded simulation.Result per line (JSONL) to this path as each game completes")
	if err := fs.Parse(args); err != nil {
		return false, err
	}

	if strings.TrimSpace(*strategies) == "" {
		return false, fmt.Errorf("--strategies is required")
	}
	if *games <= 0 {
		return false, fmt.Errorf("--games is required and must be positive")
	}
	if *format != "text" && *format != "json" {
		return false, fmt.Errorf("invalid --format %q (want text or json)", *format)
	}

	limits := simulation.DefaultLimits()
	if *maxTurns > 0 {
		limits.MaxTurns = *maxTurns
	}
	if *maxCommands > 0 {
		limits.MaxCommands = *maxCommands
	}

	cfg := tournament.Config{
		Strategies:  strings.Split(*strategies, ","),
		GameMode:    simulation.GameMode(*gameMode),
		Limits:      limits,
		SeedStart:   *seedStart,
		Games:       *games,
		Parallelism: *parallel,
	}

	registry := bot.StrategyRegistry{
		bot.StrategyBasicV1:  bot.NewBasicStrategy(),
		bot.StrategyScoredV1: bot.NewScoredStrategy(bot.DefaultWeights),
	}
	sim := simulation.NewSimulator(registry)

	var raw *rawWriter
	if *rawOutput != "" {
		raw, err = newRawWriter(*rawOutput)
		if err != nil {
			return false, fmt.Errorf("open --raw-output: %w", err)
		}
		defer raw.close()
	}

	progress := newProgressReporter(*games)
	onResult := func(result simulation.Result) {
		progress.update(result)
		if raw != nil {
			if werr := raw.write(result); werr != nil && err == nil {
				err = fmt.Errorf("write --raw-output: %w", werr)
			}
		}
	}

	start := time.Now()
	agg, runErr := tournament.Run(context.Background(), sim, registry, cfg, onResult)
	elapsed := time.Since(start)
	progress.done()
	if runErr != nil && agg.TotalGames == 0 {
		// A config-validation error -- no game ever ran.
		return false, runErr
	}
	if err != nil {
		// A raw-output write failure, surfaced after the run finishes so a
		// disk error mid-tournament doesn't also mask the aggregate.
		return false, err
	}

	w := os.Stdout
	if *output != "" {
		f, ferr := os.Create(*output)
		if ferr != nil {
			return false, fmt.Errorf("open --output: %w", ferr)
		}
		defer f.Close()
		w = f
	}

	var writeErr error
	if *format == "json" {
		writeErr = writeAggregateJSON(w, cfg, agg)
	} else {
		writeErr = writeAggregateText(w, cfg, agg, elapsed)
	}
	if writeErr != nil {
		return false, fmt.Errorf("write output: %w", writeErr)
	}

	// A tournament "completes" if every game either finished or failed
	// cleanly -- a context cancellation (runErr != nil but games did run)
	// is the only way to reach here with an incomplete batch.
	return runErr == nil, nil
}
