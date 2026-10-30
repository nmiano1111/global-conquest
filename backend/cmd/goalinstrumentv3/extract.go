package main

import (
	"context"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
	"github.com/nmiano1111/global-conquest/backend/internal/risk"
	"github.com/nmiano1111/global-conquest/backend/internal/simulation"
)

// gameSummaryV3 is one game's worth of full-lifecycle funnel data for
// SymbolicDirectorStrategy (StrategyDirectorV1) -- a per-game aggregate
// row, same simplification cmd/goalinstrumentv2's gameSummaryV2 already
// made. Same scope note as v2: fact-discovery rate ("desirable") is not
// re-measured here -- the fact-finding logic itself is shared, unmodified
// across all three goal-directed passes.
type gameSummaryV3 struct {
	GameID      string
	Seed        int64
	Completed   bool
	GameWon     bool
	GameTurns   int
	FailureType string

	TurnsGoalPresent        int
	TurnsGoalActiveObserved int
	TurnsGoalFeasible       int

	GoalCommittedCount       int
	GoalEnteredSecuringCount int
	GoalCompletedCount       int
	GoalAbandonedCount       int
	GoalStillActiveAtEnd     bool
	GoalStillSecuringAtEnd   bool

	// AbandonCauses counts each abandonment (cleared while Status was
	// Active, never reaching Securing) by DirectorAbandonCause's
	// categorical reason -- the central measurement this tool adds over
	// v2: does the director/executor split reduce goal-killbot-v2's
	// dominant fact_gone cause (design doc §19.10)?
	AbandonCauses map[string]int

	DecisionsTotal      int
	DecisionsGoalActive int
	DecisionsChanged    int
	ChangedByPhase      map[risk.Phase]int
}

// opponentStrategies builds the shared, stateless strategy set every
// goalinstrumentv3 game is played against -- identical field to
// cmd/goalinstrumentv2's own.
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

// runOneGame plays one game with a FRESH SymbolicDirectorStrategy
// instance (fresh, request-scoped DecisionEventV3 buffer) -- same
// reasoning as cmd/goalinstrumentv2's own runOneGame.
func runOneGame(shared map[string]bot.Strategy, strategies []string, cfg simulation.Config, gameID string) gameSummaryV3 {
	var events []bot.DecisionEventV3
	directorStrat := bot.NewSymbolicDirectorStrategy(bot.WithDirectorDecisionObserver(func(e bot.DecisionEventV3) {
		events = append(events, e)
	}))

	registry := make(bot.StrategyRegistry, len(shared)+1)
	for id, s := range shared {
		registry[id] = s
	}
	registry[bot.StrategyDirectorV1] = directorStrat

	directorSeat := -1
	for i, id := range strategies {
		if id == bot.StrategyDirectorV1 {
			directorSeat = i
			break
		}
	}

	summary := gameSummaryV3{GameID: gameID, Seed: cfg.Seed, ChangedByPhase: map[risk.Phase]int{}, AbandonCauses: map[string]int{}}
	var prevOK bool
	var prevStatus risk.GoalStatus
	var prevContinent risk.Continent

	cfg.OnTurnBoundary = func(tb simulation.TurnBoundary) {
		if directorSeat < 0 || directorSeat >= len(tb.Game.Players) || tb.Game.Players[directorSeat].Eliminated {
			return
		}
		goal, ok := tb.Game.StrategicGoals[directorSeat]
		if ok {
			summary.TurnsGoalPresent++
			if goal.Status == risk.GoalStatusActive {
				summary.TurnsGoalActiveObserved++
				if _, _, feasible := bot.GoalKillbotV2Feasible(tb.Game, directorSeat, goal.Continent, 2); feasible {
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
			summary.AbandonCauses[bot.DirectorAbandonCause(tb.Game, directorSeat, prevContinent)]++
		}
		prevOK, prevStatus, prevContinent = ok, goal.Status, goal.Continent
	}

	sim := simulation.NewSimulator(registry)
	result, _, _ := sim.RunOne(context.Background(), cfg, nil)

	summary.Completed = result.Completed
	summary.GameTurns = result.Turns
	if result.Failure != nil {
		summary.FailureType = string(result.Failure.Type)
	}
	if result.Completed && directorSeat >= 0 {
		summary.GameWon = result.WinnerSeat == directorSeat
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
