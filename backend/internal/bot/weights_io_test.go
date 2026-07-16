package bot

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempWeightsFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "weights.json")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write temp weights file: %v", err)
	}
	return path
}

func TestLoadWeightsOverridesOnlySpecifiedFields(t *testing.T) {
	path := writeTempWeightsFile(t, `{"ArmyAdvantage": 9.9, "ExposurePenalty": -5}`)

	w, err := LoadWeights(path)
	if err != nil {
		t.Fatalf("LoadWeights: %v", err)
	}
	if w.ArmyAdvantage != 9.9 {
		t.Errorf("ArmyAdvantage = %v, want 9.9", w.ArmyAdvantage)
	}
	if w.ExposurePenalty != -5 {
		t.Errorf("ExposurePenalty = %v, want -5", w.ExposurePenalty)
	}
	// Every other field should fall back to DefaultWeights untouched.
	if w.CaptureProbability != DefaultWeights.CaptureProbability {
		t.Errorf("CaptureProbability = %v, want unchanged default %v", w.CaptureProbability, DefaultWeights.CaptureProbability)
	}
	if w.ReinforceContinentValue != DefaultWeights.ReinforceContinentValue {
		t.Errorf("ReinforceContinentValue = %v, want unchanged default %v", w.ReinforceContinentValue, DefaultWeights.ReinforceContinentValue)
	}
	if w.FortifyEndTurnBias != DefaultWeights.FortifyEndTurnBias {
		t.Errorf("FortifyEndTurnBias = %v, want unchanged default %v", w.FortifyEndTurnBias, DefaultWeights.FortifyEndTurnBias)
	}
}

func TestLoadWeightsEmptyFileReturnsDefaults(t *testing.T) {
	path := writeTempWeightsFile(t, `{}`)

	w, err := LoadWeights(path)
	if err != nil {
		t.Fatalf("LoadWeights: %v", err)
	}
	if w != DefaultWeights {
		t.Errorf("expected an empty override file to return DefaultWeights unchanged, got %+v", w)
	}
}

func TestLoadWeightsMissingFile(t *testing.T) {
	if _, err := LoadWeights(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("expected an error for a missing weights file")
	}
}

func TestLoadWeightsInvalidJSON(t *testing.T) {
	path := writeTempWeightsFile(t, `{not valid json`)
	if _, err := LoadWeights(path); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}
