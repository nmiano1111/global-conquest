package bot

import (
	"math"
	"testing"
)

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}

// TestRoundDistributionThreeVsTwoMatchesPublishedOdds cross-checks the
// brute-force single-round enumeration against the well-published Risk
// combat odds for 3 attacker dice vs 2 defender dice (out of 7776 total
// die combinations): defender loses both ~37.17%, split ~33.58%, attacker
// loses both ~29.26%.
func TestRoundDistributionThreeVsTwoMatchesPublishedOdds(t *testing.T) {
	dist := roundDistribution(3, 2)
	if len(dist) != 3 {
		t.Fatalf("expected 3 possible outcomes, got %d: %+v", len(dist), dist)
	}

	want := map[[2]int]float64{
		{0, 2}: 2890.0 / 7776.0, // attacker loses 0, defender loses 2
		{1, 1}: 2611.0 / 7776.0, // split
		{2, 0}: 2275.0 / 7776.0, // attacker loses 2, defender loses 0
	}
	var totalP float64
	for _, o := range dist {
		key := [2]int{o.AttackerLoss, o.DefenderLoss}
		wantP, ok := want[key]
		if !ok {
			t.Fatalf("unexpected outcome %+v", o)
		}
		if !almostEqual(o.P, wantP) {
			t.Errorf("outcome %v: got P=%v, want %v", key, o.P, wantP)
		}
		totalP += o.P
	}
	if !almostEqual(totalP, 1.0) {
		t.Fatalf("distribution should sum to 1, got %v", totalP)
	}
}

// TestRoundDistributionOneVsOneMatchesHandComputedOdds: with a single die
// each, the only comparison is attacker-die vs defender-die, tie goes to
// the defender. Of 36 equally likely pairs, 15 have the attacker's die
// strictly higher (defender loses), 21 have the defender win-or-tie.
func TestRoundDistributionOneVsOneMatchesHandComputedOdds(t *testing.T) {
	dist := roundDistribution(1, 1)
	got := map[[2]int]float64{}
	for _, o := range dist {
		got[[2]int{o.AttackerLoss, o.DefenderLoss}] = o.P
	}
	if !almostEqual(got[[2]int{0, 1}], 15.0/36.0) {
		t.Errorf("expected defender-loses probability 15/36, got %v", got[[2]int{0, 1}])
	}
	if !almostEqual(got[[2]int{1, 0}], 21.0/36.0) {
		t.Errorf("expected attacker-loses probability 21/36, got %v", got[[2]int{1, 0}])
	}
}

// TestForecastAttackDefenderAlreadyEliminated: an attack against 0
// defending armies is already won.
func TestForecastAttackDefenderAlreadyEliminated(t *testing.T) {
	f := ForecastAttack(5, 0)
	if f.WinProbability != 1 {
		t.Fatalf("expected certain win against 0 defenders, got %v", f.WinProbability)
	}
}

// TestForecastAttackAttackerCannotContinue: an attacker with only 1 army
// cannot legally attack at all, so it can never win.
func TestForecastAttackAttackerCannotContinue(t *testing.T) {
	f := ForecastAttack(1, 3)
	if f.WinProbability != 0 {
		t.Fatalf("expected impossible win with 1 attacking army, got %v", f.WinProbability)
	}
}

// TestForecastAttackTwoVsOneMatchesSingleRoundOdds: 2 attacker armies vs 1
// defender army reduces to exactly one 1-vs-1 round (whatever happens, the
// fight is over: either the defender is eliminated or the attacker is down
// to 1 and must stop) — so WinProbability must equal the single-round
// attacker-wins probability computed above (15/36).
func TestForecastAttackTwoVsOneMatchesSingleRoundOdds(t *testing.T) {
	f := ForecastAttack(2, 1)
	if !almostEqual(f.WinProbability, 15.0/36.0) {
		t.Fatalf("expected win probability 15/36 (~0.4167), got %v", f.WinProbability)
	}
	if !almostEqual(f.ExpectedDefenderLosses, 15.0/36.0) {
		t.Fatalf("expected defender losses 15/36, got %v", f.ExpectedDefenderLosses)
	}
	if !almostEqual(f.ExpectedAttackerLosses, 21.0/36.0) {
		t.Fatalf("expected attacker losses 21/36, got %v", f.ExpectedAttackerLosses)
	}
}

// TestForecastAttackMonotonicInAttackerArmies: holding the defender fixed,
// more attacker armies should never decrease the win probability.
func TestForecastAttackMonotonicInAttackerArmies(t *testing.T) {
	prev := 0.0
	for a := 2; a <= 12; a++ {
		f := ForecastAttack(a, 5)
		if f.WinProbability < prev-1e-9 {
			t.Fatalf("win probability decreased from %v to %v going from a=%d to more armies", prev, f.WinProbability, a)
		}
		prev = f.WinProbability
	}
}

// TestForecastAttackMonotonicInDefenderArmies: holding the attacker fixed,
// more defender armies should never increase the win probability.
func TestForecastAttackMonotonicInDefenderArmies(t *testing.T) {
	prev := 1.0
	for d := 1; d <= 12; d++ {
		f := ForecastAttack(10, d)
		if f.WinProbability > prev+1e-9 {
			t.Fatalf("win probability increased from %v to %v going from d=%d to more defenders", prev, f.WinProbability, d)
		}
		prev = f.WinProbability
	}
}

// TestForecastAttackOverwhelmingAdvantageIsNearCertain sanity-checks a
// large realistic army advantage lands where Risk players expect it to
// (published tables put 20-vs-5 north of 99%).
func TestForecastAttackOverwhelmingAdvantageIsNearCertain(t *testing.T) {
	f := ForecastAttack(20, 5)
	if f.WinProbability < 0.99 {
		t.Fatalf("expected >99%% win probability for a 20-vs-5 attack, got %v", f.WinProbability)
	}
}
