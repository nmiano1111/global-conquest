package bot

import (
	"encoding/json"
	"fmt"
	"os"
)

// BoardValue is a linear whole-board value function: standardize a
// tdstate.Encode(...).Flatten() feature vector using the same mean/std
// computed at training time, then take a dot product plus an intercept.
// Ranking is all that matters (ValueStrategy always just picks the
// highest score), so the raw linear score is used directly -- no sigmoid
// needed, the same "sigmoid is monotonic" reasoning already used for
// bot.Weights' fitted coefficients earlier this project.
type BoardValue struct {
	Weights   []float64
	Intercept float64
	Mean      []float64
	Std       []float64
	// attackMargin/fortifyMargin are how much a candidate's afterstate
	// score must exceed the current, unmodified state's score before
	// ValueStrategy's attack/fortify phases will act on it, instead of
	// ending the phase -- a bare "is it better at all" gate (margin == 0)
	// isn't enough: a first live tournament eval with margin == 0 showed
	// ~15 attacks/turn (vs. ~1.4 captures/turn averaged across a whole
	// tournament), since every fitted coefficient is small enough that
	// nearly any legal move nudges the score marginally positive.
	//
	// These are two separate fields, not one shared margin, because
	// attack and fortify move the score on completely different scales:
	// attack changes ownership (touching many features -- is_mine,
	// continent/army/territory fractions all at once), while fortify only
	// reallocates armies between the acting player's own territories
	// (touching at most two per-territory army_fraction coefficients,
	// typically much smaller). A single shared margin calibrated to
	// attack's scale was found to suppress fortify almost entirely (12
	// fortifies/13 turns at margin 0, down to 0 fortifies at any margin
	// >= ~0.1 -- see cmd/bvcalibrate, which fits these two independently
	// from each phase's own observed score-delta distribution).
	attackMargin  float64
	fortifyMargin float64
}

// AttackMargin/FortifyMargin implement ValueFunction.
func (bv *BoardValue) AttackMargin() float64  { return bv.attackMargin }
func (bv *BoardValue) FortifyMargin() float64 { return bv.fortifyMargin }

// boardValueFile mirrors analytics/src/global_conquest_analytics/
// board_fit.py's exported JSON shape exactly.
type boardValueFile struct {
	Weights       []float64 `json:"weights"`
	Intercept     float64   `json:"intercept"`
	Mean          []float64 `json:"mean"`
	Std           []float64 `json:"std"`
	AttackMargin  float64   `json:"attack_margin"`
	FortifyMargin float64   `json:"fortify_margin"`
	FeatureNames  []string  `json:"feature_names"`
}

// LoadBoardValue reads and parses a board_fit.py-exported JSON file.
func LoadBoardValue(path string) (*BoardValue, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("bot: read board value file: %w", err)
	}
	var f boardValueFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("bot: parse board value file %s: %w", path, err)
	}
	if len(f.Weights) != len(f.Mean) || len(f.Weights) != len(f.Std) {
		return nil, fmt.Errorf("bot: board value file %s: weights/mean/std length mismatch (%d/%d/%d)", path, len(f.Weights), len(f.Mean), len(f.Std))
	}
	return &BoardValue{
		Weights:       f.Weights,
		Intercept:     f.Intercept,
		Mean:          f.Mean,
		Std:           f.Std,
		attackMargin:  f.AttackMargin,
		fortifyMargin: f.FortifyMargin,
	}, nil
}

// Score standardizes features using bv's training-set mean/std, then
// returns the linear value w . standardized + intercept.
func (bv *BoardValue) Score(features []float64) float64 {
	score := bv.Intercept
	for i, x := range features {
		std := bv.Std[i]
		if std == 0 {
			std = 1
		}
		standardized := (x - bv.Mean[i]) / std
		score += bv.Weights[i] * standardized
	}
	return score
}
