package bot

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadWeights reads a JSON file and returns a Weights value based on
// DefaultWeights, with any fields present in the file overriding the
// baseline -- a variant file only needs to specify what it's actually
// changing, not all 21 fields. Weights has no JSON tags (its fields are
// already plain exported float64s), so encoding/json's default
// reflection-based unmarshal already round-trips it correctly with no
// struct changes.
func LoadWeights(path string) (Weights, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Weights{}, fmt.Errorf("bot: read weights file: %w", err)
	}
	w := DefaultWeights
	if err := json.Unmarshal(data, &w); err != nil {
		return Weights{}, fmt.Errorf("bot: parse weights file %s: %w", path, err)
	}
	return w, nil
}
