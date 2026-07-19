// Package gcnmodel parses a gcn_fit.py-exported GCN JSON export and runs
// inference against it -- a small, self-contained forward-pass
// interpreter with no external tensor library and no dependency on
// Risk/bot/tdstate types.
//
// Architecture matches Jamie Carr's "Using Graph Convolutional Networks
// and TD(λ) to Play the Game of Risk" (arXiv:2009.06355) Figure 1 exactly
// -- see analytics/src/global_conquest_analytics/gcn_fit.py's module
// docstring for the full layer-by-layer rationale: two graph-conv layers
// (dense matmul against a precomputed propagation matrix, since the
// board is a fixed graph, not PyTorch Geometric's general variable-graph
// case), flattened (not pooled, preserving per-territory identity) into
// FC2, concatenated with a global-features path through FC1, mixed via
// FC3, then a single-scalar output layer (no activation -- ranking is
// all that matters, same as every other model this project has fit).
package gcnmodel

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Model is a parsed GCN value network, ready for inference.
type Model struct {
	gcn1, gcn2    layer
	fc1, fc2, fc3 layer
	output        layer

	mean, std         []float64
	propagationMatrix [][]float64
	numNodes          int
	nodeDim           int

	attackMargin  float64
	fortifyMargin float64
}

// layer is one fully-connected layer's parameters. weight is
// [out][in], matching PyTorch nn.Linear.weight's own
// [out_features, in_features] convention exactly as gcn_fit.py exports
// it, so this forward pass mirrors PyTorch's without any transposition.
type layer struct {
	weight [][]float64
	bias   []float64
}

// modelFile mirrors gcn_fit.py's export_gcn JSON shape exactly.
type modelFile struct {
	GCN1              layerFile   `json:"gcn1"`
	GCN2              layerFile   `json:"gcn2"`
	FC1               layerFile   `json:"fc1"`
	FC2               layerFile   `json:"fc2"`
	FC3               layerFile   `json:"fc3"`
	Output            layerFile   `json:"output"`
	Mean              []float64   `json:"mean"`
	Std               []float64   `json:"std"`
	PropagationMatrix [][]float64 `json:"propagation_matrix"`
	BoardOrder        []string    `json:"board_order"`
	FeatureNames      []string    `json:"feature_names"`
	AttackMargin      float64     `json:"attack_margin"`
	FortifyMargin     float64     `json:"fortify_margin"`
}

type layerFile struct {
	Weight [][]float64 `json:"weight"`
	Bias   []float64   `json:"bias"`
}

// LoadModel reads and parses a gcn_fit.py-exported JSON file.
func LoadModel(path string) (*Model, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("gcnmodel: read model file: %w", err)
	}
	return ParseModel(data)
}

// ParseModel parses already-in-memory export_gcn JSON bytes -- exposed
// directly for callers (e.g. tests) that already have model bytes in
// hand without needing a temp file.
func ParseModel(data []byte) (*Model, error) {
	var f modelFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("gcnmodel: parse model JSON: %w", err)
	}
	if len(f.BoardOrder) == 0 {
		return nil, fmt.Errorf("gcnmodel: model has an empty board_order")
	}
	if len(f.Mean) != len(f.Std) || len(f.Mean) != len(f.FeatureNames) {
		return nil, fmt.Errorf("gcnmodel: mean/std/feature_names length mismatch (%d/%d/%d)", len(f.Mean), len(f.Std), len(f.FeatureNames))
	}

	numNodes := len(f.BoardOrder)
	nodeDim := nodeFeatureDim(f.FeatureNames, f.BoardOrder[0])

	return &Model{
		gcn1:              layer{weight: f.GCN1.Weight, bias: f.GCN1.Bias},
		gcn2:              layer{weight: f.GCN2.Weight, bias: f.GCN2.Bias},
		fc1:               layer{weight: f.FC1.Weight, bias: f.FC1.Bias},
		fc2:               layer{weight: f.FC2.Weight, bias: f.FC2.Bias},
		fc3:               layer{weight: f.FC3.Weight, bias: f.FC3.Bias},
		output:            layer{weight: f.Output.Weight, bias: f.Output.Bias},
		mean:              f.Mean,
		std:               f.Std,
		propagationMatrix: f.PropagationMatrix,
		numNodes:          numNodes,
		nodeDim:           nodeDim,
		attackMargin:      f.AttackMargin,
		fortifyMargin:     f.FortifyMargin,
	}, nil
}

// nodeFeatureDim mirrors gcn_fit.py's node_feature_dim exactly: counts
// how many "territory_<firstTerritory>_continent_*" columns exist in
// featureNames to infer the continent one-hot's width -- the only piece
// of tdstate.TerritoryFeatures' per-territory stride (is_mine,
// army_fraction, continent one-hot, is_continent_border,
// enemy_threat_fraction) that varies by board.
func nodeFeatureDim(featureNames []string, firstTerritory string) int {
	prefix := fmt.Sprintf("territory_%s_continent_", firstTerritory)
	numContinents := 0
	for _, n := range featureNames {
		if strings.HasPrefix(n, prefix) {
			numContinents++
		}
	}
	return 2 + numContinents + 2
}

// Score runs the forward pass for one board state, given its raw
// (unstandardized) flat feature vector -- the same
// tdstate.Encode(g, pi).Flatten() shape BoardValue.Score consumes, so
// both value functions accept identical input (Model implements
// bot.ValueFunction).
func (m *Model) Score(features []float64) float64 {
	standardized := make([]float64, len(features))
	for i, x := range features {
		std := m.std[i]
		if std == 0 {
			std = 1
		}
		standardized[i] = (x - m.mean[i]) / std
	}

	territoryBlockWidth := m.numNodes * m.nodeDim
	nodeFeatures := make([][]float64, m.numNodes)
	for i := range m.numNodes {
		start := i * m.nodeDim
		nodeFeatures[i] = standardized[start : start+m.nodeDim]
	}
	globalFeatures := standardized[territoryBlockWidth:]

	h1 := applyLayerPerNode(m.gcn1, nodeFeatures)
	h1 = propagate(m.propagationMatrix, h1)
	reluMatrix(h1)

	h2 := applyLayerPerNode(m.gcn2, h1)
	h2 = propagate(m.propagationMatrix, h2)
	reluMatrix(h2)

	boardEmbedding := flattenMatrix(h2)
	fc2Out := applyLayer(m.fc2, boardEmbedding)
	reluVector(fc2Out)

	fc1Out := applyLayer(m.fc1, globalFeatures)
	reluVector(fc1Out)

	combined := make([]float64, 0, len(fc2Out)+len(fc1Out))
	combined = append(combined, fc2Out...)
	combined = append(combined, fc1Out...)

	fc3Out := applyLayer(m.fc3, combined)
	reluVector(fc3Out)

	out := applyLayer(m.output, fc3Out)
	return out[0]
}

// AttackMargin/FortifyMargin return the exported margins, mirroring
// bot.BoardValue's identically-named fields (this model plays the same
// role in bot.ValueStrategy's ValueFunction interface).
func (m *Model) AttackMargin() float64  { return m.attackMargin }
func (m *Model) FortifyMargin() float64 { return m.fortifyMargin }

// applyLayerPerNode applies l to every row of nodeFeatures independently
// (the linear transform half of one graph-conv layer, before
// propagation).
func applyLayerPerNode(l layer, nodeFeatures [][]float64) [][]float64 {
	out := make([][]float64, len(nodeFeatures))
	for i, row := range nodeFeatures {
		out[i] = applyLayer(l, row)
	}
	return out
}

// propagate multiplies p (the precomputed graph-propagation matrix,
// [numNodes][numNodes]) against h ([numNodes][dim]) -- p @ h.
func propagate(p [][]float64, h [][]float64) [][]float64 {
	numNodes := len(h)
	if numNodes == 0 {
		return h
	}
	dim := len(h[0])
	out := make([][]float64, numNodes)
	for i := range numNodes {
		row := make([]float64, dim)
		for j := range numNodes {
			pij := p[i][j]
			if pij == 0 {
				continue
			}
			for k := range dim {
				row[k] += pij * h[j][k]
			}
		}
		out[i] = row
	}
	return out
}

// applyLayer computes l.weight @ x + l.bias (l.weight is [out][in]).
func applyLayer(l layer, x []float64) []float64 {
	out := make([]float64, len(l.weight))
	for o, wRow := range l.weight {
		sum := l.bias[o]
		for i, w := range wRow {
			sum += w * x[i]
		}
		out[o] = sum
	}
	return out
}

// flattenMatrix concatenates every row of h into one vector, in row
// order -- matching gcn_fit.py's h2.reshape(h2.size(0), -1) exactly
// (row-major flatten).
func flattenMatrix(h [][]float64) []float64 {
	var out []float64
	for _, row := range h {
		out = append(out, row...)
	}
	return out
}

func reluVector(x []float64) {
	for i, v := range x {
		if v < 0 {
			x[i] = 0
		}
	}
}

func reluMatrix(h [][]float64) {
	for _, row := range h {
		reluVector(row)
	}
}
