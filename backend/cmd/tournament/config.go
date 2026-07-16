package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
	"github.com/nmiano1111/global-conquest/backend/internal/simulation"
	"github.com/nmiano1111/global-conquest/backend/internal/tournament"
)

// BatchConfig is the --config file's top-level shape: a named list of
// tournaments to run concurrently in one process.
type BatchConfig struct {
	Tournaments []TournamentEntry `json:"tournaments"`
}

// TournamentEntry mirrors cmd/tournament's own direct flags, one entry per
// tournament to run -- see main.go's flag descriptions for what each field
// means. Name must be non-empty and unique within the file: it labels this
// entry's progress bar and its section of the printed output.
//
// Zero-value fields mean "use the same default the direct flags use":
// GameMode defaults to auto_start, SeedStart to 1, Parallel to
// runtime.NumCPU(), MaxTurns/MaxCommands to simulation.DefaultLimits'
// values. One caveat: SeedStart 0 is technically a legal literal seed
// start via the direct --seed-start flag, but is indistinguishable here
// from "omitted" -- an explicit seed_start of exactly 0 in a config file
// is silently treated as "unset" and defaults to 1. Use --seed-start 0
// via the single-tournament flag mode if that specific value is needed.
type TournamentEntry struct {
	Name        string   `json:"name"`
	Strategies  []string `json:"strategies"`
	GameMode    string   `json:"game_mode"`
	Games       int      `json:"games"`
	SeedStart   int64    `json:"seed_start"`
	Parallel    int      `json:"parallel"`
	MaxTurns    int      `json:"max_turns"`
	MaxCommands int      `json:"max_commands"`
	RawOutput   string   `json:"raw_output"`
}

// loadBatchConfig reads and validates path: applies TournamentEntry's
// defaults to every entry, then validates each one against registry via
// tournament.Config.Validate before returning. One invalid entry fails
// the whole batch up front -- matching tournament.Run's own "config
// errors caught before any game starts" contract, extended here to catch
// it before any tournament in the batch starts either.
func loadBatchConfig(path string, registry bot.StrategyRegistry) (BatchConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return BatchConfig{}, fmt.Errorf("read --config: %w", err)
	}
	var cfg BatchConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return BatchConfig{}, fmt.Errorf("parse --config: %w", err)
	}
	if len(cfg.Tournaments) == 0 {
		return BatchConfig{}, fmt.Errorf("--config: no tournaments listed")
	}

	seen := make(map[string]bool, len(cfg.Tournaments))
	for i := range cfg.Tournaments {
		e := &cfg.Tournaments[i]
		if e.Name == "" {
			return BatchConfig{}, fmt.Errorf("--config: tournaments[%d]: name is required", i)
		}
		if seen[e.Name] {
			return BatchConfig{}, fmt.Errorf("--config: duplicate tournament name %q", e.Name)
		}
		seen[e.Name] = true

		if e.GameMode == "" {
			e.GameMode = string(simulation.GameModeAutoStart)
		}
		if e.SeedStart == 0 {
			e.SeedStart = 1
		}
		if e.Parallel == 0 {
			e.Parallel = runtime.NumCPU()
		}

		if _, err := e.tournamentConfig(registry); err != nil {
			return BatchConfig{}, fmt.Errorf("--config: tournaments[%d] (%q): %w", i, e.Name, err)
		}
	}
	return cfg, nil
}

// tournamentConfig converts one entry into a tournament.Config -- same
// --max-turns/--max-commands override semantics as the direct flags (0 =
// use simulation.DefaultLimits' value) -- and validates it against
// registry.
func (e TournamentEntry) tournamentConfig(registry bot.StrategyRegistry) (tournament.Config, error) {
	limits := simulation.DefaultLimits()
	if e.MaxTurns > 0 {
		limits.MaxTurns = e.MaxTurns
	}
	if e.MaxCommands > 0 {
		limits.MaxCommands = e.MaxCommands
	}
	cfg := tournament.Config{
		Strategies:  e.Strategies,
		GameMode:    simulation.GameMode(e.GameMode),
		Limits:      limits,
		SeedStart:   e.SeedStart,
		Games:       e.Games,
		Parallelism: e.Parallel,
	}
	if err := cfg.Validate(registry); err != nil {
		return tournament.Config{}, err
	}
	return cfg, nil
}
