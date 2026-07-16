// Command simulate runs one headless, reproducible bot-vs-bot Global
// Conquest game and reports the result — no Postgres, no WebSocket, no
// Discord, no HTTP, no live pacing. See internal/simulation and
// project-docs/bot_player/phase_2_first_playable_bot/05_Simulation_Framework.md.
//
// Usage:
//
//	go run ./cmd/simulate --seed 12345 --strategies basic-v1,scored-v1,scored-v1 --trace summary
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

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
	fs := flag.NewFlagSet("simulate", flag.ExitOnError)
	seed := fs.Int64("seed", 0, "Random seed (required) -- the same seed, strategies, and game mode always produce the same game")
	strategies := fs.String("strategies", "", "Comma-separated strategy ID per seat, e.g. basic-v1,scored-v1,scored-v1 (player count is this list's length)")
	gameMode := fs.String("game-mode", "auto_start", "Game construction mode: auto_start|manual")
	trace := fs.String("trace", "summary", "Trace level: none|summary|decision|full")
	maxTurns := fs.Int("max-turns", 0, "Override the default turn safety limit (0 = use the default)")
	maxCommands := fs.Int("max-commands", 0, "Override the default command safety limit (0 = use the default)")
	format := fs.String("format", "text", "Output format: text|json")
	output := fs.String("output", "", "Write output to this file instead of stdout")
	if err := fs.Parse(args); err != nil {
		return false, err
	}

	seedSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "seed" {
			seedSet = true
		}
	})
	if !seedSet {
		return false, fmt.Errorf("--seed is required")
	}
	if strings.TrimSpace(*strategies) == "" {
		return false, fmt.Errorf("--strategies is required")
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

	cfg := simulation.Config{
		Seed:       *seed,
		Strategies: strings.Split(*strategies, ","),
		GameMode:   simulation.GameMode(*gameMode),
		Trace:      simulation.TraceLevel(*trace),
		Limits:     limits,
	}

	registry := bot.StrategyRegistry{
		bot.StrategyBasicV1:  bot.NewBasicStrategy(),
		bot.StrategyScoredV1: bot.NewScoredStrategy(bot.DefaultWeights),
	}
	sim := simulation.NewSimulator(registry)

	progress := newProgressReporter()
	result, recorder, runErr := sim.RunOne(context.Background(), cfg, progress.update)
	progress.done()
	if runErr != nil && result.Failure == nil {
		// A config-validation or game-construction error -- no game was
		// ever built, so there's no partial Result worth reporting.
		return false, runErr
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
		writeErr = writeJSON(w, result, recorder)
	} else {
		writeErr = writeText(w, result)
	}
	if writeErr != nil {
		return false, fmt.Errorf("write output: %w", writeErr)
	}

	return result.Completed, nil
}
