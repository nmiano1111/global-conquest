package bot

import (
	"context"
	"sort"
	"time"

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

func (SinglePlySearcher) Search(_ context.Context, g *risk.Game, playerID string, pi int, value ValueFunction) (risk.AttackAction, float64, bool) {
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

// candidateAttacks returns the actions to explore at one level of a
// sequence search: every legal attack when breadth <= 0 (Phase 2's
// original, still-tested full-enumeration behavior), or only the top
// breadth by attackAfterstateBlend score otherwise -- a minimal,
// pulled-forward version of Phase 4's heuristic pruning (see
// SequenceSearcher.Breadth's doc comment for why this became necessary).
// Applied uniformly at every level, including the top, since the top
// level's own branching is exactly as much of the cost problem as any
// deeper level. cache memoizes the CombatForecast each candidate's
// ranking score depends on across the whole decision (see forecastCache).
// Package-level (not a method) so both SequenceSearcher and
// AnytimeSearcher (Phase 5) can share it without either owning the
// other's Breadth field.
func candidateAttacks(g *risk.Game, playerID string, pi int, value ValueFunction, breadth int, cache forecastCache) []risk.AttackAction {
	actions := risk.LegalAttacks(g, playerID)
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

// candidateAttacks delegates to the package-level candidateAttacks using
// ss.Breadth -- kept as a method purely for SequenceSearcher's own call
// sites' readability (ss.candidateAttacks(...) vs threading ss.Breadth
// through each call site by hand).
func (ss *SequenceSearcher) candidateAttacks(g *risk.Game, playerID string, pi int, value ValueFunction, cache forecastCache) []risk.AttackAction {
	return candidateAttacks(g, playerID, pi, value, ss.Breadth, cache)
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
func (ss *SequenceSearcher) Search(_ context.Context, g *risk.Game, playerID string, pi int, value ValueFunction) (a risk.AttackAction, bestScore float64, ok bool) {
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

// defaultAttackSearchBudget is the paper's own Search Time (Section
// 3.7.1) -- used whenever AnytimeSearcher.Budget is unset (<= 0). A
// documented starting point carried over verbatim from the source paper,
// same posture as defaultRisky -- expected to be empirically re-tuned
// later (Phase 6), not treated as validated yet.
const defaultAttackSearchBudget = 10 * time.Second

// AnytimeSearcher explores sequences of the acting player's own attacks
// via classic iterative deepening, bounded by a wall-clock Budget instead
// of SequenceSearcher's fixed ply count -- the paper's own "anytime"
// design (Section 3.1 of Search_Integration_Roadmap_with_References.md):
// try depth 1 fully, then depth 2 fully, and so on, keeping the last
// fully completed depth's answer as the real result whenever a deeper
// attempt gets cut off mid-walk by the deadline. Reuses
// candidateAttacks/applyTerminalOutcome/forecastCache exactly like
// SequenceSearcher, but needs its own recursive walk (searchAtDepth/
// bestContinuation below) rather than SequenceSearcher's, since it must
// be interruptible mid-recursion -- checking the deadline only between
// whole-depth attempts isn't tight enough once a single attempt can
// itself run from ~70ms to 90+ seconds depending on depth/breadth (see
// the roadmap doc's own measured costs).
type AnytimeSearcher struct {
	Budget  time.Duration
	Breadth int
	Risky   float64
}

// budget returns as.Budget, or defaultAttackSearchBudget when unset (<=
// 0).
func (as *AnytimeSearcher) budget() time.Duration {
	if as.Budget <= 0 {
		return defaultAttackSearchBudget
	}
	return as.Budget
}

// risky returns as.Risky, or defaultRisky when unset -- identical
// fallback rule to SequenceSearcher.risky().
func (as *AnytimeSearcher) risky() float64 {
	if as.Risky <= 0 {
		return defaultRisky
	}
	return as.Risky
}

// Search runs iterative deepening from depth 1 upward, each attempt a
// full tree walk at exactly that depth (searchAtDepth), until either a
// depth's walk is cut off by the deadline or ctx is already done -- in
// which case the last depth to fully complete is the real answer. ctx is
// consulted exactly once here, at the top (not inside the hot recursive
// loop below, where repeated ctx.Err() calls would add real overhead for
// no benefit over a plain time.Time comparison): an already-expired ctx
// deadline is folded into the effective deadline if tighter than Budget,
// and an already-cancelled ctx short-circuits the loop immediately.
//
// Falls back to SinglePlySearcher's own result if not even depth 1
// completes (an all-but-impossible budget given depth<=1 costs ~70ms per
// the roadmap doc's own measurements) or if depth 1 finds no legal
// attack at all -- SinglePlySearcher independently rediscovers "no legal
// attack" and reports ok=false itself, so this one fallback line handles
// both cases uniformly rather than special-casing either. Satisfies
// AttackSearcher.
func (as *AnytimeSearcher) Search(ctx context.Context, g *risk.Game, playerID string, pi int, value ValueFunction) (risk.AttackAction, float64, bool) {
	deadline := time.Now().Add(as.budget())
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	risky := as.risky()
	cache := make(forecastCache)

	// Computed once, not once per depth iteration below: the root-level
	// candidate list depends only on g/playerID/pi/value/Breadth, none of
	// which change across the depth loop (g itself is never mutated here
	// -- every depth attempt walks from the same starting state), so
	// recomputing it at every iteration was pure waste. It's also
	// typically the single most expensive ranking pass in the whole
	// search: it scores every legal attack (not just the top Breadth) via
	// a full tdstate.Encode + value.Score before pruning, and the root
	// usually has the most legal attacks of any node in the tree, since
	// nothing has been conquered away yet. Deeper candidateAttacks calls
	// inside bestContinuation operate on different, depth-dependent
	// afterstates and are not redundant the same way -- only this one,
	// root-level call is.
	actions := candidateAttacks(g, playerID, pi, value, as.Breadth, cache)

	var bestAction risk.AttackAction
	var bestScore float64
	lastComplete := false

	for depth := 1; ; depth++ {
		if ctx.Err() != nil || time.Now().After(deadline) {
			break
		}
		action, score, ok, complete := as.searchAtDepth(g, playerID, pi, actions, depth, risky, value, cache, deadline)
		if !complete || !ok {
			break
		}
		bestAction, bestScore, lastComplete = action, score, true
	}
	if lastComplete {
		return bestAction, bestScore, true
	}
	return SinglePlySearcher{}.Search(ctx, g, playerID, pi, value)
}

// searchAtDepth attempts one full tree walk at exactly depth, given the
// root-level candidates Search already computed once (see its own
// comment for why) -- otherwise mirrors SequenceSearcher.Search's own
// top-level loop. complete=false means the deadline was hit somewhere
// inside this attempt, so a/bestScore/ok must not be trusted; Search
// keeps whatever the previous, fully completed depth found instead.
func (as *AnytimeSearcher) searchAtDepth(g *risk.Game, playerID string, pi int, actions []risk.AttackAction, depth int, risky float64, value ValueFunction, cache forecastCache, deadline time.Time) (a risk.AttackAction, bestScore float64, ok, complete bool) {
	if time.Now().After(deadline) {
		return risk.AttackAction{}, 0, false, false
	}
	best := -1
	for i, candidate := range actions {
		outcome := risk.SelectTerminalState(risk.AttackTerminalStates(candidate.SourceArmies, candidate.TargetArmies), risky)
		next := applyTerminalOutcome(g, pi, candidate, outcome, candidate.MaxAttackerDice)
		score, complete := as.bestContinuation(next, playerID, pi, depth-1, risky, value, cache, deadline)
		if !complete {
			return risk.AttackAction{}, 0, false, false
		}
		if best == -1 || score > bestScore {
			best, bestScore = i, score
		}
	}
	if best == -1 {
		return risk.AttackAction{}, 0, false, true
	}
	return actions[best], bestScore, true, true
}

// bestContinuation is AnytimeSearcher's interruptible analogue of
// SequenceSearcher.bestContinuation: same recursion shape (every
// candidate at every level, "stop attacking now" always an implicit
// floor via currentStateScore), but checks the deadline once per call
// (not once per candidate, to bound overhead) and short-circuits
// (complete=false) the instant it's exceeded, rather than continuing to
// explore an already-blown budget. A false complete anywhere in the
// recursion propagates straight up unchanged -- the caller must never
// trust the paired score when complete is false.
func (as *AnytimeSearcher) bestContinuation(g *risk.Game, playerID string, pi int, depth int, risky float64, value ValueFunction, cache forecastCache, deadline time.Time) (float64, bool) {
	if time.Now().After(deadline) {
		return 0, false
	}
	best := currentStateScore(value, g, pi)
	if depth <= 0 {
		return best, true
	}
	for _, a := range candidateAttacks(g, playerID, pi, value, as.Breadth, cache) {
		outcome := risk.SelectTerminalState(risk.AttackTerminalStates(a.SourceArmies, a.TargetArmies), risky)
		next := applyTerminalOutcome(g, pi, a, outcome, a.MaxAttackerDice)
		score, complete := as.bestContinuation(next, playerID, pi, depth-1, risky, value, cache, deadline)
		if !complete {
			return 0, false
		}
		if score > best {
			best = score
		}
	}
	return best, true
}
