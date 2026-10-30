package main

import (
	"context"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
	"github.com/nmiano1111/global-conquest/backend/internal/risk"
	"github.com/nmiano1111/global-conquest/backend/internal/simulation"
)

// gameSummaryV2 is one game's worth of full-lifecycle funnel data for
// GoalKillbotV2Strategy (StrategyGoalKillbotV2) -- a per-game aggregate
// row, same simplification cmd/goalinstrument's gameSummary already
// made (far simpler to implement/verify than one row per event, and
// every number this tool reports is still directly computable by
// summing these rows across a run).
//
// NOTE on scope: fact-discovery rate ("desirable") is NOT re-measured
// here. symbolic.SelfNearCompletion/completeContinentBestCandidate --
// the fact-finding logic itself -- is shared, unmodified between
// goal-killbot-v1 and goal-killbot-v2 (see the design doc's Architecture
// Found section); the prior discovery-funnel investigation already
// measured it directly (23.0% of discovery-branch reinforce decisions
// found a fact, across 181,248 decisions) and that number applies
// unchanged here. This tool instead focuses on what's NEW in this pass:
// feasibility while committed, the Active->Securing->cleared lifecycle,
// and decision-changed rates.
type gameSummaryV2 struct {
	GameID      string
	Seed        int64
	Completed   bool
	GameWon     bool
	GameTurns   int
	FailureType string

	// TurnsGoalPresent is how many turn boundaries this seat had ANY
	// committed goal (Active or Securing).
	TurnsGoalPresent int
	// TurnsGoalActiveObserved is the subset of TurnsGoalPresent where
	// Status was Active (not yet Securing) -- the denominator for
	// TurnsGoalFeasible, since feasibility is undefined once Securing
	// (no missing territory left to check).
	TurnsGoalActiveObserved int
	// TurnsGoalFeasible is the subset of TurnsGoalActiveObserved where
	// GoalKillbotV2Feasible still held for the committed continent --
	// informative for understanding WHY a goal was abandoned (feasibility
	// lost vs. inertia cap vs. some other cause).
	TurnsGoalFeasible int

	GoalCommittedCount       int
	GoalEnteredSecuringCount int
	// GoalCompletedCount is a goal that reached Securing and then
	// cleared (a real completion). GoalAbandonedCount is a goal cleared
	// directly from Active, never reaching Securing.
	GoalCompletedCount     int
	GoalAbandonedCount     int
	GoalStillActiveAtEnd   bool
	GoalStillSecuringAtEnd bool

	DecisionsTotal      int
	DecisionsGoalActive int
	DecisionsChanged    int
	ChangedByPhase      map[risk.Phase]int
}

// opponentStrategies builds the shared, stateless strategy set every
// goalinstrumentv2 game is played against -- identical field to
// cmd/goalinstrument's own, safe to reuse by reference across concurrent
// games (none of these strategies carry mutable per-game state).
func opponentStrategies() map[string]bot.Strategy {
	return map[string]bot.Strategy{
		bot.StrategyKillbotV2:     bot.NewKillbotV2Strategy(),
		bot.StrategyBetterPixieV1: bot.NewBetterPixieStrategy(),
		bot.StrategyQuoV1:         bot.NewQuoStrategy(),
		bot.StrategyBoscoeV1:      bot.NewBoscoeStrategy(),
		bot.StrategyPixieV1:       bot.NewPixieStrategy(),
		bot.StrategyClusterV1:     bot.NewClusterStrategy(),
	}
}

// runOneGame plays one game with a FRESH GoalKillbotV2Strategy instance
// (fresh, request-scoped DecisionEventV2 buffer) -- same reasoning as
// cmd/goalinstrument's own runOneGame: WithDecisionObserverV2's callback
// has no game-identifying context of its own, so a shared instance
// across concurrent games would interleave events with no way to
// attribute them.
func runOneGame(shared map[string]bot.Strategy, strategies []string, cfg simulation.Config, gameID string) gameSummaryV2 {
	var events []bot.DecisionEventV2
	goalStrat := bot.NewGoalKillbotV2Strategy(bot.WithDecisionObserverV2(func(e bot.DecisionEventV2) {
		events = append(events, e)
	}))

	registry := make(bot.StrategyRegistry, len(shared)+1)
	for id, s := range shared {
		registry[id] = s
	}
	registry[bot.StrategyGoalKillbotV2] = goalStrat

	goalSeat := -1
	for i, id := range strategies {
		if id == bot.StrategyGoalKillbotV2 {
			goalSeat = i
			break
		}
	}

	summary := gameSummaryV2{GameID: gameID, Seed: cfg.Seed, ChangedByPhase: map[risk.Phase]int{}}
	var prevOK bool
	var prevStatus risk.GoalStatus

	cfg.OnTurnBoundary = func(tb simulation.TurnBoundary) {
		if goalSeat < 0 || goalSeat >= len(tb.Game.Players) || tb.Game.Players[goalSeat].Eliminated {
			return
		}
		goal, ok := tb.Game.StrategicGoals[goalSeat]
		if ok {
			summary.TurnsGoalPresent++
			// Feasibility is only meaningful while Active: once Securing,
			// the continent is already fully owned, so
			// completeContinentFactFor (and thus GoalKillbotV2Feasible)
			// always reports ok=false -- not a real feasibility loss, just
			// "there's no missing territory left to check." Counting
			// Securing turns here would misleadingly deflate the rate.
			if goal.Status == risk.GoalStatusActive {
				summary.TurnsGoalActiveObserved++
				if _, _, feasible := bot.GoalKillbotV2Feasible(tb.Game, goalSeat, goal.Continent, 2); feasible {
					summary.TurnsGoalFeasible++
				}
			}
		}

		switch {
		case ok && !prevOK:
			summary.GoalCommittedCount++
		case ok && prevOK && goal.Status == risk.GoalStatusSecuring && prevStatus == risk.GoalStatusActive:
			summary.GoalEnteredSecuringCount++
		case !ok && prevOK && prevStatus == risk.GoalStatusSecuring:
			summary.GoalCompletedCount++
		case !ok && prevOK && prevStatus == risk.GoalStatusActive:
			summary.GoalAbandonedCount++
		}
		prevOK, prevStatus = ok, goal.Status
	}

	sim := simulation.NewSimulator(registry)
	result, _, _ := sim.RunOne(context.Background(), cfg, nil)

	summary.Completed = result.Completed
	summary.GameTurns = result.Turns
	if result.Failure != nil {
		summary.FailureType = string(result.Failure.Type)
	}
	if result.Completed && goalSeat >= 0 {
		summary.GameWon = result.WinnerSeat == goalSeat
	}
	summary.GoalStillActiveAtEnd = prevOK && prevStatus == risk.GoalStatusActive
	summary.GoalStillSecuringAtEnd = prevOK && prevStatus == risk.GoalStatusSecuring

	for _, e := range events {
		summary.DecisionsTotal++
		if e.GoalActive {
			summary.DecisionsGoalActive++
			if e.Changed {
				summary.DecisionsChanged++
				summary.ChangedByPhase[e.Phase]++
			}
		}
	}
	return summary
}
