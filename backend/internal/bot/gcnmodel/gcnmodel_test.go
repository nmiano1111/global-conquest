package gcnmodel

import (
	"encoding/json"
	"math"
	"testing"
)

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// twoNodeModelJSON is a hand-computable 2-node (A, B, edge A-B) model.
// feature_names matches tdstate's real per-territory stride (is_mine,
// army_fraction, continent one-hot, is_continent_border,
// enemy_threat_fraction) with zero continent columns, so node_dim = 4;
// only each node's first feature (is_mine) actually contributes (gcn1's
// weight is zero on the other three), keeping the arithmetic simple
// while still exercising the real reshape logic.
//
// Propagation matrix for a 2-node graph with one edge, Kipf & Welling
// renormalized (P = D^-1/2(A+I)D^-1/2, degree 2 each): every entry is
// 0.5.
//
// Forward pass by hand, mean=zeros/std=ones (standardization is a
// no-op), raw features node A=[1.0,0,0,0], node B=[2.0,0,0,0], global=[1.0]:
//
//	gcn1 (w=[2,0,0,0],b=0): A'=2*1=2, B'=2*2=4; propagate (all entries 0.5): h1=[3,3]; ReLU: [3,3]
//	gcn2 (w=1,b=0): unchanged [3,3]; propagate: h2=[3,3]; ReLU: [3,3]
//	flatten: [3,3]
//	fc2 (w=[1,1],b=0): 3+3=6; ReLU: 6
//	fc1 (w=[3],b=0): 3*1=3; ReLU: 3
//	combined (fc2 then fc1): [6,3]
//	fc3 (w=[1,1],b=0): 6+3=9; ReLU: 9
//	output (w=[1],b=0.5): 9+0.5=9.5
const twoNodeModelJSON = `{
	"gcn1": {"weight": [[2.0, 0.0, 0.0, 0.0]], "bias": [0.0]},
	"gcn2": {"weight": [[1.0]], "bias": [0.0]},
	"fc1": {"weight": [[3.0]], "bias": [0.0]},
	"fc2": {"weight": [[1.0, 1.0]], "bias": [0.0]},
	"fc3": {"weight": [[1.0, 1.0]], "bias": [0.0]},
	"output": {"weight": [[1.0]], "bias": [0.5]},
	"mean": [0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0],
	"std": [1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0],
	"propagation_matrix": [[0.5, 0.5], [0.5, 0.5]],
	"board_order": ["A", "B"],
	"feature_names": [
		"territory_A_is_mine", "territory_A_army_fraction", "territory_A_is_continent_border", "territory_A_enemy_threat_fraction",
		"territory_B_is_mine", "territory_B_army_fraction", "territory_B_is_continent_border", "territory_B_enemy_threat_fraction",
		"global1"
	],
	"attack_margin": 0.1,
	"fortify_margin": 0.02
}`

var rawFeatures = []float64{1.0, 0, 0, 0, 2.0, 0, 0, 0, 1.0}

func TestScoreMatchesHandComputedForwardPass(t *testing.T) {
	m, err := ParseModel([]byte(twoNodeModelJSON))
	if err != nil {
		t.Fatalf("ParseModel: %v", err)
	}

	got := m.Score(rawFeatures)
	want := 9.5
	if !almostEqual(got, want) {
		t.Errorf("Score(...) = %v, want %v", got, want)
	}
}

func TestScoreStandardizesBeforeForwardPass(t *testing.T) {
	// Same model, but mean/std set so that raw features standardize down
	// to rawFeatures's values used above -- (x - mean) / std = target.
	var dump map[string]any
	if err := json.Unmarshal([]byte(twoNodeModelJSON), &dump); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	mean := make([]float64, len(rawFeatures))
	std := make([]float64, len(rawFeatures))
	scaled := make([]float64, len(rawFeatures))
	for i, target := range rawFeatures {
		mean[i] = 1.0
		std[i] = 2.0
		scaled[i] = mean[i] + std[i]*target
	}
	dump["mean"] = mean
	dump["std"] = std
	data, err := json.Marshal(dump)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	m, err := ParseModel(data)
	if err != nil {
		t.Fatalf("ParseModel: %v", err)
	}

	got := m.Score(scaled)
	want := 9.5
	if !almostEqual(got, want) {
		t.Errorf("Score(...) = %v, want %v", got, want)
	}
}

func TestScoreHandlesZeroStdWithoutDividingByZero(t *testing.T) {
	var dump map[string]any
	if err := json.Unmarshal([]byte(twoNodeModelJSON), &dump); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	std := make([]float64, len(rawFeatures))
	for i := range std {
		std[i] = 1.0
	}
	std[len(std)-1] = 0.0 // constant global feature in training
	dump["std"] = std
	data, err := json.Marshal(dump)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	m, err := ParseModel(data)
	if err != nil {
		t.Fatalf("ParseModel: %v", err)
	}

	got := m.Score(rawFeatures)
	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Fatalf("Score(...) = %v, expected a finite value (std=0 should fall back to 1, not divide by zero)", got)
	}
}

func TestAttackMarginAndFortifyMargin(t *testing.T) {
	m, err := ParseModel([]byte(twoNodeModelJSON))
	if err != nil {
		t.Fatalf("ParseModel: %v", err)
	}
	if m.AttackMargin() != 0.1 {
		t.Errorf("AttackMargin() = %v, want 0.1", m.AttackMargin())
	}
	if m.FortifyMargin() != 0.02 {
		t.Errorf("FortifyMargin() = %v, want 0.02", m.FortifyMargin())
	}
}

func TestParseModelRejectsMismatchedMeanStdFeatureNames(t *testing.T) {
	var dump map[string]any
	if err := json.Unmarshal([]byte(twoNodeModelJSON), &dump); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	dump["mean"] = []float64{0.0, 0.0} // wrong length vs. std/feature_names
	data, err := json.Marshal(dump)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if _, err := ParseModel(data); err == nil {
		t.Fatal("expected an error for mismatched mean/std/feature_names lengths")
	}
}

func TestLoadModelMissingFile(t *testing.T) {
	if _, err := LoadModel("testdata/does-not-exist.json"); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}
