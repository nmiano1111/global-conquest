package main

import (
	"os"
	"path/filepath"
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

func writeTempConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestLoadBatchConfigValid(t *testing.T) {
	path := writeTempConfig(t, `{
		"tournaments": [
			{"name": "a", "strategies": ["basic-v1", "scored-v1", "scored-v1"], "games": 10},
			{"name": "b", "strategies": ["basic-v1", "basic-v1", "scored-v1"], "games": 20, "game_mode": "manual", "seed_start": 5, "parallel": 2, "max_turns": 100, "max_commands": 500, "raw_output": "b.jsonl"}
		]
	}`)

	cfg, err := loadBatchConfig(path, testRegistry())
	if err != nil {
		t.Fatalf("loadBatchConfig: %v", err)
	}
	if len(cfg.Tournaments) != 2 {
		t.Fatalf("expected 2 tournaments, got %d", len(cfg.Tournaments))
	}

	a := cfg.Tournaments[0]
	if a.GameMode != string(simulation.GameModeAutoStart) {
		t.Errorf("entry a: GameMode = %q, want default %q", a.GameMode, simulation.GameModeAutoStart)
	}
	if a.SeedStart != 1 {
		t.Errorf("entry a: SeedStart = %d, want default 1", a.SeedStart)
	}
	if a.Parallel <= 0 {
		t.Errorf("entry a: Parallel = %d, want a positive default", a.Parallel)
	}

	b := cfg.Tournaments[1]
	if b.GameMode != "manual" || b.SeedStart != 5 || b.Parallel != 2 {
		t.Errorf("entry b: explicit fields not preserved, got %+v", b)
	}
	if b.RawOutput != "b.jsonl" {
		t.Errorf("entry b: RawOutput = %q, want %q", b.RawOutput, "b.jsonl")
	}
}

func TestLoadBatchConfigRejectsEmptyTournamentsList(t *testing.T) {
	path := writeTempConfig(t, `{"tournaments": []}`)
	if _, err := loadBatchConfig(path, testRegistry()); err == nil {
		t.Fatal("expected an error for an empty tournaments list")
	}
}

func TestLoadBatchConfigRejectsMissingName(t *testing.T) {
	path := writeTempConfig(t, `{
		"tournaments": [
			{"strategies": ["basic-v1", "basic-v1", "basic-v1"], "games": 10}
		]
	}`)
	if _, err := loadBatchConfig(path, testRegistry()); err == nil {
		t.Fatal("expected an error for a missing name")
	}
}

func TestLoadBatchConfigRejectsDuplicateNames(t *testing.T) {
	path := writeTempConfig(t, `{
		"tournaments": [
			{"name": "dup", "strategies": ["basic-v1", "basic-v1", "basic-v1"], "games": 10},
			{"name": "dup", "strategies": ["scored-v1", "scored-v1", "scored-v1"], "games": 10}
		]
	}`)
	if _, err := loadBatchConfig(path, testRegistry()); err == nil {
		t.Fatal("expected an error for duplicate tournament names")
	}
}

func TestLoadBatchConfigRejectsInvalidEntry(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{"unknown strategy", `{"tournaments": [{"name": "x", "strategies": ["not-a-real-strategy", "basic-v1", "basic-v1"], "games": 10}]}`},
		{"too few players", `{"tournaments": [{"name": "x", "strategies": ["basic-v1", "basic-v1"], "games": 10}]}`},
		{"zero games", `{"tournaments": [{"name": "x", "strategies": ["basic-v1", "basic-v1", "basic-v1"], "games": 0}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeTempConfig(t, tc.json)
			if _, err := loadBatchConfig(path, testRegistry()); err == nil {
				t.Fatalf("expected an error for %s", tc.name)
			}
		})
	}
}

func TestLoadBatchConfigRejectsMissingFile(t *testing.T) {
	if _, err := loadBatchConfig(filepath.Join(t.TempDir(), "nope.json"), testRegistry()); err == nil {
		t.Fatal("expected an error for a missing config file")
	}
}

func TestLoadBatchConfigRejectsInvalidJSON(t *testing.T) {
	path := writeTempConfig(t, `{not valid json`)
	if _, err := loadBatchConfig(path, testRegistry()); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}
