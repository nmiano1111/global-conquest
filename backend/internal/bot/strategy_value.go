package bot

import (
	"context"
	"fmt"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
	"github.com/nmiano1111/global-conquest/backend/internal/tdstate"
)

// StrategyGCNV1 is the registry ID for a live-play GCN-backed
// ValueStrategy (bot.NewBoardValueStrategy over a *gcnmodel.Model) --
// unlike basic-v1/scored-v1/killbot-v1, this one has no fixed weights
// baked into the binary: cmd/backend only registers it when GCN_MODEL_PATH
// is set (see main.go), loading whichever exported gcn_fit.py weights
// file that path points at. Win rate depends heavily on how the model
// was trained/calibrated: naive supervised training plus median-based
// margin calibration stayed at a hard 0% across hundreds of evaluation
// games; TD(lambda) training (gcn_fit.fit_gcn_td) plus --percentile 0
// margin calibration reached a real, reproducible ~17% (12 epochs, see
// models/ for whichever export GCN_MODEL_PATH points at). A deeper
// decision-time lookahead was also tried and made things worse, not
// better (see project-docs/bot_player/proposals/
// Search_Integration_Roadmap_with_References.md) -- removed rather than
// kept as a knob, since it's a validated-negative result, not an
// unproven option. Still registered for local experimentation, not as a
// compiled-in production default.
const StrategyGCNV1 = "gcn-v1"

// ValueStrategy scores candidates by the value of the resulting *board
// state* (via internal/bot's afterstate helpers + internal/tdstate.Encode),
// not local per-candidate features -- see
// project-docs/bot_player/proposals/GCN_Strategy_Roadmap_with_References.md
// and 11_Learned_Board_Evaluation.md. Diagnostic work this project did
// (comparing this whole-board representation against every local
// per-candidate feature set tried) found it discriminates far better
// offline; this Strategy is the first test of whether that translates
// into winning real games.
//
// Generic over ValueFunction: works identically whether the underlying
// model is the linear BoardValue or a gcnmodel.Model (GCN) -- the
// registry ID/CLI flag ("board-value-candidate", --board-value-variant,
// --gcn-variant) names which value function is loaded, not which
// strategy shell runs it, so those stay as-is regardless of the model
// class.
//
// ValueStrategy can score the *current, unmodified* state the same way
// it scores any candidate's afterstate -- so "should I keep
// attacking/fortifying" becomes a comparison against a real baseline
// (does any real candidate beat doing nothing) rather than an arbitrary
// absolute cutoff. That comparison still needs a margin, not just a bare
// "beats it at all" -- see ValueFunction.AttackMargin/FortifyMargin.
type ValueStrategy struct {
	value    ValueFunction
	fallback *BasicStrategy

	// Observer, if non-nil, is called with the raw (bestScore,
	// currentScore) pair computed by attack/fortify before the margin gate
	// is applied -- a purely additive side-channel (same pattern as
	// simulation.Config.OnTurnBoundary) that never influences the
	// decision itself. Used by cmd/bvcalibrate to collect each phase's
	// natural score-delta distribution across many real decisions, in
	// order to fit AttackMargin/FortifyMargin -- not used during normal
	// play.
	Observer func(phase string, bestScore, currentScore float64)

	// AttackSearchDepth, when > 0, replaces attack()'s single-ply
	// attackAfterstateBlend scoring with a real search over sequences of
	// up to this many of our own consecutive attacks (Phase 2 of
	// project-docs/bot_player/proposals/
	// Search_Integration_Roadmap_with_References.md) -- see
	// attack_search.go. Every legal attack is explored at every level
	// (unlike the removed LookaheadDepth, which only ever followed one
	// greedily-picked path), and each attack is materialized via
	// AttackTerminalStates/SelectTerminalState into one concrete
	// deterministic board state, not a probability blend. Zero (the
	// default) keeps the original, already-validated single-ply behavior
	// unchanged.
	AttackSearchDepth int

	// Risky is the Attack Handler's terminal-state selection threshold
	// (paper Section 3.3) -- higher walks further toward
	// attacker-favorable outcomes before committing. Only consulted when
	// AttackSearchDepth > 0. Values <= 0 fall back to the paper's own
	// default, 0.3 (see attack_search.go's risky()).
	Risky float64

	// AttackSearchBreadth, when > 0 and AttackSearchDepth > 0, caps how
	// many top-scoring legal attacks are explored at each level of the
	// sequence search, ranked by the existing single-ply
	// attackAfterstateBlend score -- a minimal, pulled-forward version of
	// Phase 4's heuristic pruning (project-docs/bot_player/proposals/
	// Search_Integration_Roadmap_with_References.md), found necessary in
	// practice: unpruned search at depth >= 2 is too slow to finish
	// enough tournament games inside the default 30s/game budget for a
	// meaningful win-rate sample (measured: depth=2 ~2.5s/decision,
	// depth=3 ~88.6s/decision, vs ~70ms for depth<=1). Zero (the
	// default) means unlimited -- explore every legal attack, matching
	// Phase 2's original, already-tested behavior.
	AttackSearchBreadth int
}

// NewBoardValueStrategy constructs a ValueStrategy from an already-loaded
// ValueFunction (a *BoardValue from LoadBoardValue, or a *gcnmodel.Model),
// falling back to a BasicStrategy for any phase this strategy doesn't
// itself handle (setup_claim -- see ScoredStrategy's identical fallback
// rationale). Named after the registry-facing concept ("board value"
// strategy) rather than the Go type, which stays stable across whichever
// ValueFunction is passed in.
func NewBoardValueStrategy(value ValueFunction) *ValueStrategy {
	return &ValueStrategy{value: value, fallback: NewBasicStrategy()}
}

func (s *ValueStrategy) NextCommand(ctx context.Context, g *risk.Game, playerID string) (Command, Explanation, error) {
	switch g.Phase {
	case risk.PhaseSetupReinforce:
		return s.setupReinforce(g, playerID)
	case risk.PhaseReinforce:
		return s.reinforce(g, playerID)
	case risk.PhaseAttack:
		return s.attack(g, playerID)
	case risk.PhaseOccupy:
		return s.occupy(g, playerID)
	case risk.PhaseFortify:
		return s.fortify(g, playerID)
	default:
		return s.fallback.NextCommand(ctx, g, playerID)
	}
}

// currentStateScore scores g's current, unmodified state from pi's
// perspective -- the "value of doing nothing" baseline attack/fortify
// compare their best real candidate against.
func (s *ValueStrategy) currentStateScore(g *risk.Game, pi int) float64 {
	return s.value.Score(tdstate.Encode(g, pi).Flatten())
}

// attack picks a candidate to attack with, ending the attack phase
// instead when there's no legal attack or the best one doesn't beat the
// current state's own score. When AttackSearchDepth > 0, the candidate
// comes from a real search over sequences of our own attacks
// (attackSequenceSearch, attack_search.go); otherwise (the original,
// already-validated default) every legal attack's afterstate is scored
// independently via the single-ply attackAfterstateBlend and the
// highest picked.
func (s *ValueStrategy) attack(g *risk.Game, playerID string) (Command, Explanation, error) {
	pi := playerIndex(g, playerID)
	currentScore := s.currentStateScore(g, pi)

	var a risk.AttackAction
	best := -1
	var bestScore float64
	if s.AttackSearchDepth > 0 {
		var ok bool
		a, bestScore, ok = s.attackSequenceSearch(g, playerID, pi, s.AttackSearchDepth, s.risky())
		if ok {
			best = 0
		}
	} else {
		actions := risk.LegalAttacks(g, playerID)
		for i, candidate := range actions {
			score := s.value.Score(attackAfterstateBlend(g, pi, candidate))
			if best == -1 || score > bestScore {
				best, bestScore, a = i, score, candidate
			}
		}
	}

	if !s.clearsMargin("attack", best, bestScore, currentScore, s.value.AttackMargin()) {
		return Command{Action: ActionEndAttack}, Explanation{Score: bestScore}, nil
	}
	return Command{
		Action:       ActionAttack,
		From:         string(a.From),
		To:           string(a.To),
		AttackerDice: a.MaxAttackerDice,
	}, Explanation{Score: bestScore}, nil
}

// clearsMargin reports whether attack/fortify should act on the best
// candidate found (best != -1) rather than end the phase: its score must
// exceed currentScore by more than margin. Reports to Observer first
// (only when a real candidate existed at all) so a calibration pass
// observes every phase's natural score delta before this gate decides
// whether to act on it.
func (s *ValueStrategy) clearsMargin(phase string, best int, bestScore, currentScore, margin float64) bool {
	if best == -1 {
		return false
	}
	if s.Observer != nil {
		s.Observer(phase, bestScore, currentScore)
	}
	return bestScore > currentScore+margin
}

// reinforce decides card timing first (scoredCardTurnIn, shared with
// ScoredStrategy -- card-timing policy doesn't depend on any
// weights/value function), then scores every legal reinforcement
// territory's afterstate and places a capped batch at the top scorer --
// same batching rule as ScoredStrategy.reinforce.
func (s *ValueStrategy) reinforce(g *risk.Game, playerID string) (Command, Explanation, error) {
	if cmd, expl, ok := scoredCardTurnIn(g, playerID); ok {
		return cmd, expl, nil
	}

	actions := risk.LegalReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, Explanation{}, fmt.Errorf("bot: no legal reinforcement for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	armies := min(g.PendingReinforcements, max(1, g.PendingReinforcements/3))

	territories := make([]risk.Territory, len(actions))
	for i, a := range actions {
		territories[i] = a.Territory
	}
	best, bestScore := s.bestReinforceCandidateTerritories(g, playerID, pi, territories, armies)

	cmd := Command{Action: ActionPlaceReinforcement, Territory: string(actions[best].Territory), Armies: armies}
	return cmd, Explanation{Score: bestScore}, nil
}

// setupReinforce uses the same afterstate scoring as reinforce, but
// places exactly one army per call (risk.PlaceInitialArmy's only legal
// amount).
func (s *ValueStrategy) setupReinforce(g *risk.Game, playerID string) (Command, Explanation, error) {
	actions := risk.LegalSetupReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, Explanation{}, fmt.Errorf("bot: no legal setup reinforcement for player %s", playerID)
	}
	pi := playerIndex(g, playerID)

	territories := make([]risk.Territory, len(actions))
	for i, a := range actions {
		territories[i] = a.Territory
	}
	best, bestScore := s.bestReinforceCandidateTerritories(g, playerID, pi, territories, 1)
	return Command{Action: ActionPlaceInitialArmy, Territory: string(actions[best].Territory)}, Explanation{Score: bestScore}, nil
}

func (s *ValueStrategy) bestReinforceCandidateTerritories(g *risk.Game, playerID string, pi int, territories []risk.Territory, armies int) (best int, bestScore float64) {
	for i, t := range territories {
		after := reinforceAfterstate(g, playerID, t, armies)
		score := s.value.Score(tdstate.Encode(after, pi).Flatten())
		if i == 0 || score > bestScore {
			best, bestScore = i, score
		}
	}
	return best, bestScore
}

// occupy scores every legal army count to move into the just-conquered
// territory's afterstate and picks the highest.
func (s *ValueStrategy) occupy(g *risk.Game, playerID string) (Command, Explanation, error) {
	actions := risk.LegalOccupations(g, playerID)
	if len(actions) == 0 {
		return Command{}, Explanation{}, fmt.Errorf("bot: no legal occupation for player %s", playerID)
	}
	pi := playerIndex(g, playerID)

	best := 0
	var bestScore float64
	for i, a := range actions {
		after := occupyAfterstate(g, playerID, a.Armies)
		score := s.value.Score(tdstate.Encode(after, pi).Flatten())
		if i == 0 || score > bestScore {
			best, bestScore = i, score
		}
	}

	return Command{Action: ActionOccupy, Armies: actions[best].Armies}, Explanation{Score: bestScore}, nil
}

// fortify scores every legal fortification move's afterstate, ending the
// turn without fortifying instead when there's no legal move or the best
// one doesn't beat the current state's own score.
func (s *ValueStrategy) fortify(g *risk.Game, playerID string) (Command, Explanation, error) {
	actions := risk.LegalFortifications(g, playerID)
	pi := playerIndex(g, playerID)
	currentScore := s.currentStateScore(g, pi)

	best := -1
	var bestScore float64
	for i, a := range actions {
		after := fortifyAfterstate(g, playerID, a.From, a.To, a.MaxArmies)
		score := s.value.Score(tdstate.Encode(after, pi).Flatten())
		if best == -1 || score > bestScore {
			best, bestScore = i, score
		}
	}

	if !s.clearsMargin("fortify", best, bestScore, currentScore, s.value.FortifyMargin()) {
		return Command{Action: ActionEndTurn}, Explanation{Score: bestScore}, nil
	}
	a := actions[best]
	return Command{Action: ActionFortify, From: string(a.From), To: string(a.To), Armies: a.MaxArmies}, Explanation{Score: bestScore}, nil
}
