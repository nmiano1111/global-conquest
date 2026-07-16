package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
)

func TestWeightsVariantFlagSetParsesIDAndPath(t *testing.T) {
	var f weightsVariantFlag
	if err := f.Set("candidate-a=weights.json"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if len(f) != 1 || f[0].StrategyID != "candidate-a" || f[0].Path != "weights.json" {
		t.Fatalf("unexpected parse result: %+v", f)
	}
}

func TestWeightsVariantFlagSetAccumulates(t *testing.T) {
	var f weightsVariantFlag
	if err := f.Set("a=1.json"); err != nil {
		t.Fatalf("Set a: %v", err)
	}
	if err := f.Set("b=2.json"); err != nil {
		t.Fatalf("Set b: %v", err)
	}
	if len(f) != 2 {
		t.Fatalf("expected 2 accumulated entries, got %d", len(f))
	}
}

func TestWeightsVariantFlagSetRejectsMalformed(t *testing.T) {
	cases := []string{"", "no-equals-sign", "=missing-id.json", "missing-path="}
	for _, c := range cases {
		var f weightsVariantFlag
		if err := f.Set(c); err == nil {
			t.Errorf("Set(%q): expected an error", c)
		}
	}
}

func TestRegisterWeightsVariantsAddsCustomStrategy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "candidate.json")
	if err := os.WriteFile(path, []byte(`{"ArmyAdvantage": 4.2}`), 0o644); err != nil {
		t.Fatalf("write temp weights file: %v", err)
	}

	registry := bot.StrategyRegistry{
		bot.StrategyBasicV1:  bot.NewBasicStrategy(),
		bot.StrategyScoredV1: bot.NewScoredStrategy(bot.DefaultWeights),
	}
	variants := weightsVariantFlag{{StrategyID: "scored-v1-candidate", Path: path}}

	if err := registerWeightsVariants(registry, variants); err != nil {
		t.Fatalf("registerWeightsVariants: %v", err)
	}
	if _, ok := registry.Get("scored-v1-candidate"); !ok {
		t.Fatal("expected scored-v1-candidate to be registered")
	}
}

func TestRegisterWeightsVariantsRejectsBuiltinCollision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "candidate.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write temp weights file: %v", err)
	}

	registry := bot.StrategyRegistry{
		bot.StrategyBasicV1:  bot.NewBasicStrategy(),
		bot.StrategyScoredV1: bot.NewScoredStrategy(bot.DefaultWeights),
	}
	variants := weightsVariantFlag{{StrategyID: bot.StrategyScoredV1, Path: path}}

	if err := registerWeightsVariants(registry, variants); err == nil {
		t.Fatal("expected an error when a --weights-variant ID collides with a built-in strategy")
	}
}

func TestRegisterWeightsVariantsRejectsDuplicateVariantID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "candidate.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write temp weights file: %v", err)
	}

	registry := bot.StrategyRegistry{
		bot.StrategyBasicV1:  bot.NewBasicStrategy(),
		bot.StrategyScoredV1: bot.NewScoredStrategy(bot.DefaultWeights),
	}
	variants := weightsVariantFlag{
		{StrategyID: "candidate", Path: path},
		{StrategyID: "candidate", Path: path},
	}

	if err := registerWeightsVariants(registry, variants); err == nil {
		t.Fatal("expected an error for a duplicate --weights-variant strategy ID")
	}
}

func TestRegisterWeightsVariantsPropagatesLoadError(t *testing.T) {
	registry := bot.StrategyRegistry{
		bot.StrategyBasicV1: bot.NewBasicStrategy(),
	}
	variants := weightsVariantFlag{{StrategyID: "candidate", Path: filepath.Join(t.TempDir(), "nope.json")}}

	if err := registerWeightsVariants(registry, variants); err == nil {
		t.Fatal("expected an error when the weights file doesn't exist")
	}
}
