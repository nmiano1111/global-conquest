// Package gbtmodel parses a LightGBM dump_model() JSON export and runs
// inference against it -- a small, self-contained tree-ensemble
// interpreter with no dependency on Risk/bot types at all.
//
// LightGBM was chosen (over sklearn's HistGradientBoostingClassifier)
// specifically because dump_model()'s JSON shape is stable, documented,
// and explicitly designed for this kind of cross-language portability --
// unlike sklearn's internal _predictors tree representation, which isn't
// a public API and could change across versions with no notice.
package gbtmodel

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
)

// Model is a parsed LightGBM tree ensemble, ready for inference.
type Model struct {
	trees []node
	// endPhaseThreshold is the exported end_phase_threshold value (see
	// analytics/src/global_conquest_analytics/gbt_fit.py's GBTPhaseFit),
	// used only by attack/fortify models; hasEndPhaseThreshold is false
	// for reinforce/occupy models, which never export one.
	endPhaseThreshold    float64
	hasEndPhaseThreshold bool
}

// node is one internal split or leaf in a parsed tree. A leaf has
// leafValue set and both children nil; an internal node has both
// children set and routes by comparing features[splitFeature] against
// threshold.
type node struct {
	isLeaf       bool
	leafValue    float64
	splitFeature int
	threshold    float64
	left, right  *node
}

// dumpFile/dumpTree/dumpNode mirror LightGBM's dump_model() JSON shape
// exactly (see https://lightgbm.readthedocs.io/ -- Booster.dump_model),
// used only to unmarshal before converting into the leaner node tree
// above.
type dumpFile struct {
	TreeInfo          []dumpTree `json:"tree_info"`
	EndPhaseThreshold *float64   `json:"end_phase_threshold"`
}

type dumpTree struct {
	TreeStructure dumpNode `json:"tree_structure"`
}

type dumpNode struct {
	SplitFeature *int      `json:"split_feature"`
	Threshold    *float64  `json:"threshold"`
	DecisionType *string   `json:"decision_type"`
	LeftChild    *dumpNode `json:"left_child"`
	RightChild   *dumpNode `json:"right_child"`
	LeafValue    *float64  `json:"leaf_value"`
}

// LoadModel reads and parses a LightGBM dump_model() JSON file.
func LoadModel(path string) (*Model, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("gbtmodel: read model file: %w", err)
	}
	return ParseModel(data)
}

// ParseModel parses already-in-memory dump_model() JSON bytes -- the same
// logic LoadModel uses after reading a file, exposed directly for callers
// that already have model bytes in hand (e.g. tests building small
// synthetic models) without needing a temp file.
func ParseModel(data []byte) (*Model, error) {
	var dump dumpFile
	if err := json.Unmarshal(data, &dump); err != nil {
		return nil, fmt.Errorf("gbtmodel: parse model JSON: %w", err)
	}
	if len(dump.TreeInfo) == 0 {
		return nil, fmt.Errorf("gbtmodel: model has no trees")
	}

	trees := make([]node, len(dump.TreeInfo))
	for i, t := range dump.TreeInfo {
		n, err := convertNode(t.TreeStructure)
		if err != nil {
			return nil, fmt.Errorf("gbtmodel: tree %d: %w", i, err)
		}
		trees[i] = *n
	}

	m := &Model{trees: trees}
	if dump.EndPhaseThreshold != nil {
		m.endPhaseThreshold = *dump.EndPhaseThreshold
		m.hasEndPhaseThreshold = true
	}
	return m, nil
}

// convertNode recursively converts one dumpNode into the leaner node
// representation. LightGBM's missing-value routing (default_left) is
// deliberately not implemented -- every Risk feature this model ever
// scores is always fully computed, never missing/NaN, so that routing
// path is unreachable in practice.
func convertNode(d dumpNode) (*node, error) {
	if d.LeafValue != nil {
		return &node{isLeaf: true, leafValue: *d.LeafValue}, nil
	}
	if d.SplitFeature == nil || d.Threshold == nil || d.LeftChild == nil || d.RightChild == nil {
		return nil, fmt.Errorf("malformed internal node: missing split_feature/threshold/children")
	}
	if d.DecisionType != nil && *d.DecisionType != "<=" {
		return nil, fmt.Errorf("unsupported decision_type %q (only \"<=\" is supported)", *d.DecisionType)
	}
	left, err := convertNode(*d.LeftChild)
	if err != nil {
		return nil, fmt.Errorf("left child: %w", err)
	}
	right, err := convertNode(*d.RightChild)
	if err != nil {
		return nil, fmt.Errorf("right child: %w", err)
	}
	return &node{
		splitFeature: *d.SplitFeature,
		threshold:    *d.Threshold,
		left:         left,
		right:        right,
	}, nil
}

// Predict returns the raw sum-of-trees score (log-odds space) for
// features, indexed in the same fixed order the model was trained on.
func (m *Model) Predict(features []float64) float64 {
	var total float64
	for i := range m.trees {
		total += evalTree(&m.trees[i], features)
	}
	return total
}

func evalTree(n *node, features []float64) float64 {
	for !n.isLeaf {
		if features[n.splitFeature] <= n.threshold {
			n = n.left
		} else {
			n = n.right
		}
	}
	return n.leafValue
}

// PredictProba applies the sigmoid to Predict's raw score, matching
// LightGBM's own binary objective convention -- returns a genuine 0-1
// probability.
func (m *Model) PredictProba(features []float64) float64 {
	return 1 / (1 + math.Exp(-m.Predict(features)))
}

// EndPhaseThreshold returns the model's exported end_phase_threshold and
// true, or (0, false) if this model never exported one (reinforce/occupy
// models never do -- see gbt_fit.py's GBTPhaseFit).
func (m *Model) EndPhaseThreshold() (float64, bool) {
	return m.endPhaseThreshold, m.hasEndPhaseThreshold
}
