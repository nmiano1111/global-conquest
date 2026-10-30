package bot

import (
	"context"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// StrategyDirectorV1 is the registry identifier for SymbolicDirectorStrategy,
// the fourth-pass symbolic strategic layer. Every prior pass
// (goal-killbot-v1, goal-killbot-v2) patched KillbotV2Strategy's own
// decisions after the fact in narrow, phase-specific spots -- the
// heuristic bot's own decision tree still ran and decided almost
// everything, and the "symbolic layer" only ever vetoed or nudged its
// output. This pass is a genuine director/executor split instead:
// SymbolicDirectorStrategy only ever decides WHETHER to pursue a goal
// (via the same desirable/feasible/lifecycle machinery goal-killbot-v2
// already validated, reused unchanged) and produces a Directive;
// DirectedPixieStrategy (strategy_directed_pixie.go) -- a genuine fork of
// BetterPixieStrategy, which stays completely untouched -- decides HOW
// using its own native tactical machinery, now directive-aware. See the
// design doc for the full architecture and the root-cause investigation
// (goal-killbot-v2's diagnosed tunnel-vision cost) that motivated this
// pass. Tournament/experimental only -- registered in cmd/tournament's
// registry, deliberately not cmd/backend's live registry.
const StrategyDirectorV1 = "director-v1"

// DirectorAbandonCause categorizes why continent might no longer be
// pursuable for pi -- exported for instrumentation (cmd/goalinstrumentv3,
// package main, cannot reach unexported functions). Re-walks the same
// fact/route/required-army checks completeContinentFeasible
// (strategy_goal_killbot_v2.go) makes internally, broken out so a caller
// can see WHICH check failed, not just whether. Built to answer this
// pass's central measurement question -- does unioning the goal
// continent into `wanted` reduce goal-killbot-v2's diagnosed 66.9%
// fact_gone abandonment rate (design doc §19.10)? -- kept as permanent
// instrumentation this time, not a throwaway diagnostic hook.
func DirectorAbandonCause(g *risk.Game, pi int, continent risk.Continent) string {
	fact, ok := completeContinentFactFor(g, pi, continent)
	if !ok {
		return "fact_gone"
	}
	from, to, _, ok := cheapestAttackHopToTerritory(g, pi, fact.MissingTerr)
	if !ok || to != fact.MissingTerr {
		return "route_gone"
	}
	required, ok := requiredAttackArmies(g.Territories[to].Armies, completeContinentAttackBiasFloor)
	if !ok {
		return "defender_too_strong"
	}
	if g.Territories[from].Armies >= required {
		return "feasible_only_inertia_cap_hit"
	}
	return "too_slow_to_build"
}

// DecisionEventV3 mirrors DecisionEventV2's instrumentation shape
// (strategy_goal_killbot_v2.go) -- a separate type, not reused, keeping
// this pass fully decoupled from goal-killbot-v2's file the same way
// goal-killbot-v2 stayed decoupled from goal-killbot-v1's. No
// BaseScore/StrategicModifier/FinalScore fields this pass: with tactical
// decisions now fully delegated to DirectedPixieStrategy's own priority-
// tier logic (the same no-numeric-score architecture BetterPixieStrategy
// has always had), there is no blended score to report even for attack,
// unlike goal-killbot-v2's WithAttackBias-based mechanism.
type DecisionEventV3 struct {
	Turn            int
	Phase           risk.Phase
	PlayerID        string
	GoalActive      bool
	GoalStatus      risk.GoalStatus
	GoalContinent   risk.Continent
	BaselineCommand Command
	ActualCommand   Command
	Changed         bool
}

// WithDirectorDecisionObserver registers a callback invoked once per
// decision with what the director considered and chose -- see
// DecisionEventV3. A nil observer (the default) costs nothing: NextCommand
// skips the extra baseline comparison entirely when observe is nil.
func WithDirectorDecisionObserver(fn func(DecisionEventV3)) func(*SymbolicDirectorStrategy) {
	return func(s *SymbolicDirectorStrategy) { s.observe = fn }
}

// SymbolicDirectorStrategy is the strategic decision-maker: it commits
// to, maintains, and abandons a CompleteContinentGoal exactly like
// GoalKillbotV2Strategy does (same desirable/feasible/lifecycle checks:
// completeContinentBestCandidate, completeContinentFeasible,
// completeContinentFactFor, all reused unchanged from strategy_goal_killbot_v2.go),
// but never constructs a tactical Command by hand. Every decision is
// delegated to a DirectedPixieStrategy instance, steered only via a
// Directive -- the director sets the goal, the executor decides the
// tactics (which territory, how many armies, which attack, how to
// occupy, how to fortify) using its own native heuristics.
type SymbolicDirectorStrategy struct {
	baseline *KillbotV2Strategy
	observe  func(DecisionEventV3)
}

// NewSymbolicDirectorStrategy creates a SymbolicDirectorStrategy.
func NewSymbolicDirectorStrategy(opts ...func(*SymbolicDirectorStrategy)) *SymbolicDirectorStrategy {
	s := &SymbolicDirectorStrategy{baseline: NewKillbotV2Strategy()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// executorFor builds a fresh DirectedPixieStrategy configured from tuning
// -- mirrors KillbotV2Strategy.backerFor exactly (strategy_killbot_v2.go:227-229),
// since tuning is admin-configurable per game (g.BotTuning) and a
// comparator baked in at construction time would go stale.
func executorFor(tuning risk.BotTuning) *DirectedPixieStrategy {
	return NewDirectedPixieStrategy(WithDirectedAttackComparator(winProbabilityComparator(tuning.MinAttackWinProbability)))
}

// NextCommand dispatches on g.Phase. When an observer is registered,
// also computes what plain KillbotV2Strategy would have chosen at the
// same, unmutated g and reports the comparison -- safe since
// Strategy.NextCommand is contractually read-only, same pattern every
// prior pass's instrumentation already used.
func (s *SymbolicDirectorStrategy) NextCommand(ctx context.Context, g *risk.Game, playerID string) (Command, Explanation, error) {
	cmd, err := s.nextCommand(g, playerID)
	if s.observe != nil && err == nil {
		if baselineCmd, _, baseErr := s.baseline.NextCommand(ctx, g, playerID); baseErr == nil {
			pi := playerIndex(g, playerID)
			goal, active := g.StrategicGoals[pi]
			s.observe(DecisionEventV3{
				Turn:            g.TurnNumber,
				Phase:           g.Phase,
				PlayerID:        playerID,
				GoalActive:      active,
				GoalStatus:      goal.Status,
				GoalContinent:   goal.Continent,
				BaselineCommand: baselineCmd,
				ActualCommand:   cmd,
				Changed:         cmd != baselineCmd,
			})
		}
	}
	return cmd, Explanation{}, err
}

func (s *SymbolicDirectorStrategy) nextCommand(g *risk.Game, playerID string) (Command, error) {
	tuning := effectiveBotTuning(g)
	executor := executorFor(tuning)
	switch g.Phase {
	case risk.PhaseReinforce:
		return s.reinforce(g, playerID, executor)
	case risk.PhaseAttack:
		return s.attack(g, playerID, executor)
	case risk.PhaseOccupy:
		return s.occupy(g, playerID, executor)
	case risk.PhaseFortify:
		return s.fortify(g, playerID, executor)
	default:
		// PhaseSetupReinforce and anything else: no goal applies before a
		// normal turn begins -- fall back to plain BetterPixieStrategy's
		// own setup handling via a fresh KillbotV2Strategy (matching every
		// prior pass's identical fallback for phases the goal mechanism
		// never touches).
		cmd, _, err := s.baseline.NextCommand(context.Background(), g, playerID)
		return cmd, err
	}
}

// reinforce owns the FULL goal lifecycle -- commit on FEASIBLE (not
// executable-now, the same fix goal-killbot-v2 already validated),
// advance to Securing on completion, abandon on lost feasibility or the
// inertia cap -- but, unlike goal-killbot-v2, never computes a specific
// territory or army count itself. It only ever builds a Directive
// (WantedContinents = {goal.Continent: true}, unioned by the executor
// with its own independent judgment, never replacing it) and delegates
// to executor.reinforce. Communicates the strategic layer's own computed
// army requirement via Directive.ArmyCap -- added after measurement
// (design doc §20.7) showed that WITHOUT it, ceding the stopping
// decision entirely to the executor's generic
// betterPixieContinentNeedsHelp threshold (pixieBorderForce = 20,
// unrelated to any specific tactical requirement) made the diagnosed
// tunnel-vision cost WORSE, not better: the goal continent, being by
// definition not fully owned while Active, always "needs help," so
// without a cap the executor kept reinforcing it turn after turn with no
// natural stopping point tied to what's actually needed.
func (s *SymbolicDirectorStrategy) reinforce(g *risk.Game, playerID string, executor *DirectedPixieStrategy) (Command, error) {
	if cmd, ok := voluntaryCardTurnIn(g, playerID); ok {
		return cmd, nil
	}
	pi := playerIndex(g, playerID)

	if goal, ok := g.StrategicGoals[pi]; ok && goal.Committed {
		if goal.Status == risk.GoalStatusSecuring {
			cmd, err := executor.reinforce(g, playerID, Directive{})
			if err == nil && cmd.Action == ActionPlaceReinforcement && g.TurnNumber-goal.StatusSince > completeContinentSecuringTurns {
				cmd.AbandonStrategicGoal = true
			}
			return cmd, err
		}

		if ownsContinent(g, pi, goal.Continent) {
			cmd, err := executor.reinforce(g, playerID, Directive{})
			if err == nil && cmd.Action == ActionPlaceReinforcement {
				cmd.StrategicGoalStatus = string(risk.GoalStatusSecuring)
			}
			return cmd, err
		}

		required, _, feasible := completeContinentFeasible(g, pi, goal.Continent, strategicHorizonTurns)
		if !feasible || g.TurnNumber-goal.StartTurn > completeContinentInertiaTurns {
			cmd, err := executor.reinforce(g, playerID, Directive{})
			if err == nil && cmd.Action == ActionPlaceReinforcement {
				cmd.AbandonStrategicGoal = true
			}
			return cmd, err
		}

		cmd, err := executor.reinforce(g, playerID, Directive{
			WantedContinents: map[risk.Continent]bool{goal.Continent: true},
			ArmyCap:          map[risk.Continent]int{goal.Continent: required},
		})
		if err == nil && cmd.Action == ActionPlaceReinforcement {
			cmd.StrategicGoal = string(goal.Continent)
		}
		return cmd, err
	}

	if fact, ok := completeContinentBestCandidate(g, pi); ok {
		if required, _, feasible := completeContinentFeasible(g, pi, fact.Continent, strategicHorizonTurns); feasible {
			cmd, err := executor.reinforce(g, playerID, Directive{
				WantedContinents: map[risk.Continent]bool{fact.Continent: true},
				ArmyCap:          map[risk.Continent]int{fact.Continent: required},
			})
			if err == nil && cmd.Action == ActionPlaceReinforcement {
				cmd.StrategicGoal = string(fact.Continent)
			}
			return cmd, err
		}
	}

	return executor.reinforce(g, playerID, Directive{})
}

// attack, occupy, and fortify all follow the same shape: build a
// Directive from the currently-committed goal (if Active; empty during
// Securing or with no goal at all -- occupy/fortify already had, and
// still have, no reinforce-analogous lifecycle work to do), delegate
// entirely to the executor. No PreserveAttackStack-style hard-constraint
// guard exists in this pass -- there is nothing to guard against: the
// executor's own attack search (betterPixieAttackInContinent) already
// only considers wanted continents before falling through to its
// unrelated-attack fallbacks, so an unrelated attack from the staging
// territory is only ever reached once the goal continent's own attack
// options are exhausted, not raced against them the way goal-killbot-v2's
// after-the-fact patch had to guard against.
func (s *SymbolicDirectorStrategy) attack(g *risk.Game, playerID string, executor *DirectedPixieStrategy) (Command, error) {
	return executor.attack(g, playerID, s.activeDirective(g, playerID))
}

func (s *SymbolicDirectorStrategy) occupy(g *risk.Game, playerID string, executor *DirectedPixieStrategy) (Command, error) {
	return executor.occupy(g, playerID, s.activeDirective(g, playerID))
}

// fortify uses FocusTerritory (the missing territory) while Active,
// mirroring goal-killbot-v2's own "prefer moves toward the missing
// territory's border" behavior. Unlike goal-killbot-v2, this pass does
// NOT reproduce the Securing-phase border-defense preference (continentBorders-based) --
// Directive only carries a single FocusTerritory, not a border set, and
// expanding it was out of scope for this pass's approved plan. During
// Securing, fortify falls back to the executor's plain, undirected
// judgment. A known, deliberate scope reduction versus goal-killbot-v2,
// flagged for a future pass if it turns out to matter.
func (s *SymbolicDirectorStrategy) fortify(g *risk.Game, playerID string, executor *DirectedPixieStrategy) (Command, error) {
	pi := playerIndex(g, playerID)
	goal, ok := g.StrategicGoals[pi]
	if !ok || !goal.Committed || goal.Status != risk.GoalStatusActive {
		return executor.fortify(g, playerID, Directive{})
	}
	fact, ok := completeContinentFactFor(g, pi, goal.Continent)
	if !ok {
		return executor.fortify(g, playerID, Directive{})
	}
	return executor.fortify(g, playerID, Directive{FocusTerritory: fact.MissingTerr})
}

// activeDirective builds the Directive for attack/occupy -- the goal's
// continent while Active, empty otherwise (no goal, or Securing, where
// there's no missing territory left to steer toward).
func (s *SymbolicDirectorStrategy) activeDirective(g *risk.Game, playerID string) Directive {
	pi := playerIndex(g, playerID)
	goal, ok := g.StrategicGoals[pi]
	if !ok || !goal.Committed || goal.Status != risk.GoalStatusActive {
		return Directive{}
	}
	return Directive{WantedContinents: map[risk.Continent]bool{goal.Continent: true}}
}
