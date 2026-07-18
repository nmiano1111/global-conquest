package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
	"github.com/nmiano1111/global-conquest/backend/internal/tdstate"
)

// BoardValue is a linear whole-board value function: standardize a
// tdstate.Encode(...).Flatten() feature vector using the same mean/std
// computed at training time, then take a dot product plus an intercept.
// Ranking is all that matters (BoardValueStrategy always just picks the
// highest score), so the raw linear score is used directly -- no sigmoid
// needed, the same "sigmoid is monotonic" reasoning already used for
// bot.Weights' fitted coefficients earlier this project.
type BoardValue struct {
	Weights   []float64
	Intercept float64
	Mean      []float64
	Std       []float64
	// AttackMargin/FortifyMargin are how much a candidate's afterstate
	// score must exceed the current, unmodified state's score before
	// BoardValueStrategy's attack/fortify phases will act on it, instead
	// of ending the phase -- a bare "is it better at all" gate (margin ==
	// 0) isn't enough: a first live tournament eval with margin == 0
	// showed ~15 attacks/turn (vs. ~1.4 captures/turn averaged across a
	// whole tournament), since every fitted coefficient is small enough
	// that nearly any legal move nudges the score marginally positive.
	//
	// These are two separate fields, not one shared Margin, because
	// attack and fortify move the score on completely different scales:
	// attack changes ownership (touching many features -- is_mine,
	// continent/army/territory fractions all at once), while fortify only
	// reallocates armies between the acting player's own territories
	// (touching at most two per-territory army_fraction coefficients,
	// typically much smaller). A single shared margin calibrated to
	// attack's scale was found to suppress fortify almost entirely (12
	// fortifies/13 turns at margin 0, down to 0 fortifies at any margin
	// >= ~0.1 -- see cmd/bvcalibrate, which fits these two independently
	// from each phase's own observed score-delta distribution).
	AttackMargin  float64
	FortifyMargin float64
}

// boardValueFile mirrors analytics/src/global_conquest_analytics/
// board_fit.py's exported JSON shape exactly.
type boardValueFile struct {
	Weights       []float64 `json:"weights"`
	Intercept     float64   `json:"intercept"`
	Mean          []float64 `json:"mean"`
	Std           []float64 `json:"std"`
	AttackMargin  float64   `json:"attack_margin"`
	FortifyMargin float64   `json:"fortify_margin"`
	FeatureNames  []string  `json:"feature_names"`
}

// LoadBoardValue reads and parses a board_fit.py-exported JSON file.
func LoadBoardValue(path string) (*BoardValue, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("bot: read board value file: %w", err)
	}
	var f boardValueFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("bot: parse board value file %s: %w", path, err)
	}
	if len(f.Weights) != len(f.Mean) || len(f.Weights) != len(f.Std) {
		return nil, fmt.Errorf("bot: board value file %s: weights/mean/std length mismatch (%d/%d/%d)", path, len(f.Weights), len(f.Mean), len(f.Std))
	}
	return &BoardValue{
		Weights:       f.Weights,
		Intercept:     f.Intercept,
		Mean:          f.Mean,
		Std:           f.Std,
		AttackMargin:  f.AttackMargin,
		FortifyMargin: f.FortifyMargin,
	}, nil
}

// Score standardizes features using bv's training-set mean/std, then
// returns the linear value w . standardized + intercept.
func (bv *BoardValue) Score(features []float64) float64 {
	score := bv.Intercept
	for i, x := range features {
		std := bv.Std[i]
		if std == 0 {
			std = 1
		}
		standardized := (x - bv.Mean[i]) / std
		score += bv.Weights[i] * standardized
	}
	return score
}

// BoardValueStrategy scores candidates by the value of the resulting
// *board state* (via internal/bot's afterstate helpers +
// internal/tdstate.Encode), not local per-candidate features -- see
// project-docs/bot_player/proposals/GCN_Strategy_Roadmap_with_References.md
// and 11_Learned_Board_Evaluation.md. Diagnostic work this project did
// (comparing this whole-board representation against every local
// per-candidate feature set tried) found it discriminates far better
// offline; this Strategy is the first test of whether that translates
// into winning real games.
//
// Unlike GBTStrategy, which needed a percentile-based end_phase_threshold
// (see gbt_fit.py) since it had no baseline to compare a candidate
// against, BoardValueStrategy can score the *current, unmodified* state
// the same way it scores any candidate's afterstate -- so "should I keep
// attacking/fortifying" becomes a comparison against a real baseline
// (does any real candidate beat doing nothing) rather than an arbitrary
// absolute cutoff. That comparison still needs a margin, not just a bare
// "beats it at all" -- see BoardValue.AttackMargin/FortifyMargin.
type BoardValueStrategy struct {
	value    *BoardValue
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
}

// NewBoardValueStrategy constructs a BoardValueStrategy from an
// already-loaded BoardValue (see LoadBoardValue), falling back to a
// BasicStrategy for any phase this strategy doesn't itself handle
// (setup_claim -- see ScoredStrategy's identical fallback rationale).
func NewBoardValueStrategy(value *BoardValue) *BoardValueStrategy {
	return &BoardValueStrategy{value: value, fallback: NewBasicStrategy()}
}

func (s *BoardValueStrategy) NextCommand(ctx context.Context, g *risk.Game, playerID string) (Command, Explanation, error) {
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
func (s *BoardValueStrategy) currentStateScore(g *risk.Game, pi int) float64 {
	return s.value.Score(tdstate.Encode(g, pi).Flatten())
}

// attack scores every legal attack's afterstate (attackAfterstateBlend)
// and picks the highest, ending the attack phase instead when there's no
// legal attack or the best one doesn't beat the current state's own
// score.
func (s *BoardValueStrategy) attack(g *risk.Game, playerID string) (Command, Explanation, error) {
	actions := risk.LegalAttacks(g, playerID)
	pi := playerIndex(g, playerID)
	currentScore := s.currentStateScore(g, pi)

	best := -1
	var bestScore float64
	for i, a := range actions {
		score := s.value.Score(attackAfterstateBlend(g, pi, a))
		if best == -1 || score > bestScore {
			best, bestScore = i, score
		}
	}

	if best != -1 && s.Observer != nil {
		s.Observer("attack", bestScore, currentScore)
	}
	if best == -1 || bestScore <= currentScore+s.value.AttackMargin {
		return Command{Action: ActionEndAttack}, Explanation{Score: bestScore}, nil
	}
	a := actions[best]
	return Command{
		Action:       ActionAttack,
		From:         string(a.From),
		To:           string(a.To),
		AttackerDice: a.MaxAttackerDice,
	}, Explanation{Score: bestScore}, nil
}

// reinforce decides card timing first (scoredCardTurnIn, shared with
// ScoredStrategy/GBTStrategy -- card-timing policy doesn't depend on any
// weights/value function), then scores every legal reinforcement
// territory's afterstate and places a capped batch at the top scorer --
// same batching rule as ScoredStrategy.reinforce/GBTStrategy.reinforce.
func (s *BoardValueStrategy) reinforce(g *risk.Game, playerID string) (Command, Explanation, error) {
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
func (s *BoardValueStrategy) setupReinforce(g *risk.Game, playerID string) (Command, Explanation, error) {
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

func (s *BoardValueStrategy) bestReinforceCandidateTerritories(g *risk.Game, playerID string, pi int, territories []risk.Territory, armies int) (best int, bestScore float64) {
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
func (s *BoardValueStrategy) occupy(g *risk.Game, playerID string) (Command, Explanation, error) {
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
func (s *BoardValueStrategy) fortify(g *risk.Game, playerID string) (Command, Explanation, error) {
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

	if best != -1 && s.Observer != nil {
		s.Observer("fortify", bestScore, currentScore)
	}
	if best == -1 || bestScore <= currentScore+s.value.FortifyMargin {
		return Command{Action: ActionEndTurn}, Explanation{Score: bestScore}, nil
	}
	a := actions[best]
	return Command{Action: ActionFortify, From: string(a.From), To: string(a.To), Armies: a.MaxArmies}, Explanation{Score: bestScore}, nil
}
