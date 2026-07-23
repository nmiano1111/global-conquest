package bot

import "sort"

// TerminalState is one possible fight-to-a-conclusion outcome of an
// attack: either the defender is eliminated (DefenderRemaining == 0) or
// the attacker is reduced to a single army and must stop
// (AttackerRemaining == 1).
type TerminalState struct {
	AttackerRemaining int
	DefenderRemaining int
	Probability       float64
}

// AttackTerminalStates computes the full probability distribution over
// every way fighting attackerArmies against defenderArmies to a
// conclusion could end -- the Attack Handler's tree G/R from the paper
// (Section 3.3), generalizing ForecastAttack's identical recursive walk
// of roundDistribution from a scalar collapse (win probability, expected
// losses) to the full distribution over terminal states. Terminal
// condition matches the engine's actual rules (not the paper's own
// slightly different a=0 abstraction): d <= 0 (defender eliminated) or
// a <= 1 (attacker can't legally continue, matching risk.LegalAttacks'
// >1 army requirement and ForecastAttack's identical base case).
//
// Computed via a forward dynamic-programming sweep over the (a, d)
// grid, O(attackerArmies * defenderArmies) time and space -- not
// backward recursion with a full terminal-state list memoized at every
// internal (a, d) node, which this replaced after a full-game/
// multi-game memory diagnostic found that approach spiking to hundreds
// of MB (in a single call, not a leak across calls) once a real game
// let armies pile into the hundreds on one border territory: every
// internal node's memoized list can itself be O(a+d) long, so total
// memory was roughly O(a*d*(a+d)) -- invisible at the small hand-picked
// army counts unit tests use, explosive at realistic ones. Every
// non-terminal state strictly decreases a+d each round (compared =
// min(attackerDice, defenderDice) >= 1 whenever a>1 and d>0), so
// processing (a, d) states in decreasing order of a+d is a valid
// topological order: everything that can contribute to a state has
// already been added by the time that state is processed.
//
// Returned in canonical worst-for-attacker -> best-for-attacker order
// (see sortTerminalStates), matching the paper's own R ordering so
// SelectTerminalState can walk the result directly with no separate
// sort step.
func AttackTerminalStates(attackerArmies, defenderArmies int) []TerminalState {
	// Preserves the exact base-case priority defenderArmies<=0 checked
	// before attackerArmies<=1, matching how the recursion this replaced
	// resolved degenerate top-level inputs (never actually reached via
	// risk.LegalAttacks-derived candidates in practice, but kept for a
	// stable contract).
	if defenderArmies <= 0 {
		return []TerminalState{{AttackerRemaining: attackerArmies, DefenderRemaining: 0, Probability: 1}}
	}
	if attackerArmies <= 1 {
		return []TerminalState{{AttackerRemaining: attackerArmies, DefenderRemaining: defenderArmies, Probability: 1}}
	}

	a0, d0 := attackerArmies, defenderArmies
	prob := make([][]float64, a0+1) // prob[a][d]: probability mass currently at (a, d)
	for i := range prob {
		prob[i] = make([]float64, d0+1)
	}
	prob[a0][d0] = 1

	roundCache := make(map[[2]int][]diceOutcome)
	terminal := make(map[[2]int]float64) // keyed by (finalAttacker, finalDefender)

	for sum := a0 + d0; sum >= 2; sum-- {
		for a := 1; a <= a0 && a <= sum; a++ {
			d := sum - a
			if d < 0 || d > d0 {
				continue
			}
			p := prob[a][d]
			if p == 0 {
				continue
			}
			if d == 0 {
				terminal[[2]int{a, 0}] += p
				continue
			}
			if a == 1 {
				terminal[[2]int{1, d}] += p
				continue
			}

			attackerDice, defenderDice := min(3, a-1), min(2, d)
			key := [2]int{attackerDice, defenderDice}
			dist, ok := roundCache[key]
			if !ok {
				dist = roundDistribution(attackerDice, defenderDice)
				roundCache[key] = dist
			}
			for _, o := range dist {
				prob[a-o.AttackerLoss][d-o.DefenderLoss] += p * o.P
			}
		}
	}

	out := make([]TerminalState, 0, len(terminal))
	for k, p := range terminal {
		out = append(out, TerminalState{AttackerRemaining: k[0], DefenderRemaining: k[1], Probability: p})
	}
	return sortTerminalStates(out)
}

// sortTerminalStates orders terminal states worst-for-attacker (best-
// for-defender) to best-for-attacker, matching the paper's R ordering:
// every "attacker-stopped" state (DefenderRemaining > 0) sorts before
// every "attacker-won" state (DefenderRemaining == 0) -- any loss is
// worse for the attacker than any win -- ordered worst -> least-bad by
// DefenderRemaining descending within the stopped group, then barest ->
// most-convincing win by AttackerRemaining ascending within the won
// group.
func sortTerminalStates(states []TerminalState) []TerminalState {
	sort.SliceStable(states, func(i, j int) bool {
		si, sj := states[i], states[j]
		iWon := si.DefenderRemaining == 0
		jWon := sj.DefenderRemaining == 0
		if iWon != jWon {
			return jWon
		}
		if !iWon {
			return si.DefenderRemaining > sj.DefenderRemaining
		}
		return si.AttackerRemaining < sj.AttackerRemaining
	})
	return states
}

// SelectTerminalState picks the terminal state to commit to for
// deterministic search: walk states worst-for-attacker to best-for-
// attacker (states must already be in AttackTerminalStates' order),
// accumulating probability, and commit to the first state where
// cumulative probability reaches risky. A higher risky walks further
// toward attacker-favorable outcomes before committing -- more
// optimistic; the paper's default is 0.3 (see the roadmap doc for why
// that's balanced, not extreme). The last state is always returned as a
// fallback, covering risky >= 1 and floating-point shortfall in the
// cumulative sum.
func SelectTerminalState(states []TerminalState, risky float64) TerminalState {
	cumulative := 0.0
	for _, s := range states {
		cumulative += s.Probability
		if cumulative >= risky {
			return s
		}
	}
	return states[len(states)-1]
}
