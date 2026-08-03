package mapgen

import (
	"errors"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

func sampleSpec() MapSpec {
	return MapSpec{
		Name: "Test Map",
		Continents: []ContinentSpec{
			{Name: "Redland", Bonus: 3, TerritoryCount: 4},
			{Name: "Blueland", Bonus: 2, TerritoryCount: 3},
			{Name: "Greenland", Bonus: 5, TerritoryCount: 5},
		},
		Borders: []ContinentBorder{
			{A: "Redland", B: "Blueland", Crossings: 2},
			{A: "Blueland", B: "Greenland", Crossings: 1},
		},
	}
}

func TestGenerate_ProducesValidBoard(t *testing.T) {
	def, err := Generate(sampleSpec(), nil)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if err := def.Board.Validate(); err != nil {
		t.Fatalf("generated board failed Validate(): %v", err)
	}
	if len(def.Board.Order) != 4+3+5 {
		t.Fatalf("expected 12 territories, got %d", len(def.Board.Order))
	}
	if got := len(def.Board.Continents["Redland"].Territories); got != 4 {
		t.Errorf("Redland: expected 4 territories, got %d", got)
	}
	if got := def.Board.Continents["Greenland"].Bonus; got != 5 {
		t.Errorf("Greenland: expected bonus 5, got %d", got)
	}

	// Layout: every territory must have a coordinate within bounds.
	for _, t2 := range def.Board.Order {
		c, ok := def.Layout[t2]
		if !ok {
			t.Errorf("territory %q missing layout coordinate", t2)
			continue
		}
		if c.X < 0 || c.X > 1 || c.Y < 0 || c.Y > 1 {
			t.Errorf("territory %q layout out of [0,1] bounds: %+v", t2, c)
		}
	}
}

func TestGenerate_BoardIsFullyConnected(t *testing.T) {
	def, err := Generate(sampleSpec(), nil)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	visited := map[risk.Territory]struct{}{}
	var stack []risk.Territory
	start := def.Board.Order[0]
	stack = append(stack, start)
	visited[start] = struct{}{}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for next := range def.Board.Adjacent[cur] {
			if _, ok := visited[next]; ok {
				continue
			}
			visited[next] = struct{}{}
			stack = append(stack, next)
		}
	}
	if len(visited) != len(def.Board.Order) {
		t.Fatalf("board not fully connected: visited %d of %d territories", len(visited), len(def.Board.Order))
	}
}

func TestGenerate_RespectsBorderCrossingCount(t *testing.T) {
	spec := sampleSpec()
	def, err := Generate(spec, nil)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	redland := def.Board.Continents["Redland"].Territories
	blueland := def.Board.Continents["Blueland"].Territories
	crossings := 0
	for _, r := range redland {
		for _, b := range blueland {
			if def.Board.IsAdjacent(r, b) {
				crossings++
			}
		}
	}
	if crossings != 2 {
		t.Errorf("expected 2 Redland-Blueland crossings, got %d", crossings)
	}
}

func TestGenerate_RejectsInvalidSpecs(t *testing.T) {
	cases := []struct {
		name string
		spec MapSpec
	}{
		{"too few continents", MapSpec{Continents: []ContinentSpec{{Name: "A", TerritoryCount: 1}}}},
		{"duplicate continent name", MapSpec{Continents: []ContinentSpec{
			{Name: "A", TerritoryCount: 1}, {Name: "A", TerritoryCount: 1},
		}}},
		{"zero territories", MapSpec{Continents: []ContinentSpec{
			{Name: "A", TerritoryCount: 0}, {Name: "B", TerritoryCount: 1},
		}}},
		{"negative bonus", MapSpec{Continents: []ContinentSpec{
			{Name: "A", Bonus: -1, TerritoryCount: 1}, {Name: "B", TerritoryCount: 1},
		}}},
		{"self border", MapSpec{
			Continents: []ContinentSpec{{Name: "A", TerritoryCount: 2}, {Name: "B", TerritoryCount: 2}},
			Borders:    []ContinentBorder{{A: "A", B: "A", Crossings: 1}},
		}},
		{"undeclared continent border", MapSpec{
			Continents: []ContinentSpec{{Name: "A", TerritoryCount: 2}, {Name: "B", TerritoryCount: 2}},
			Borders:    []ContinentBorder{{A: "A", B: "C", Crossings: 1}},
		}},
		{"duplicate border", MapSpec{
			Continents: []ContinentSpec{{Name: "A", TerritoryCount: 2}, {Name: "B", TerritoryCount: 2}},
			Borders:    []ContinentBorder{{A: "A", B: "B", Crossings: 1}, {A: "B", B: "A", Crossings: 1}},
		}},
		{"zero crossings", MapSpec{
			Continents: []ContinentSpec{{Name: "A", TerritoryCount: 2}, {Name: "B", TerritoryCount: 2}},
			Borders:    []ContinentBorder{{A: "A", B: "B", Crossings: 0}},
		}},
		{"too many crossings", MapSpec{
			Continents: []ContinentSpec{{Name: "A", TerritoryCount: 2}, {Name: "B", TerritoryCount: 2}},
			Borders:    []ContinentBorder{{A: "A", B: "B", Crossings: 5}},
		}},
		{"disconnected continents", MapSpec{
			Continents: []ContinentSpec{
				{Name: "A", TerritoryCount: 2}, {Name: "B", TerritoryCount: 2}, {Name: "C", TerritoryCount: 2},
			},
			Borders: []ContinentBorder{{A: "A", B: "B", Crossings: 1}},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Generate(tc.spec, nil)
			if !errors.Is(err, ErrInvalidSpec) {
				t.Fatalf("expected ErrInvalidSpec, got %v", err)
			}
		})
	}
}

// fixedRNG is a deterministic risk.RNG for tests that need reproducible
// output rather than just "doesn't error".
type fixedRNG struct{ seq []int }

func (r *fixedRNG) IntN(n int) int {
	if len(r.seq) == 0 {
		return 0
	}
	v := r.seq[0] % n
	r.seq = r.seq[1:]
	return v
}

func TestGenerate_NilRNGUsesCryptoDefault(t *testing.T) {
	if _, err := Generate(sampleSpec(), nil); err != nil {
		t.Fatalf("Generate with nil rng returned error: %v", err)
	}
}

func TestGenerate_AcceptsInjectedRNG(t *testing.T) {
	rng := &fixedRNG{seq: make([]int, 200)}
	if _, err := Generate(sampleSpec(), rng); err != nil {
		t.Fatalf("Generate with injected rng returned error: %v", err)
	}
}
