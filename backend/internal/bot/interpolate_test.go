package bot

import (
	"reflect"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

func TestInterpolatedCountsHandComputedCases(t *testing.T) {
	cases := []struct {
		name      string
		lo, hi, n int
		want      []int
	}{
		{"even spacing across 3 points", 1, 10, 3, []int{1, 6, 10}},
		{"n exceeds range, duplicates collapse", 1, 3, 10, []int{1, 2, 3}},
		{"degenerate range (lo == hi)", 5, 5, 4, []int{5}},
		{"n == 1 returns just lo", 1, 10, 1, []int{1}},
		{"lo > hi returns nil", 5, 1, 3, nil},
		{"n <= 0 returns nil", 1, 10, 0, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := interpolatedCounts(c.lo, c.hi, c.n)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("interpolatedCounts(%d, %d, %d) = %v, want %v", c.lo, c.hi, c.n, got, c.want)
			}
		})
	}
}

func TestOccupyArmyCountsUnlimitedByDefault(t *testing.T) {
	actions := []risk.OccupationAction{{Armies: 1}, {Armies: 2}, {Armies: 3}, {Armies: 4}}
	got := occupyArmyCounts(actions, 0)
	want := []int{1, 2, 3, 4}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("occupyArmyCounts(_, 0) = %v, want %v (unlimited, matching pre-Ga behavior)", got, want)
	}
}

func TestOccupyArmyCountsCapsAndInterpolates(t *testing.T) {
	actions := make([]risk.OccupationAction, 9)
	for i := range actions {
		actions[i] = risk.OccupationAction{Armies: i + 1} // 1..9
	}
	got := occupyArmyCounts(actions, 3)
	want := interpolatedCounts(1, 9, 3)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("occupyArmyCounts(1..9, 3) = %v, want %v (delegates to interpolatedCounts over the legal range)", got, want)
	}
}

func TestFortifyArmyCountsSingleCandidateByDefault(t *testing.T) {
	a := risk.FortificationAction{From: "A", To: "B", MaxArmies: 7}
	got := fortifyArmyCounts(a, 0)
	want := []int{7}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("fortifyArmyCounts(MaxArmies=7, 0) = %v, want %v -- zero must mean \"just MaxArmies\", not exhaustive (asymmetric with occupyArmyCounts, see its doc comment)", got, want)
	}
}

func TestFortifyArmyCountsInterpolatesWhenBreadthSet(t *testing.T) {
	a := risk.FortificationAction{From: "A", To: "B", MaxArmies: 9}
	got := fortifyArmyCounts(a, 3)
	want := interpolatedCounts(1, 9, 3)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("fortifyArmyCounts(MaxArmies=9, 3) = %v, want %v (interpolated across [1, MaxArmies])", got, want)
	}
}
