package bot

import (
	"sort"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// defaultRisky is the Attack Handler's paper-specified terminal-state
// selection threshold (Section 3.7.1) -- see attack_handler.go's
// SelectTerminalState for what it controls. Used whenever
// ValueStrategy.Risky is left at its zero value.
const defaultRisky = 0.3

// risky returns s.Risky, or defaultRisky when s.Risky is unset (<= 0).
func (s *ValueStrategy) risky() float64 {
	if s.Risky <= 0 {
		return defaultRisky
	}
	return s.Risky
}

// forecastCache memoizes CombatForecast across an entire sequence-search
// decision (attackSequenceSearch and every bestContinuation call it
// makes), not just within one ForecastAttack call. candidateAttacks'
// ranking pass calls ForecastAttack once per legal attack at every tree
// node visited (uncapped by breadth -- every legal attack must be
// scored to know the top ones), and once armies reach the scale a long
// game can produce, repeatedly re-deriving the same (a, d) forecast at
// many different nodes becomes a real, measured cost: a depth=3 search's
// ranking passes alone failed to finish within 30 minutes on one real
// decision before this existed. Scoped to a single attack() call (a
// fresh map per call, threaded through by value since map is a
// reference type) -- not shared globally or across games/goroutines,
// so it needs no locking and is bounded by how many distinct (a, d)
// pairs one decision's search tree can visit, not by process lifetime.
type forecastCache map[[2]int]CombatForecast

func (c forecastCache) forecast(a, d int) CombatForecast {
	key := [2]int{a, d}
	if cached, ok := c[key]; ok {
		return cached
	}
	result := ForecastAttack(a, d)
	c[key] = result
	return result
}

// candidateAttacks returns the actions to explore at one level of the
// sequence search: every legal attack when breadth <= 0 (Phase 2's
// original, still-tested full-enumeration behavior), or only the top
// breadth by attackAfterstateBlend score otherwise -- a minimal,
// pulled-forward version of Phase 4's heuristic pruning (see
// ValueStrategy.AttackSearchBreadth's doc comment for why this became
// necessary). Applied uniformly at every level, including the top,
// since the top level's own branching is exactly as much of the cost
// problem as any deeper level. cache memoizes the CombatForecast each
// candidate's ranking score depends on across the whole decision (see
// forecastCache).
func (s *ValueStrategy) candidateAttacks(g *risk.Game, playerID string, pi int, breadth int, cache forecastCache) []risk.AttackAction {
	actions := risk.LegalAttacks(g, playerID)
	if breadth <= 0 || len(actions) <= breadth {
		return actions
	}
	scores := make([]float64, len(actions))
	for i, a := range actions {
		scores[i] = s.value.Score(attackAfterstateBlendWithForecast(g, pi, a, cache.forecast))
	}
	idx := make([]int, len(actions))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(i, j int) bool { return scores[idx[i]] > scores[idx[j]] })

	out := make([]risk.AttackAction, breadth)
	for i := 0; i < breadth; i++ {
		out[i] = actions[idx[i]]
	}
	return out
}

// attackSequenceSearch explores every sequence of up to maxDepth of our
// own attacks from g's current attack-phase state (Phase 2 of
// Search_Integration_Roadmap_with_References.md), returning the first
// action of the best-scoring sequence found -- the only action ever
// committed to, matching the paper's own design of re-running the whole
// search after every single real attack (Section 3.5.4) rather than
// planning multiple real moves ahead. ok is false only when there is no
// legal attack at all.
//
// Unlike the removed LookaheadDepth, which only ever followed one
// greedily-picked path per ply, this explores every legal attack at
// every level (see bestContinuation) -- real branching, not a chain of
// single best guesses.
func (s *ValueStrategy) attackSequenceSearch(g *risk.Game, playerID string, pi int, maxDepth int, risky float64) (a risk.AttackAction, bestScore float64, ok bool) {
	cache := make(forecastCache)
	actions := s.candidateAttacks(g, playerID, pi, s.AttackSearchBreadth, cache)
	best := -1
	for i, candidate := range actions {
		outcome := SelectTerminalState(AttackTerminalStates(candidate.SourceArmies, candidate.TargetArmies), risky)
		next := applyTerminalOutcome(g, pi, candidate, outcome, candidate.MaxAttackerDice)
		score := s.bestContinuation(next, playerID, pi, maxDepth-1, risky, cache)
		if best == -1 || score > bestScore {
			best, bestScore = i, score
		}
	}
	if best == -1 {
		return risk.AttackAction{}, 0, false
	}
	return actions[best], bestScore, true
}

// bestContinuation returns the best achievable leaf score reachable from
// g by chaining up to depth more of our own attacks. cache is the same
// forecastCache instance the top-level attackSequenceSearch call
// created, shared across every recursive call so the whole decision's
// tree reuses CombatForecast results instead of each node re-deriving
// them. Always includes "stop attacking now" (s.currentStateScore(g,
// pi)) as a candidate -- a sequence search must never be forced to keep
// attacking just because it explored further, matching attack()'s
// existing margin-gated
// "does anything beat doing nothing" contract at every level, not just
// the top one.
//
// No pruning: every legal attack at every level is explored (Phase 4 of
// the roadmap adds heuristic pruning later), so runtime grows with
// (legal attacks)^depth -- callers are responsible for keeping depth
// small.
func (s *ValueStrategy) bestContinuation(g *risk.Game, playerID string, pi int, depth int, risky float64, cache forecastCache) float64 {
	best := s.currentStateScore(g, pi)
	if depth <= 0 {
		return best
	}
	for _, a := range s.candidateAttacks(g, playerID, pi, s.AttackSearchBreadth, cache) {
		outcome := SelectTerminalState(AttackTerminalStates(a.SourceArmies, a.TargetArmies), risky)
		next := applyTerminalOutcome(g, pi, a, outcome, a.MaxAttackerDice)
		score := s.bestContinuation(next, playerID, pi, depth-1, risky, cache)
		if score > best {
			best = score
		}
	}
	return best
}
