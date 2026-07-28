package bot

import (
	"sort"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// defaultRisky is the Attack Handler's paper-specified terminal-state
// selection threshold (Section 3.7.1) -- see attack_handler.go's
// SelectTerminalState for what it controls. Used whenever
// SequenceSearcher.Risky is left at its zero value.
const defaultRisky = 0.3

// SinglePlySearcher is ValueStrategy's original, always-validated default
// attack-phase search: score every legal attack independently via the
// single-ply attackAfterstateBlend and pick the highest -- no lookahead,
// no branching. Stateless; this is what ValueStrategy uses whenever
// neither Searcher nor AttackSearchDepth is set.
type SinglePlySearcher struct{}

func (SinglePlySearcher) Search(g *risk.Game, playerID string, pi int, value ValueFunction) (risk.AttackAction, float64, bool) {
	actions := risk.LegalAttacks(g, playerID)
	best := -1
	var bestScore float64
	var bestAction risk.AttackAction
	for i, candidate := range actions {
		score := value.Score(attackAfterstateBlend(g, pi, candidate))
		if best == -1 || score > bestScore {
			best, bestScore, bestAction = i, score, candidate
		}
	}
	if best == -1 {
		return risk.AttackAction{}, 0, false
	}
	return bestAction, bestScore, true
}

// SequenceSearcher explores sequences of up to Depth of the acting
// player's own consecutive attacks (Phase 2/3 of project-docs/bot_player/
// proposals/Search_Integration_Roadmap_with_References.md) -- see Search's
// doc comment for the exact algorithm. Zero-value Depth means "no legal
// attack ever explored"; callers wanting the single-ply default should use
// SinglePlySearcher instead.
type SequenceSearcher struct {
	Depth   int
	Breadth int
	Risky   float64
}

// risky returns ss.Risky, or defaultRisky when ss.Risky is unset (<= 0).
func (ss *SequenceSearcher) risky() float64 {
	if ss.Risky <= 0 {
		return defaultRisky
	}
	return ss.Risky
}

// forecastCache memoizes CombatForecast across an entire sequence-search
// decision (SequenceSearcher.Search and every bestContinuation call it
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
type forecastCache map[[2]int]risk.CombatForecast

func (c forecastCache) forecast(a, d int) risk.CombatForecast {
	key := [2]int{a, d}
	if cached, ok := c[key]; ok {
		return cached
	}
	result := risk.ForecastAttack(a, d)
	c[key] = result
	return result
}

// candidateAttacks returns the actions to explore at one level of the
// sequence search: every legal attack when ss.Breadth <= 0 (Phase 2's
// original, still-tested full-enumeration behavior), or only the top
// ss.Breadth by attackAfterstateBlend score otherwise -- a minimal,
// pulled-forward version of Phase 4's heuristic pruning (see
// SequenceSearcher.Breadth's doc comment for why this became necessary).
// Applied uniformly at every level, including the top, since the top
// level's own branching is exactly as much of the cost problem as any
// deeper level. cache memoizes the CombatForecast each candidate's
// ranking score depends on across the whole decision (see forecastCache).
func (ss *SequenceSearcher) candidateAttacks(g *risk.Game, playerID string, pi int, value ValueFunction, cache forecastCache) []risk.AttackAction {
	actions := risk.LegalAttacks(g, playerID)
	breadth := ss.Breadth
	if breadth <= 0 || len(actions) <= breadth {
		return actions
	}
	scores := make([]float64, len(actions))
	for i, a := range actions {
		scores[i] = value.Score(attackAfterstateBlendWithForecast(g, pi, a, cache.forecast))
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

// Search explores every sequence of up to ss.Depth of the acting
// player's own attacks from g's current attack-phase state (Phase 2/3 of
// Search_Integration_Roadmap_with_References.md), returning the first
// action of the best-scoring sequence found -- the only action ever
// committed to, matching the paper's own design of re-running the whole
// search after every single real attack (Section 3.5.4) rather than
// planning multiple real moves ahead. ok is false only when there is no
// legal attack at all. Satisfies AttackSearcher.
//
// Unlike the removed LookaheadDepth, which only ever followed one
// greedily-picked path per ply, this explores every legal attack at
// every level (see bestContinuation) -- real branching, not a chain of
// single best guesses. No opponent-reply modeling exists here, matching
// the paper exactly (see Search_Integration_Roadmap_with_References.md's
// gap analysis).
func (ss *SequenceSearcher) Search(g *risk.Game, playerID string, pi int, value ValueFunction) (a risk.AttackAction, bestScore float64, ok bool) {
	risky := ss.risky()
	cache := make(forecastCache)
	actions := ss.candidateAttacks(g, playerID, pi, value, cache)
	best := -1
	for i, candidate := range actions {
		outcome := risk.SelectTerminalState(risk.AttackTerminalStates(candidate.SourceArmies, candidate.TargetArmies), risky)
		next := applyTerminalOutcome(g, pi, candidate, outcome, candidate.MaxAttackerDice)
		score := ss.bestContinuation(next, playerID, pi, ss.Depth-1, risky, value, cache)
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
// g by chaining up to depth more of the acting player's own attacks.
// cache is the same forecastCache instance the top-level Search call
// created, shared across every recursive call so the whole decision's
// tree reuses CombatForecast results instead of each node re-deriving
// them. Always includes "stop attacking now" (currentStateScore(value, g,
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
func (ss *SequenceSearcher) bestContinuation(g *risk.Game, playerID string, pi int, depth int, risky float64, value ValueFunction, cache forecastCache) float64 {
	best := currentStateScore(value, g, pi)
	if depth <= 0 {
		return best
	}
	for _, a := range ss.candidateAttacks(g, playerID, pi, value, cache) {
		outcome := risk.SelectTerminalState(risk.AttackTerminalStates(a.SourceArmies, a.TargetArmies), risky)
		next := applyTerminalOutcome(g, pi, a, outcome, a.MaxAttackerDice)
		score := ss.bestContinuation(next, playerID, pi, depth-1, risky, value, cache)
		if score > best {
			best = score
		}
	}
	return best
}
