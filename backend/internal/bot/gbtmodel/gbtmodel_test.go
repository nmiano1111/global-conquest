package gbtmodel

import (
	"math"
	"testing"
)

// two_tree_binary.json is a real dump_model() export from a 2-round
// LightGBM binary classifier, hand-verified against booster.predict(...,
// raw_score=True) and booster.predict(...) at test point [0.0, 5.0]:
// raw_score=-0.38187308119005766, proba=0.40567521.
const twoTreeFixture = "testdata/two_tree_binary.json"

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

func TestPredictSumsLeavesAcrossTrees(t *testing.T) {
	m, err := LoadModel(twoTreeFixture)
	if err != nil {
		t.Fatalf("LoadModel: %v", err)
	}

	// f0=0.0 routes left in both trees.
	got := m.Predict([]float64{0.0, 5.0})
	want := -0.19999999999999996 + -0.18187308119005766
	if !almostEqual(got, want) {
		t.Errorf("Predict([0.0, 5.0]) = %v, want %v", got, want)
	}
}

func TestPredictProbaAppliesSigmoid(t *testing.T) {
	m, err := LoadModel(twoTreeFixture)
	if err != nil {
		t.Fatalf("LoadModel: %v", err)
	}

	got := m.PredictProba([]float64{0.0, 5.0})
	want := 0.40567521 // hand-verified against booster.predict(...) at this point
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("PredictProba([0.0, 5.0]) = %v, want %v", got, want)
	}
}

func TestPredictRoutesThroughNestedSplit(t *testing.T) {
	m, err := LoadModel(twoTreeFixture)
	if err != nil {
		t.Fatalf("LoadModel: %v", err)
	}

	// f0=2.0: tree0 routes right (0.2); tree1 routes right then left at the
	// nested split (0.1818730811900577).
	got := m.Predict([]float64{2.0, 1.0})
	want := 0.2 + 0.1818730811900577
	if !almostEqual(got, want) {
		t.Errorf("Predict([2.0, 1.0]) = %v, want %v", got, want)
	}

	// f0=3.0: tree0 routes right (0.2); tree1 routes right then right at the
	// nested split (also 0.1818730811900577 in this fixture).
	got = m.Predict([]float64{3.0, 1.0})
	want = 0.2 + 0.1818730811900577
	if !almostEqual(got, want) {
		t.Errorf("Predict([3.0, 1.0]) = %v, want %v", got, want)
	}
}

func TestEndPhaseThresholdAbsentByDefault(t *testing.T) {
	m, err := LoadModel(twoTreeFixture)
	if err != nil {
		t.Fatalf("LoadModel: %v", err)
	}
	if _, ok := m.EndPhaseThreshold(); ok {
		t.Fatal("expected EndPhaseThreshold to report false when the model never exported one")
	}
}

func TestEndPhaseThresholdParsedWhenPresent(t *testing.T) {
	data := []byte(`{"tree_info": [{"tree_structure": {"leaf_value": 0.5}}], "end_phase_threshold": 0.187}`)
	m, err := ParseModel(data)
	if err != nil {
		t.Fatalf("ParseModel: %v", err)
	}
	got, ok := m.EndPhaseThreshold()
	if !ok {
		t.Fatal("expected EndPhaseThreshold to report true when the model exported one")
	}
	if !almostEqual(got, 0.187) {
		t.Errorf("EndPhaseThreshold() = %v, want 0.187", got)
	}
}

func TestLoadModelMissingFile(t *testing.T) {
	_, err := LoadModel("testdata/does-not-exist.json")
	if err == nil {
		t.Fatal("expected an error for a missing model file")
	}
}

func TestLoadModelInvalidJSON(t *testing.T) {
	_, err := ParseModel([]byte("not json"))
	if err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
}

func TestLoadModelNoTrees(t *testing.T) {
	_, err := ParseModel([]byte(`{"tree_info": []}`))
	if err == nil {
		t.Fatal("expected an error for a model with no trees")
	}
}

func TestLoadModelRejectsUnsupportedDecisionType(t *testing.T) {
	data := []byte(`{"tree_info": [{"tree_structure": {
		"split_feature": 0, "threshold": 1.0, "decision_type": ">",
		"left_child": {"leaf_value": 1.0}, "right_child": {"leaf_value": 2.0}
	}}]}`)
	_, err := ParseModel(data)
	if err == nil {
		t.Fatal("expected an error for an unsupported decision_type")
	}
}

func TestLoadModelRejectsMalformedInternalNode(t *testing.T) {
	// Missing threshold and children entirely, and no leaf_value either.
	data := []byte(`{"tree_info": [{"tree_structure": {"split_feature": 0}}]}`)
	_, err := ParseModel(data)
	if err == nil {
		t.Fatal("expected an error for a malformed internal node")
	}
}

func TestLoadModelSingleLeafTree(t *testing.T) {
	// A degenerate tree that's just one leaf (num_leaves=1) -- must not be
	// treated as a malformed internal node.
	data := []byte(`{"tree_info": [{"tree_structure": {"leaf_value": 0.5}}]}`)
	m, err := ParseModel(data)
	if err != nil {
		t.Fatalf("parseModel: %v", err)
	}
	if got := m.Predict([]float64{999}); got != 0.5 {
		t.Errorf("Predict on a single-leaf tree = %v, want 0.5", got)
	}
}
