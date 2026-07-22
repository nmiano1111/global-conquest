package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
)

func TestGCNVariantFlagSetParsesIDAndPath(t *testing.T) {
	var f gcnVariantFlag
	if err := f.Set("candidate-a=weights.json"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if len(f) != 1 || f[0].StrategyID != "candidate-a" || f[0].WeightsPath != "weights.json" {
		t.Fatalf("unexpected parse result: %+v", f)
	}
}

func TestGCNVariantFlagSetAccumulates(t *testing.T) {
	var f gcnVariantFlag
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

func TestGCNVariantFlagSetRejectsMalformed(t *testing.T) {
	cases := []string{"", "no-equals-sign", "=missing-id.json", "missing-path="}
	for _, c := range cases {
		var f gcnVariantFlag
		if err := f.Set(c); err == nil {
			t.Errorf("Set(%q): expected an error", c)
		}
	}
}

// writeMinimalGCNFile writes a trivial 2-node GCN model JSON file,
// matching gcnmodel.LoadModel's expected shape.
func writeMinimalGCNFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gcn.json")
	data, err := json.Marshal(map[string]any{
		"gcn1":   map[string]any{"weight": [][]float64{{1.0, 0.0, 0.0, 0.0}}, "bias": []float64{0.0}},
		"gcn2":   map[string]any{"weight": [][]float64{{1.0}}, "bias": []float64{0.0}},
		"fc1":    map[string]any{"weight": [][]float64{{1.0}}, "bias": []float64{0.0}},
		"fc2":    map[string]any{"weight": [][]float64{{1.0, 1.0}}, "bias": []float64{0.0}},
		"fc3":    map[string]any{"weight": [][]float64{{1.0, 1.0}}, "bias": []float64{0.0}},
		"output": map[string]any{"weight": [][]float64{{1.0}}, "bias": []float64{0.0}},
		"mean":   make([]float64, 9),
		"std":    []float64{1, 1, 1, 1, 1, 1, 1, 1, 1},
		"propagation_matrix": [][]float64{
			{0.5, 0.5},
			{0.5, 0.5},
		},
		"board_order": []string{"A", "B"},
		"feature_names": []string{
			"territory_A_is_mine", "territory_A_army_fraction", "territory_A_is_continent_border", "territory_A_enemy_threat_fraction",
			"territory_B_is_mine", "territory_B_army_fraction", "territory_B_is_continent_border", "territory_B_enemy_threat_fraction",
			"global1",
		},
		"attack_margin":  0.0,
		"fortify_margin": 0.0,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write gcn file: %v", err)
	}
	return path
}

func TestRegisterGCNVariantsAddsCustomStrategy(t *testing.T) {
	path := writeMinimalGCNFile(t)

	registry := bot.StrategyRegistry{
		bot.StrategyBasicV1:  bot.NewBasicStrategy(),
		bot.StrategyScoredV1: bot.NewScoredStrategy(bot.DefaultWeights),
	}
	variants := gcnVariantFlag{{StrategyID: "gcn-candidate", WeightsPath: path}}

	if err := registerGCNVariants(registry, variants); err != nil {
		t.Fatalf("registerGCNVariants: %v", err)
	}
	if _, ok := registry.Get("gcn-candidate"); !ok {
		t.Fatal("expected gcn-candidate to be registered")
	}
}

func TestRegisterGCNVariantsRejectsBuiltinCollision(t *testing.T) {
	path := writeMinimalGCNFile(t)

	registry := bot.StrategyRegistry{
		bot.StrategyBasicV1:  bot.NewBasicStrategy(),
		bot.StrategyScoredV1: bot.NewScoredStrategy(bot.DefaultWeights),
	}
	variants := gcnVariantFlag{{StrategyID: bot.StrategyScoredV1, WeightsPath: path}}

	if err := registerGCNVariants(registry, variants); err == nil {
		t.Fatal("expected an error when a --gcn-variant ID collides with a built-in strategy")
	}
}

func TestRegisterGCNVariantsRejectsDuplicateVariantID(t *testing.T) {
	path := writeMinimalGCNFile(t)

	registry := bot.StrategyRegistry{
		bot.StrategyBasicV1:  bot.NewBasicStrategy(),
		bot.StrategyScoredV1: bot.NewScoredStrategy(bot.DefaultWeights),
	}
	variants := gcnVariantFlag{
		{StrategyID: "candidate", WeightsPath: path},
		{StrategyID: "candidate", WeightsPath: path},
	}

	if err := registerGCNVariants(registry, variants); err == nil {
		t.Fatal("expected an error for a duplicate --gcn-variant strategy ID")
	}
}

func TestRegisterGCNVariantsPropagatesLoadError(t *testing.T) {
	registry := bot.StrategyRegistry{
		bot.StrategyBasicV1: bot.NewBasicStrategy(),
	}
	variants := gcnVariantFlag{{StrategyID: "candidate", WeightsPath: filepath.Join(t.TempDir(), "nope.json")}}

	if err := registerGCNVariants(registry, variants); err == nil {
		t.Fatal("expected an error when the model file doesn't exist")
	}
}
