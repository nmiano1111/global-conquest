package bot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBoardValueScoreStandardizesAndComputesDotProduct(t *testing.T) {
	bv := &BoardValue{
		Weights:   []float64{2.0, -1.0},
		Intercept: 0.5,
		Mean:      []float64{1.0, 0.0},
		Std:       []float64{2.0, 1.0},
	}
	// standardized = [(3-1)/2, (4-0)/1] = [1.0, 4.0]
	// score = 0.5 + 2*1.0 + (-1)*4.0 = 0.5 + 2 - 4 = -1.5
	got := bv.Score([]float64{3.0, 4.0})
	if got != -1.5 {
		t.Errorf("Score(...) = %v, want -1.5", got)
	}
}

func TestBoardValueScoreHandlesZeroStd(t *testing.T) {
	bv := &BoardValue{
		Weights:   []float64{3.0},
		Intercept: 0,
		Mean:      []float64{5.0},
		Std:       []float64{0.0}, // a constant training-set feature
	}
	// std=0 falls back to 1.0 to avoid divide-by-zero: standardized = 5-5=0
	got := bv.Score([]float64{5.0})
	if got != 0 {
		t.Errorf("Score(...) = %v, want 0", got)
	}
}

func TestLoadBoardValueRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "value.json")
	data, _ := json.Marshal(map[string]any{
		"weights":        []float64{1.0, 2.0},
		"intercept":      0.5,
		"mean":           []float64{0.0, 0.0},
		"std":            []float64{1.0, 1.0},
		"attack_margin":  0.75,
		"fortify_margin": 0.05,
		"feature_names":  []string{"a", "b"},
	})
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	bv, err := LoadBoardValue(path)
	if err != nil {
		t.Fatalf("LoadBoardValue: %v", err)
	}
	if len(bv.Weights) != 2 || bv.Intercept != 0.5 || bv.AttackMargin() != 0.75 || bv.FortifyMargin() != 0.05 {
		t.Errorf("unexpected loaded BoardValue: %+v", bv)
	}
}

func TestLoadBoardValueMissingFile(t *testing.T) {
	if _, err := LoadBoardValue(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

func TestLoadBoardValueRejectsLengthMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "value.json")
	data, _ := json.Marshal(map[string]any{
		"weights": []float64{1.0, 2.0},
		"mean":    []float64{0.0}, // wrong length
		"std":     []float64{1.0, 1.0},
	})
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadBoardValue(path); err == nil {
		t.Fatal("expected an error for mismatched weights/mean/std lengths")
	}
}
