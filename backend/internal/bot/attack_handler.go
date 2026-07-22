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
// Returned in canonical worst-for-attacker -> best-for-attacker order
// (see sortTerminalStates), matching the paper's own R ordering so
// SelectTerminalState can walk the result directly with no separate
// sort step.
func AttackTerminalStates(attackerArmies, defenderArmies int) []TerminalState {
	roundCache := make(map[[2]int][]diceOutcome)
	memo := make(map[[2]int][]TerminalState)

	var walk func(a, d int) []TerminalState
	walk = func(a, d int) []TerminalState {
		if d <= 0 {
			return []TerminalState{{AttackerRemaining: a, DefenderRemaining: 0, Probability: 1}}
		}
		if a <= 1 {
			return []TerminalState{{AttackerRemaining: a, DefenderRemaining: d, Probability: 1}}
		}
		key := [2]int{a, d}
		if cached, ok := memo[key]; ok {
			return cached
		}

		attackerDice := min(3, a-1)
		defenderDice := min(2, d)
		dist, ok := roundCache[[2]int{attackerDice, defenderDice}]
		if !ok {
			dist = roundDistribution(attackerDice, defenderDice)
			roundCache[[2]int{attackerDice, defenderDice}] = dist
		}

		// Merged into a map keyed by (a, d) but accumulated in a
		// first-seen order slice, not map iteration -- same
		// determinism concern as roundDistribution's own comment:
		// floating-point addition isn't associative, so the order
		// terms are summed in has to be stable call to call.
		merged := make(map[[2]int]float64)
		var order [][2]int
		for _, o := range dist {
			for _, sub := range walk(a-o.AttackerLoss, d-o.DefenderLoss) {
				subKey := [2]int{sub.AttackerRemaining, sub.DefenderRemaining}
				if _, seen := merged[subKey]; !seen {
					order = append(order, subKey)
				}
				merged[subKey] += o.P * sub.Probability
			}
		}

		out := make([]TerminalState, 0, len(order))
		for _, k := range order {
			out = append(out, TerminalState{AttackerRemaining: k[0], DefenderRemaining: k[1], Probability: merged[k]})
		}
		out = sortTerminalStates(out)
		memo[key] = out
		return out
	}

	return walk(attackerArmies, defenderArmies)
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
