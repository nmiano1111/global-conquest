package main

import (
	"context"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
	"github.com/nmiano1111/global-conquest/backend/internal/risk"
	"github.com/nmiano1111/global-conquest/backend/internal/simulation"
)

// gameSummary is one game's worth of funnel data for GoalKillbotStrategy
// (StrategyGoalKillbotV1) -- a per-game AGGREGATE row, not one row per
// individual event as the design plan's first sketch proposed: far
// simpler to implement and verify correctly, and every number the plan
// actually calls "the single most important number" (decisions with
// Changed=true / decisions where a goal was committed) is still directly
// computable by summing these rows across a run.
type gameSummary struct {
	GameID    string
	Seed      int64
	Completed bool
	// GameWon reports whether goal-killbot-v1 won this game -- only
	// meaningful when Completed.
	GameWon bool

	// GoalCommittedCount is how many times a NEW commitment was made
	// (including re-commitments to a different continent after an
	// earlier one ended) -- see runOneGame's OnTurnBoundary callback.
	GoalCommittedCount int
	GoalCompletedCount int
	GoalAbandonedCount int
	// GoalStillActiveAtEnd reports whether a goal was committed and never
	// resolved (completed or abandoned) by the time the game ended --
	// neither a success nor a failure, just an open commitment when the
	// clock ran out.
	GoalStillActiveAtEnd bool

	// DecisionsTotal is every reinforce/attack/occupy/fortify decision
	// GoalKillbotStrategy made that had an observer attached.
	DecisionsTotal int
	// DecisionsGoalActive is the subset of DecisionsTotal where a goal
	// was committed at decision time -- see DecisionEvent.GoalActive's
	// own doc comment for why this is also the best-effort proxy for
	// "the goal mechanism had an opinion to offer here."
	DecisionsGoalActive int
	// DecisionsChanged is the subset of DecisionsGoalActive where the
	// actual command differed from what plain killbot-v2 would have
	// chosen -- the single most important number this tool produces.
	DecisionsChanged int
	// ChangedByPhase breaks DecisionsChanged down by risk.Phase, so a
	// summary can report e.g. "changed 40% of attack decisions but only
	// 5% of fortify decisions."
	ChangedByPhase map[risk.Phase]int
}

// opponentStrategies builds the shared, stateless (no per-game
// observer/closure) strategy set every goalinstrument game is played
// against -- constructed once, safe to reuse by reference across
// concurrent games, since none of these strategies carry any mutable
// per-game state (the same "one shared Strategy instance serves every
// game" pattern this whole codebase's tournament tooling already relies
// on).
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

// runOneGame plays one game with a FRESH GoalKillbotStrategy instance
// (and thus a fresh, request-scoped DecisionEvent buffer) -- unlike a
// normal tournament, where one shared Strategy instance is reused across
// every concurrent game, GoalKillbotStrategy's WithDecisionObserver
// callback has no game-identifying context of its own, so a shared
// instance's observer would interleave events from many concurrent games
// with no way to attribute them. Building a small per-game registry
// (goal-killbot-v1 fresh, every opponent shared/stateless) keeps this
// simple and correct rather than threading a game ID through
// DecisionEvent.
func runOneGame(shared map[string]bot.Strategy, strategies []string, cfg simulation.Config, gameID string) gameSummary {
	var events []bot.DecisionEvent
	goalStrat := bot.NewGoalKillbotStrategy(bot.WithDecisionObserver(func(e bot.DecisionEvent) {
		events = append(events, e)
	}))

	registry := make(bot.StrategyRegistry, len(shared)+1)
	for id, s := range shared {
		registry[id] = s
	}
	registry[bot.StrategyGoalKillbotV1] = goalStrat

	goalSeat := -1
	for i, id := range strategies {
		if id == bot.StrategyGoalKillbotV1 {
			goalSeat = i
			break
		}
	}

	summary := gameSummary{GameID: gameID, Seed: cfg.Seed, ChangedByPhase: map[risk.Phase]int{}}
	var prevGoal risk.StrategicGoal
	var hadGoal bool

	cfg.OnTurnBoundary = func(tb simulation.TurnBoundary) {
		if goalSeat < 0 || goalSeat >= len(tb.Game.Players) || tb.Game.Players[goalSeat].Eliminated {
			return
		}
		goal, ok := tb.Game.StrategicGoals[goalSeat]
		switch {
		case ok && !hadGoal:
			summary.GoalCommittedCount++
		case ok && hadGoal && goal.Continent != prevGoal.Continent:
			summary.GoalCommittedCount++
		case !ok && hadGoal:
			if continentFullyOwned(tb.Game, goalSeat, prevGoal.Continent) {
				summary.GoalCompletedCount++
			} else {
				summary.GoalAbandonedCount++
			}
		}
		hadGoal, prevGoal = ok, goal
	}

	sim := simulation.NewSimulator(registry)
	result, _, _ := sim.RunOne(context.Background(), cfg, nil)

	summary.Completed = result.Completed
	if result.Completed && goalSeat >= 0 {
		summary.GameWon = result.WinnerSeat == goalSeat
	}
	summary.GoalStillActiveAtEnd = hadGoal

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

// continentFullyOwned reports whether pi owns every territory in cont --
// duplicated locally (a one-line check) rather than importing
// internal/bot's unexported ownsContinent, which this package-main
// binary can't reach anyway.
func continentFullyOwned(g *risk.Game, pi int, cont risk.Continent) bool {
	for _, t := range g.Board.Continents[cont].Territories {
		if g.Territories[t].Owner != pi {
			return false
		}
	}
	return true
}
