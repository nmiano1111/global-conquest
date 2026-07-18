package bot

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/nmiano1111/global-conquest/backend/internal/bot/gbtmodel"
	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// fallbackEndPhaseThreshold is used only if a loaded attack/fortify model
// is somehow missing its exported end_phase_threshold (see
// gbtmodel.Model.EndPhaseThreshold) -- shouldn't happen in practice, since
// gbt_fit.py's GBTPhaseFit always computes one for these two phases, but a
// bare 0.5 ("better than even odds") is a defensible fallback rather than
// a hard error, given a live tournament eval already showed *why* a fixed
// global constant is the wrong default: a GBT model's predicted
// probability is P(this player wins the whole 60+-turn game | this one
// decision's features), not P(this specific move is good) -- attack's own
// predicted probabilities had a median of just ~0.34 on real training
// data (most individual decisions, even perfectly reasonable ones, don't
// move a diffuse whole-game outcome prediction anywhere near 0.5), so a
// flat 0.5 threshold rejected nearly every legal attack (0% win rate, ~6
// captures/game vs ~100 for the baselines, in that eval). Training now
// computes each phase's own threshold from its own predicted-probability
// distribution instead (the 10th percentile -- "only skip roughly the
// worst 10% of legal candidates") and embeds it directly in the exported
// model.
const fallbackEndPhaseThreshold = 0.5

// endPhaseThreshold reads model's own exported threshold, falling back to
// fallbackEndPhaseThreshold if it's somehow absent.
func endPhaseThreshold(model *gbtmodel.Model) float64 {
	if t, ok := model.EndPhaseThreshold(); ok {
		return t
	}
	return fallbackEndPhaseThreshold
}

// GBTStrategy scores candidates with a per-phase gradient-boosted-trees
// model (internal/bot/gbtmodel) instead of ScoredStrategy's weighted
// linear sum. A diagnostic comparison (see
// project-docs/bot_player/Next_Phase_Bot_ML_Roadmap.md) found GBT
// recovers real predictive signal from several features (weakness,
// expected_loss_cost, exposure_penalty) that every logistic-regression
// fitting attempt could never assign a non-trivial coefficient to --
// evidence the bottleneck was the linear model's handling of collinear
// features, not the features or training objective themselves.
//
// Unlike a fitted Weights value, a GBT model's decision function isn't
// linear -- it can't be reduced to a small set of multipliers, so this is
// a genuinely different Strategy implementation, not a variant
// constructed from ScoredStrategy the way NewExploringScoredStrategy is.
// It reuses ScoredStrategy's raw-signal computation
// (computeAttackSignals/computeReinforceSignals/computeOccupySignals/
// computeFortifySignals) directly -- the same candidate signal, scored
// two different ways.
type GBTStrategy struct {
	// models is keyed by phase name ("attack", "reinforce" -- shared by
	// setup_reinforce, same as cmd/traindata's phaseFeatures table --
	// "occupy", "fortify"), one independently-trained model per phase,
	// mirroring fit.PHASE_FEATURES/fit_phase's per-phase structure.
	models   map[string]*gbtmodel.Model
	fallback *BasicStrategy
}

// NewGBTStrategy constructs a GBTStrategy from already-loaded per-phase
// models (see LoadGBTModels), falling back to a BasicStrategy for any
// phase this strategy doesn't itself handle (setup_claim -- see
// ScoredStrategy's identical fallback rationale).
func NewGBTStrategy(models map[string]*gbtmodel.Model) *GBTStrategy {
	return &GBTStrategy{models: models, fallback: NewBasicStrategy()}
}

// LoadGBTModels loads the four per-phase models from dir, expecting
// files named exactly as analytics/src/global_conquest_analytics/
// gbt_fit.py's export_gbt writes them: attack.json, reinforce.json,
// occupy.json, fortify.json.
func LoadGBTModels(dir string) (map[string]*gbtmodel.Model, error) {
	phases := []string{"attack", "reinforce", "occupy", "fortify"}
	models := make(map[string]*gbtmodel.Model, len(phases))
	for _, phase := range phases {
		m, err := gbtmodel.LoadModel(filepath.Join(dir, phase+".json"))
		if err != nil {
			return nil, fmt.Errorf("bot: load %s model: %w", phase, err)
		}
		models[phase] = m
	}
	return models, nil
}

func (s *GBTStrategy) NextCommand(ctx context.Context, g *risk.Game, playerID string) (Command, Explanation, error) {
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

// attack scores every legal attack with the "attack" model and picks the
// highest predicted win probability, ending the attack phase instead when
// there's no legal attack or the best one falls below the model's own
// endPhaseThreshold.
func (s *GBTStrategy) attack(g *risk.Game, playerID string) (Command, Explanation, error) {
	actions := risk.LegalAttacks(g, playerID)
	pi := playerIndex(g, playerID)
	model := s.models["attack"]

	best := -1
	var bestProba float64
	for i, a := range actions {
		proba := model.PredictProba(attackSignalVector(computeAttackSignals(g, pi, a)))
		if best == -1 || proba > bestProba {
			best, bestProba = i, proba
		}
	}

	if best == -1 || bestProba < endPhaseThreshold(model) {
		return Command{Action: ActionEndAttack}, Explanation{Score: bestProba}, nil
	}
	a := actions[best]
	return Command{
		Action:       ActionAttack,
		From:         string(a.From),
		To:           string(a.To),
		AttackerDice: a.MaxAttackerDice,
	}, Explanation{Score: bestProba}, nil
}

// reinforce decides card timing first (scoredCardTurnIn, shared unchanged
// with ScoredStrategy), then scores every legal reinforcement territory
// with the "reinforce" model and places a capped batch at the top scorer
// -- same batching rule as ScoredStrategy.reinforce.
func (s *GBTStrategy) reinforce(g *risk.Game, playerID string) (Command, Explanation, error) {
	if cmd, expl, ok := scoredCardTurnIn(g, playerID); ok {
		return cmd, expl, nil
	}

	actions := risk.LegalReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, Explanation{}, fmt.Errorf("bot: no legal reinforcement for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	model := s.models["reinforce"]

	territories := make([]risk.Territory, len(actions))
	for i, a := range actions {
		territories[i] = a.Territory
	}
	best, bestProba := bestReinforceCandidate(g, pi, territories, model)

	cmd := Command{Action: ActionPlaceReinforcement, Territory: string(actions[best].Territory)}
	cmd.Armies = min(g.PendingReinforcements, max(1, g.PendingReinforcements/3))
	return cmd, Explanation{Score: bestProba}, nil
}

// setupReinforce uses the same "reinforce" model as reinforce, but places
// exactly one army per call (risk.PlaceInitialArmy's only legal amount).
func (s *GBTStrategy) setupReinforce(g *risk.Game, playerID string) (Command, Explanation, error) {
	actions := risk.LegalSetupReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, Explanation{}, fmt.Errorf("bot: no legal setup reinforcement for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	model := s.models["reinforce"]

	territories := make([]risk.Territory, len(actions))
	for i, a := range actions {
		territories[i] = a.Territory
	}
	best, bestProba := bestReinforceCandidate(g, pi, territories, model)
	return Command{Action: ActionPlaceInitialArmy, Territory: string(actions[best].Territory)}, Explanation{Score: bestProba}, nil
}

func bestReinforceCandidate(g *risk.Game, pi int, territories []risk.Territory, model *gbtmodel.Model) (best int, bestProba float64) {
	for i, t := range territories {
		proba := model.PredictProba(reinforceSignalVector(computeReinforceSignals(g, pi, t)))
		if i == 0 || proba > bestProba {
			best, bestProba = i, proba
		}
	}
	return best, bestProba
}

// occupy scores every legal army count to move into the just-conquered
// territory with the "occupy" model.
func (s *GBTStrategy) occupy(g *risk.Game, playerID string) (Command, Explanation, error) {
	actions := risk.LegalOccupations(g, playerID)
	if len(actions) == 0 {
		return Command{}, Explanation{}, fmt.Errorf("bot: no legal occupation for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	model := s.models["occupy"]

	sourceArmies := g.Territories[g.Occupy.From].Armies
	sourceThreat := adjacentEnemyArmies(g, g.Occupy.From, pi)
	destThreat := adjacentEnemyArmies(g, g.Occupy.To, pi)

	best := 0
	var bestProba float64
	for i, a := range actions {
		proba := model.PredictProba(occupySignalVector(computeOccupySignals(a.Armies, sourceArmies, sourceThreat, destThreat)))
		if i == 0 || proba > bestProba {
			best, bestProba = i, proba
		}
	}

	return Command{Action: ActionOccupy, Armies: actions[best].Armies}, Explanation{Score: bestProba}, nil
}

// fortify scores every legal fortification move with the "fortify"
// model, ending the turn without fortifying instead when there's no legal
// move or the best one falls below the model's own endPhaseThreshold.
func (s *GBTStrategy) fortify(g *risk.Game, playerID string) (Command, Explanation, error) {
	actions := risk.LegalFortifications(g, playerID)
	pi := playerIndex(g, playerID)
	model := s.models["fortify"]

	best := -1
	var bestProba float64
	for i, a := range actions {
		proba := model.PredictProba(fortifySignalVector(computeFortifySignals(g, pi, a)))
		if best == -1 || proba > bestProba {
			best, bestProba = i, proba
		}
	}

	if best == -1 || bestProba < endPhaseThreshold(model) {
		return Command{Action: ActionEndTurn}, Explanation{Score: bestProba}, nil
	}
	a := actions[best]
	return Command{Action: ActionFortify, From: string(a.From), To: string(a.To), Armies: a.MaxArmies}, Explanation{Score: bestProba}, nil
}

// attackSignalVector/reinforceSignalVector/occupySignalVector/
// fortifySignalVector convert a phase's raw signal struct into the fixed-
// order float64 vector the corresponding model expects. This order MUST
// exactly match analytics/src/global_conquest_analytics/fit.py's
// PHASE_FEATURES lists (the column order LightGBM's split_feature
// indices refer to at training time) -- hand-verified against it, not
// derived, for the same cross-language reason cmd/traindata's
// phaseFeatures table gives.

func attackSignalVector(sig attackSignals) []float64 {
	return []float64{
		sig.ArmyAdvantage,
		sig.CaptureProbability,
		sig.ExpectedLossCost,
		boolToFloat(sig.CompletesContinent),
		boolToFloat(sig.BreaksEnemyContinent),
		derefOrZero(sig.CardOpportunity),
		boolToFloat(sig.EliminatesPlayer),
		derefOrZero(sig.ExposurePenalty),
	}
}

func reinforceSignalVector(sig reinforceSignals) []float64 {
	return []float64{
		sig.EnemyThreat,
		sig.EnemyTerritoryCount,
		sig.Weakness,
		sig.ContinentValue,
		sig.ConcentrationPenalty,
	}
}

func occupySignalVector(sig occupySignals) []float64 {
	return []float64{sig.DefenseCoverage, sig.Momentum, sig.MomentumSurplus}
}

func fortifySignalVector(sig fortifySignals) []float64 {
	return []float64{sig.DestinationThreat, sig.ContinentValue, sig.SourceExposureCost}
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func derefOrZero(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
