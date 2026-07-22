package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
)

func TestBoardValueVariantFlagSetParsesIDAndPath(t *testing.T) {
	var f boardValueVariantFlag
	if err := f.Set("candidate-a=weights.json"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if len(f) != 1 || f[0].StrategyID != "candidate-a" || f[0].WeightsPath != "weights.json" {
		t.Fatalf("unexpected parse result: %+v", f)
	}
}

func TestBoardValueVariantFlagSetAccumulates(t *testing.T) {
	var f boardValueVariantFlag
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

func TestBoardValueVariantFlagSetRejectsMalformed(t *testing.T) {
	cases := []string{"", "no-equals-sign", "=missing-id.json", "missing-path="}
	for _, c := range cases {
		var f boardValueVariantFlag
		if err := f.Set(c); err == nil {
			t.Errorf("Set(%q): expected an error", c)
		}
	}
}

// writeMinimalBoardValueFile writes a trivial single-weight board value
// JSON file, matching bot.LoadBoardValue's expected shape.
func writeMinimalBoardValueFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "value.json")
	data, err := json.Marshal(map[string]any{
		"weights":       []float64{0.0},
		"intercept":     0.0,
		"mean":          []float64{0.0},
		"std":           []float64{1.0},
		"feature_names": []string{"x"},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write board value file: %v", err)
	}
	return path
}

func TestRegisterBoardValueVariantsAddsCustomStrategy(t *testing.T) {
	path := writeMinimalBoardValueFile(t)

	registry := bot.StrategyRegistry{
		bot.StrategyBasicV1:  bot.NewBasicStrategy(),
		bot.StrategyScoredV1: bot.NewScoredStrategy(bot.DefaultWeights),
	}
	variants := boardValueVariantFlag{{StrategyID: "board-value-candidate", WeightsPath: path}}

	if err := registerBoardValueVariants(registry, variants); err != nil {
		t.Fatalf("registerBoardValueVariants: %v", err)
	}
	if _, ok := registry.Get("board-value-candidate"); !ok {
		t.Fatal("expected board-value-candidate to be registered")
	}
}

func TestRegisterBoardValueVariantsRejectsBuiltinCollision(t *testing.T) {
	path := writeMinimalBoardValueFile(t)

	registry := bot.StrategyRegistry{
		bot.StrategyBasicV1:  bot.NewBasicStrategy(),
		bot.StrategyScoredV1: bot.NewScoredStrategy(bot.DefaultWeights),
	}
	variants := boardValueVariantFlag{{StrategyID: bot.StrategyScoredV1, WeightsPath: path}}

	if err := registerBoardValueVariants(registry, variants); err == nil {
		t.Fatal("expected an error when a --board-value-variant ID collides with a built-in strategy")
	}
}

func TestRegisterBoardValueVariantsRejectsDuplicateVariantID(t *testing.T) {
	path := writeMinimalBoardValueFile(t)

	registry := bot.StrategyRegistry{
		bot.StrategyBasicV1:  bot.NewBasicStrategy(),
		bot.StrategyScoredV1: bot.NewScoredStrategy(bot.DefaultWeights),
	}
	variants := boardValueVariantFlag{
		{StrategyID: "candidate", WeightsPath: path},
		{StrategyID: "candidate", WeightsPath: path},
	}

	if err := registerBoardValueVariants(registry, variants); err == nil {
		t.Fatal("expected an error for a duplicate --board-value-variant strategy ID")
	}
}

func TestRegisterBoardValueVariantsPropagatesLoadError(t *testing.T) {
	registry := bot.StrategyRegistry{
		bot.StrategyBasicV1: bot.NewBasicStrategy(),
	}
	variants := boardValueVariantFlag{{StrategyID: "candidate", WeightsPath: filepath.Join(t.TempDir(), "nope.json")}}

	if err := registerBoardValueVariants(registry, variants); err == nil {
		t.Fatal("expected an error when the weights file doesn't exist")
	}
}
