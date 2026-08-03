package bot

import (
	"context"
	"fmt"
	"time"

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
// class. A single shared model (passed to NewBoardValueStrategy) scores
// every phase by default, but AttackValue/ReinforceValue/FortifyValue
// let a specific phase use a different model instead -- see their doc
// comments.
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

	// Searcher, if non-nil, is used for attack()'s search verbatim, taking
	// priority over AttackSearchDepth/Risky/AttackSearchBreadth entirely
	// -- the fully-general escape hatch for plugging in any AttackSearcher
	// (see attack_searcher.go), including ones with no depth/breadth/risky
	// notion at all (e.g. a future heuristic-pruned or time-budgeted
	// searcher, Phase 4/5 of project-docs/bot_player/proposals/
	// Search_Integration_Roadmap_with_References.md). Most callers should
	// keep this nil and use AttackSearchDepth instead; Searcher exists for
	// cases those three scalar fields can't express.
	Searcher AttackSearcher

	// AttackSearchDepth, when > 0 and Searcher is nil, makes attack()
	// build a default *SequenceSearcher{Depth: AttackSearchDepth, Breadth:
	// AttackSearchBreadth, Risky: Risky} and use that -- a real search
	// over sequences of up to this many of the acting player's own
	// consecutive attacks (Phase 2/3 of project-docs/bot_player/proposals/
	// Search_Integration_Roadmap_with_References.md) -- see
	// attack_search.go. Every legal attack is explored at every level
	// (unlike the removed LookaheadDepth, which only ever followed one
	// greedily-picked path), and each attack is materialized via
	// AttackTerminalStates/SelectTerminalState into one concrete
	// deterministic board state, not a probability blend. Zero (the
	// default) keeps attack() on SinglePlySearcher, the original,
	// already-validated single-ply behavior, unchanged.
	AttackSearchDepth int

	// Risky is the Attack Handler's terminal-state selection threshold
	// (paper Section 3.3) -- higher walks further toward
	// attacker-favorable outcomes before committing. Only consulted when
	// AttackSearchDepth > 0 and Searcher is nil. Values <= 0 fall back to
	// the paper's own default, 0.3 (see attack_search.go's
	// SequenceSearcher.risky()).
	Risky float64

	// AttackSearchBreadth, when > 0, AttackSearchDepth > 0, and Searcher
	// is nil, caps how many top-scoring legal attacks are explored at each
	// level of the sequence search, ranked by the existing single-ply
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

	// AttackSearchBudget, when > 0 and neither Searcher nor
	// AttackSearchDepth is set, makes attack() use a *AnytimeSearcher{
	// Budget: AttackSearchBudget, Breadth: AttackSearchBreadth, Risky:
	// Risky} -- iterative-deepening search bounded by wall-clock time
	// instead of a fixed ply count (Phase 5 of project-docs/bot_player/
	// proposals/Search_Integration_Roadmap_with_References.md), matching
	// the paper's own anytime design more closely than AttackSearchDepth's
	// fixed cutoff does. AttackSearchDepth > 0 takes priority when both
	// are set -- fixed-depth is the currently tournament-validated
	// configuration, so an explicit depth shouldn't be silently preempted
	// by the newer, not-yet-tournament-validated anytime mode. Zero (the
	// default) keeps attack() off this path entirely. Shares
	// AttackSearchBreadth/Risky with the fixed-depth mode rather than
	// having its own copies -- unpruned search is far too slow (2.5s-88s/
	// decision, see AttackSearchBreadth's own doc comment) for a 10s
	// budget to reach useful depth without the same breadth pruning.
	AttackSearchBudget time.Duration

	// AttackValue, ReinforceValue, and FortifyValue, when non-nil,
	// override which ValueFunction scores that specific phase's
	// candidates/margin instead of the shared model passed to
	// NewBoardValueStrategy -- e.g. a model trained/calibrated
	// specifically on attack-phase afterstates can be plugged in without
	// also being forced onto reinforce/fortify decisions. Each defaults
	// to the shared model when left nil, so existing callers that only
	// ever set one model (via NewBoardValueStrategy) are unaffected.
	// Occupy (choosing how many armies to move into a just-conquered
	// territory) deliberately has no separate knob -- it's part of the
	// same conquest as the attack that triggered it, so it shares
	// AttackValue's resolved model rather than adding a fourth field.
	AttackValue    ValueFunction
	ReinforceValue ValueFunction
	FortifyValue   ValueFunction

	// OccupySearchBreadth, when > 0, caps occupy() to that many army
	// counts (Phase 4/Ga of project-docs/bot_player/proposals/
	// Search_Integration_Roadmap_with_References.md -- the paper's "Ga
	// variations, linearly interpolated from the minimum ... and the
	// maximum, disregarding duplicates"), instead of scoring every legal
	// count exhaustively. Zero (the default) means unlimited -- score
	// every risk.LegalOccupations result, matching today's original,
	// already-tested behavior.
	OccupySearchBreadth int

	// FortifySearchBreadth, when > 0, caps fortify() to that many army
	// counts per legal (from, to) pair, interpolated across [1, MaxArmies]
	// (Phase 4/Gf). Unlike OccupySearchBreadth, zero (the default) does
	// NOT mean "exhaustive" -- risk.LegalFortifications only ever reports
	// one count (MaxArmies) per pair, so zero reproduces that exact
	// single-candidate behavior, unchanged.
	FortifySearchBreadth int

	// ReinforceSearcher, ReinforceSearchDepth, Tp, and Gp are reinforce()'s
	// analogues of Searcher/AttackSearchDepth/AttackSearchBreadth/Risky --
	// see ReinforceSearcher's doc comment (reinforce_searcher.go) and
	// GroupReinforcer (reinforce_search.go). ReinforceSearcher, if
	// non-nil, is used verbatim; otherwise ReinforceSearchDepth > 0 builds
	// a default *GroupReinforcer{Tp, Gp, Depth: ReinforceSearchDepth}, the
	// paper's Tp/Gp placing search (Phase 4). Zero (the default) keeps
	// reinforce() on SingleBatchReinforcer, the original, already-tested
	// behavior, unchanged. Tp/Gp fall back to the paper's own defaults (2
	// and 3) when <= 0.
	ReinforceSearcher    ReinforceSearcher
	ReinforceSearchDepth int
	Tp                   int
	Gp                   int
}

// searcher returns the AttackSearcher attack() should use, in order:
// s.Searcher if set; otherwise a *SequenceSearcher built from
// AttackSearchDepth/Risky/AttackSearchBreadth if AttackSearchDepth > 0
// (fixed-depth, the currently tournament-validated configuration);
// otherwise a *AnytimeSearcher built from AttackSearchBudget/
// AttackSearchBreadth/Risky if AttackSearchBudget > 0 (Phase 5, not yet
// tournament-validated); otherwise SinglePlySearcher{} -- the original,
// always-validated default.
func (s *ValueStrategy) searcher() AttackSearcher {
	if s.Searcher != nil {
		return s.Searcher
	}
	if s.AttackSearchDepth > 0 {
		return &SequenceSearcher{Depth: s.AttackSearchDepth, Breadth: s.AttackSearchBreadth, Risky: s.Risky}
	}
	if s.AttackSearchBudget > 0 {
		return &AnytimeSearcher{Budget: s.AttackSearchBudget, Breadth: s.AttackSearchBreadth, Risky: s.Risky}
	}
	return SinglePlySearcher{}
}

// reinforceSearcher returns the ReinforceSearcher reinforce() should use:
// s.ReinforceSearcher if set, otherwise a *GroupReinforcer built from
// Tp/Gp/ReinforceSearchDepth if ReinforceSearchDepth > 0, otherwise
// SingleBatchReinforcer{} -- the original, always-validated default.
func (s *ValueStrategy) reinforceSearcher() ReinforceSearcher {
	if s.ReinforceSearcher != nil {
		return s.ReinforceSearcher
	}
	if s.ReinforceSearchDepth > 0 {
		return &GroupReinforcer{Tp: s.Tp, Gp: s.Gp, Depth: s.ReinforceSearchDepth}
	}
	return SingleBatchReinforcer{}
}

// attackValue returns s.AttackValue if set, otherwise the shared model --
// also used by occupy(), which piggybacks on the attack phase's model
// (see AttackValue's doc comment).
func (s *ValueStrategy) attackValue() ValueFunction {
	if s.AttackValue != nil {
		return s.AttackValue
	}
	return s.value
}

// reinforceValue returns s.ReinforceValue if set, otherwise the shared
// model -- used by both reinforce() and setupReinforce().
func (s *ValueStrategy) reinforceValue() ValueFunction {
	if s.ReinforceValue != nil {
		return s.ReinforceValue
	}
	return s.value
}

// fortifyValue returns s.FortifyValue if set, otherwise the shared model.
func (s *ValueStrategy) fortifyValue() ValueFunction {
	if s.FortifyValue != nil {
		return s.FortifyValue
	}
	return s.value
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
		return s.attack(ctx, g, playerID)
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
// compare their best real candidate against. Package-level (not a
// *ValueStrategy method) since SequenceSearcher.bestContinuation also
// needs it as its recursive base case and has no access to a
// *ValueStrategy.
func currentStateScore(value ValueFunction, g *risk.Game, pi int) float64 {
	return value.Score(tdstate.Encode(g, pi).Flatten())
}

// attack picks a candidate to attack with, ending the attack phase
// instead when there's no legal attack or the best one doesn't beat the
// current state's own score. The candidate comes from s.searcher() --
// SinglePlySearcher (the original, already-validated single-ply default)
// unless Searcher, AttackSearchDepth, or AttackSearchBudget says
// otherwise (see searcher()). ctx is threaded straight through to
// Search -- only AnytimeSearcher (Phase 5) actually consults it.
func (s *ValueStrategy) attack(ctx context.Context, g *risk.Game, playerID string) (Command, Explanation, error) {
	pi := playerIndex(g, playerID)
	value := s.attackValue()
	currentScore := currentStateScore(value, g, pi)

	a, bestScore, ok := s.searcher().Search(ctx, g, playerID, pi, value)
	best := -1
	if ok {
		best = 0
	}

	if !s.clearsMargin("attack", best, bestScore, currentScore, value.AttackMargin()) {
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
// weights/value function), then delegates to reinforceSearcher() --
// SingleBatchReinforcer (the original, always-validated default: score
// every legal territory independently, place a capped batch at the top
// scorer, same batching rule as ScoredStrategy.reinforce) unless
// ReinforceSearcher or ReinforceSearchDepth says otherwise (Phase 4/Tp/Gp,
// see reinforceSearcher()).
func (s *ValueStrategy) reinforce(g *risk.Game, playerID string) (Command, Explanation, error) {
	if cmd, expl, ok := scoredCardTurnIn(g, playerID); ok {
		return cmd, expl, nil
	}

	t, armies, score, ok := s.reinforceSearcher().Search(g, playerID, playerIndex(g, playerID), s.reinforceValue())
	if !ok {
		return Command{}, Explanation{}, fmt.Errorf("bot: no legal reinforcement for player %s", playerID)
	}
	return Command{Action: ActionPlaceReinforcement, Territory: string(t), Armies: armies}, Explanation{Score: score}, nil
}

// setupReinforce uses the same afterstate scoring as SingleBatchReinforcer,
// but places exactly one army per call (risk.PlaceInitialArmy's only
// legal amount) over every owned territory, unfiltered -- Tp/Gp (Phase 4)
// describes the ongoing reinforce search, not pre-game setup allocation,
// so this phase is untouched by ReinforceSearcher entirely.
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
	best, bestScore := bestReinforceCandidateTerritories(g, playerID, pi, territories, 1, s.reinforceValue())
	return Command{Action: ActionPlaceInitialArmy, Territory: string(actions[best].Territory)}, Explanation{Score: bestScore}, nil
}

// bestReinforceCandidateTerritories scores territories independently
// (each afterstate assumes armies is placed there alone) and returns the
// top scorer's index -- shared by setupReinforce and
// SingleBatchReinforcer.Search. Package-level (not a *ValueStrategy
// method) since SingleBatchReinforcer has no access to a *ValueStrategy,
// the same reason currentStateScore is package-level.
func bestReinforceCandidateTerritories(g *risk.Game, playerID string, pi int, territories []risk.Territory, armies int, value ValueFunction) (best int, bestScore float64) {
	for i, t := range territories {
		after := reinforceAfterstate(g, playerID, t, armies)
		score := value.Score(tdstate.Encode(after, pi).Flatten())
		if i == 0 || score > bestScore {
			best, bestScore = i, score
		}
	}
	return best, bestScore
}

// occupy scores every candidate army count to move into the just-conquered
// territory's afterstate and picks the highest -- occupyArmyCounts caps
// the candidates to OccupySearchBreadth (Phase 4/Ga) when set, otherwise
// every legal count is scored exhaustively, unchanged. Uses attackValue()
// (not a separate knob) since occupy is part of the attack sequence that
// triggered it -- see AttackValue's doc comment.
func (s *ValueStrategy) occupy(g *risk.Game, playerID string) (Command, Explanation, error) {
	actions := risk.LegalOccupations(g, playerID)
	if len(actions) == 0 {
		return Command{}, Explanation{}, fmt.Errorf("bot: no legal occupation for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	value := s.attackValue()
	counts := occupyArmyCounts(actions, s.OccupySearchBreadth)

	var bestScore float64
	var bestArmies int
	for i, armies := range counts {
		after := occupyAfterstate(g, playerID, armies)
		score := value.Score(tdstate.Encode(after, pi).Flatten())
		if i == 0 || score > bestScore {
			bestScore, bestArmies = score, armies
		}
	}

	return Command{Action: ActionOccupy, Armies: bestArmies}, Explanation{Score: bestScore}, nil
}

// fortify scores every legal fortification move's afterstate -- for each
// legal (from, to) pair, fortifyArmyCounts (Phase 4/Gf) supplies the army
// counts to try, capped to FortifySearchBreadth when set, otherwise just
// the single MaxArmies candidate, unchanged from before Gf existed -- and
// ends the turn without fortifying instead when there's no legal move or
// the best one doesn't beat the current state's own score.
func (s *ValueStrategy) fortify(g *risk.Game, playerID string) (Command, Explanation, error) {
	actions := risk.LegalFortifications(g, playerID)
	pi := playerIndex(g, playerID)
	value := s.fortifyValue()
	currentScore := currentStateScore(value, g, pi)

	found := false
	var bestScore float64
	var bestFrom, bestTo risk.Territory
	var bestArmies int
	for _, a := range actions {
		for _, armies := range fortifyArmyCounts(a, s.FortifySearchBreadth) {
			after := fortifyAfterstate(g, playerID, a.From, a.To, armies)
			score := value.Score(tdstate.Encode(after, pi).Flatten())
			if !found || score > bestScore {
				found, bestScore = true, score
				bestFrom, bestTo, bestArmies = a.From, a.To, armies
			}
		}
	}

	best := -1
	if found {
		best = 0
	}
	if !s.clearsMargin("fortify", best, bestScore, currentScore, value.FortifyMargin()) {
		return Command{Action: ActionEndTurn}, Explanation{Score: bestScore}, nil
	}
	return Command{Action: ActionFortify, From: string(bestFrom), To: string(bestTo), Armies: bestArmies}, Explanation{Score: bestScore}, nil
}
