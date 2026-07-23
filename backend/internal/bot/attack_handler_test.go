package bot

import (
	"math"
	"testing"
	"time"
)

func sumProbabilities(states []TerminalState) float64 {
	total := 0.0
	for _, s := range states {
		total += s.Probability
	}
	return total
}

func TestAttackTerminalStates_ProbabilitiesSumToOne(t *testing.T) {
	cases := []struct{ a, d int }{
		{2, 1}, {2, 2}, {3, 1}, {3, 3}, {4, 2}, {8, 6}, {20, 15},
	}
	for _, c := range cases {
		states := AttackTerminalStates(c.a, c.d)
		if len(states) == 0 {
			t.Errorf("AttackTerminalStates(%d, %d): got no terminal states", c.a, c.d)
			continue
		}
		total := sumProbabilities(states)
		if math.Abs(total-1) > 1e-9 {
			t.Errorf("AttackTerminalStates(%d, %d): probabilities sum to %v, want 1", c.a, c.d, total)
		}
	}
}

func TestAttackTerminalStates_MatchesForecastAttackWinProbability(t *testing.T) {
	cases := []struct{ a, d int }{
		{2, 1}, {2, 2}, {3, 1}, {3, 3}, {4, 2}, {8, 6}, {20, 15},
	}
	for _, c := range cases {
		states := AttackTerminalStates(c.a, c.d)
		var won float64
		for _, s := range states {
			if s.DefenderRemaining == 0 {
				won += s.Probability
			}
		}
		want := ForecastAttack(c.a, c.d).WinProbability
		if math.Abs(won-want) > 1e-9 {
			t.Errorf("AttackTerminalStates(%d, %d): summed win probability = %v, ForecastAttack says %v", c.a, c.d, won, want)
		}
	}
}

// TestAttackTerminalStates_HandComputed verifies a=2, d=1 against a
// hand-derived distribution: attacker rolls 1 die, defender rolls 1
// die, ties favor the defender. Of 36 equally-likely (attackerRoll,
// defenderRoll) pairs, the attacker's roll is strictly greater in 15
// (attacker wins, defender eliminated) and the attacker's roll is <=
// the defender's in the remaining 21 (attacker loses its one spare army
// and must stop at 1).
func TestAttackTerminalStates_HandComputed(t *testing.T) {
	states := AttackTerminalStates(2, 1)
	if len(states) != 2 {
		t.Fatalf("AttackTerminalStates(2, 1): got %d terminal states, want 2: %+v", len(states), states)
	}

	stopped, won := states[0], states[1]
	if stopped.AttackerRemaining != 1 || stopped.DefenderRemaining != 1 {
		t.Errorf("expected first (worst-for-attacker) state to be (a=1, d=1), got %+v", stopped)
	}
	if math.Abs(stopped.Probability-21.0/36) > 1e-9 {
		t.Errorf("stopped state probability = %v, want 21/36", stopped.Probability)
	}

	if won.AttackerRemaining != 2 || won.DefenderRemaining != 0 {
		t.Errorf("expected second (best-for-attacker) state to be (a=2, d=0), got %+v", won)
	}
	if math.Abs(won.Probability-15.0/36) > 1e-9 {
		t.Errorf("won state probability = %v, want 15/36", won.Probability)
	}
}

// TestAttackTerminalStates_LargeArmiesStayFast is a regression test for
// a real bug: the original backward-recursive implementation memoized a
// full terminal-state list (up to O(a+d) long) at every one of the
// O(a*d) internal (a, d) states it visited, so total memory was roughly
// O(a*d*(a+d)) -- invisible at every other test's small hand-picked
// army counts, but a multi-hundred-MB single-call spike once a real,
// longer game piled armies into the hundreds on one border territory
// (found via a full-game/multi-game memory diagnostic, not a unit
// test). The current forward-DP implementation is O(a*d); this asserts
// it stays fast and correct at army counts a genuinely long game can
// reach.
func TestAttackTerminalStates_LargeArmiesStayFast(t *testing.T) {
	const a, d = 300, 250

	start := time.Now()
	states := AttackTerminalStates(a, d)
	elapsed := time.Since(start)

	if elapsed > time.Second {
		t.Errorf("AttackTerminalStates(%d, %d) took %v, want well under 1s", a, d, elapsed)
	}
	if len(states) == 0 {
		t.Fatalf("AttackTerminalStates(%d, %d): got no terminal states", a, d)
	}
	if total := sumProbabilities(states); math.Abs(total-1) > 1e-9 {
		t.Errorf("AttackTerminalStates(%d, %d): probabilities sum to %v, want 1", a, d, total)
	}

	var won float64
	for _, s := range states {
		if s.DefenderRemaining == 0 {
			won += s.Probability
		}
	}
	if want := ForecastAttack(a, d).WinProbability; math.Abs(won-want) > 1e-9 {
		t.Errorf("AttackTerminalStates(%d, %d): summed win probability = %v, ForecastAttack says %v", a, d, won, want)
	}
}

func TestSelectTerminalState_Monotonicity(t *testing.T) {
	states := AttackTerminalStates(8, 6)

	rankOf := func(s TerminalState) int {
		for i, cand := range states {
			if cand.AttackerRemaining == s.AttackerRemaining && cand.DefenderRemaining == s.DefenderRemaining {
				return i
			}
		}
		t.Fatalf("state %+v not found in distribution", s)
		return -1
	}

	riskyValues := []float64{0.05, 0.1, 0.2, 0.3, 0.4, 0.5, 0.7, 0.9, 0.99}
	prevRank := -1
	for _, risky := range riskyValues {
		selected := SelectTerminalState(states, risky)
		rank := rankOf(selected)
		if rank < prevRank {
			t.Errorf("risky=%v selected a state ranked %d, worse for the attacker than a lower risky's rank %d", risky, rank, prevRank)
		}
		prevRank = rank
	}
}

func TestSelectTerminalState_Edges(t *testing.T) {
	states := AttackTerminalStates(8, 6)

	// A risky at or below the first state's own probability must select
	// the first (worst-for-attacker) state.
	first := SelectTerminalState(states, states[0].Probability/2)
	if first.AttackerRemaining != states[0].AttackerRemaining || first.DefenderRemaining != states[0].DefenderRemaining {
		t.Errorf("risky below first state's probability selected %+v, want %+v", first, states[0])
	}

	// risky=1.0 must select the last (best-for-attacker) state.
	last := SelectTerminalState(states, 1.0)
	wantLast := states[len(states)-1]
	if last.AttackerRemaining != wantLast.AttackerRemaining || last.DefenderRemaining != wantLast.DefenderRemaining {
		t.Errorf("risky=1.0 selected %+v, want %+v", last, wantLast)
	}
}
