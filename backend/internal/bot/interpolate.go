package bot

import (
	"math"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// interpolatedCounts returns up to n integer values linearly spaced across
// [lo, hi] inclusive, ascending, with duplicates from rounding collapsed --
// the paper's "variations, linearly interpolated from the minimum ... and
// the maximum, disregarding duplicates" rule (Section 3.5.1), shared by
// Ga (occupy) and Gf (fortify) so the interpolation math lives in one
// place. n <= 0 or lo > hi returns nil; n == 1 or lo == hi returns [lo].
// The first and last elements of a non-degenerate result are always
// exactly lo and hi.
func interpolatedCounts(lo, hi, n int) []int {
	if n <= 0 || lo > hi {
		return nil
	}
	if n == 1 || lo == hi {
		return []int{lo}
	}
	out := make([]int, 0, n)
	seen := make(map[int]bool, n)
	for i := 0; i < n; i++ {
		frac := float64(i) / float64(n-1)
		v := lo + int(math.Round(frac*float64(hi-lo)))
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// occupyArmyCounts returns the army counts occupy() should score:
// every legal count unchanged when breadth <= 0 or the list already fits
// (today's exact exhaustive behavior, unchanged), otherwise breadth
// counts linearly interpolated across the legal range -- risk.LegalOccupations
// already returns an ascending, consecutive MinMove..MaxMove run, so its
// first/last elements are exactly that range's bounds.
func occupyArmyCounts(actions []risk.OccupationAction, breadth int) []int {
	if breadth <= 0 || len(actions) <= breadth {
		counts := make([]int, len(actions))
		for i, a := range actions {
			counts[i] = a.Armies
		}
		return counts
	}
	return interpolatedCounts(actions[0].Armies, actions[len(actions)-1].Armies, breadth)
}

// fortifyArmyCounts returns the army counts fortify() should score for one
// legal (from, to) pair. Unlike occupyArmyCounts, breadth <= 0 does NOT
// mean "exhaustive" -- risk.LegalFortifications only ever reports one
// count (MaxArmies) per pair, so there is no pre-existing exhaustive
// behavior to preserve; breadth <= 0 reproduces that exact single-candidate
// default. A positive breadth interpolates across [1, MaxArmies].
func fortifyArmyCounts(a risk.FortificationAction, breadth int) []int {
	if breadth <= 0 {
		return []int{a.MaxArmies}
	}
	return interpolatedCounts(1, a.MaxArmies, breadth)
}
