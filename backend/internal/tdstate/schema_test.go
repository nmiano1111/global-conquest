package tdstate

import (
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

func TestNewBoardSchemaOrderMatchesBoard(t *testing.T) {
	board := risk.ClassicBoard()
	schema := NewBoardSchema(board)

	if len(schema.Order) != len(board.Order) {
		t.Fatalf("Order length = %d, want %d", len(schema.Order), len(board.Order))
	}
	for i, t2 := range board.Order {
		if schema.Order[i] != string(t2) {
			t.Errorf("Order[%d] = %q, want %q", i, schema.Order[i], t2)
		}
	}
}

func TestNewBoardSchemaEdgeCountMatchesAdjacency(t *testing.T) {
	board := risk.ClassicBoard()
	schema := NewBoardSchema(board)

	wantUndirected := 0
	for _, neighbors := range board.Adjacent {
		wantUndirected += len(neighbors)
	}
	wantUndirected /= 2 // every adjacency counted from both sides

	if len(schema.Edges) != wantUndirected {
		t.Errorf("len(Edges) = %d, want %d", len(schema.Edges), wantUndirected)
	}
}

func TestNewBoardSchemaEdgesAreOrderedAndContainKnownAdjacency(t *testing.T) {
	board := risk.ClassicBoard()
	schema := NewBoardSchema(board)

	index := make(map[string]int, len(schema.Order))
	for i, name := range schema.Order {
		index[name] = i
	}
	alaska, kamchatka := index["Alaska"], index["Kamchatka"]

	found := false
	for _, e := range schema.Edges {
		if e[0] >= e[1] {
			t.Fatalf("expected every edge to be ordered i < j, got %v", e)
		}
		if (e[0] == alaska && e[1] == kamchatka) || (e[0] == kamchatka && e[1] == alaska) {
			found = true
		}
	}
	if !found {
		t.Error("expected Alaska-Kamchatka to appear as an edge (they're adjacent on the classic board)")
	}
}

func TestNewBoardSchemaNoDuplicateEdges(t *testing.T) {
	board := risk.ClassicBoard()
	schema := NewBoardSchema(board)

	seen := make(map[[2]int]bool, len(schema.Edges))
	for _, e := range schema.Edges {
		if seen[e] {
			t.Errorf("duplicate edge %v", e)
		}
		seen[e] = true
	}
}
