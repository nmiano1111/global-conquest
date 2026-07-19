package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
)

func TestPositivePercentileIgnoresNonPositiveValues(t *testing.T) {
	got := positivePercentile([]float64{-5, 0, 1, 2, 3}, 50)
	// Only [1, 2, 3] are positive; median of those is 2.
	if got != 2 {
		t.Errorf("positivePercentile(...) = %v, want 2", got)
	}
}

func TestPositivePercentileInterpolates(t *testing.T) {
	got := positivePercentile([]float64{1, 2, 3, 4}, 50)
	// rank = 0.5 * 3 = 1.5 -> interpolate between index 1 (2) and 2 (3).
	want := 2.5
	if got != want {
		t.Errorf("positivePercentile(...) = %v, want %v", got, want)
	}
}

func TestPositivePercentileEmptyReturnsZero(t *testing.T) {
	if got := positivePercentile(nil, 50); got != 0 {
		t.Errorf("positivePercentile(nil, 50) = %v, want 0", got)
	}
	if got := positivePercentile([]float64{-1, -2}, 50); got != 0 {
		t.Errorf("positivePercentile(all-negative, 50) = %v, want 0", got)
	}
}

func TestPositivePercentileZeroth(t *testing.T) {
	got := positivePercentile([]float64{5, 1, 3}, 0)
	if got != 1 {
		t.Errorf("positivePercentile(..., 0) = %v, want the minimum positive value 1", got)
	}
}

func TestWriteCalibratedPreservesOtherFieldsAndSetsMargins(t *testing.T) {
	inputPath := filepath.Join(t.TempDir(), "value.json")
	inputData, _ := json.Marshal(map[string]any{
		"weights":        []float64{1.0, 2.0},
		"intercept":      0.5,
		"mean":           []float64{0.0, 0.0},
		"std":            []float64{1.0, 1.0},
		"attack_margin":  0.0,
		"fortify_margin": 0.0,
		"feature_names":  []string{"a", "b"},
	})
	if err := os.WriteFile(inputPath, inputData, 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	outputPath := filepath.Join(t.TempDir(), "calibrated.json")

	if err := writeCalibrated(inputPath, outputPath, 0.75, 0.05); err != nil {
		t.Fatalf("writeCalibrated: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("parse output: %v", err)
	}
	if payload["attack_margin"] != 0.75 || payload["fortify_margin"] != 0.05 {
		t.Errorf("unexpected margins in output: attack=%v fortify=%v", payload["attack_margin"], payload["fortify_margin"])
	}
	if len(payload["weights"].([]any)) != 2 || payload["intercept"] != 0.5 {
		t.Errorf("expected other fields to pass through unchanged, got %+v", payload)
	}
}

func TestWriteCalibratedMissingInput(t *testing.T) {
	err := writeCalibrated(filepath.Join(t.TempDir(), "nope.json"), filepath.Join(t.TempDir(), "out.json"), 0, 0)
	if err == nil {
		t.Fatal("expected an error for a missing input file")
	}
}

func writeLinearFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "linear.json")
	data, _ := json.Marshal(map[string]any{
		"weights":        []float64{1.0},
		"intercept":      0.0,
		"mean":           []float64{0.0},
		"std":            []float64{1.0},
		"attack_margin":  0.5,
		"fortify_margin": 0.1,
		"feature_names":  []string{"x"},
	})
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write linear fixture: %v", err)
	}
	return path
}

func writeGCNFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gcn.json")
	data, _ := json.Marshal(map[string]any{
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
		"attack_margin":  0.3,
		"fortify_margin": 0.05,
	})
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write gcn fixture: %v", err)
	}
	return path
}

func TestLoadValueFunctionLinear(t *testing.T) {
	value, err := loadValueFunction(writeLinearFixture(t), "linear")
	if err != nil {
		t.Fatalf("loadValueFunction: %v", err)
	}
	if value.AttackMargin() != 0.5 || value.FortifyMargin() != 0.1 {
		t.Errorf("unexpected margins: attack=%v fortify=%v", value.AttackMargin(), value.FortifyMargin())
	}
	if got := value.Score([]float64{2.0}); got != 2.0 {
		t.Errorf("Score([2.0]) = %v, want 2.0", got)
	}
}

func TestLoadValueFunctionGCN(t *testing.T) {
	value, err := loadValueFunction(writeGCNFixture(t), "gcn")
	if err != nil {
		t.Fatalf("loadValueFunction: %v", err)
	}
	if value.AttackMargin() != 0.3 || value.FortifyMargin() != 0.05 {
		t.Errorf("unexpected margins: attack=%v fortify=%v", value.AttackMargin(), value.FortifyMargin())
	}
	// Just confirm it runs without error/panic and returns a finite score.
	got := value.Score(make([]float64, 9))
	if got != got { // NaN check without importing math
		t.Errorf("Score(...) = %v, expected a finite value", got)
	}
}

func TestLoadValueFunctionUnknownType(t *testing.T) {
	if _, err := loadValueFunction(writeLinearFixture(t), "nonsense"); err == nil {
		t.Fatal("expected an error for an unknown --model-type")
	}
}

func TestZeroMarginValueFunctionAlwaysReturnsZeroMargins(t *testing.T) {
	value, err := loadValueFunction(writeLinearFixture(t), "linear")
	if err != nil {
		t.Fatalf("loadValueFunction: %v", err)
	}
	wrapped := zeroMarginValueFunction{value}

	if wrapped.AttackMargin() != 0 || wrapped.FortifyMargin() != 0 {
		t.Errorf("expected zero margins from the wrapper, got attack=%v fortify=%v", wrapped.AttackMargin(), wrapped.FortifyMargin())
	}
	// Score still delegates to the wrapped value function unchanged.
	if got, want := wrapped.Score([]float64{2.0}), value.Score([]float64{2.0}); got != want {
		t.Errorf("Score(...) = %v, want %v (should delegate unchanged)", got, want)
	}
}

var _ bot.ValueFunction = zeroMarginValueFunction{}
