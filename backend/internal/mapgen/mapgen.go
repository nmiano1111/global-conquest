// Package mapgen generates a custom risk.Board (and a matching visual
// layout) from a small, human-authored MapSpec: how many continents, how
// many territories each has, which continents border each other and how
// many territory-to-territory crossings each border has, and each
// continent's reinforcement bonus. It is an authoring tool, not gameplay
// logic — the risk package remains the sole authority for game rules; this
// package only ever produces a risk.Board value that a game can be started
// with, the same way risk.ClassicBoard() does.
package mapgen

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// ErrInvalidSpec is returned by Generate when spec fails validation, with
// a wrapped message describing the first problem found.
var ErrInvalidSpec = errors.New("mapgen: invalid map spec")

// ContinentSpec describes one continent to generate: its display name,
// reinforcement bonus, and how many territories it should contain.
type ContinentSpec struct {
	Name           string
	Bonus          int
	TerritoryCount int
}

// ContinentBorder declares that two continents share a border and how many
// territory-to-territory adjacency edges connect them (Crossings). A and B
// must name continents declared in MapSpec.Continents.
type ContinentBorder struct {
	A, B      risk.Continent
	Crossings int
}

// MapSpec is the admin-authored input to Generate.
type MapSpec struct {
	Name       string
	Continents []ContinentSpec
	Borders    []ContinentBorder
}

// Coord is a territory's position on a normalized [0,1]x[0,1] canvas,
// independent of any particular frontend pixel viewbox.
type Coord struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// MapDefinition is the persisted result of Generate: the playable board,
// an automatic visual layout for it, and the spec it was generated from
// (kept for display and as a basis for future regeneration/editing).
type MapDefinition struct {
	Board  risk.Board               `json:"board"`
	Layout map[risk.Territory]Coord `json:"layout"`
	Spec   MapSpec                  `json:"spec"`
}

const (
	minContinents         = 2
	maxContinents         = 12
	minTerritoriesPerCont = 1
	maxTerritoriesPerCont = 20
)

// Generate builds a validated, playable risk.Board (and matching layout)
// from spec. If rng is nil, a crypto/rand-backed RNG is used. It returns a
// wrapped ErrInvalidSpec if spec is malformed (too few/many continents or
// territories, undeclared or duplicate continent borders, a border whose
// crossing count exceeds the number of possible territory pairs it could
// draw from, or continents that aren't all reachable from one another via
// borders), or a risk.Board validation error in the unexpected case that
// the generated adjacency graph itself is inconsistent.
func Generate(spec MapSpec, rng risk.RNG) (MapDefinition, error) {
	if rng == nil {
		rng = stdRNG{}
	}
	if err := validateSpec(spec); err != nil {
		return MapDefinition{}, err
	}

	continents := make(map[risk.Continent]risk.ContinentInfo, len(spec.Continents))
	terrsBySpecIndex := make([][]risk.Territory, len(spec.Continents))
	var order []risk.Territory
	for ci, cs := range spec.Continents {
		name := risk.Continent(cs.Name)
		terrs := make([]risk.Territory, cs.TerritoryCount)
		for i := 0; i < cs.TerritoryCount; i++ {
			terrs[i] = risk.Territory(fmt.Sprintf("%s %d", cs.Name, i+1))
		}
		continents[name] = risk.ContinentInfo{Bonus: cs.Bonus, Territories: terrs}
		terrsBySpecIndex[ci] = terrs
		order = append(order, terrs...)
	}

	adj := make(map[risk.Territory]map[risk.Territory]struct{}, len(order))
	for _, t := range order {
		adj[t] = map[risk.Territory]struct{}{}
	}
	edges, positions := generateGeometry(spec, terrsBySpecIndex, rng)
	for _, e := range edges {
		adj[e[0]][e[1]] = struct{}{}
		adj[e[1]][e[0]] = struct{}{}
	}

	board := risk.Board{Continents: continents, Adjacent: adj, Order: order}
	if err := board.Validate(); err != nil {
		return MapDefinition{}, fmt.Errorf("mapgen: generated board failed validation: %w", err)
	}

	return MapDefinition{
		Board:  board,
		Layout: normalizePoints(positions),
		Spec:   spec,
	}, nil
}

func validateSpec(spec MapSpec) error {
	if len(spec.Continents) < minContinents || len(spec.Continents) > maxContinents {
		return fmt.Errorf("%w: must have between %d and %d continents", ErrInvalidSpec, minContinents, maxContinents)
	}
	names := make(map[string]struct{}, len(spec.Continents))
	for _, cs := range spec.Continents {
		if cs.Name == "" {
			return fmt.Errorf("%w: continent name must not be empty", ErrInvalidSpec)
		}
		if _, dup := names[cs.Name]; dup {
			return fmt.Errorf("%w: duplicate continent name %q", ErrInvalidSpec, cs.Name)
		}
		names[cs.Name] = struct{}{}
		if cs.TerritoryCount < minTerritoriesPerCont || cs.TerritoryCount > maxTerritoriesPerCont {
			return fmt.Errorf("%w: continent %q must have between %d and %d territories", ErrInvalidSpec, cs.Name, minTerritoriesPerCont, maxTerritoriesPerCont)
		}
		if cs.Bonus < 0 {
			return fmt.Errorf("%w: continent %q bonus must be non-negative", ErrInvalidSpec, cs.Name)
		}
	}

	territoryCount := make(map[string]int, len(spec.Continents))
	for _, cs := range spec.Continents {
		territoryCount[cs.Name] = cs.TerritoryCount
	}

	seenPairs := make(map[[2]string]struct{}, len(spec.Borders))
	for _, b := range spec.Borders {
		if b.A == b.B {
			return fmt.Errorf("%w: continent %q cannot border itself", ErrInvalidSpec, b.A)
		}
		countA, okA := territoryCount[string(b.A)]
		countB, okB := territoryCount[string(b.B)]
		if !okA || !okB {
			return fmt.Errorf("%w: border references undeclared continent (%q, %q)", ErrInvalidSpec, b.A, b.B)
		}
		pair := pairKey(string(b.A), string(b.B))
		if _, dup := seenPairs[pair]; dup {
			return fmt.Errorf("%w: duplicate border between %q and %q", ErrInvalidSpec, b.A, b.B)
		}
		seenPairs[pair] = struct{}{}
		if b.Crossings < 1 {
			return fmt.Errorf("%w: border between %q and %q must have at least 1 crossing", ErrInvalidSpec, b.A, b.B)
		}
		if b.Crossings > countA*countB {
			return fmt.Errorf("%w: border between %q and %q requests %d crossings but only %d distinct territory pairs exist", ErrInvalidSpec, b.A, b.B, b.Crossings, countA*countB)
		}
	}

	if !continentsConnected(spec) {
		return fmt.Errorf("%w: every continent must be reachable from every other continent via borders", ErrInvalidSpec)
	}
	return nil
}

func pairKey(a, b string) [2]string {
	if a < b {
		return [2]string{a, b}
	}
	return [2]string{b, a}
}

// continentsConnected reports whether every continent in spec is reachable
// from every other continent by following ContinentBorder edges.
func continentsConnected(spec MapSpec) bool {
	if len(spec.Continents) == 0 {
		return true
	}
	adjacency := make(map[string][]string, len(spec.Continents))
	for _, cs := range spec.Continents {
		adjacency[cs.Name] = nil
	}
	for _, b := range spec.Borders {
		adjacency[string(b.A)] = append(adjacency[string(b.A)], string(b.B))
		adjacency[string(b.B)] = append(adjacency[string(b.B)], string(b.A))
	}

	visited := map[string]struct{}{spec.Continents[0].Name: {}}
	queue := []string{spec.Continents[0].Name}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, next := range adjacency[cur] {
			if _, ok := visited[next]; ok {
				continue
			}
			visited[next] = struct{}{}
			queue = append(queue, next)
		}
	}
	return len(visited) == len(spec.Continents)
}

type stdRNG struct{}

// IntN returns a cryptographically random integer in [0, n), mirroring
// risk's own unexported default RNG (which this package cannot reuse since
// it isn't exported) so Generate(spec, nil) is usable outside of tests.
func (stdRNG) IntN(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		panic(fmt.Sprintf("mapgen: crypto/rand unavailable: %v", err))
	}
	return int(v.Int64())
}
