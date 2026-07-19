package tdstate

import "github.com/nmiano1111/global-conquest/backend/internal/risk"

// BoardSchema is a JSON-serializable snapshot of a risk.Board's static
// topology -- the one shared source of truth both Go's inference code and
// Python's training code build an identical graph-propagation matrix
// from, so the two sides can never silently disagree on adjacency. The
// classic board never changes shape at runtime (Board is never mutated by
// any risk.Game action), so this is computed once and reused, not
// recomputed per game state.
type BoardSchema struct {
	// Order lists every territory name in the same canonical order used
	// throughout this package (Features.Territories, Flatten,
	// FeatureNames) -- a node's row index in any feature matrix is its
	// index into this slice.
	Order []string `json:"order"`
	// Edges is every undirected adjacency, deduplicated (each pair listed
	// once, as [i, j] indices into Order with i < j) -- not self-loops;
	// building a graph-propagation matrix (e.g. Kipf & Welling's
	// D^-1/2(A+I)D^-1/2 renormalization) is left to the consumer, which
	// adds the identity term itself.
	Edges [][2]int `json:"edges"`
}

// NewBoardSchema builds board's schema from its Order and Adjacent
// fields. Adjacency is symmetric (risk.Board.Validate checks this), so
// each undirected pair is only ever emitted once, as [i, j] with i < j.
func NewBoardSchema(board risk.Board) BoardSchema {
	index := make(map[risk.Territory]int, len(board.Order))
	for i, t := range board.Order {
		index[t] = i
	}

	order := make([]string, len(board.Order))
	for i, t := range board.Order {
		order[i] = string(t)
	}

	var edges [][2]int
	for _, t := range board.Order {
		i := index[t]
		for neighbor := range board.Adjacent[t] {
			j := index[neighbor]
			if i < j {
				edges = append(edges, [2]int{i, j})
			}
		}
	}

	return BoardSchema{Order: order, Edges: edges}
}
