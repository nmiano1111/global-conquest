package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
