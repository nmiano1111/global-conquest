package bot

import (
	"context"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// TestGoalKillbotDisabledPathMatchesPlainKillbotV2 confirms that with no
// near-complete continents anywhere on the board, GoalKillbotStrategy
// produces the exact same command plain KillbotV2Strategy does.
func TestGoalKillbotDisabledPathMatchesPlainKillbotV2(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	pi := 0
	g.Territories["Alaska"] = risk.TerritoryState{Owner: pi, Armies: 5}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1}

	v2Cmd, _, err := NewKillbotV2Strategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("KillbotV2Strategy NextCommand: %v", err)
	}
	goalCmd, _, err := NewGoalKillbotStrategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("GoalKillbotStrategy NextCommand: %v", err)
	}
	if v2Cmd != goalCmd {
		t.Fatalf("expected identical commands with no strategic facts in play, got v2=%+v goal=%+v", v2Cmd, goalCmd)
	}
}

// TestGoalKillbotReinforceCommitsWhenGateClears sets South America to pi
// owning Peru/Brazil/Argentina (3 of 4), player 1 holding the missing
// piece Venezuela (4 armies). Peru and Brazil both directly border
// Venezuela; cheapestAttackHopToTerritory's board-order tie-break picks
// Peru (7 armies + 5 pending = 12) as the route source -- comfortably
// clears the reinforce-phase gate (0.35). Confirms first commitment:
// Territory=Peru, StrategicGoal set.
func TestGoalKillbotReinforceCommitsWhenGateClears(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 5
	pi := 0
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 7}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 1, Armies: 4}

	cmd, _, err := NewGoalKillbotStrategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Territory != "Peru" || cmd.StrategicGoal != "south_america" {
		t.Fatalf("expected a fresh commitment via Peru with StrategicGoal set, got %+v", cmd)
	}
}

// TestGoalKillbotReinforceDoesNotCommitBelowGate mirrors the above with
// Venezuela strong enough (20 armies) that even the routed source's
// pending-reinforced stack can't clear the 0.35 gate -- expects no
// commitment.
func TestGoalKillbotReinforceDoesNotCommitBelowGate(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 5
	pi := 0
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 7}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 1, Armies: 20}

	forecast := risk.ForecastAttack(7+5, 20)
	if forecast.WinProbability >= completeContinentReinforceMinWinProbability {
		t.Fatalf("test setup invalid: expected the reinforce-phase gate to fail, got win probability %.4f", forecast.WinProbability)
	}

	cmd, _, err := NewGoalKillbotStrategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.StrategicGoal != "" {
		t.Fatalf("expected no commitment when the gate doesn't clear, got %+v", cmd)
	}
}

// TestGoalKillbotAttackFiresRelaxedBiasForCommittedGoal pre-commits pi to
// south_america (bypassing discovery) and sets up a Brazil(5)->Venezuela(4)
// matchup, ForecastAttack ~= 0.4765 -- below the normal 0.5 bar (plain
// killbot-v2 must decline it, per attackForCard's pure-ratio logic
// preferring a much weaker default-owned neighbor instead) but above the
// relaxed 0.35 bar (goal-killbot must take it).
func TestGoalKillbotAttackFiresRelaxedBiasForCommittedGoal(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	pi := 0
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 5}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 1, Armies: 4}
	g.StrategicGoals[pi] = risk.StrategicGoal{Continent: "south_america", StartTurn: 0, Committed: true}

	forecast := risk.ForecastAttack(5, 4)
	if forecast.WinProbability >= killbotV2MinAttackWinProbability || forecast.WinProbability < completeContinentAttackBiasFloor {
		t.Fatalf("test setup invalid: expected Brazil->Venezuela's win probability between the floor and the normal bar, got %.4f", forecast.WinProbability)
	}

	v2Cmd, _, err := NewKillbotV2Strategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("KillbotV2Strategy NextCommand: %v", err)
	}
	if v2Cmd.Action == ActionAttack && v2Cmd.To == "Venezuela" {
		t.Fatalf("test setup invalid: expected plain KillbotV2Strategy to decline the sub-bar attack on Venezuela, got %+v", v2Cmd)
	}

	goalCmd, _, err := NewGoalKillbotStrategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("GoalKillbotStrategy NextCommand: %v", err)
	}
	if goalCmd.Action != ActionAttack || goalCmd.From != "Brazil" || goalCmd.To != "Venezuela" {
		t.Fatalf("expected GoalKillbotStrategy to attack Venezuela from Brazil, got %+v", goalCmd)
	}
}

// TestGoalKillbotOccupyForcesMaxArmiesOnMissingTerritoryOnly confirms the
// occupy preemption fires only when the just-conquered territory is the
// exact committed goal's missing territory.
func TestGoalKillbotOccupyForcesMaxArmiesOnMissingTerritoryOnly(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseOccupy
	pi := 0
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 8}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: pi, Armies: 0}
	g.StrategicGoals[pi] = risk.StrategicGoal{Continent: "south_america", StartTurn: 0, Committed: true}
	g.Occupy = &risk.OccupyState{From: "Peru", To: "Venezuela", MinMove: 2, MaxMove: 7}

	cmd, _, err := NewGoalKillbotStrategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionOccupy || cmd.Armies != 7 {
		t.Fatalf("expected max-armies (7) occupation of the goal's missing territory, got %+v", cmd)
	}
}

// TestGoalKillbotOccupyDelegatesForUnrelatedConquest confirms occupying a
// territory that ISN'T the committed goal's missing territory falls
// through to KillbotV2Strategy's own occupy handling unchanged, even
// with a goal actively committed.
func TestGoalKillbotOccupyDelegatesForUnrelatedConquest(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseOccupy
	pi := 0
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 8}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 1, Armies: 4}
	g.Territories["Alaska"] = risk.TerritoryState{Owner: pi, Armies: 6}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: pi, Armies: 0}
	g.StrategicGoals[pi] = risk.StrategicGoal{Continent: "south_america", StartTurn: 0, Committed: true}
	g.Occupy = &risk.OccupyState{From: "Alaska", To: "Kamchatka", MinMove: 2, MaxMove: 5}

	goalStrat := NewGoalKillbotStrategy()
	goalCmd, _, err := goalStrat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("GoalKillbotStrategy NextCommand: %v", err)
	}
	v2Cmd, _, err := NewKillbotV2Strategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("KillbotV2Strategy NextCommand: %v", err)
	}
	if goalCmd != v2Cmd {
		t.Fatalf("expected delegation to KillbotV2Strategy's own occupy handling for an unrelated conquest, got goal=%+v v2=%+v", goalCmd, v2Cmd)
	}
}

// TestGoalKillbotFortifySteersTowardMissingTerritoryBorder confirms the
// fortify preemption moves armies toward a border adjacent to the
// committed goal's missing territory.
func TestGoalKillbotFortifySteersTowardMissingTerritoryBorder(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	pi := 0
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 6}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 4}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 1, Armies: 4}
	g.StrategicGoals[pi] = risk.StrategicGoal{Continent: "south_america", StartTurn: 0, Committed: true}

	cmd, _, err := NewGoalKillbotStrategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionFortify || cmd.To != "Brazil" {
		t.Fatalf("expected fortify to steer armies toward Brazil (adjacent to the missing territory, Venezuela), got %+v", cmd)
	}
}

// TestGoalKillbotAbandonsAfterInertiaCap confirms a committed goal past
// completeContinentInertiaTurns is cleared as an abandonment on the next
// reinforce decision, even though it's still nominally viable.
func TestGoalKillbotAbandonsAfterInertiaCap(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	g.TurnNumber = completeContinentInertiaTurns + 1
	pi := 0
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 7}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 1, Armies: 4}
	g.StrategicGoals[pi] = risk.StrategicGoal{Continent: "south_america", StartTurn: 0, Committed: true}

	cmd, _, err := NewGoalKillbotStrategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if !cmd.AbandonStrategicGoal {
		t.Fatalf("expected the goal to be abandoned past the inertia cap, got %+v", cmd)
	}
}

// TestGoalKillbotClearsGoalOnCompletion confirms a committed goal whose
// continent pi now fully owns is cleared (via the same AbandonStrategicGoal
// field -- completion and abandonment are indistinguishable at the
// engine level by design, see ClearStrategicGoal's doc comment).
func TestGoalKillbotClearsGoalOnCompletion(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	pi := 0
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.StrategicGoals[pi] = risk.StrategicGoal{Continent: "south_america", StartTurn: 0, Committed: true}

	cmd, _, err := NewGoalKillbotStrategy().NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if !cmd.AbandonStrategicGoal {
		t.Fatalf("expected the completed goal to be cleared, got %+v", cmd)
	}
}

// TestWithDecisionObserverReportsChanged confirms Changed is true exactly
// when GoalKillbotStrategy's actual command differs from what plain
// KillbotV2Strategy would have chosen at the same state.
func TestWithDecisionObserverReportsChanged(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	pi := 0
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 5}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 1, Armies: 4}
	g.StrategicGoals[pi] = risk.StrategicGoal{Continent: "south_america", StartTurn: 0, Committed: true}

	var events []DecisionEvent
	strat := NewGoalKillbotStrategy(WithDecisionObserver(func(e DecisionEvent) { events = append(events, e) }))
	if _, _, err := strat.NextCommand(context.Background(), g, p0); err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected exactly one DecisionEvent, got %d", len(events))
	}
	if !events[0].Changed || !events[0].GoalActive {
		t.Fatalf("expected Changed=true and GoalActive=true for a bias-driven decision, got %+v", events[0])
	}
}

// TestWithDecisionObserverNilCostsNothing confirms a nil observer (the
// default) never triggers the extra baseline comparison call.
func TestWithDecisionObserverNilCostsNothing(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1}

	strat := NewGoalKillbotStrategy()
	if strat.observe != nil {
		t.Fatalf("expected observe to be nil by default")
	}
	if _, _, err := strat.NextCommand(context.Background(), g, p0); err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
}
