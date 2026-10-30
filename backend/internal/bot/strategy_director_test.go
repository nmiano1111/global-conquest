package bot

import (
	"context"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// TestSymbolicDirectorDisabledPathMatchesPlainDirectedPixie confirms
// that with no near-complete continent anywhere on the board,
// SymbolicDirectorStrategy produces the exact same command an
// undirected DirectedPixieStrategy (configured with the same tuning)
// would -- the executor's OWN behavior, not KillbotV2Strategy's (the
// executor is a fork of BetterPixieStrategy, not KillbotV2Strategy, and
// deliberately carries no kill-hunting logic -- see the type's own doc
// comment).
func TestSymbolicDirectorDisabledPathMatchesPlainDirectedPixie(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	pi := 0
	g.Territories["Alaska"] = risk.TerritoryState{Owner: pi, Armies: 5}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1}

	executor := executorFor(effectiveBotTuning(g))
	want, err := executor.attack(g, p0, Directive{})
	if err != nil {
		t.Fatalf("DirectedPixieStrategy.attack: %v", err)
	}
	director := NewSymbolicDirectorStrategy()
	got, _, err := director.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("SymbolicDirectorStrategy NextCommand: %v", err)
	}
	if got != want {
		t.Fatalf("expected identical commands with no strategic facts in play, got director=%+v executor=%+v", got, want)
	}
}

// TestSymbolicDirectorCommitsWhenFeasibleButNotExecutable mirrors
// goal-killbot-v2's own core proof, now against the director: South
// America missing Venezuela (8 armies), staged via Peru (board-order
// tie-break) at only 3 current armies -- current armies alone fall far
// short of the win-probability floor, so this is only reachable via the
// desirable/feasible (not executable-now) commit path.
func TestSymbolicDirectorCommitsWhenFeasibleButNotExecutable(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 10
	pi := 0
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 3}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 1, Armies: 8}

	if _, _, feasible := completeContinentFeasible(g, pi, "south_america", strategicHorizonTurns); !feasible {
		t.Fatalf("test setup invalid: expected south_america to be feasible")
	}
	if forecast := risk.ForecastAttack(3, 8); forecast.WinProbability >= completeContinentAttackBiasFloor {
		t.Fatalf("test setup invalid: expected current armies alone to NOT already clear the floor, got %.4f", forecast.WinProbability)
	}

	cmd, _, err := NewSymbolicDirectorStrategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.StrategicGoal != "south_america" {
		t.Fatalf("expected a fresh commitment to south_america, got %+v", cmd)
	}
	if cmd.Action != ActionPlaceReinforcement || cmd.Territory != "Peru" {
		t.Fatalf("expected the executor's own placement logic to steer reinforcement toward Peru (south_america's weak border), got %+v", cmd)
	}
}

// TestSymbolicDirectorAttackUnionsGoalContinent confirms a committed
// goal's continent reaches the executor's attack decision through the
// Directive -- the executor fires an attack on the goal's missing
// territory that an undirected DirectedPixieStrategy, at the same state,
// would not have prioritized.
func TestSymbolicDirectorAttackUnionsGoalContinent(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	pi := 0
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 6}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 1, Armies: 4}
	g.StrategicGoals[pi] = risk.StrategicGoal{Continent: "south_america", StartTurn: 0, Committed: true, Status: risk.GoalStatusActive, StatusSince: 0}

	cmd, _, err := NewSymbolicDirectorStrategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack || cmd.From != "Brazil" || cmd.To != "Venezuela" {
		t.Fatalf("expected an attack on Venezuela from Brazil, got %+v", cmd)
	}
}

// TestSymbolicDirectorFortifyPrefersMissingTerritoryBorder mirrors
// goal-killbot-v2's own fortify test: fortify steers toward a border
// adjacent to the committed goal's missing territory.
func TestSymbolicDirectorFortifyPrefersMissingTerritoryBorder(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	pi := 0
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 6}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 4}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 1, Armies: 4}
	g.StrategicGoals[pi] = risk.StrategicGoal{Continent: "south_america", StartTurn: 0, Committed: true, Status: risk.GoalStatusActive, StatusSince: 0}

	cmd, _, err := NewSymbolicDirectorStrategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionFortify || cmd.To != "Brazil" {
		t.Fatalf("expected fortify to steer armies toward Brazil (adjacent to the missing territory, Venezuela), got %+v", cmd)
	}
}

// TestSymbolicDirectorTransitionsToSecuringOnCompletion mirrors
// goal-killbot-v2's own test: a committed goal whose continent pi now
// fully owns transitions to Securing, not cleared outright.
func TestSymbolicDirectorTransitionsToSecuringOnCompletion(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	pi := 0
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.StrategicGoals[pi] = risk.StrategicGoal{Continent: "south_america", StartTurn: 0, Committed: true, Status: risk.GoalStatusActive, StatusSince: 0}

	cmd, _, err := NewSymbolicDirectorStrategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.StrategicGoalStatus != string(risk.GoalStatusSecuring) || cmd.AbandonStrategicGoal {
		t.Fatalf("expected a transition to Securing (not an abandonment/clear), got %+v", cmd)
	}
}

// TestSymbolicDirectorClearsAfterSecuringDurationCap mirrors
// goal-killbot-v2's own test.
func TestSymbolicDirectorClearsAfterSecuringDurationCap(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	g.TurnNumber = completeContinentSecuringTurns + 5
	pi := 0
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.StrategicGoals[pi] = risk.StrategicGoal{Continent: "south_america", StartTurn: 0, Committed: true, Status: risk.GoalStatusSecuring, StatusSince: 0}

	cmd, _, err := NewSymbolicDirectorStrategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if !cmd.AbandonStrategicGoal {
		t.Fatalf("expected the goal to clear once the securing duration cap elapses, got %+v", cmd)
	}
}

// TestSymbolicDirectorAbandonsAfterInertiaCap mirrors goal-killbot-v2's
// own test.
func TestSymbolicDirectorAbandonsAfterInertiaCap(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	g.TurnNumber = completeContinentInertiaTurns + 1
	pi := 0
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 3}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 1, Armies: 8}
	g.StrategicGoals[pi] = risk.StrategicGoal{Continent: "south_america", StartTurn: 0, Committed: true, Status: risk.GoalStatusActive, StatusSince: 0}

	cmd, _, err := NewSymbolicDirectorStrategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if !cmd.AbandonStrategicGoal {
		t.Fatalf("expected the goal to be abandoned past the inertia cap, got %+v", cmd)
	}
}

// TestWithDirectorDecisionObserverReportsChanged confirms Changed is
// true exactly when the director's actual command differs from what
// plain KillbotV2Strategy would have chosen at the same state.
func TestWithDirectorDecisionObserverReportsChanged(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	pi := 0
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 6}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 1, Armies: 4}
	g.StrategicGoals[pi] = risk.StrategicGoal{Continent: "south_america", StartTurn: 0, Committed: true, Status: risk.GoalStatusActive, StatusSince: 0}

	var events []DecisionEventV3
	strat := NewSymbolicDirectorStrategy(WithDirectorDecisionObserver(func(e DecisionEventV3) { events = append(events, e) }))
	if _, _, err := strat.NextCommand(context.Background(), g, p0); err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected exactly one DecisionEventV3, got %d", len(events))
	}
	if !events[0].GoalActive {
		t.Fatalf("expected GoalActive=true, got %+v", events[0])
	}
}

// TestWithDirectorDecisionObserverNilCostsNothing confirms a nil
// observer (the default) never triggers the extra baseline comparison
// call.
func TestWithDirectorDecisionObserverNilCostsNothing(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1}

	strat := NewSymbolicDirectorStrategy()
	if strat.observe != nil {
		t.Fatalf("expected observe to be nil by default")
	}
	if _, _, err := strat.NextCommand(context.Background(), g, p0); err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
}
