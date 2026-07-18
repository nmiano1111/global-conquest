package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
)

func TestGBTVariantFlagSetParsesIDAndDir(t *testing.T) {
	var f gbtVariantFlag
	if err := f.Set("candidate-a=models/"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if len(f) != 1 || f[0].StrategyID != "candidate-a" || f[0].ModelDir != "models/" {
		t.Fatalf("unexpected parse result: %+v", f)
	}
}

func TestGBTVariantFlagSetAccumulates(t *testing.T) {
	var f gbtVariantFlag
	if err := f.Set("a=1/"); err != nil {
		t.Fatalf("Set a: %v", err)
	}
	if err := f.Set("b=2/"); err != nil {
		t.Fatalf("Set b: %v", err)
	}
	if len(f) != 2 {
		t.Fatalf("expected 2 accumulated entries, got %d", len(f))
	}
}

func TestGBTVariantFlagSetRejectsMalformed(t *testing.T) {
	cases := []string{"", "no-equals-sign", "=missing-id/", "missing-dir="}
	for _, c := range cases {
		var f gbtVariantFlag
		if err := f.Set(c); err == nil {
			t.Errorf("Set(%q): expected an error", c)
		}
	}
}

// writeMinimalGBTModelDir writes four trivial single-leaf models (one per
// phase) into a new temp directory, matching bot.LoadGBTModels' expected
// file naming.
func writeMinimalGBTModelDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	data := []byte(`{"tree_info": [{"tree_structure": {"leaf_value": 0.0}}]}`)
	for _, phase := range []string{"attack", "reinforce", "occupy", "fortify"} {
		if err := os.WriteFile(filepath.Join(dir, phase+".json"), data, 0o644); err != nil {
			t.Fatalf("write %s model: %v", phase, err)
		}
	}
	return dir
}

func TestRegisterGBTVariantsAddsCustomStrategy(t *testing.T) {
	dir := writeMinimalGBTModelDir(t)

	registry := bot.StrategyRegistry{
		bot.StrategyBasicV1:  bot.NewBasicStrategy(),
		bot.StrategyScoredV1: bot.NewScoredStrategy(bot.DefaultWeights),
	}
	variants := gbtVariantFlag{{StrategyID: "gbt-candidate", ModelDir: dir}}

	if err := registerGBTVariants(registry, variants); err != nil {
		t.Fatalf("registerGBTVariants: %v", err)
	}
	if _, ok := registry.Get("gbt-candidate"); !ok {
		t.Fatal("expected gbt-candidate to be registered")
	}
}

func TestRegisterGBTVariantsRejectsBuiltinCollision(t *testing.T) {
	dir := writeMinimalGBTModelDir(t)

	registry := bot.StrategyRegistry{
		bot.StrategyBasicV1:  bot.NewBasicStrategy(),
		bot.StrategyScoredV1: bot.NewScoredStrategy(bot.DefaultWeights),
	}
	variants := gbtVariantFlag{{StrategyID: bot.StrategyScoredV1, ModelDir: dir}}

	if err := registerGBTVariants(registry, variants); err == nil {
		t.Fatal("expected an error when a --gbt-variant ID collides with a built-in strategy")
	}
}

func TestRegisterGBTVariantsRejectsDuplicateVariantID(t *testing.T) {
	dir := writeMinimalGBTModelDir(t)

	registry := bot.StrategyRegistry{
		bot.StrategyBasicV1:  bot.NewBasicStrategy(),
		bot.StrategyScoredV1: bot.NewScoredStrategy(bot.DefaultWeights),
	}
	variants := gbtVariantFlag{
		{StrategyID: "candidate", ModelDir: dir},
		{StrategyID: "candidate", ModelDir: dir},
	}

	if err := registerGBTVariants(registry, variants); err == nil {
		t.Fatal("expected an error for a duplicate --gbt-variant strategy ID")
	}
}

func TestRegisterGBTVariantsPropagatesLoadError(t *testing.T) {
	registry := bot.StrategyRegistry{
		bot.StrategyBasicV1: bot.NewBasicStrategy(),
	}
	variants := gbtVariantFlag{{StrategyID: "candidate", ModelDir: filepath.Join(t.TempDir(), "nope")}}

	if err := registerGBTVariants(registry, variants); err == nil {
		t.Fatal("expected an error when the model directory doesn't exist")
	}
}
