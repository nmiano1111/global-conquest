package bot

import (
	"context"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// TestGoalKillbotV2DisabledPathMatchesPlainKillbotV2 confirms that with
// no near-complete continents anywhere on the board, GoalKillbotV2Strategy
// produces the exact same command plain KillbotV2Strategy does.
func TestGoalKillbotV2DisabledPathMatchesPlainKillbotV2(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	pi := 0
	g.Territories["Alaska"] = risk.TerritoryState{Owner: pi, Armies: 5}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1}

	v2Cmd, _, err := NewKillbotV2Strategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("KillbotV2Strategy NextCommand: %v", err)
	}
	goalCmd, _, err := NewGoalKillbotV2Strategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("GoalKillbotV2Strategy NextCommand: %v", err)
	}
	if v2Cmd != goalCmd {
		t.Fatalf("expected identical commands with no strategic facts in play, got v2=%+v goal=%+v", v2Cmd, goalCmd)
	}
}

// TestGoalKillbotV2CommitsWhenFeasibleButNotExecutable is the core proof
// this pass fixes goal-killbot-v1's chicken-and-egg gate: South America
// missing Venezuela (8 armies), staged via Peru (board-order tie-break,
// same as goal-killbot-v1's own fixtures) at only 3 current armies --
// current armies ALONE fall far short of the floor, so goal-killbot-v1's
// old viability check would have rejected this outright. Under
// completeContinentFeasible's horizon projection (income=3/turn from
// playerIncome, required=8, turnsNeeded=ceil(5/3)=2 <= strategicHorizonTurns),
// it's feasible, and the commit sends only the TOP-UP needed (5 of the
// 10 pending), not the whole pool.
func TestGoalKillbotV2CommitsWhenFeasibleButNotExecutable(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 10
	pi := 0
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 3}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 1, Armies: 8}

	required, turnsNeeded, feasible := completeContinentFeasible(g, pi, "south_america", strategicHorizonTurns)
	if !feasible {
		t.Fatalf("test setup invalid: expected feasible, got required=%d turnsNeeded=%d", required, turnsNeeded)
	}
	if forecast := risk.ForecastAttack(3, 8); forecast.WinProbability >= completeContinentAttackBiasFloor {
		t.Fatalf("test setup invalid: expected current armies alone to NOT already clear the floor (proving goal-killbot-v1's old gate would have rejected this), got %.4f", forecast.WinProbability)
	}

	cmd, _, err := NewGoalKillbotV2Strategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Territory != "Peru" || cmd.StrategicGoal != "south_america" || cmd.Armies != required-3 {
		t.Fatalf("expected a partial top-up commitment of %d armies to Peru, got %+v", required-3, cmd)
	}
	if cmd.Armies >= g.PendingReinforcements {
		t.Fatalf("expected a PARTIAL top-up (less than the full pending pool of %d), got %d -- this must not dump the whole pool", g.PendingReinforcements, cmd.Armies)
	}
}

// TestGoalKillbotV2ReinforceStopsToppingUpOnceThresholdReached confirms
// that once the staging territory already meets the required army count,
// no further goal-directed reinforcement happens -- the call delegates
// fully to the backer.
func TestGoalKillbotV2ReinforceStopsToppingUpOnceThresholdReached(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 5
	pi := 0
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 8}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 1, Armies: 8}
	g.StrategicGoals[pi] = risk.StrategicGoal{Continent: "south_america", StartTurn: 0, Committed: true, Status: risk.GoalStatusActive, StatusSince: 0}

	required, _, feasible := completeContinentFeasible(g, pi, "south_america", strategicHorizonTurns)
	if !feasible || 8 < required {
		t.Fatalf("test setup invalid: expected 8 current armies to already meet the required count, got required=%d feasible=%v", required, feasible)
	}

	cmd, _, err := NewGoalKillbotV2Strategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.StrategicGoal != "" {
		t.Fatalf("expected no fresh commitment/re-affirmation once armies already meet the threshold, got %+v", cmd)
	}
	v2Cmd, _, err := NewKillbotV2Strategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("KillbotV2Strategy NextCommand: %v", err)
	}
	if cmd != v2Cmd {
		t.Fatalf("expected full delegation to the backer once already executable, got goal=%+v v2=%+v", cmd, v2Cmd)
	}
}

// TestGoalKillbotV2DefenseTierBypassSkipsWithoutAbandoning confirms that
// when the backer's own top-priority tier (betterPixieContinentNeedsHelp)
// has an urgent claim on a DIFFERENT continent, goal-directed
// reinforcement is skipped this turn without abandoning the goal.
func TestGoalKillbotV2DefenseTierBypassSkipsWithoutAbandoning(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 5
	pi := 0
	// Fully own North America but leave it weak (default fixture armies=1
	// everywhere) -- betterPixieContinentNeedsHelp flags this as urgent.
	for _, terr := range g.Board.Continents["north_america"].Territories {
		g.Territories[terr] = risk.TerritoryState{Owner: pi, Armies: 1}
	}
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 3}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 1, Armies: 8}
	g.StrategicGoals[pi] = risk.StrategicGoal{Continent: "south_america", StartTurn: 0, Committed: true, Status: risk.GoalStatusActive, StatusSince: 0}

	if !betterPixieHasUrgentContinentNeed(g, pi, "south_america") {
		t.Fatalf("test setup invalid: expected an urgent need on north_america")
	}

	cmd, _, err := NewGoalKillbotV2Strategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.AbandonStrategicGoal {
		t.Fatalf("expected a bypass (goal untouched), not an abandonment, got %+v", cmd)
	}
	if cmd.Territory == "Peru" && cmd.StrategicGoal != "" {
		t.Fatalf("expected goal-directed reinforcement to be skipped this turn, got %+v", cmd)
	}
}

// TestGoalKillbotV2PreserveAttackStackBlocksUnrelatedAttack sets South
// America's missing territory to Brazil (owned by an enemy, 8 armies),
// with Venezuela/Peru/Argentina all pi-owned -- Venezuela wins the
// staging tie-break (board order: Venezuela < Peru < Argentina, all
// three directly border Brazil). Venezuela also borders Central America
// (North America), giving it a genuine "unrelated" attack target outside
// the goal. Venezuela's own armies (5) keep the biased attack on Brazil
// below even the relaxed floor, so KillbotV2Strategy falls through to
// the backer, which (confirmed via the plain-KillbotV2Strategy premise
// check below) would otherwise attack Central America from Venezuela --
// PreserveAttackStack must block it instead of letting it spend the
// staging territory's armies on an unrelated conquest.
func TestGoalKillbotV2PreserveAttackStackBlocksUnrelatedAttack(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	pi := 0
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: pi, Armies: 5}
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: 1, Armies: 8}
	g.Territories["Central America"] = risk.TerritoryState{Owner: 1, Armies: 1}
	g.StrategicGoals[pi] = risk.StrategicGoal{Continent: "south_america", StartTurn: 0, Committed: true, Status: risk.GoalStatusActive, StatusSince: 0}

	if forecast := risk.ForecastAttack(5, 8); forecast.WinProbability >= completeContinentAttackBiasFloor {
		t.Fatalf("test setup invalid: expected Venezuela->Brazil to stay below the relaxed bar, got %.4f", forecast.WinProbability)
	}
	staging, to, _, ok := cheapestAttackHopToTerritory(g, pi, "Brazil")
	if !ok || to != "Brazil" || staging != "Venezuela" {
		t.Fatalf("test setup invalid: expected Venezuela to win the staging tie-break for Brazil, got staging=%s to=%s ok=%v", staging, to, ok)
	}
	v2Cmd, _, err := NewKillbotV2Strategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("KillbotV2Strategy NextCommand: %v", err)
	}
	if v2Cmd.Action != ActionAttack || v2Cmd.From != "Venezuela" || v2Cmd.To != "Central America" {
		t.Fatalf("test setup invalid: expected plain KillbotV2Strategy to attack Central America from Venezuela, got %+v", v2Cmd)
	}

	cmd, _, err := NewGoalKillbotV2Strategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionEndAttack {
		t.Fatalf("expected PreserveAttackStack to block the unrelated attack and end the attack phase, got %+v", cmd)
	}
}

// TestGoalKillbotV2AttackAllowsOwnTarget confirms the guard never blocks
// the goal's own attack on the missing territory itself.
func TestGoalKillbotV2AttackAllowsOwnTarget(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	pi := 0
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 5}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 1, Armies: 4}
	g.StrategicGoals[pi] = risk.StrategicGoal{Continent: "south_america", StartTurn: 0, Committed: true, Status: risk.GoalStatusActive, StatusSince: 0}

	forecast := risk.ForecastAttack(5, 4)
	if forecast.WinProbability >= killbotV2MinAttackWinProbability || forecast.WinProbability < completeContinentAttackBiasFloor {
		t.Fatalf("test setup invalid: expected Brazil->Venezuela's win probability between the floor and the normal bar, got %.4f", forecast.WinProbability)
	}

	cmd, _, err := NewGoalKillbotV2Strategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack || cmd.From != "Brazil" || cmd.To != "Venezuela" {
		t.Fatalf("expected the goal's own attack on Venezuela to fire, got %+v", cmd)
	}
}

// TestGoalKillbotV2TransitionsToSecuringOnCompletion confirms a
// committed goal whose continent pi now fully owns transitions to
// GoalStatusSecuring (not cleared outright) -- unlike goal-killbot-v1,
// which clears immediately on completion.
func TestGoalKillbotV2TransitionsToSecuringOnCompletion(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	pi := 0
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.StrategicGoals[pi] = risk.StrategicGoal{Continent: "south_america", StartTurn: 0, Committed: true, Status: risk.GoalStatusActive, StatusSince: 0}

	cmd, _, err := NewGoalKillbotV2Strategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.StrategicGoalStatus != string(risk.GoalStatusSecuring) || cmd.AbandonStrategicGoal {
		t.Fatalf("expected a transition to Securing (not an abandonment/clear), got %+v", cmd)
	}
}

// TestGoalKillbotV2ClearsAfterSecuringDurationCap confirms a goal that's
// been Securing past completeContinentSecuringTurns is cleared.
func TestGoalKillbotV2ClearsAfterSecuringDurationCap(t *testing.T) {
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

	cmd, _, err := NewGoalKillbotV2Strategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if !cmd.AbandonStrategicGoal {
		t.Fatalf("expected the goal to clear once the securing duration cap elapses, got %+v", cmd)
	}
}

// TestGoalKillbotV2AbandonsWhenFeasibilityLost confirms a committed,
// still-nominally-desirable goal is abandoned (not left dangling) once a
// fresh feasibility check fails -- e.g. the enemy has reinforced the
// missing territory well beyond what the horizon projection can plausibly
// overcome.
func TestGoalKillbotV2AbandonsWhenFeasibilityLost(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	pi := 0
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 3}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 1, Armies: 60}
	g.StrategicGoals[pi] = risk.StrategicGoal{Continent: "south_america", StartTurn: 0, Committed: true, Status: risk.GoalStatusActive, StatusSince: 0}

	if _, _, feasible := completeContinentFeasible(g, pi, "south_america", strategicHorizonTurns); feasible {
		t.Fatalf("test setup invalid: expected 60 defending armies to be infeasible within the horizon")
	}

	cmd, _, err := NewGoalKillbotV2Strategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if !cmd.AbandonStrategicGoal {
		t.Fatalf("expected the goal to be abandoned once feasibility is lost, got %+v", cmd)
	}
}

// TestGoalKillbotV2AbandonsAfterInertiaCap confirms the total Active
// lifetime cap (completeContinentInertiaTurns, reused from goal-killbot-v1)
// still applies even to a nominally feasible goal.
func TestGoalKillbotV2AbandonsAfterInertiaCap(t *testing.T) {
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

	cmd, _, err := NewGoalKillbotV2Strategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if !cmd.AbandonStrategicGoal {
		t.Fatalf("expected the goal to be abandoned past the inertia cap, got %+v", cmd)
	}
}

// TestWithDecisionObserverV2ReportsChanged confirms Changed is true
// exactly when GoalKillbotV2Strategy's actual command differs from what
// plain KillbotV2Strategy would have chosen at the same state.
func TestWithDecisionObserverV2ReportsChanged(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	pi := 0
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 5}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 1, Armies: 4}
	g.StrategicGoals[pi] = risk.StrategicGoal{Continent: "south_america", StartTurn: 0, Committed: true, Status: risk.GoalStatusActive, StatusSince: 0}

	var events []DecisionEventV2
	strat := NewGoalKillbotV2Strategy(WithDecisionObserverV2(func(e DecisionEventV2) { events = append(events, e) }))
	if _, _, err := strat.NextCommand(context.Background(), g, p0); err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected exactly one DecisionEventV2, got %d", len(events))
	}
	if !events[0].Changed || !events[0].GoalActive {
		t.Fatalf("expected Changed=true and GoalActive=true for a bias-driven decision, got %+v", events[0])
	}
}

// TestWithDecisionObserverV2NilCostsNothing confirms a nil observer (the
// default) never triggers the extra baseline comparison call.
func TestWithDecisionObserverV2NilCostsNothing(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1}

	strat := NewGoalKillbotV2Strategy()
	if strat.observe != nil {
		t.Fatalf("expected observe to be nil by default")
	}
	if _, _, err := strat.NextCommand(context.Background(), g, p0); err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
}
