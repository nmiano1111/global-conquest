package bot

import (
	"context"
	"math/rand/v2"
	"slices"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// StrategyScoredV1 is the first strategy built on the candidate-scoring
// architecture described in
// project-docs/bot_player/phase_2_first_playable_bot/heuristic_framework.md:
// every legal action (including "end this phase") is scored by named,
// weighted features and the highest score wins, rather than hand-rolled
// if/else thresholds. Every phase (setup_reinforce, reinforce, attack,
// occupy, fortify) is now migrated onto this pipeline — only card-timing
// strategy, continent valuation beyond attack/reinforce/fortify's simple
// forms, elimination-chasing, difficulty weights, and personalities remain
// as later doc work.
const StrategyScoredV1 = "scored-v1"

// StrategyScoredV1Exploring identifies a ScoredStrategy built with
// NewExploringScoredStrategy -- see that constructor's doc comment. Only
// registered by cmd/traindata; never used in real gameplay, cmd/simulate,
// or cmd/tournament's built-in registry.
const StrategyScoredV1Exploring = "scored-v1-exploring"

// ScoredStrategy implements StrategyScoredV1.
type ScoredStrategy struct {
	weights Weights
	// fallback only matters for risk.PhaseSetupClaim, which every real
	// game path skips (see CLAUDE.md — engine-only, unused in practice)
	// — kept as a defensive default rather than assuming every phase the
	// engine could theoretically report is enumerated above.
	fallback *BasicStrategy

	// explorationRate is the probability selectBest picks a uniformly
	// random legal candidate instead of the highest-scoring one. Zero for
	// every strategy built via NewScoredStrategy -- only
	// NewExploringScoredStrategy sets it, and only for generating
	// training data (never real gameplay or evaluation).
	explorationRate float64
	// randFloat64/randIntN back explorationRate's random draws. Default
	// to math/rand/v2's package-level functions in
	// NewExploringScoredStrategy -- already safe for concurrent use with
	// no shared instance state (unlike a seeded *rand.Rand), so a single
	// ScoredStrategy instance stays safe to reuse across concurrent
	// RunOne calls exactly as before (internal/tournament.Run and
	// cmd/traindata's worker pools both already rely on that). Left nil
	// on every plain NewScoredStrategy instance -- never called, since
	// explorationRate <= 0 short-circuits before either is invoked. Tests
	// inject deterministic stubs instead.
	randFloat64 func() float64
	randIntN    func(n int) int

	// recordCandidates, when true, populates Explanation.AllCandidates
	// with every legal candidate's full feature breakdown on every
	// decision -- see Candidate's doc comment. Only ever set by
	// NewExploringScoredStrategy (again, only for cmd/traindata); false
	// on every plain NewScoredStrategy instance, so real gameplay pays no
	// extra allocation.
	recordCandidates bool
}

// NewScoredStrategy creates a ScoredStrategy that scores candidates using
// the given Weights, falling back to a BasicStrategy for any phase this
// strategy doesn't itself handle.
func NewScoredStrategy(w Weights) *ScoredStrategy {
	return &ScoredStrategy{weights: w, fallback: NewBasicStrategy()}
}

// NewExploringScoredStrategy returns a ScoredStrategy that, with
// probability explorationRate on each decision, picks a uniformly random
// legal candidate instead of the highest-scoring one -- injecting the
// action-outcome contrast Approach A's fitting needs (see
// project-docs/bot_player/phase_3_continuous_improvement/10_Bot_Weight_Tuning.md
// and Next_Phase_Bot_ML_Roadmap.md's diagnosis of the first fit's
// near-zero reinforce coefficients: an always-argmax policy never
// generates a "we tried something worse here and it correlated with
// losing" example for logistic regression to learn from). Also always
// records every legal candidate's full feature breakdown on every
// decision (Explanation.AllCandidates, recordCandidates: true) -- not
// just the chosen one -- so cmd/traindata can build one training row per
// candidate instead of per decision, avoiding the chosen-only selection
// bias diagnosed for the collinearity that crushed several fitted
// coefficients to near-zero. Only ever used by cmd/traindata to generate
// training data -- cmd/backend, cmd/simulate, and cmd/tournament's
// built-in registry all still use plain NewScoredStrategy.
func NewExploringScoredStrategy(w Weights, explorationRate float64) *ScoredStrategy {
	return &ScoredStrategy{
		weights:          w,
		fallback:         NewBasicStrategy(),
		explorationRate:  explorationRate,
		randFloat64:      rand.Float64,
		randIntN:         rand.IntN,
		recordCandidates: true,
	}
}

// selectBest wraps the package-level selectBest, injecting exploration
// when explorationRate is set -- a no-op wrapper (identical to calling
// the package-level selectBest directly) for every ScoredStrategy built
// via the plain NewScoredStrategy, since explorationRate's zero value
// short-circuits the first condition below.
func (s *ScoredStrategy) selectBest(options []scoredOption, maxAlternatives int) (Command, Explanation) {
	all := rankOptions(options)
	if s.explorationRate <= 0 || len(all) <= 1 || s.randFloat64() >= s.explorationRate {
		return explanationFor(all, 0, maxAlternatives, false, s.recordCandidates)
	}
	idx := s.randIntN(len(all))
	return explanationFor(all, idx, maxAlternatives, idx != 0, s.recordCandidates)
}

// NextCommand picks the next command for playerID by scoring every legal
// candidate for the game's current phase and returning the highest-scoring
// one, along with an Explanation of why it won. Phases not yet migrated
// onto the scoring pipeline fall back to BasicStrategy.
func (s *ScoredStrategy) NextCommand(ctx context.Context, g *risk.Game, playerID string) (Command, Explanation, error) {
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

// attack scores every legal attack plus a synthetic "end_attack" candidate
// and picks the max — ending the phase is never special-cased, exactly per
// the doc's "End Attack Is A Candidate" section. Since that sentinel is
// always present, options is never empty and this never errors.
func (s *ScoredStrategy) attack(g *risk.Game, playerID string) (Command, Explanation, error) {
	actions := risk.LegalAttacks(g, playerID)
	pi := playerIndex(g, playerID)

	options := make([]scoredOption, 0, len(actions)+1)
	for _, a := range actions {
		options = append(options, scoredOption{
			Command: Command{
				Action:       ActionAttack,
				From:         string(a.From),
				To:           string(a.To),
				AttackerDice: a.MaxAttackerDice,
			},
			Features: s.attackFeatures(g, pi, a),
		})
	}
	options = append(options, scoredOption{
		Command:  Command{Action: ActionEndAttack},
		Features: []Feature{{Name: "end_phase_bias", Value: s.weights.EndPhaseBias}},
	})

	cmd, expl := s.selectBest(options, 3)
	return cmd, expl, nil
}

// attackFeatures scores one legal attack. Every feature besides the first
// three is conditional (omitted, not zeroed) so an Explanation only lists
// what actually applied to this candidate.
func (s *ScoredStrategy) attackFeatures(g *risk.Game, pi int, a risk.AttackAction) []Feature {
	w := s.weights
	forecast := ForecastAttack(a.SourceArmies, a.TargetArmies)
	targetOwner := g.Territories[a.To].Owner

	features := []Feature{
		{Name: "army_advantage", Value: w.ArmyAdvantage * float64(a.SourceArmies-a.TargetArmies)},
		{Name: "capture_probability", Value: w.CaptureProbability * forecast.WinProbability},
		{Name: "expected_loss_cost", Value: w.ExpectedLossCost * forecast.ExpectedAttackerLosses},
	}
	if wouldCompleteContinent(g, pi, a.To) {
		features = append(features, Feature{Name: "completes_continent", Value: w.CompletesContinent})
	}
	if breaksEnemyContinent(g, targetOwner, a.To) {
		features = append(features, Feature{Name: "breaks_enemy_continent", Value: w.BreaksEnemyContinent})
	}
	if !g.ConqueredThisTurn {
		features = append(features, Feature{Name: "card_opportunity", Value: w.CardOpportunity * forecast.WinProbability})
	}
	if isLastTerritory(g, targetOwner, a.To) {
		features = append(features, Feature{Name: "eliminates_player", Value: w.EliminatesPlayer})
	}
	if exposure := adjacentEnemyArmies(g, a.From, pi); exposure > 0 {
		features = append(features, Feature{Name: "exposure_penalty", Value: w.ExposurePenalty * float64(exposure)})
	}
	return features
}

// continentsContaining returns every continent t belongs to (in practice
// always exactly one on the classic board, but the board format allows
// more, so this doesn't assume it).
func continentsContaining(g *risk.Game, t risk.Territory) []risk.Continent {
	var out []risk.Continent
	for name, info := range g.Board.Continents {
		if slices.Contains(info.Territories, t) {
			out = append(out, name)
		}
	}
	return out
}

// continentOwnershipCounts reports how many of continent c's territories
// player pi owns, out of the continent's total.
func continentOwnershipCounts(g *risk.Game, pi int, c risk.Continent) (owned, total int) {
	info := g.Board.Continents[c]
	total = len(info.Territories)
	for _, t := range info.Territories {
		if g.Territories[t].Owner == pi {
			owned++
		}
	}
	return owned, total
}

// wouldCompleteContinent reports whether player pi capturing territory t
// (currently not owned by pi) would complete some continent for them.
func wouldCompleteContinent(g *risk.Game, pi int, t risk.Territory) bool {
	for _, c := range continentsContaining(g, t) {
		if owned, total := continentOwnershipCounts(g, pi, c); owned == total-1 {
			return true
		}
	}
	return false
}

// breaksEnemyContinent reports whether defenderPI currently fully owns a
// continent containing t — i.e. whether losing t would break that bonus.
func breaksEnemyContinent(g *risk.Game, defenderPI int, t risk.Territory) bool {
	if defenderPI < 0 {
		return false
	}
	for _, c := range continentsContaining(g, t) {
		if owned, total := continentOwnershipCounts(g, defenderPI, c); owned == total {
			return true
		}
	}
	return false
}

// isLastTerritory reports whether ownerPI owns exactly one territory on
// the whole board: t itself.
func isLastTerritory(g *risk.Game, ownerPI int, t risk.Territory) bool {
	if ownerPI < 0 {
		return false
	}
	count := 0
	for _, terr := range g.Board.Order {
		if g.Territories[terr].Owner == ownerPI {
			count++
			if count > 1 {
				return false
			}
		}
	}
	return count == 1
}
