package bot

import (
	"github.com/nmiano1111/global-conquest/backend/internal/risk"
	"github.com/nmiano1111/global-conquest/backend/internal/tdstate"
)

// bestOpponentReply finds defenderIdx's own single best legal attack from
// s (by defenderIdx's greedy 1-ply score against value, the same way
// ValueStrategy.attack would choose for itself), reporting false if
// defenderIdx has no legal attack from s at all -- e.g. the "conquered"
// branch left them owning nothing adjacent to counter-attack with.
func bestOpponentReply(s *risk.Game, defenderIdx int, value ValueFunction) (risk.AttackAction, bool) {
	candidates := risk.LegalAttacks(s, s.Players[defenderIdx].ID)
	if len(candidates) == 0 {
		return risk.AttackAction{}, false
	}

	best := 0
	bestScore := value.Score(attackAfterstateBlend(s, defenderIdx, candidates[0]))
	for i := 1; i < len(candidates); i++ {
		score := value.Score(attackAfterstateBlend(s, defenderIdx, candidates[i]))
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	return candidates[best], true
}

// replyAdjustedScore scores branch state s from pi's perspective after
// rolling one more ply forward: if defenderIdx has a legal attack from s,
// its win-probability-blended outcome is scored instead of s itself; if
// not, s is scored as-is (no reply to roll forward).
func replyAdjustedScore(s *risk.Game, pi, defenderIdx int, value ValueFunction) float64 {
	reply, ok := bestOpponentReply(s, defenderIdx, value)
	if !ok {
		return value.Score(tdstate.Encode(s, pi).Flatten())
	}
	conquered, held, p := attackBranches(s, defenderIdx, reply)
	return value.Score(blendFeatures(conquered, held, pi, p))
}

// lookaheadAttackScore scores candidate attack a from pi's perspective the
// same way attackAfterstateBlend does, but rolls one additional ply
// forward on each branch: does the position this attack creates actually
// survive a's former target owner's own best immediate counter-attack?
// Scoped to that single opponent (rather than every adjacent enemy, a
// real search) as a deliberately cheap validation of whether lookahead
// moves the needle at all before committing to the paper's full
// search-based Attack Handler -- see
// project-docs/bot_player/proposals/GCN_Strategy_Roadmap_with_References.md.
func lookaheadAttackScore(g *risk.Game, pi int, a risk.AttackAction, value ValueFunction) float64 {
	defenderIdx := g.Territories[a.To].Owner
	conquered, held, p := attackBranches(g, pi, a)
	return p*replyAdjustedScore(conquered, pi, defenderIdx, value) + (1-p)*replyAdjustedScore(held, pi, defenderIdx, value)
}
