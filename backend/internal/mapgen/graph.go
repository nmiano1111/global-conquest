package mapgen

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// GraphContinent is one continent of a hand-drawn MapGraph: its display
// name and reinforcement bonus. Membership comes from each GraphTerritory's
// Continent field, not from a list here.
type GraphContinent struct {
	Name  string
	Bonus int
}

// GraphTerritory is one territory of a hand-drawn MapGraph: its name, the
// continent it belongs to, and its canvas position. X/Y are in whatever
// coordinate space the editor uses; FromGraph normalizes them to the same
// [0,1] space Generate's layouts use.
type GraphTerritory struct {
	Name      string
	Continent string
	X         float64
	Y         float64
}

// MapGraph is a fully hand-authored board, as drawn in the admin map
// editor: every territory, continent and adjacency edge given explicitly,
// in contrast to MapSpec, which only gives counts for Generate to fill in.
type MapGraph struct {
	Name        string
	Continents  []GraphContinent
	Territories []GraphTerritory
	Edges       [][2]string
}

const maxTerritories = maxContinents * maxTerritoriesPerCont

// FromGraph builds a validated, playable risk.Board (and matching layout)
// from a hand-drawn graph. Territory order follows graph.Territories. The
// returned definition's Spec is derived from the graph (per-continent
// territory counts and per-continent-pair crossing counts), so a drawn map
// reads the same as a generated one in list views. It returns a wrapped
// ErrInvalidSpec if the graph is malformed: continent or territory counts
// out of range, empty or duplicate names, a territory in an undeclared
// continent, an empty continent, an edge to an unknown territory or to
// itself, or a board that isn't one connected graph.
func FromGraph(graph MapGraph) (MapDefinition, error) {
	if len(graph.Continents) < minContinents || len(graph.Continents) > maxContinents {
		return MapDefinition{}, fmt.Errorf("%w: must have between %d and %d continents", ErrInvalidSpec, minContinents, maxContinents)
	}
	if len(graph.Territories) > maxTerritories {
		return MapDefinition{}, fmt.Errorf("%w: at most %d territories allowed", ErrInvalidSpec, maxTerritories)
	}

	continents := make(map[risk.Continent]risk.ContinentInfo, len(graph.Continents))
	for _, c := range graph.Continents {
		name := strings.TrimSpace(c.Name)
		if name == "" {
			return MapDefinition{}, fmt.Errorf("%w: continent name must not be empty", ErrInvalidSpec)
		}
		if _, dup := continents[risk.Continent(name)]; dup {
			return MapDefinition{}, fmt.Errorf("%w: duplicate continent name %q", ErrInvalidSpec, name)
		}
		if c.Bonus < 0 {
			return MapDefinition{}, fmt.Errorf("%w: continent %q bonus must be non-negative", ErrInvalidSpec, name)
		}
		continents[risk.Continent(name)] = risk.ContinentInfo{Bonus: c.Bonus}
	}

	order := make([]risk.Territory, 0, len(graph.Territories))
	continentOf := make(map[risk.Territory]risk.Continent, len(graph.Territories))
	for _, t := range graph.Territories {
		name := risk.Territory(strings.TrimSpace(t.Name))
		cont := risk.Continent(strings.TrimSpace(t.Continent))
		if name == "" {
			return MapDefinition{}, fmt.Errorf("%w: territory name must not be empty", ErrInvalidSpec)
		}
		if _, dup := continentOf[name]; dup {
			return MapDefinition{}, fmt.Errorf("%w: duplicate territory name %q", ErrInvalidSpec, name)
		}
		info, ok := continents[cont]
		if !ok {
			return MapDefinition{}, fmt.Errorf("%w: territory %q references undeclared continent %q", ErrInvalidSpec, name, cont)
		}
		info.Territories = append(info.Territories, name)
		continents[cont] = info
		continentOf[name] = cont
		order = append(order, name)
	}
	for name, info := range continents {
		if len(info.Territories) < minTerritoriesPerCont || len(info.Territories) > maxTerritoriesPerCont {
			return MapDefinition{}, fmt.Errorf("%w: continent %q must have between %d and %d territories", ErrInvalidSpec, name, minTerritoriesPerCont, maxTerritoriesPerCont)
		}
	}

	adj := make(map[risk.Territory]map[risk.Territory]struct{}, len(order))
	for _, t := range order {
		adj[t] = map[risk.Territory]struct{}{}
	}
	crossings := map[[2]string]int{}
	for _, e := range graph.Edges {
		a, b := risk.Territory(strings.TrimSpace(e[0])), risk.Territory(strings.TrimSpace(e[1]))
		if _, ok := adj[a]; !ok {
			return MapDefinition{}, fmt.Errorf("%w: edge references unknown territory %q", ErrInvalidSpec, a)
		}
		if _, ok := adj[b]; !ok {
			return MapDefinition{}, fmt.Errorf("%w: edge references unknown territory %q", ErrInvalidSpec, b)
		}
		if a == b {
			return MapDefinition{}, fmt.Errorf("%w: territory %q cannot border itself", ErrInvalidSpec, a)
		}
		if _, dup := adj[a][b]; dup {
			continue
		}
		adj[a][b] = struct{}{}
		adj[b][a] = struct{}{}
		if ca, cb := continentOf[a], continentOf[b]; ca != cb {
			crossings[pairKey(string(ca), string(cb))]++
		}
	}

	if !territoriesConnected(order, adj) {
		return MapDefinition{}, fmt.Errorf("%w: every territory must be reachable from every other territory", ErrInvalidSpec)
	}

	board := risk.Board{Continents: continents, Adjacent: adj, Order: order}
	if err := board.Validate(); err != nil {
		return MapDefinition{}, fmt.Errorf("mapgen: drawn board failed validation: %w", err)
	}

	return MapDefinition{
		Board:  board,
		Layout: normalizeLayout(graph.Territories),
		Spec:   deriveSpec(graph, continents, crossings),
	}, nil
}

// territoriesConnected reports whether every territory in order is
// reachable from the first one by following adj.
func territoriesConnected(order []risk.Territory, adj map[risk.Territory]map[risk.Territory]struct{}) bool {
	if len(order) == 0 {
		return true
	}
	visited := map[risk.Territory]struct{}{order[0]: {}}
	queue := []risk.Territory{order[0]}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for next := range adj[cur] {
			if _, ok := visited[next]; ok {
				continue
			}
			visited[next] = struct{}{}
			queue = append(queue, next)
		}
	}
	return len(visited) == len(order)
}

// normalizeLayout maps editor coordinates into [0.03, 0.97] on both axes,
// preserving aspect ratio (the larger extent fills the range, the smaller
// one is centered), matching the bounds computeLayout uses.
func normalizeLayout(territories []GraphTerritory) map[risk.Territory]Coord {
	const minCoord, maxCoord = 0.03, 0.97
	layout := make(map[risk.Territory]Coord, len(territories))
	if len(territories) == 0 {
		return layout
	}
	minX, maxX := territories[0].X, territories[0].X
	minY, maxY := territories[0].Y, territories[0].Y
	for _, t := range territories[1:] {
		minX, maxX = min(minX, t.X), max(maxX, t.X)
		minY, maxY = min(minY, t.Y), max(maxY, t.Y)
	}
	extent := max(maxX-minX, maxY-minY)
	span := maxCoord - minCoord
	for _, t := range territories {
		c := Coord{X: 0.5, Y: 0.5}
		if extent > 0 {
			c.X = minCoord + span*((t.X-minX)+(extent-(maxX-minX))/2)/extent
			c.Y = minCoord + span*((t.Y-minY)+(extent-(maxY-minY))/2)/extent
		}
		layout[risk.Territory(strings.TrimSpace(t.Name))] = c
	}
	return layout
}

func deriveSpec(graph MapGraph, continents map[risk.Continent]risk.ContinentInfo, crossings map[[2]string]int) MapSpec {
	spec := MapSpec{Name: graph.Name}
	for _, c := range graph.Continents {
		name := strings.TrimSpace(c.Name)
		spec.Continents = append(spec.Continents, ContinentSpec{
			Name:           name,
			Bonus:          c.Bonus,
			TerritoryCount: len(continents[risk.Continent(name)].Territories),
		})
	}
	pairs := make([][2]string, 0, len(crossings))
	for p := range crossings {
		pairs = append(pairs, p)
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i][0] != pairs[j][0] {
			return pairs[i][0] < pairs[j][0]
		}
		return pairs[i][1] < pairs[j][1]
	})
	for _, p := range pairs {
		spec.Borders = append(spec.Borders, ContinentBorder{A: risk.Continent(p[0]), B: risk.Continent(p[1]), Crossings: crossings[p]})
	}
	return spec
}
