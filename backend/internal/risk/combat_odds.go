package risk

import "sort"

// CombatForecast estimates the outcome of attacking a target down to a
// conclusion: either the defender is eliminated (WinProbability trends
// toward 1) or the attacker is reduced to a single army and must stop
// (WinProbability trends toward 0).
type CombatForecast struct {
	// WinProbability is the probability the attacker eliminates the
	// defender's armies before being reduced to a single army itself.
	WinProbability float64
	// ExpectedAttackerLosses is the expected number of attacker armies lost
	// fighting to a conclusion.
	ExpectedAttackerLosses float64
	// ExpectedDefenderLosses is the expected number of defender armies lost
	// fighting to a conclusion.
	ExpectedDefenderLosses float64
}

// diceOutcome is one possible (attackerLoss, defenderLoss) result of a
// single combat round, with its probability of occurring.
type diceOutcome struct {
	AttackerLoss, DefenderLoss int
	P                          float64
}

// roundDistributions is every roundDistribution(attackerDice,
// defenderDice) result, precomputed once at package init rather than
// recomputed (or even map-cached) on every ForecastAttack call --
// attackerDice/defenderDice only ever take 4 and 3 possible values
// respectively (0-3, 0-2), a fixed domain completely independent of
// army counts, so this is a pure, one-time cost. Indexed
// [attackerDice][defenderDice]; entries for combinations that can never
// actually occur (e.g. defenderDice 0) are left nil/unused.
var roundDistributions = precomputeRoundDistributions()

func precomputeRoundDistributions() [][][]diceOutcome {
	table := make([][][]diceOutcome, 4)
	for ad := 1; ad <= 3; ad++ {
		table[ad] = make([][]diceOutcome, 3)
		for dd := 1; dd <= 2; dd++ {
			table[ad][dd] = roundDistribution(ad, dd)
		}
	}
	return table
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
// Computed via memoized recursion, memo indexed by a dense
// (attackerArmies+1) x (defenderArmies+1) slice rather than a map --
// this function can be called deep inside hot paths (e.g.
// tdstate.Encode, called from the Attack Handler's search) at army
// counts in the hundreds, and a fresh map's per-entry hashing/bucket
// overhead dominated actual cost far more than the state-space size
// did (measured: capping army counts an order of magnitude barely
// moved wall-clock time, while switching this memo from map to slice
// did). No shared/cross-call cache still, by design -- see below.
func ForecastAttack(attackerArmies, defenderArmies int) CombatForecast {
	// Flat, single-allocation memo/reached tables (index a*stride+d)
	// rather than (attackerArmies+1) separate row slices each -- a
	// nested 2D slice's per-row make() calls are themselves a real,
	// measured cost at army counts in the hundreds (hundreds of small
	// heap allocations per ForecastAttack call), on top of the
	// map-to-slice win already made below. reached[i] is true only if
	// memo[i] is live; CombatForecast's zero value (all-zero) is
	// itself a valid forecast (a hopeless attack), so it can't double
	// as its own "not yet computed" sentinel.
	stride := defenderArmies + 1
	memo := make([]CombatForecast, (attackerArmies+1)*stride)
	reached := make([]bool, (attackerArmies+1)*stride)

	var forecast func(a, d int) CombatForecast
	forecast = func(a, d int) CombatForecast {
		if d <= 0 {
			return CombatForecast{WinProbability: 1}
		}
		if a <= 1 {
			return CombatForecast{}
		}
		idx := a*stride + d
		if reached[idx] {
			return memo[idx]
		}

		attackerDice := min(3, a-1)
		defenderDice := min(2, d)
		dist := roundDistributions[attackerDice][defenderDice]

		var win, expA, expD float64
		for _, o := range dist {
			sub := forecast(a-o.AttackerLoss, d-o.DefenderLoss)
			win += o.P * sub.WinProbability
			expA += o.P * (float64(o.AttackerLoss) + sub.ExpectedAttackerLosses)
			expD += o.P * (float64(o.DefenderLoss) + sub.ExpectedDefenderLosses)
		}
		result := CombatForecast{WinProbability: win, ExpectedAttackerLosses: expA, ExpectedDefenderLosses: expD}
		memo[idx] = result
		reached[idx] = true
		return result
	}

	return forecast(attackerArmies, defenderArmies)
}

// roundDistribution enumerates every one of the 6^attackerDice * 6^defenderDice
// possible die combinations for a single round and tallies the resulting
// (attackerLoss, defenderLoss) outcomes into a probability distribution.
// Tallied into a slice indexed by defenderLoss (attackerLoss is always
// compared-defenderLoss, since every one of the compared die pairs
// assigns exactly one loss) rather than a map: ForecastAttack sums over
// this result with floating-point addition, which isn't associative, so
// the iteration order has to be deterministic call to call -- a map's
// range order is randomized per call in Go, which was silently making
// ForecastAttack (and therefore any strategy decision comparing two
// closely-scored candidates) nondeterministic in the last few bits.
func roundDistribution(attackerDice, defenderDice int) []diceOutcome {
	compared := min(attackerDice, defenderDice)
	counts := make([]int, compared+1) // indexed by defenderLoss
	total := 0

	forEachRoll(attackerDice, func(att []int) {
		forEachRoll(defenderDice, func(def []int) {
			total++
			as, ds := sortedDesc(att), sortedDesc(def)
			defenderLoss := 0
			for i := 0; i < compared; i++ {
				if as[i] > ds[i] {
					defenderLoss++
				}
			}
			counts[defenderLoss]++
		})
	})

	out := make([]diceOutcome, 0, compared+1)
	for defenderLoss, c := range counts {
		if c == 0 {
			continue
		}
		out = append(out, diceOutcome{
			AttackerLoss: compared - defenderLoss,
			DefenderLoss: defenderLoss,
			P:            float64(c) / float64(total),
		})
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
