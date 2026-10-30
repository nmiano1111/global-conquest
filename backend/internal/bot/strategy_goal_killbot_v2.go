package bot

import (
	"context"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

const (
	// StrategyGoalKillbotV2 is the registry identifier for
	// GoalKillbotV2Strategy, the third-pass symbolic strategic reasoning
	// layer: unlike goal-killbot-v1 (StrategyGoalKillbotV1,
	// strategy_goal_killbot.go, kept unmodified as tournament baseline
	// B), a goal here commits on FEASIBILITY within a short horizon, not
	// on already being executable with current armies -- see
	// project-docs/bot_player/phase_3_continuous_improvement/
	// 12_Symbolic_Strategic_Reasoning_Layer.md for the full design and
	// the root-cause investigation that motivated this pass.
	// Tournament/experimental only -- registered in cmd/tournament's
	// registry, deliberately not cmd/backend's live registry.
	StrategyGoalKillbotV2 = "goal-killbot-v2"

	// strategicHorizonTurns bounds how many future turns of projected
	// reinforcement income a goal may plausibly need before it's
	// considered infeasible -- a small, conservative first guess, not
	// tuned.
	strategicHorizonTurns = 2

	// completeContinentSecuringTurns is how long a goal stays in
	// GoalStatusSecuring (border-defense fortify bias) after its
	// continent is fully owned, before clearing outright -- a first
	// guess, not tuned.
	completeContinentSecuringTurns = 2

	// maxRequiredAttackArmiesSearch bounds requiredAttackArmies' forward
	// scan -- no plausible near-completion target needs anywhere close
	// to this many armies; it exists only to guarantee termination.
	maxRequiredAttackArmiesSearch = 60
)

// requiredAttackArmies returns the minimal attacker army count whose
// risk.ForecastAttack win probability against defenderArmies clears
// floor. risk.ForecastAttack's WinProbability is monotonically
// non-decreasing in attacker armies, so a simple bounded forward scan
// suffices -- no result under maxRequiredAttackArmiesSearch means "not
// plausible," not "keep searching."
func requiredAttackArmies(defenderArmies int, floor float64) (armies int, ok bool) {
	for a := 2; a <= maxRequiredAttackArmiesSearch; a++ {
		if risk.ForecastAttack(a, defenderArmies).WinProbability >= floor {
			return a, true
		}
	}
	return 0, false
}

// completeContinentFeasible answers "could continent's missing territory
// plausibly become executable within horizonTurns" -- NOT "will it
// succeed." Requires a real one-hop route right now (cheapestAttackHopToTerritory,
// geometry.go, unchanged), a bounded required-army estimate against the
// missing territory's CURRENT defenders (deliberately not projecting
// enemy reinforcement -- re-evaluated fresh every call, so an enemy
// buildup erodes feasibility on its own next turn rather than needing a
// separate model), and a conservative turns-to-build estimate using
// playerIncome (geometry.go, unchanged) that assumes ALL future income
// goes toward this one goal -- optimistic on that axis, offset by
// betterPixieHasUrgentContinentNeed's bypass below, which can skip a
// turn's contribution to it entirely.
// GoalKillbotV2Feasible exports completeContinentFeasible for
// instrumentation only (cmd/goalinstrumentv2, package main, cannot reach
// an unexported function) -- never consulted by the strategy itself
// through this name.
func GoalKillbotV2Feasible(g *risk.Game, pi int, continent risk.Continent, horizonTurns int) (requiredArmies, turnsNeeded int, ok bool) {
	return completeContinentFeasible(g, pi, continent, horizonTurns)
}

func completeContinentFeasible(g *risk.Game, pi int, continent risk.Continent, horizonTurns int) (requiredArmies, turnsNeeded int, ok bool) {
	fact, ok := completeContinentFactFor(g, pi, continent)
	if !ok {
		return 0, 0, false
	}
	from, to, _, ok := cheapestAttackHopToTerritory(g, pi, fact.MissingTerr)
	if !ok || to != fact.MissingTerr {
		return 0, 0, false
	}
	required, ok := requiredAttackArmies(g.Territories[to].Armies, completeContinentAttackBiasFloor)
	if !ok {
		return 0, 0, false
	}
	current := g.Territories[from].Armies
	if current >= required {
		return required, 0, true
	}
	income := max(1, playerIncome(g, pi))
	turnsNeeded = (required - current + income - 1) / income
	return required, turnsNeeded, turnsNeeded <= horizonTurns
}

// betterPixieHasUrgentContinentNeed reports whether the backer's OWN
// top-priority reinforcement tier (betterPixieContinentNeedsHelp,
// strategy_betterpixie.go, reused unchanged) currently has an urgent
// claim on some continent OTHER than excluding -- the "don't ignore
// existential threats" bypass: on any turn this is true, goal-directed
// reinforcement is skipped entirely (the backer handles the turn
// normally) without abandoning the goal. excluding is always the goal's
// own continent, since betterPixieContinentNeedsHelp trivially returns
// true for any continent pi doesn't yet fully own -- without excluding
// it, this check would always fire and the goal could never act.
func betterPixieHasUrgentContinentNeed(g *risk.Game, pi int, excluding risk.Continent) bool {
	wanted := betterPixieWantedContinents(g, pi, g.PendingReinforcements)
	for _, cont := range continentOrder(g) {
		if cont == excluding || !wanted[cont] {
			continue
		}
		if betterPixieContinentNeedsHelp(g, pi, cont) {
			return true
		}
	}
	return false
}

// GoalKillbotV2Strategy wraps KillbotV2Strategy (composition, matching
// goal-killbot-v1's own convention), deliberately independent of
// GoalKillbotStrategy (strategy_goal_killbot.go) rather than embedding
// it -- keeping the two passes fully decoupled so goal-killbot-v1 stays
// an intact, unmodified tournament baseline. Attack-phase influence
// reuses goalAttackBias (strategy_goal_killbot.go) verbatim via
// WithAttackBias, since it only reads StrategicGoal.Committed/Continent,
// neither removed by this pass. Reinforce/occupy/fortify are handled
// entirely within this file.
type GoalKillbotV2Strategy struct {
	*KillbotV2Strategy
	baseline *KillbotV2Strategy
	observe  func(DecisionEventV2)
}

// DecisionEventV2 reports what one GoalKillbotV2Strategy decision
// considered and chose -- instrumentation only, never consulted by the
// strategy itself. BaseScore/StrategicModifier/FinalScore are populated
// only for attack decisions, where risk.ForecastAttack's win probability
// and the bias delta are real numbers to report; reinforce/occupy/
// fortify have no numeric per-candidate score anywhere in this
// codebase's decision logic (see the design doc's Architecture Found
// section), so those three phases report zero there and rely on Reason
// alone -- a deliberate, honestly-documented limitation, not an
// oversight.
type DecisionEventV2 struct {
	Turn              int
	Phase             risk.Phase
	PlayerID          string
	GoalActive        bool
	GoalStatus        risk.GoalStatus
	GoalContinent     risk.Continent
	BaselineCommand   Command
	ActualCommand     Command
	Changed           bool
	Reason            string
	BaseScore         float64
	StrategicModifier float64
	FinalScore        float64
}

// WithDecisionObserverV2 registers a callback invoked once per decision
// with what the strategy considered and chose -- see DecisionEventV2. A
// nil observer (the default) costs nothing: NextCommand skips the extra
// baseline comparison entirely when observe is nil.
func WithDecisionObserverV2(fn func(DecisionEventV2)) func(*GoalKillbotV2Strategy) {
	return func(s *GoalKillbotV2Strategy) { s.observe = fn }
}

// NewGoalKillbotV2Strategy creates a GoalKillbotV2Strategy.
func NewGoalKillbotV2Strategy(opts ...func(*GoalKillbotV2Strategy)) *GoalKillbotV2Strategy {
	s := &GoalKillbotV2Strategy{
		KillbotV2Strategy: NewKillbotV2Strategy(WithAttackBias(goalAttackBias)),
		baseline:          NewKillbotV2Strategy(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// NextCommand dispatches on g.Phase, mirroring GoalKillbotStrategy's own
// shape (Go's struct embedding is not virtual dispatch). When an
// observer is registered, also computes what plain KillbotV2Strategy
// would have chosen at the same, unmutated g and reports the
// comparison -- safe since Strategy.NextCommand is contractually
// read-only.
func (s *GoalKillbotV2Strategy) NextCommand(ctx context.Context, g *risk.Game, playerID string) (Command, Explanation, error) {
	cmd, err := s.nextCommand(g, playerID)
	if s.observe != nil && err == nil {
		if baselineCmd, _, baseErr := s.baseline.NextCommand(ctx, g, playerID); baseErr == nil {
			pi := playerIndex(g, playerID)
			goal, active := g.StrategicGoals[pi]
			changed := cmd != baselineCmd
			s.observe(DecisionEventV2{
				Turn:            g.TurnNumber,
				Phase:           g.Phase,
				PlayerID:        playerID,
				GoalActive:      active,
				GoalStatus:      goal.Status,
				GoalContinent:   goal.Continent,
				BaselineCommand: baselineCmd,
				ActualCommand:   cmd,
				Changed:         changed,
				Reason:          decisionReasonV2(g, active, goal, changed),
			})
		}
	}
	return cmd, Explanation{}, err
}

// decisionReasonV2 is a coarse, best-effort categorical label for why a
// decision may have changed -- not a full per-candidate explanation
// (nothing in this codebase's reinforce/occupy/fortify logic exposes
// one, see DecisionEventV2's own doc comment), but enough to distinguish
// "goal preparing" from "goal securing" from "no active goal" in the
// instrumentation output.
func decisionReasonV2(g *risk.Game, active bool, goal risk.StrategicGoal, changed bool) string {
	switch {
	case !active:
		return "no active goal"
	case !changed:
		return "goal active, decision unchanged"
	case goal.Status == risk.GoalStatusSecuring:
		return "securing: border-defense bias"
	default:
		return "active: goal-directed bias/preemption toward " + string(goal.Continent)
	}
}

func (s *GoalKillbotV2Strategy) nextCommand(g *risk.Game, playerID string) (Command, error) {
	switch g.Phase {
	case risk.PhaseReinforce:
		return s.reinforce(g, playerID)
	case risk.PhaseAttack:
		return s.attack(g, playerID)
	case risk.PhaseOccupy:
		return s.occupy(g, playerID)
	case risk.PhaseFortify:
		return s.fortify(g, playerID)
	default:
		return s.KillbotV2Strategy.nextCommand(g, playerID)
	}
}

// reinforce manages CompleteContinentGoal's full lifecycle under the
// desirable/feasible/executable-now split (see the design doc): a goal
// commits on FEASIBLE alone (completeContinentFeasible, a bounded
// horizon projection), not on already being executable with current
// armies -- the direct fix for goal-killbot-v1's chicken-and-egg gate.
// Once committed and not yet executable, each call sends only the
// TOP-UP needed to reach the required army count (never the whole
// pending pool) to the staging territory, leaving any remainder for the
// backer's own placement later in the same turn (risk.Game.PlaceReinforcement
// allows partial placement across multiple same-turn calls -- an
// engine capability every prior strategy in this codebase left unused).
func (s *GoalKillbotV2Strategy) reinforce(g *risk.Game, playerID string) (Command, error) {
	if cmd, ok := voluntaryCardTurnIn(g, playerID); ok {
		return cmd, nil
	}
	pi := playerIndex(g, playerID)

	if goal, ok := g.StrategicGoals[pi]; ok && goal.Committed {
		if goal.Status == risk.GoalStatusSecuring {
			cmd, err := s.KillbotV2Strategy.reinforce(g, playerID)
			if err == nil && cmd.Action == ActionPlaceReinforcement && g.TurnNumber-goal.StatusSince > completeContinentSecuringTurns {
				cmd.AbandonStrategicGoal = true
			}
			return cmd, err
		}

		if ownsContinent(g, pi, goal.Continent) {
			cmd, err := s.KillbotV2Strategy.reinforce(g, playerID)
			if err == nil && cmd.Action == ActionPlaceReinforcement {
				cmd.StrategicGoalStatus = string(risk.GoalStatusSecuring)
			}
			return cmd, err
		}

		required, _, feasible := completeContinentFeasible(g, pi, goal.Continent, strategicHorizonTurns)
		if !feasible || g.TurnNumber-goal.StartTurn > completeContinentInertiaTurns {
			cmd, err := s.KillbotV2Strategy.reinforce(g, playerID)
			if err == nil && cmd.Action == ActionPlaceReinforcement {
				cmd.AbandonStrategicGoal = true
			}
			return cmd, err
		}
		if betterPixieHasUrgentContinentNeed(g, pi, goal.Continent) {
			return s.KillbotV2Strategy.reinforce(g, playerID)
		}
		fact, ok := completeContinentFactFor(g, pi, goal.Continent)
		if !ok {
			cmd, err := s.KillbotV2Strategy.reinforce(g, playerID)
			if err == nil && cmd.Action == ActionPlaceReinforcement {
				cmd.AbandonStrategicGoal = true
			}
			return cmd, err
		}
		from, to, _, ok := cheapestAttackHopToTerritory(g, pi, fact.MissingTerr)
		if !ok || to != fact.MissingTerr {
			cmd, err := s.KillbotV2Strategy.reinforce(g, playerID)
			if err == nil && cmd.Action == ActionPlaceReinforcement {
				cmd.AbandonStrategicGoal = true
			}
			return cmd, err
		}
		need := required - g.Territories[from].Armies
		if need <= 0 {
			return s.KillbotV2Strategy.reinforce(g, playerID)
		}
		send := min(need, g.PendingReinforcements)
		cmd := Command{Action: ActionPlaceReinforcement, Territory: string(from), Armies: send, StrategicGoal: string(goal.Continent)}
		return s.KillbotV2Strategy.guardPlacement(g, playerID, cmd, risk.LegalReinforcements(g, playerID)), nil
	}

	// Deliberately no betterPixieHasUrgentContinentNeed bypass here on
	// fresh discovery (unlike the already-committed branch above): before
	// pi owns any full continent, betterPixieWantedContinents always
	// names exactly one "wanted" continent (its own early-game
	// pick-a-target heuristic) -- and since pi owns 0% of it,
	// betterPixieContinentNeedsHelp trivially returns true for it
	// regardless of whether it's a real threat. Applying the bypass here
	// would block nearly every fresh commitment on that false signal
	// (confirmed empirically: with only south_america partially owned,
	// betterPixieWantedContinents named an unrelated, entirely
	// unclaimed continent as "urgent"). The bypass's real purpose --
	// protecting an ALREADY-committed goal from a genuinely newer,
	// competing need -- only makes sense once something is actually
	// committed to protect.
	if fact, ok := completeContinentBestCandidate(g, pi); ok {
		required, _, feasible := completeContinentFeasible(g, pi, fact.Continent, strategicHorizonTurns)
		if feasible {
			from, to, _, routeOK := cheapestAttackHopToTerritory(g, pi, fact.MissingTerr)
			if routeOK && to == fact.MissingTerr {
				need := required - g.Territories[from].Armies
				if need <= 0 {
					cmd, err := s.KillbotV2Strategy.reinforce(g, playerID)
					if err == nil && cmd.Action == ActionPlaceReinforcement {
						cmd.StrategicGoal = string(fact.Continent)
					}
					return cmd, err
				}
				send := min(need, g.PendingReinforcements)
				cmd := Command{Action: ActionPlaceReinforcement, Territory: string(from), Armies: send, StrategicGoal: string(fact.Continent)}
				return s.KillbotV2Strategy.guardPlacement(g, playerID, cmd, risk.LegalReinforcements(g, playerID)), nil
			}
		}
	}

	return s.KillbotV2Strategy.reinforce(g, playerID)
}

// attack delegates to KillbotV2Strategy.attack first (which already
// tries the goal's own biased target via the embedded WithAttackBias
// hook -- see goalAttackBias, strategy_goal_killbot.go, reused
// verbatim). Then, only while the goal is Active, blocks (PreserveAttackStack)
// an unrelated attack the backer chose to launch FROM the goal's staging
// territory -- letting that fire would spend armies this pass has been
// specifically accumulating there, possibly over several turns, on a
// conquest unrelated to the goal. This is a narrow HARD constraint, not
// a soft bias: no numeric attack-priority score exists anywhere in this
// codebase to blend a bias into for this decision (see the design doc's
// Architecture Found section). Ending the attack phase early on this
// specific, narrow trigger forfeits any other attacks that same call --
// a real, deliberately-flagged risk, measured directly by the
// "harmful opportunity cost" diagnostic question.
func (s *GoalKillbotV2Strategy) attack(g *risk.Game, playerID string) (Command, error) {
	cmd, err := s.KillbotV2Strategy.attack(g, playerID)
	if err != nil || cmd.Action != ActionAttack {
		return cmd, err
	}
	pi := playerIndex(g, playerID)
	goal, ok := g.StrategicGoals[pi]
	if !ok || !goal.Committed || goal.Status != risk.GoalStatusActive {
		return cmd, nil
	}
	fact, ok := completeContinentFactFor(g, pi, goal.Continent)
	if !ok || risk.Territory(cmd.To) == fact.MissingTerr {
		return cmd, nil
	}
	staging, to, _, ok := cheapestAttackHopToTerritory(g, pi, fact.MissingTerr)
	if !ok || to != fact.MissingTerr || risk.Territory(cmd.From) != staging {
		return cmd, nil
	}
	return Command{Action: ActionEndAttack}, nil
}

// occupy reuses goal-killbot-v1's own logic verbatim (copied, not
// shared via embedding, to keep the two passes structurally independent
// -- see the type's own doc comment): force max legal armies only when
// the just-conquered territory is the exact missing territory.
func (s *GoalKillbotV2Strategy) occupy(g *risk.Game, playerID string) (Command, error) {
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

// fortify: while Active, prefers moves adjacent to the missing territory
// (BuildAttackForce's fortify-side counterpart), and if no such
// candidate exists, blocks (PreserveAttackStack, fortify's low-blast-radius
// counterpart to attack's guard above -- fortify is a single decision
// per turn regardless, so declining it via ActionEndTurn forfeits at
// most one move) the backer's own choice if it would move armies OUT of
// the staging territory. While Securing, prefers moves toward the
// completed continent's external borders (SecureContinentBorder).
func (s *GoalKillbotV2Strategy) fortify(g *risk.Game, playerID string) (Command, error) {
	pi := playerIndex(g, playerID)
	goal, ok := g.StrategicGoals[pi]
	if !ok || !goal.Committed {
		return s.KillbotV2Strategy.fortify(g, playerID)
	}

	if goal.Status == risk.GoalStatusSecuring {
		var candidates []risk.FortificationAction
		for _, a := range risk.LegalFortifications(g, playerID) {
			for _, b := range continentBorders(g, goal.Continent) {
				if a.To == b {
					candidates = append(candidates, a)
					break
				}
			}
		}
		if best, _, ok := bestFortifyDestination(g, pi, candidates); ok {
			return Command{Action: ActionFortify, From: string(best.From), To: string(best.To), Armies: best.MaxArmies}, nil
		}
		return s.KillbotV2Strategy.fortify(g, playerID)
	}

	fact, ok := completeContinentFactFor(g, pi, goal.Continent)
	if !ok {
		return s.KillbotV2Strategy.fortify(g, playerID)
	}
	var candidates []risk.FortificationAction
	for _, a := range risk.LegalFortifications(g, playerID) {
		if _, adj := g.Board.Adjacent[a.To][fact.MissingTerr]; adj {
			candidates = append(candidates, a)
		}
	}
	if best, _, ok := bestFortifyDestination(g, pi, candidates); ok {
		return Command{Action: ActionFortify, From: string(best.From), To: string(best.To), Armies: best.MaxArmies}, nil
	}
	staging, to, _, ok := cheapestAttackHopToTerritory(g, pi, fact.MissingTerr)
	if !ok || to != fact.MissingTerr {
		return s.KillbotV2Strategy.fortify(g, playerID)
	}
	cmd, err := s.KillbotV2Strategy.fortify(g, playerID)
	if err == nil && cmd.Action == ActionFortify && risk.Territory(cmd.From) == staging {
		return Command{Action: ActionEndTurn}, nil
	}
	return cmd, err
}
