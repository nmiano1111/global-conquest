package bot

import "sort"

// CombatForecast estimates the outcome of attacking a target down to a
// conclusion: either the defender is eliminated (WinProbability trends
// toward 1) or the attacker is reduced to a single army and must stop
// (WinProbability trends toward 0).
type CombatForecast struct {
	WinProbability         float64
	ExpectedAttackerLosses float64
	ExpectedDefenderLosses float64
}

// diceOutcome is one possible (attackerLoss, defenderLoss) result of a
// single combat round, with its probability of occurring.
type diceOutcome struct {
	AttackerLoss, DefenderLoss int
	P                          float64
}

// ForecastAttack estimates fighting from attackerArmies against
// defenderArmies to a conclusion, assuming the attacker always commits the
// maximum legal dice each round (matching how every bot strategy actually
// attacks — see risk.AttackAction.MaxAttackerDice). It is read-only
// forecasting math in the same spirit as risk/legal_actions.go: it
// duplicates the engine's public combat rules (dice caps, descending sort,
// pairwise comparison, ties favor the defender — see engine.go's Attack)
// without touching actual randomness or engine state; the engine remains
// authoritative for what really happens when dice are rolled.
//
// Computed via memoized recursion over single-round outcome distributions,
// enumerated fresh per call (at most 6 distinct dice-count pairings, each
// at most 7776 die combinations — cheap enough not to need a shared cache,
// which also means no concurrency hazard from multiple games' bot turns
// calling this in parallel).
func ForecastAttack(attackerArmies, defenderArmies int) CombatForecast {
	roundCache := make(map[[2]int][]diceOutcome)
	memo := make(map[[2]int]CombatForecast)

	var forecast func(a, d int) CombatForecast
	forecast = func(a, d int) CombatForecast {
		if d <= 0 {
			return CombatForecast{WinProbability: 1}
		}
		if a <= 1 {
			return CombatForecast{}
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

		var win, expA, expD float64
		for _, o := range dist {
			sub := forecast(a-o.AttackerLoss, d-o.DefenderLoss)
			win += o.P * sub.WinProbability
			expA += o.P * (float64(o.AttackerLoss) + sub.ExpectedAttackerLosses)
			expD += o.P * (float64(o.DefenderLoss) + sub.ExpectedDefenderLosses)
		}
		result := CombatForecast{WinProbability: win, ExpectedAttackerLosses: expA, ExpectedDefenderLosses: expD}
		memo[key] = result
		return result
	}

	return forecast(attackerArmies, defenderArmies)
}

// roundDistribution enumerates every one of the 6^attackerDice * 6^defenderDice
// possible die combinations for a single round and tallies the resulting
// (attackerLoss, defenderLoss) outcomes into a probability distribution.
func roundDistribution(attackerDice, defenderDice int) []diceOutcome {
	compared := min(attackerDice, defenderDice)
	counts := make(map[[2]int]int)
	total := 0

	forEachRoll(attackerDice, func(att []int) {
		forEachRoll(defenderDice, func(def []int) {
			total++
			as, ds := sortedDesc(att), sortedDesc(def)
			var attackerLoss, defenderLoss int
			for i := 0; i < compared; i++ {
				if as[i] > ds[i] {
					defenderLoss++
				} else {
					attackerLoss++
				}
			}
			counts[[2]int{attackerLoss, defenderLoss}]++
		})
	})

	out := make([]diceOutcome, 0, len(counts))
	for k, c := range counts {
		out = append(out, diceOutcome{AttackerLoss: k[0], DefenderLoss: k[1], P: float64(c) / float64(total)})
	}
	return out
}

// forEachRoll calls fn once for every possible outcome of rolling n dice
// (each face 1-6), via odometer-style enumeration.
func forEachRoll(n int, fn func(roll []int)) {
	roll := make([]int, n)
	for i := range roll {
		roll[i] = 1
	}
	for {
		fn(roll)
		i := n - 1
		for i >= 0 {
			roll[i]++
			if roll[i] <= 6 {
				break
			}
			roll[i] = 1
			i--
		}
		if i < 0 {
			return
		}
	}
}

func sortedDesc(vals []int) []int {
	out := append([]int(nil), vals...)
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out
}
