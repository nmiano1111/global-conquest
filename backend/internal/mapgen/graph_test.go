package mapgen

import (
	"errors"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// sampleGraph is two continents joined by a single crossing:
// A1-A2-A3 in Redland, B1-B2 in Blueland, A3-B1 across the border.
func sampleGraph() MapGraph {
	return MapGraph{
		Name: "Drawn Map",
		Continents: []GraphContinent{
			{Name: "Redland", Bonus: 3},
			{Name: "Blueland", Bonus: 2},
		},
		Territories: []GraphTerritory{
			{Name: "A1", Continent: "Redland", X: 0, Y: 0},
			{Name: "A2", Continent: "Redland", X: 100, Y: 0},
			{Name: "A3", Continent: "Redland", X: 200, Y: 50},
			{Name: "B1", Continent: "Blueland", X: 300, Y: 50},
			{Name: "B2", Continent: "Blueland", X: 400, Y: 100},
		},
		Edges: [][2]string{{"A1", "A2"}, {"A2", "A3"}, {"A3", "B1"}, {"B1", "B2"}},
	}
}

func TestFromGraph_ProducesValidBoard(t *testing.T) {
	def, err := FromGraph(sampleGraph())
	if err != nil {
		t.Fatalf("FromGraph returned error: %v", err)
	}
	if err := def.Board.Validate(); err != nil {
		t.Fatalf("board failed Validate(): %v", err)
	}
	wantOrder := []risk.Territory{"A1", "A2", "A3", "B1", "B2"}
	if len(def.Board.Order) != len(wantOrder) {
		t.Fatalf("order: got %v, want %v", def.Board.Order, wantOrder)
	}
	for i, terr := range wantOrder {
		if def.Board.Order[i] != terr {
			t.Fatalf("order: got %v, want %v", def.Board.Order, wantOrder)
		}
	}
	if got := def.Board.Continents["Redland"]; got.Bonus != 3 || len(got.Territories) != 3 {
		t.Errorf("Redland: got %+v", got)
	}
	if !def.Board.IsAdjacent("A3", "B1") || !def.Board.IsAdjacent("B1", "A3") {
		t.Error("expected symmetric A3-B1 adjacency")
	}
	if def.Board.IsAdjacent("A1", "B2") {
		t.Error("unexpected A1-B2 adjacency")
	}
}

func TestFromGraph_DerivesSpec(t *testing.T) {
	def, err := FromGraph(sampleGraph())
	if err != nil {
		t.Fatalf("FromGraph returned error: %v", err)
	}
	if len(def.Spec.Continents) != 2 || def.Spec.Continents[0].TerritoryCount != 3 || def.Spec.Continents[1].TerritoryCount != 2 {
		t.Errorf("spec continents: got %+v", def.Spec.Continents)
	}
	if len(def.Spec.Borders) != 1 || def.Spec.Borders[0].Crossings != 1 {
		t.Errorf("spec borders: got %+v", def.Spec.Borders)
	}
}

func TestFromGraph_NormalizesLayoutPreservingAspect(t *testing.T) {
	def, err := FromGraph(sampleGraph())
	if err != nil {
		t.Fatalf("FromGraph returned error: %v", err)
	}
	for terr, c := range def.Layout {
		if c.X < 0.03-1e-9 || c.X > 0.97+1e-9 || c.Y < 0.03-1e-9 || c.Y > 0.97+1e-9 {
			t.Errorf("%s: coord %+v outside [0.03, 0.97]", terr, c)
		}
	}
	// X spans 400, Y spans 100: X fills the range, Y is centered in it.
	if got := def.Layout["A1"].X; got != 0.03 {
		t.Errorf("A1.X: got %v, want 0.03", got)
	}
	if got := def.Layout["B2"].X; got != 0.97 {
		t.Errorf("B2.X: got %v, want 0.97", got)
	}
	midY := (def.Layout["A1"].Y + def.Layout["B2"].Y) / 2
	if midY < 0.5-1e-9 || midY > 0.5+1e-9 {
		t.Errorf("expected Y centered on 0.5, got midpoint %v", midY)
	}
}

func TestFromGraph_IgnoresDuplicateEdges(t *testing.T) {
	g := sampleGraph()
	g.Edges = append(g.Edges, [2]string{"B1", "A3"}, [2]string{"A3", "B1"})
	def, err := FromGraph(g)
	if err != nil {
		t.Fatalf("FromGraph returned error: %v", err)
	}
	if len(def.Spec.Borders) != 1 || def.Spec.Borders[0].Crossings != 1 {
		t.Errorf("duplicate edges should count once, got %+v", def.Spec.Borders)
	}
}

func TestFromGraph_RejectsInvalidGraphs(t *testing.T) {
	cases := map[string]func(g *MapGraph){
		"too few continents": func(g *MapGraph) {
			g.Continents = g.Continents[:1]
			for i := range g.Territories {
				g.Territories[i].Continent = "Redland"
			}
		},
		"empty continent name":      func(g *MapGraph) { g.Continents[0].Name = " " },
		"duplicate continent":       func(g *MapGraph) { g.Continents[1].Name = "Redland" },
		"negative bonus":            func(g *MapGraph) { g.Continents[0].Bonus = -1 },
		"empty territory name":      func(g *MapGraph) { g.Territories[0].Name = "" },
		"duplicate territory":       func(g *MapGraph) { g.Territories[1].Name = "A1" },
		"undeclared continent":      func(g *MapGraph) { g.Territories[0].Continent = "Nowhere" },
		"empty continent":           func(g *MapGraph) { g.Continents = append(g.Continents, GraphContinent{Name: "Ghostland"}) },
		"edge to unknown territory": func(g *MapGraph) { g.Edges = append(g.Edges, [2]string{"A1", "Z9"}) },
		"self edge":                 func(g *MapGraph) { g.Edges = append(g.Edges, [2]string{"A1", "A1"}) },
		"disconnected":              func(g *MapGraph) { g.Edges = g.Edges[:3] },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			g := sampleGraph()
			mutate(&g)
			if _, err := FromGraph(g); !errors.Is(err, ErrInvalidSpec) {
				t.Fatalf("expected ErrInvalidSpec, got %v", err)
			}
		})
	}
}
