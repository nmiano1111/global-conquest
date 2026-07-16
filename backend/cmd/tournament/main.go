// Command tournament runs many headless, reproducible bot-vs-bot Global
// Conquest games in parallel for a fixed strategy matchup across a sweep of
// seeds, printing an aggregated summary and optionally dumping every raw
// per-game result as JSONL. Built on internal/tournament, which itself
// builds on internal/simulation.Simulator.RunOne -- see cmd/simulate for
// running exactly one game.
//
// --config <path> runs several tournaments concurrently from one JSON
// file instead of a single one from flags -- see config.go.
//
// Usage:
//
//	go run ./cmd/tournament --strategies basic-v1,scored-v1,scored-v1 --games 100
//	go run ./cmd/tournament --config batch.json
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
	configPath := fs.String("config", "", "Path to a JSON batch config running several tournaments concurrently -- mutually exclusive with the flags below")
	strategies := fs.String("strategies", "", "Comma-separated strategy ID per seat, e.g. basic-v1,scored-v1,scored-v1 -- fixed for every game (player count is this list's length)")
	games := fs.Int("games", 0, "Number of games to run (required)")
	seedStart := fs.Int64("seed-start", 1, "First seed used; games run with seeds seed-start..seed-start+games-1")
	parallel := fs.Int("parallel", runtime.NumCPU(), "Number of games to run concurrently")
	gameMode := fs.String("game-mode", "auto_start", "Game construction mode: auto_start|manual")
	maxTurns := fs.Int("max-turns", 0, "Override the default turn safety limit per game (0 = use the default)")
	maxCommands := fs.Int("max-commands", 0, "Override the default command safety limit per game (0 = use the default)")
	format := fs.String("format", "text", "Aggregate output format: text|json")
	output := fs.String("output", "", "Write the aggregate summary to this file instead of stdout")
	rawOutput := fs.String("raw-output", "", "If set, write one JSON-encoded simulation.Result per line (JSONL) to this path as each game completes")
	if err := fs.Parse(args); err != nil {
		return false, err
	}

	directFlags := map[string]bool{
		"strategies": true, "games": true, "seed-start": true, "parallel": true,
		"game-mode": true, "max-turns": true, "max-commands": true, "raw-output": true,
	}
	var conflicting []string
	fs.Visit(func(f *flag.Flag) {
		if directFlags[f.Name] {
			conflicting = append(conflicting, "--"+f.Name)
		}
	})

	if *format != "text" && *format != "json" {
		return false, fmt.Errorf("invalid --format %q (want text or json)", *format)
	}

	registry := bot.StrategyRegistry{
		bot.StrategyBasicV1:  bot.NewBasicStrategy(),
		bot.StrategyScoredV1: bot.NewScoredStrategy(bot.DefaultWeights),
	}
	sim := simulation.NewSimulator(registry)

	if *configPath != "" {
		if len(conflicting) > 0 {
			return false, fmt.Errorf("--config is mutually exclusive with %s", strings.Join(conflicting, ", "))
		}
		return runBatch(*configPath, sim, registry, *format, *output)
	}

	if strings.TrimSpace(*strategies) == "" {
		return false, fmt.Errorf("--strategies is required")
	}
	if *games <= 0 {
		return false, fmt.Errorf("--games is required and must be positive")
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

	var raw *rawWriter
	if *rawOutput != "" {
		raw, err = newRawWriter(*rawOutput)
		if err != nil {
			return false, fmt.Errorf("open --raw-output: %w", err)
		}
		defer raw.close()
	}

	progress := newProgressReporter()
	bar := progress.newBar("tournament", *games)
	onResult := func(result simulation.Result) {
		bar.update(result)
		if raw != nil {
			if werr := raw.write(result); werr != nil && err == nil {
				err = fmt.Errorf("write --raw-output: %w", werr)
			}
		}
	}

	start := time.Now()
	agg, runErr := tournament.Run(context.Background(), sim, registry, cfg, onResult)
	elapsed := time.Since(start)
	bar.done()
	progress.wait()
	if runErr != nil && agg.TotalGames == 0 {
		// A config-validation error -- no game was ever run.
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

// batchResult holds one --config entry's outcome, collected after every
// tournament goroutine below has finished.
type batchResult struct {
	name    string
	cfg     tournament.Config
	agg     tournament.Aggregate
	elapsed time.Duration
	// err is non-nil only for a context-cancellation-level failure or a
	// raw-output write error -- individual game failures within a
	// tournament are already reflected in agg.FailedGames, not here,
	// matching tournament.Run's own error contract.
	err error
}

// runBatch loads path, then runs every tournament it lists concurrently,
// each in its own goroutine with its own progress bar (see progress.go)
// and its own raw-output file. All tournaments' configs are validated up
// front by loadBatchConfig -- nothing here should fail validation again,
// that's a defensive re-check, not the primary gate.
func runBatch(path string, sim *simulation.Simulator, registry bot.StrategyRegistry, format, outputPath string) (completed bool, err error) {
	batch, err := loadBatchConfig(path, registry)
	if err != nil {
		return false, err
	}

	progress := newProgressReporter()
	results := make([]batchResult, len(batch.Tournaments))
	var wg sync.WaitGroup

	for i, entry := range batch.Tournaments {
		cfg, verr := entry.tournamentConfig(registry)
		if verr != nil {
			// Unreachable: loadBatchConfig already validated every entry.
			return false, fmt.Errorf("tournaments[%d] (%q): %w", i, entry.Name, verr)
		}

		var raw *rawWriter
		if entry.RawOutput != "" {
			raw, err = newRawWriter(entry.RawOutput)
			if err != nil {
				return false, fmt.Errorf("open raw_output for %q: %w", entry.Name, err)
			}
		}

		bar := progress.newBar(entry.Name, entry.Games)

		wg.Add(1)
		go func(i int, name string, cfg tournament.Config, raw *rawWriter, bar *tournamentBar) {
			defer wg.Done()
			if raw != nil {
				defer raw.close()
			}
			var rawErr error
			onResult := func(result simulation.Result) {
				bar.update(result)
				if raw != nil {
					if werr := raw.write(result); werr != nil && rawErr == nil {
						rawErr = werr
					}
				}
			}

			start := time.Now()
			agg, runErr := tournament.Run(context.Background(), sim, registry, cfg, onResult)
			elapsed := time.Since(start)
			bar.done()
			if runErr == nil {
				runErr = rawErr
			}
			results[i] = batchResult{name: name, cfg: cfg, agg: agg, elapsed: elapsed, err: runErr}
		}(i, entry.Name, cfg, raw, bar)
	}

	wg.Wait()
	progress.wait()

	w := os.Stdout
	if outputPath != "" {
		f, ferr := os.Create(outputPath)
		if ferr != nil {
			return false, fmt.Errorf("open --output: %w", ferr)
		}
		defer f.Close()
		w = f
	}

	allCompleted := true
	for _, r := range results {
		if r.err != nil {
			allCompleted = false
		}
	}

	var writeErr error
	if format == "json" {
		writeErr = writeBatchJSON(w, results)
	} else {
		writeErr = writeBatchText(w, results)
	}
	if writeErr != nil {
		return false, fmt.Errorf("write output: %w", writeErr)
	}

	return allCompleted, nil
}
