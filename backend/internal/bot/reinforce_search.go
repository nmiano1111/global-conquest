package bot

import (
	"sort"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
	"github.com/nmiano1111/global-conquest/backend/internal/tdstate"
)

// defaultTp and defaultGp are the paper's own values (Section 3.7.1) for
// GroupReinforcer's Tp/Gp, used whenever those fields are left <= 0.
const (
	defaultTp = 2
	defaultGp = 3
)

// SingleBatchReinforcer is ValueStrategy's original, always-validated
// default reinforce-phase search: score every legal territory
// independently and place a single capped batch
// (min(PendingReinforcements, max(1, PendingReinforcements/3))) at the
// top scorer -- no filtering, no grouping, no lookahead. Stateless; this
// is what ValueStrategy uses whenever neither ReinforceSearcher nor
// ReinforceSearchDepth is set.
type SingleBatchReinforcer struct{}

func (SingleBatchReinforcer) Search(g *risk.Game, playerID string, pi int, value ValueFunction) (risk.Territory, int, float64, bool) {
	actions := risk.LegalReinforcements(g, playerID)
	if len(actions) == 0 {
		return "", 0, 0, false
	}
	armies := min(g.PendingReinforcements, max(1, g.PendingReinforcements/3))
	territories := make([]risk.Territory, len(actions))
	for i, a := range actions {
		territories[i] = a.Territory
	}
	best, bestScore := bestReinforceCandidateTerritories(g, playerID, pi, territories, armies, value)
	return territories[best], armies, bestScore, true
}

// GroupReinforcer implements the paper's Tp/Gp placing search (Section
// 3.5.1.1): at each step, restrict candidates to (up to) Tp
// enemy-bordering territories and place a Gp-sized group on whichever
// leads to the best position up to Depth steps ahead, chaining via
// reinforceAfterstate the same way SequenceSearcher chains attacks via
// applyTerminalOutcome. Only the first step's action is ever returned --
// bot.Runner's reload-and-reinvoke loop drives the rest once
// PendingReinforcements is real and reduced. Zero-value Tp/Gp fall back
// to the paper's own defaults via tp()/gp(); Depth <= 0 means no legal
// placement is ever explored, so callers should treat Depth as required.
type GroupReinforcer struct {
	Tp    int
	Gp    int
	Depth int
}

// tp returns gr.Tp, or defaultTp when gr.Tp is unset (<= 0).
func (gr *GroupReinforcer) tp() int {
	if gr.Tp <= 0 {
		return defaultTp
	}
	return gr.Tp
}

// gp returns gr.Gp, or defaultGp when gr.Gp is unset (<= 0).
func (gr *GroupReinforcer) gp() int {
	if gr.Gp <= 0 {
		return defaultGp
	}
	return gr.Gp
}

// Search explores every candidate in candidateReinforcements(gr.tp()),
// placing one Gp-sized group on each and recursing via bestContinuation,
// returning the first step of the best sequence found. Satisfies
// ReinforceSearcher. ok is false only when there is no legal
// reinforcement to consider at all.
func (gr *GroupReinforcer) Search(g *risk.Game, playerID string, pi int, value ValueFunction) (risk.Territory, int, float64, bool) {
	candidates := candidateReinforcements(g, playerID, pi, value, gr.tp())
	if len(candidates) == 0 {
		return "", 0, 0, false
	}
	gp := gr.gp()
	found := false
	var bestScore float64
	var bestTerritory risk.Territory
	var bestArmies int
	for _, t := range candidates {
		armies := min(g.PendingReinforcements, gp)
		after := reinforceAfterstate(g, playerID, t, armies)
		score := gr.bestContinuation(after, playerID, pi, gr.Depth-1, value)
		if !found || score > bestScore {
			found, bestScore, bestTerritory, bestArmies = true, score, t, armies
		}
	}
	return bestTerritory, bestArmies, bestScore, true
}

// bestContinuation returns the best achievable leaf score reachable from
// g by chaining up to depth more Gp-sized reinforcement groups. Unlike
// SequenceSearcher.bestContinuation, there is no "stop now" candidate at
// each branch -- reinforcement placement is mandatory until
// PendingReinforcements reaches zero (risk.PlaceReinforcement enforces
// this, and ValueFunction has no ReinforceMargin to gate against). The
// base case (depth exhausted, or nothing left to place) scores the
// position as-is via currentStateScore -- an exact evaluation once
// PendingReinforcements is genuinely zero, an approximation (search
// cutoff) otherwise.
func (gr *GroupReinforcer) bestContinuation(g *risk.Game, playerID string, pi int, depth int, value ValueFunction) float64 {
	if depth <= 0 || g.PendingReinforcements <= 0 {
		return currentStateScore(value, g, pi)
	}
	candidates := candidateReinforcements(g, playerID, pi, value, gr.tp())
	if len(candidates) == 0 {
		return currentStateScore(value, g, pi)
	}
	gp := gr.gp()
	found := false
	var bestScore float64
	for _, t := range candidates {
		armies := min(g.PendingReinforcements, gp)
		after := reinforceAfterstate(g, playerID, t, armies)
		score := gr.bestContinuation(after, playerID, pi, depth-1, value)
		if !found || score > bestScore {
			found, bestScore = true, score
		}
	}
	return bestScore
}

// candidateReinforcements returns the territories to explore at one step
// of the group search: every legal territory that borders a living enemy
// (adjacentEnemyTerritoryCount, reused as-is from
// strategy_scored_reinforce.go -- no new adjacency logic), falling back
// to every legal territory unfiltered if none border an enemy (should be
// unreachable on a connected board where pi owns a proper subset, but
// cheap insurance against ever returning zero candidates when
// risk.LegalReinforcements itself is non-empty). When the (possibly
// filtered) pool exceeds tp, it's capped to the top tp by a cheap
// one-shot single-group afterstate score (using defaultGp as a relative
// ranking signal, not the real group size -- this is a coarse pre-filter,
// mirroring candidateAttacks' own "cheap pre-rank, then branch deeper
// only on survivors" shape), not the real recursive score.
func candidateReinforcements(g *risk.Game, playerID string, pi int, value ValueFunction, tp int) []risk.Territory {
	actions := risk.LegalReinforcements(g, playerID)
	if len(actions) == 0 {
		return nil
	}
	territories := make([]risk.Territory, len(actions))
	for i, a := range actions {
		territories[i] = a.Territory
	}

	bordering := make([]risk.Territory, 0, len(territories))
	for _, t := range territories {
		if adjacentEnemyTerritoryCount(g, t, pi) > 0 {
			bordering = append(bordering, t)
		}
	}
	if len(bordering) > 0 {
		territories = bordering
	}

	if tp <= 0 || len(territories) <= tp {
		return territories
	}

	armies := min(g.PendingReinforcements, defaultGp)
	scores := make([]float64, len(territories))
	for i, t := range territories {
		after := reinforceAfterstate(g, playerID, t, armies)
		scores[i] = value.Score(tdstate.Encode(after, pi).Flatten())
	}
	idx := make([]int, len(territories))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(i, j int) bool { return scores[idx[i]] > scores[idx[j]] })

	out := make([]risk.Territory, tp)
	for i := 0; i < tp; i++ {
		out[i] = territories[idx[i]]
	}
	return out
}
