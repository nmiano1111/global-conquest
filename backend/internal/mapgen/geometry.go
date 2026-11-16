package mapgen

import (
	"math"
	"sort"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// Geometry-first generation: territories are placed first, inside
// irregular continent blobs arranged so bordering continents sit next to
// each other, and adjacency is then derived from proximity. That way the
// board reads like a map -- neighbors are near each other and borders
// rarely cross -- instead of a random graph drawn on top of a layout that
// has nothing to do with it.
//
// Units: territories are roughly territorySpacing apart; the final layout
// is normalized into [0.03, 0.97] like every other layout.

const (
	territorySpacing = 1.0
	// Blob radius per sqrt(territory): sunflower packing of n points at
	// unit spacing fills a disk of radius ~0.55*sqrt(n); a little slack
	// keeps them from feeling crammed.
	blobRadiusPerSqrtTerritory = 0.62
	// Gap between bordering continents' blob edges, and the larger gap
	// non-bordering continents are pushed apart to.
	borderGap   = 0.6
	strangerGap = 1.6
)

type point struct{ X, Y float64 }

func (p point) sub(q point) point     { return point{p.X - q.X, p.Y - q.Y} }
func (p point) add(q point) point     { return point{p.X + q.X, p.Y + q.Y} }
func (p point) scale(f float64) point { return point{p.X * f, p.Y * f} }
func (p point) dist(q point) float64  { return math.Hypot(p.X-q.X, p.Y-q.Y) }
func (p point) dist2(q point) float64 { d := p.sub(q); return d.X*d.X + d.Y*d.Y }

// cross is the 2D cross product of (a-o) and (b-o). The explicit
// float64 conversions stop the compiler fusing it into an FMA (it does on
// arm64), which would make collinear/shared-endpoint cases come out as
// tiny nonzero values instead of exactly 0.
func cross(o, a, b point) float64 {
	return float64((a.X-o.X)*(b.Y-o.Y)) - float64((a.Y-o.Y)*(b.X-o.X))
}
func randFloat(rng risk.RNG) float64  { return float64(rng.IntN(1<<30)) / float64(1<<30) }
func randSigned(rng risk.RNG) float64 { return 2*randFloat(rng) - 1 }

// blob is one continent's irregular outline: a disk of radius r around c,
// wobbled by two low-frequency harmonics and stretched along angle tilt.
type blob struct {
	c              point
	r              float64
	a2, p2, a3, p3 float64 // harmonic amplitudes and phases
	stretch, tilt  float64
}

func newBlob(territories int, rng risk.RNG) blob {
	return blob{
		r:       blobRadiusPerSqrtTerritory * math.Sqrt(float64(territories)) * territorySpacing,
		a2:      0.08 + 0.12*randFloat(rng),
		p2:      2 * math.Pi * randFloat(rng),
		a3:      0.05 + 0.10*randFloat(rng),
		p3:      2 * math.Pi * randFloat(rng),
		stretch: 1 + 0.3*randFloat(rng),
		tilt:    math.Pi * randFloat(rng),
	}
}

// size is the blob's typical radius, used for spacing continents apart;
// stretched blobs reach further along their long axis, so they count a
// bit larger.
func (b blob) size() float64 { return b.r * (1 + (b.a2+b.a3)/2) * math.Sqrt(b.stretch) }

// at maps a point of the unit disk (radius rho in [0,1], angle theta) onto
// the blob's outline-shaped interior.
func (b blob) at(rho, theta float64) point {
	edge := 1 + b.a2*math.Sin(2*theta+b.p2) + b.a3*math.Sin(3*theta+b.p3)
	x := rho * edge * b.r * math.Cos(theta) * b.stretch
	y := rho * edge * b.r * math.Sin(theta) / b.stretch
	ct, st := math.Cos(b.tilt), math.Sin(b.tilt)
	return point{b.c.X + x*ct - y*st, b.c.Y + x*st + y*ct}
}

// placeContinents arranges blob centers with a small force simulation:
// bordering continents are pulled to just-touching distance, every pair is
// pushed apart if closer than that (further for non-bordering pairs). A
// single run often settles with a third continent wedged between two that
// border each other, so it runs from several starting arrangements and
// keeps the one that scores best (see continentLayoutScore).
func placeContinents(spec MapSpec, blobs []blob, rng risk.RNG) {
	n := len(blobs)
	index := make(map[string]int, n)
	for i, cs := range spec.Continents {
		index[cs.Name] = i
	}
	borders := make(map[[2]int]bool, len(spec.Borders))
	for _, b := range spec.Borders {
		i, j := index[string(b.A)], index[string(b.B)]
		borders[[2]int{min(i, j), max(i, j)}] = true
	}

	var spread float64
	for _, b := range blobs {
		spread += b.size()
	}
	const attempts = 32
	var best []point
	bestScore := math.Inf(1)
	centers := make([]point, n)
	for attempt := range attempts {
		// Attempt 0 is an evenly spaced ring; later ones scatter randomly.
		// The attempt-dependent angle stride keeps starts distinct even
		// when the RNG yields nothing but zeros.
		for i := range centers {
			angle := 2 * math.Pi * float64(i*(attempt+1)) / float64(n)
			radius := spread / math.Pi
			if attempt > 0 {
				angle += 2 * math.Pi * randFloat(rng)
				radius *= 0.3 + randFloat(rng)
			}
			centers[i] = point{radius * math.Cos(angle), radius * math.Sin(angle)}
		}
		relaxContinents(centers, blobs, borders)
		if score := continentLayoutScore(centers, blobs, borders); score < bestScore {
			bestScore = score
			best = append(best[:0], centers...)
		}
	}
	for i := range blobs {
		blobs[i].c = best[i]
	}
}

func continentTargets(blobs []blob, i, j int, bordering bool) (want float64, exact bool) {
	touching := blobs[i].size() + blobs[j].size()
	if bordering {
		return touching + borderGap, true
	}
	return touching + strangerGap, false
}

func relaxContinents(centers []point, blobs []blob, borders map[[2]int]bool) {
	n := len(centers)
	const iterations = 400
	for it := range iterations {
		step := 0.3 * (1 - float64(it)/iterations)
		moves := make([]point, n)
		for i := range n {
			for j := i + 1; j < n; j++ {
				d := centers[j].sub(centers[i])
				dist := math.Max(math.Hypot(d.X, d.Y), 1e-6)
				dir := d.scale(1 / dist)
				if dist < 1e-6 {
					dir = point{math.Cos(float64(i + j)), math.Sin(float64(i + j))}
				}
				want, exact := continentTargets(blobs, i, j, borders[[2]int{i, j}])
				var pull float64 // positive pulls the pair together
				if exact || dist < want {
					pull = dist - want
				}
				moves[i] = moves[i].add(dir.scale(pull / 2))
				moves[j] = moves[j].sub(dir.scale(pull / 2))
			}
		}
		for i := range centers {
			centers[i] = centers[i].add(moves[i].scale(step))
		}
	}
}

// continentLayoutScore is lower for better arrangements: bordering pairs
// at their target distance, nobody overlapping, and -- weighted heavily --
// no continent sitting on the line between two that border each other, and
// no two such lines crossing (either forces border crossings to run long
// and through other continents).
func continentLayoutScore(centers []point, blobs []blob, borders map[[2]int]bool) float64 {
	n := len(centers)
	var score float64
	var links [][2]int
	for i := range n {
		for j := i + 1; j < n; j++ {
			bordering := borders[[2]int{i, j}]
			want, exact := continentTargets(blobs, i, j, bordering)
			if dist := centers[i].dist(centers[j]); exact || dist < want {
				score += (dist - want) * (dist - want)
			}
			if bordering {
				links = append(links, [2]int{i, j})
			}
		}
	}
	const penalty = 1000
	for _, l := range links {
		for k := range n {
			if k != l[0] && k != l[1] && distToSegment(centers[k], centers[l[0]], centers[l[1]]) < blobs[k].size() {
				score += penalty
			}
		}
	}
	for a := range links {
		for b := a + 1; b < len(links); b++ {
			la, lb := links[a], links[b]
			if la[0] == lb[0] || la[0] == lb[1] || la[1] == lb[0] || la[1] == lb[1] {
				continue
			}
			if segmentsCross(centers[la[0]], centers[la[1]], centers[lb[0]], centers[lb[1]]) {
				score += penalty
			}
		}
	}
	return score
}

func distToSegment(p, a, b point) float64 {
	ab := b.sub(a)
	l2 := ab.X*ab.X + ab.Y*ab.Y
	if l2 == 0 {
		return p.dist(a)
	}
	t := math.Max(0, math.Min(1, ((p.X-a.X)*ab.X+(p.Y-a.Y)*ab.Y)/l2))
	return p.dist(a.add(ab.scale(t)))
}

// scatterTerritories fills each blob with evenly spread points: a
// sunflower (Vogel spiral) pattern for even spacing, jittered so it doesn't
// look machine-made. The spiral alone keeps points distinct even when the
// RNG returns nothing but zeros.
func scatterTerritories(blobs []blob, counts []int, rng risk.RNG) [][]point {
	golden := math.Pi * (3 - math.Sqrt(5))
	out := make([][]point, len(blobs))
	for ci, b := range blobs {
		n := counts[ci]
		pts := make([]point, n)
		spin := 2 * math.Pi * randFloat(rng)
		for i := 0; i < n; i++ {
			rho := math.Sqrt((float64(i) + 0.5) / float64(n))
			if n == 1 {
				rho = 0
			}
			p := b.at(rho, float64(i)*golden+spin)
			jitter := 0.22 * territorySpacing
			pts[i] = p.add(point{jitter * randSigned(rng), jitter * randSigned(rng)})
		}
		out[ci] = pts
	}
	relaxPoints(out)
	return out
}

// relaxPoints nudges apart any two territories (in any continents) closer
// than ~0.85 spacing, so cards and dots never pile up.
func relaxPoints(groups [][]point) {
	minDist := 0.85 * territorySpacing
	for it := 0; it < 40; it++ {
		moved := false
		for gi := range groups {
			for i := range groups[gi] {
				for gj := gi; gj < len(groups); gj++ {
					start := 0
					if gj == gi {
						start = i + 1
					}
					for j := start; j < len(groups[gj]); j++ {
						a, b := groups[gi][i], groups[gj][j]
						d := a.dist(b)
						if d >= minDist {
							continue
						}
						dir := point{1, 0}
						if d > 1e-9 {
							dir = b.sub(a).scale(1 / d)
						}
						shift := dir.scale((minDist - d) / 2)
						groups[gi][i] = a.sub(shift)
						groups[gj][j] = b.add(shift)
						moved = true
					}
				}
			}
		}
		if !moved {
			return
		}
	}
}

// gabrielEdges returns the index pairs of the Gabriel graph of pts: a pair
// is joined when no other point lies inside the circle having that pair as
// diameter. It is planar and links each point to its natural neighbors,
// which is what makes the board read like a map.
func gabrielEdges(pts []point) map[[2]int]bool {
	n := len(pts)
	edges := map[[2]int]bool{}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			mid := pts[i].add(pts[j]).scale(0.5)
			r2 := pts[i].dist2(pts[j]) / 4
			blocked := false
			for k := 0; k < n && !blocked; k++ {
				if k != i && k != j && mid.dist2(pts[k]) < r2*(1-1e-9) {
					blocked = true
				}
			}
			if !blocked {
				edges[[2]int{i, j}] = true
			}
		}
	}
	return edges
}

// linkComponents returns the extra edges that join pts into one connected
// piece given its existing edges: repeatedly the shortest link between two
// pieces that crosses no segment in existing, or simply the shortest link
// when every option crosses something.
func linkComponents(pts []point, edges map[[2]int]bool, existing [][2]point) [][2]int {
	n := len(pts)
	comp := make([]int, n)
	for i := range comp {
		comp[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		if comp[i] != i {
			comp[i] = find(comp[i])
		}
		return comp[i]
	}
	for e := range edges {
		comp[find(e[0])] = find(e[1])
	}
	var added [][2]int
	for {
		bestClear, bestAny := [2]int{-1, -1}, [2]int{-1, -1}
		dClear, dAny := math.Inf(1), math.Inf(1)
		for i := range n {
			for j := i + 1; j < n; j++ {
				if find(i) == find(j) {
					continue
				}
				d := pts[i].dist2(pts[j])
				if d < dAny {
					dAny, bestAny = d, [2]int{i, j}
				}
				if d >= dClear {
					continue
				}
				crosses := false
				for _, seg := range existing {
					if segmentsCross(pts[i], pts[j], seg[0], seg[1]) {
						crosses = true
						break
					}
				}
				if !crosses {
					dClear, bestClear = d, [2]int{i, j}
				}
			}
		}
		if bestAny[0] < 0 {
			return added
		}
		e := bestAny
		if bestClear[0] >= 0 {
			e = bestClear
		}
		comp[find(e[0])] = find(e[1])
		added = append(added, e)
		existing = append(existing, [2]point{pts[e[0]], pts[e[1]]})
	}
}

// segmentsCross reports whether segments ab and cd properly intersect.
// Segments that merely share an endpoint don't count.
func segmentsCross(a, b, c, d point) bool {
	if a == c || a == d || b == c || b == d {
		return false
	}
	d1, d2 := cross(c, d, a), cross(c, d, b)
	d3, d4 := cross(a, b, c), cross(a, b, d)
	return ((d1 > 0) != (d2 > 0)) && ((d3 > 0) != (d4 > 0)) && d1 != 0 && d2 != 0 && d3 != 0 && d4 != 0
}

// chooseCrossings picks exactly k distinct (a in A, b in B) pairs:
// natural neighbors across the two continents (natural, keyed by A index
// then B index) first, then shortest first. It prefers pairs whose line
// crosses no existing border and passes over no other territory, and
// spreads them across different territories, relaxing those preferences
// in stages so k is always met (validateSpec guarantees k <= |A|*|B|).
func chooseCrossings(ptsA, ptsB []point, k int, existing [][2]point, all []point, natural map[[2]int]bool) [][2]int {
	type cand struct {
		i, j    int
		d       float64
		natural bool
	}
	cands := make([]cand, 0, len(ptsA)*len(ptsB))
	for i := range ptsA {
		for j := range ptsB {
			cands = append(cands, cand{i, j, ptsA[i].dist2(ptsB[j]), natural[[2]int{i, j}]})
		}
	}
	// Natural neighbors across the two continents first, then by length.
	sort.Slice(cands, func(x, y int) bool {
		if cands[x].natural != cands[y].natural {
			return cands[x].natural
		}
		return cands[x].d < cands[y].d
	})

	chosen := make([][2]int, 0, k)
	taken := map[[2]int]bool{}
	usesA, usesB := map[int]int{}, map[int]int{}
	crossesBorder := func(c cand) bool {
		for _, seg := range existing {
			if segmentsCross(ptsA[c.i], ptsB[c.j], seg[0], seg[1]) {
				return true
			}
		}
		return false
	}
	overTerritory := func(c cand) bool {
		a, b := ptsA[c.i], ptsB[c.j]
		for _, p := range all {
			if p != a && p != b && distToSegment(p, a, b) < 0.4*territorySpacing {
				return true
			}
		}
		return false
	}
	// Each stage relaxes one preference; a crossing border line is the
	// very last resort.
	stages := []struct {
		perTerritory              int
		avoidCross, avoidOverTerr bool
	}{
		{1, true, true},
		{2, true, true},
		{k, true, true},
		{k, true, false},
		{k, false, false},
	}
	for _, st := range stages {
		for _, c := range cands {
			if len(chosen) == k {
				return chosen
			}
			key := [2]int{c.i, c.j}
			if taken[key] || usesA[c.i] >= st.perTerritory || usesB[c.j] >= st.perTerritory {
				continue
			}
			if (st.avoidCross && crossesBorder(c)) || (st.avoidOverTerr && overTerritory(c)) {
				continue
			}
			taken[key] = true
			usesA[c.i]++
			usesB[c.j]++
			chosen = append(chosen, key)
			existing = append(existing, [2]point{ptsA[c.i], ptsB[c.j]})
		}
	}
	return chosen
}

// buildGeometricBoard lays out every territory and derives adjacency from
// the layout. terrs[i] are continent i's territories (spec order). It
// returns the edges via addEdge and the raw (unnormalized) positions.
func buildGeometricBoard(spec MapSpec, terrs [][]risk.Territory, rng risk.RNG, addEdge func(a, b risk.Territory)) map[risk.Territory]point {
	blobs := make([]blob, len(spec.Continents))
	counts := make([]int, len(spec.Continents))
	index := make(map[string]int, len(spec.Continents))
	for i, cs := range spec.Continents {
		blobs[i] = newBlob(cs.TerritoryCount, rng)
		counts[i] = cs.TerritoryCount
		index[cs.Name] = i
	}
	placeContinents(spec, blobs, rng)
	pts := scatterTerritories(blobs, counts, rng)

	// One Gabriel graph over every territory: its edges within a continent
	// become that continent's borders, and its edges between two bordering
	// continents are the preferred crossings, so the board stays planar
	// wherever the spec allows.
	type ref struct{ c, i int }
	var all []point
	var refs []ref
	for ci := range pts {
		for i, p := range pts[ci] {
			all = append(all, p)
			refs = append(refs, ref{ci, i})
		}
	}
	natural := map[[2]int]map[[2]int]bool{} // continent pair -> index pairs
	intra := make([]map[[2]int]bool, len(pts))
	for ci := range intra {
		intra[ci] = map[[2]int]bool{}
	}
	for e := range gabrielEdges(all) {
		a, b := refs[e[0]], refs[e[1]]
		if a.c == b.c {
			intra[a.c][[2]int{min(a.i, b.i), max(a.i, b.i)}] = true
			continue
		}
		if a.c > b.c {
			a, b = b, a
		}
		key := [2]int{a.c, b.c}
		if natural[key] == nil {
			natural[key] = map[[2]int]bool{}
		}
		natural[key][[2]int{a.i, b.i}] = true
	}

	var segments [][2]point
	for ci := range pts {
		for e := range intra[ci] {
			addEdge(terrs[ci][e[0]], terrs[ci][e[1]])
			segments = append(segments, [2]point{pts[ci][e[0]], pts[ci][e[1]]})
		}
	}
	// Another continent's territories can split this one's Gabriel edges
	// apart; link the pieces back up, preferring links that cross nothing.
	for ci := range pts {
		for _, e := range linkComponents(pts[ci], intra[ci], segments) {
			addEdge(terrs[ci][e[0]], terrs[ci][e[1]])
			segments = append(segments, [2]point{pts[ci][e[0]], pts[ci][e[1]]})
		}
	}
	for _, b := range spec.Borders {
		ia, ib := index[string(b.A)], index[string(b.B)]
		nat := map[[2]int]bool{}
		for pair := range natural[[2]int{min(ia, ib), max(ia, ib)}] {
			if ia < ib {
				nat[pair] = true
			} else {
				nat[[2]int{pair[1], pair[0]}] = true
			}
		}
		for _, pair := range chooseCrossings(pts[ia], pts[ib], b.Crossings, segments, all, nat) {
			addEdge(terrs[ia][pair[0]], terrs[ib][pair[1]])
			segments = append(segments, [2]point{pts[ia][pair[0]], pts[ib][pair[1]]})
		}
	}

	positions := make(map[risk.Territory]point)
	for ci := range pts {
		for i, p := range pts[ci] {
			positions[terrs[ci][i]] = p
		}
	}
	return positions
}

// normalizePoints maps raw positions into [0.03, 0.97] on both axes,
// preserving aspect ratio (the larger extent fills the range, the smaller
// one is centered).
func normalizePoints(raw map[risk.Territory]point) map[risk.Territory]Coord {
	const minCoord, maxCoord = 0.03, 0.97
	layout := make(map[risk.Territory]Coord, len(raw))
	first := true
	var minX, maxX, minY, maxY float64
	for _, p := range raw {
		if first {
			minX, maxX, minY, maxY = p.X, p.X, p.Y, p.Y
			first = false
			continue
		}
		minX, maxX = min(minX, p.X), max(maxX, p.X)
		minY, maxY = min(minY, p.Y), max(maxY, p.Y)
	}
	extent := max(maxX-minX, maxY-minY)
	span := maxCoord - minCoord
	for name, p := range raw {
		c := Coord{X: 0.5, Y: 0.5}
		if extent > 0 {
			c.X = minCoord + span*((p.X-minX)+(extent-(maxX-minX))/2)/extent
			c.Y = minCoord + span*((p.Y-minY)+(extent-(maxY-minY))/2)/extent
		}
		layout[name] = c
	}
	return layout
}

// maxGeometryAttempts bounds how many layouts generateGeometry tries.
// Roughly 3% of classic-sized layouts end with a crossing border line
// (when the continent arrangement leaves no clean option), so a handful
// of attempts makes that vanishingly rare.
const maxGeometryAttempts = 8

// generateGeometry runs buildGeometricBoard until it produces a board with
// no crossing border lines (or attempts run out), returning the attempt
// with the fewest crossings.
func generateGeometry(spec MapSpec, terrs [][]risk.Territory, rng risk.RNG) ([][2]risk.Territory, map[risk.Territory]point) {
	var bestEdges [][2]risk.Territory
	var bestPositions map[risk.Territory]point
	bestCrossings := -1
	for range maxGeometryAttempts {
		var edges [][2]risk.Territory
		positions := buildGeometricBoard(spec, terrs, rng, func(a, b risk.Territory) {
			edges = append(edges, [2]risk.Territory{a, b})
		})
		crossings := 0
		for i := range edges {
			for j := i + 1; j < len(edges); j++ {
				a, b := edges[i], edges[j]
				if segmentsCross(positions[a[0]], positions[a[1]], positions[b[0]], positions[b[1]]) {
					crossings++
				}
			}
		}
		if bestCrossings < 0 || crossings < bestCrossings {
			bestEdges, bestPositions, bestCrossings = edges, positions, crossings
		}
		if crossings == 0 {
			break
		}
	}
	return bestEdges, bestPositions
}
