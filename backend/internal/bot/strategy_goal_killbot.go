package bot

import (
	"context"

	"github.com/nmiano1111/global-conquest/backend/internal/bot/symbolic"
	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

const (
	// StrategyGoalKillbotV1 is the registry identifier for
	// GoalKillbotStrategy, the second-pass symbolic strategic reasoning
	// layer: a single persistent goal type (CompleteContinentGoal) with a
	// real lifecycle, integrated across all four turn phases -- unlike
	// the first pass's single-decision attack-only nudge (reverted; see
	// project-docs/bot_player/phase_3_continuous_improvement/
	// 12_Symbolic_Strategic_Reasoning_Layer.md for the full retrospective
	// and this pass's design). Tournament/experimental only -- registered
	// in cmd/tournament's registry, deliberately not cmd/backend's live
	// registry until validated.
	StrategyGoalKillbotV1 = "goal-killbot-v1"

	// completeContinentAttackBiasDelta/Floor mirror the first pass's
	// constants exactly (same untuned first guess, same floor mechanism)
	// -- the attack-phase mechanism itself is unchanged from pass 1, only
	// now gated by a persisted, multi-turn commitment instead of a fact
	// recomputed fresh every single decision.
	completeContinentAttackBiasDelta = 0.15
	completeContinentAttackBiasFloor = 0.35

	// completeContinentReinforceMinWinProbability gates the reinforce-phase
	// preemption tier (see (*GoalKillbotStrategy).reinforce) -- a strict,
	// single-territory, single-route condition, exactly as narrow as
	// killTarget/cheapestAttackHopToPlayer's own gate in
	// KillbotV2Strategy.reinforce.
	completeContinentReinforceMinWinProbability = 0.35

	// completeContinentInertiaTurns caps how many turns a committed goal
	// may persist without completing before it's abandoned -- a first
	// guess (long enough to survive normal reinforcement buildup toward
	// one territory, short enough not to tie the bot to a stalled front
	// indefinitely), untuned, same treatment every other constant in this
	// file's history has gotten.
	completeContinentInertiaTurns = 8
)

// DecisionEvent reports what one GoalKillbotStrategy decision considered
// and chose, for instrumentation only -- never consulted by the strategy
// itself, and computing it (the extra baseline.NextCommand call) only
// happens when an observer is registered via WithDecisionObserver.
type DecisionEvent struct {
	Turn     int
	Phase    risk.Phase
	PlayerID string
	// GoalActive reports whether a committed StrategicGoal existed for
	// this player at the moment of this decision. Also used as a
	// best-effort proxy for "the goal mechanism had an opinion to offer
	// here" (BiasConsidered) -- this package does not currently thread a
	// finer per-phase "the strict gate actually matched" signal out to
	// instrumentation; GoalActive is the honest, cheaply-available
	// approximation, not a precise measurement of gate evaluation.
	GoalActive    bool
	GoalContinent risk.Continent
	// BaselineCommand is what plain, bias-free KillbotV2Strategy chose at
	// this same, unmutated game state -- safe to compute because
	// Strategy.NextCommand is contractually read-only.
	BaselineCommand Command
	// ActualCommand is what GoalKillbotStrategy actually chose.
	ActualCommand Command
	// Changed reports whether ActualCommand differs from BaselineCommand
	// -- the single most important number this instrumentation produces:
	// a rule firing that wouldn't have changed the baseline's choice
	// anyway is not interesting.
	Changed bool
}

// GoalKillbotStrategy wraps KillbotV2Strategy (composition, matching the
// project's established v1->v2 wrapping convention), adding one
// persistent goal type -- CompleteContinentGoal -- integrated across all
// four turn phases. Attack-phase influence is a true soft bias, supplied
// to the embedded KillbotV2Strategy via WithAttackBias
// (strategy_killbot_v2.go); reinforce/occupy/fortify are handled
// entirely within this file via narrowly-gated preemption -- see each
// method's own doc comment for why that's the correct shape (not a soft
// bias) for those three phases, and the design doc's revised
// hard-vs-soft section for the justification this requires.
type GoalKillbotStrategy struct {
	*KillbotV2Strategy
	// baseline is a plain, bias-free KillbotV2Strategy used only for
	// instrumentation (see WithDecisionObserver) -- never consulted by
	// this strategy's own decisions.
	baseline *KillbotV2Strategy
	observe  func(DecisionEvent)
}

// WithDecisionObserver registers a callback invoked once per decision
// with what the strategy considered and chose -- see DecisionEvent. A
// nil observer (the default) costs nothing: NextCommand skips the extra
// baseline comparison entirely when observe is nil.
func WithDecisionObserver(fn func(DecisionEvent)) func(*GoalKillbotStrategy) {
	return func(s *GoalKillbotStrategy) { s.observe = fn }
}

// NewGoalKillbotStrategy creates a GoalKillbotStrategy.
func NewGoalKillbotStrategy(opts ...func(*GoalKillbotStrategy)) *GoalKillbotStrategy {
	s := &GoalKillbotStrategy{
		KillbotV2Strategy: NewKillbotV2Strategy(WithAttackBias(goalAttackBias)),
		baseline:          NewKillbotV2Strategy(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// NextCommand dispatches on g.Phase, mirroring KillbotV2Strategy's own
// shape -- GoalKillbotStrategy needs its own dispatch (not just embedded
// KillbotV2Strategy.NextCommand) because Go's struct embedding is not
// virtual dispatch: KillbotV2Strategy.nextCommand calls its own
// k.reinforce/k.attack, statically bound to *KillbotV2Strategy, never
// this type's overrides. When an observer is registered, also computes
// what plain KillbotV2Strategy would have chosen at the same,
// unmutated g and reports the comparison -- safe since Strategy is
// contractually read-only.
func (s *GoalKillbotStrategy) NextCommand(ctx context.Context, g *risk.Game, playerID string) (Command, Explanation, error) {
	cmd, err := s.nextCommand(g, playerID)
	if s.observe != nil && err == nil {
		if baselineCmd, _, baseErr := s.baseline.NextCommand(ctx, g, playerID); baseErr == nil {
			pi := playerIndex(g, playerID)
			goal, active := g.StrategicGoals[pi]
			s.observe(DecisionEvent{
				Turn:            g.TurnNumber,
				Phase:           g.Phase,
				PlayerID:        playerID,
				GoalActive:      active,
				GoalContinent:   goal.Continent,
				BaselineCommand: baselineCmd,
				ActualCommand:   cmd,
				Changed:         cmd != baselineCmd,
			})
		}
	}
	return cmd, Explanation{}, err
}

func (s *GoalKillbotStrategy) nextCommand(g *risk.Game, playerID string) (Command, error) {
	switch g.Phase {
	case risk.PhaseReinforce:
		return s.reinforce(g, playerID)
	case risk.PhaseAttack:
		return s.KillbotV2Strategy.attack(g, playerID)
	case risk.PhaseOccupy:
		return s.occupy(g, playerID)
	case risk.PhaseFortify:
		return s.fortify(g, playerID)
	default:
		// PhaseSetupReinforce and anything else: no goal applies before
		// a normal turn begins, or at all -- delegate straight through.
		return s.KillbotV2Strategy.nextCommand(g, playerID)
	}
}

// reinforce manages CompleteContinentGoal's full lifecycle (discovery,
// viability, inertia-based abandonment, completion) and, only when a
// committed goal's missing territory has a real same-turn attack route
// that would already clear a (relaxed) win-probability bar with this
// turn's pending reinforcements added, steers reinforcement there -- the
// same narrow shape as KillbotV2Strategy.reinforce's own kill-hunt tier
// (strict match on one specific target/route, not "a goal is active"
// broadly). Everything else (no goal, or a goal whose gate doesn't clear
// this turn) falls through to KillbotV2Strategy.reinforce completely
// unchanged.
func (s *GoalKillbotStrategy) reinforce(g *risk.Game, playerID string) (Command, error) {
	// Mandatory/voluntary card trade-in must be checked before any goal
	// logic, exactly as KillbotV2Strategy.reinforce's own first line
	// does -- a card trade can be required (ForcedCardTrade) or simply
	// available before any reinforcement placement is legal at all;
	// skipping this check let goal logic return a place_reinforcement
	// command the engine correctly rejected ("must trade cards first",
	// or "invalid reinforcement count" when PendingReinforcements was
	// still 0 pending the trade -- found via a 300-game tournament run
	// that surfaced a real engine_rejected_command failure class no
	// prior killbot-v2-family strategy this session ever produced).
	if cmd, ok := voluntaryCardTurnIn(g, playerID); ok {
		return cmd, nil
	}

	pi := playerIndex(g, playerID)

	if goal, ok := g.StrategicGoals[pi]; ok && goal.Committed {
		if ownsContinent(g, pi, goal.Continent) {
			return s.abandonGoalCommand(g, playerID)
		}
		if g.TurnNumber-goal.StartTurn > completeContinentInertiaTurns || !completeContinentGoalViable(g, pi, goal.Continent) {
			return s.abandonGoalCommand(g, playerID)
		}
		if cmd, ok := tryCompleteContinentReinforce(g, playerID, pi, goal.Continent); ok {
			return s.KillbotV2Strategy.guardPlacement(g, playerID, cmd, risk.LegalReinforcements(g, playerID)), nil
		}
		return s.KillbotV2Strategy.reinforce(g, playerID)
	}

	if fact, ok := completeContinentBestCandidate(g, pi); ok {
		if completeContinentGoalViable(g, pi, fact.Continent) {
			if cmd, ok := tryCompleteContinentReinforce(g, playerID, pi, fact.Continent); ok {
				cmd.StrategicGoal = string(fact.Continent)
				return s.KillbotV2Strategy.guardPlacement(g, playerID, cmd, risk.LegalReinforcements(g, playerID)), nil
			}
		}
	}

	return s.KillbotV2Strategy.reinforce(g, playerID)
}

// abandonGoalCommand builds the reinforce command that clears the
// current goal (completion or abandonment -- indistinguishable at the
// engine level, see ClearStrategicGoal's doc comment) while still making
// a normal placement decision this turn via the backer, so abandoning a
// goal never wastes this turn's reinforcements.
func (s *GoalKillbotStrategy) abandonGoalCommand(g *risk.Game, playerID string) (Command, error) {
	cmd, err := s.KillbotV2Strategy.reinforce(g, playerID)
	if err != nil || cmd.Action != ActionPlaceReinforcement {
		return cmd, err
	}
	cmd.AbandonStrategicGoal = true
	return cmd, nil
}

// occupy forces max-legal-armies occupation only when the just-conquered
// territory IS the committed goal's exact missing territory -- stricter
// than KillbotStrategy.occupy's own committed-kill-plan precedent (which
// fires for any conquest during a committed kill), deliberately
// tightened per the design doc's caution against broad preemption.
// Anything else falls through to KillbotV2Strategy's own occupy handling
// (KillbotStrategy.occupy, unchanged) completely unchanged.
func (s *GoalKillbotStrategy) occupy(g *risk.Game, playerID string) (Command, error) {
	pi := playerIndex(g, playerID)
	if goal, ok := g.StrategicGoals[pi]; ok && goal.Committed && g.Occupy != nil {
		if fact, ok := completeContinentFactFor(g, pi, goal.Continent); ok && g.Occupy.To == fact.MissingTerr {
			actions := risk.LegalOccupations(g, playerID)
			if len(actions) > 0 {
				return Command{Action: ActionOccupy, Armies: actions[len(actions)-1].Armies}, nil
			}
		}
	}
	return s.KillbotV2Strategy.nextCommand(g, playerID)
}

// fortify steers the one allowed fortify move toward the committed
// goal's attack front -- pre-filters risk.LegalFortifications to
// candidates whose To is directly adjacent to the missing territory
// (i.e. the specific staging point for next turn's attack), then reuses
// the existing bestFortifyDestination helper unchanged (geometry.go,
// zero new scoring code). An empty filtered set (no owned territory
// borders the missing territory yet) falls through to
// KillbotV2Strategy's own fortify handling unchanged.
func (s *GoalKillbotStrategy) fortify(g *risk.Game, playerID string) (Command, error) {
	pi := playerIndex(g, playerID)
	if goal, ok := g.StrategicGoals[pi]; ok && goal.Committed {
		if fact, ok := completeContinentFactFor(g, pi, goal.Continent); ok {
			var candidates []risk.FortificationAction
			for _, a := range risk.LegalFortifications(g, playerID) {
				if _, adjacent := g.Board.Adjacent[a.To][fact.MissingTerr]; adjacent {
					candidates = append(candidates, a)
				}
			}
			if best, _, ok := bestFortifyDestination(g, pi, candidates); ok {
				return Command{Action: ActionFortify, From: string(best.From), To: string(best.To), Armies: best.MaxArmies}, nil
			}
		}
	}
	return s.KillbotV2Strategy.nextCommand(g, playerID)
}

// goalAttackBias is the attackBiasFunc (strategy_killbot_v2.go) supplied
// to the embedded KillbotV2Strategy: relaxes the win-probability bar for
// the exact missing territory of pi's own committed CompleteContinentGoal,
// floored so it never drops below completeContinentAttackBiasFloor.
// Reads g.StrategicGoals directly rather than internal/bot/symbolic's
// own SelfNearCompletion, since the specific continent is already fixed
// by the commitment -- only its current missing territory needs
// re-deriving fresh (completeContinentFactFor).
func goalAttackBias(g *risk.Game, pi int, from, to risk.Territory) (float64, string) {
	goal, ok := g.StrategicGoals[pi]
	if !ok || !goal.Committed {
		return 0, ""
	}
	fact, ok := completeContinentFactFor(g, pi, goal.Continent)
	if !ok || fact.MissingTerr != to {
		return 0, ""
	}
	return completeContinentAttackBiasDelta, "completes a committed CompleteContinentGoal"
}

// completeContinentFactFor re-derives whether continent is still exactly
// one territory from pi's full ownership, fresh from current board
// state -- StrategicGoal deliberately doesn't persist MissingTerr itself
// (see its doc comment), so every reader re-derives it this way.
func completeContinentFactFor(g *risk.Game, pi int, continent risk.Continent) (symbolic.ContinentCompletionFact, bool) {
	for _, fact := range symbolic.SelfNearCompletion(g, pi) {
		if fact.Continent == continent {
			return fact, true
		}
	}
	return symbolic.ContinentCompletionFact{}, false
}

// completeContinentBestCandidate picks the highest-Bonus fresh candidate
// among symbolic.SelfNearCompletion -- a trivial tie-break since this
// pass ships exactly one goal type (no cross-goal arbitration problem
// yet; see the design doc's Goal Lifecycle section for why that only
// becomes real once a second, competing goal type exists).
func completeContinentBestCandidate(g *risk.Game, pi int) (symbolic.ContinentCompletionFact, bool) {
	facts := symbolic.SelfNearCompletion(g, pi)
	if len(facts) == 0 {
		return symbolic.ContinentCompletionFact{}, false
	}
	best := facts[0]
	for _, f := range facts[1:] {
		if f.Bonus > best.Bonus {
			best = f
		}
	}
	return best, true
}

// completeContinentGoalViable reports whether continent is still a
// realistic target for pi: still a genuine near-completion fact (not
// already complete, not lost to more contested ownership), a route to
// the missing territory exists at all, and the missing territory's
// defense isn't hopeless even generously assuming pi's whole current
// army stack on the routed source territory -- a deliberately looser bar
// than the reinforce-phase commit gate itself (completeContinentReinforceMinWinProbability),
// so a merely-not-favorable-this-turn goal isn't abandoned the instant a
// single turn's forecast dips.
func completeContinentGoalViable(g *risk.Game, pi int, continent risk.Continent) bool {
	fact, ok := completeContinentFactFor(g, pi, continent)
	if !ok {
		return false
	}
	from, to, _, ok := cheapestAttackHopToTerritory(g, pi, fact.MissingTerr)
	if !ok || to != fact.MissingTerr {
		return false
	}
	src := g.Territories[from]
	forecast := risk.ForecastAttack(src.Armies, g.Territories[to].Armies)
	return forecast.WinProbability >= completeContinentAttackBiasFloor
}

// tryCompleteContinentReinforce is reinforce's strict gate: the missing
// territory of continent must be reachable in exactly one hop right now
// (not merely somewhere in the continent), and the forecasted win
// probability after adding this turn's pending reinforcements must clear
// completeContinentReinforceMinWinProbability.
func tryCompleteContinentReinforce(g *risk.Game, playerID string, pi int, continent risk.Continent) (Command, bool) {
	fact, ok := completeContinentFactFor(g, pi, continent)
	if !ok {
		return Command{}, false
	}
	from, to, _, ok := cheapestAttackHopToTerritory(g, pi, fact.MissingTerr)
	if !ok || to != fact.MissingTerr {
		return Command{}, false
	}
	src := g.Territories[from]
	forecast := risk.ForecastAttack(src.Armies+g.PendingReinforcements, g.Territories[to].Armies)
	if forecast.WinProbability < completeContinentReinforceMinWinProbability {
		return Command{}, false
	}
	return Command{Action: ActionPlaceReinforcement, Territory: string(from), Armies: g.PendingReinforcements}, true
}
