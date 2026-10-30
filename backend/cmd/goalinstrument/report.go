package main

import (
	"fmt"
	"sort"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// printReport aggregates every game's gameSummary and prints the numbers
// the design doc's Stage 1 -> Stage 2 gate is measured against: goal
// commitment rate across completed games, and the changed-decision rate
// among committed-goal decisions.
func printReport(all []gameSummary, elapsed time.Duration) {
	var completed, gamesWon, gamesWithCommit int
	var totalCommitted, totalCompleted, totalAbandoned, totalStillActive int
	var decisionsTotal, decisionsGoalActive, decisionsChanged int
	changedByPhase := map[risk.Phase]int{}

	for _, s := range all {
		if !s.Completed {
			continue
		}
		completed++
		if s.GameWon {
			gamesWon++
		}
		if s.GoalCommittedCount > 0 {
			gamesWithCommit++
		}
		totalCommitted += s.GoalCommittedCount
		totalCompleted += s.GoalCompletedCount
		totalAbandoned += s.GoalAbandonedCount
		if s.GoalStillActiveAtEnd {
			totalStillActive++
		}
		decisionsTotal += s.DecisionsTotal
		decisionsGoalActive += s.DecisionsGoalActive
		decisionsChanged += s.DecisionsChanged
		for phase, n := range s.ChangedByPhase {
			changedByPhase[phase] += n
		}
	}

	fmt.Printf("goalinstrument: %d games (%d completed, %d failed) in %s\n",
		len(all), completed, len(all)-completed, elapsed.Round(10*time.Millisecond))
	if completed == 0 {
		fmt.Println("no completed games -- nothing to report")
		return
	}

	commitRate := 100 * float64(gamesWithCommit) / float64(completed)
	fmt.Printf("\ngoal-killbot-v1 win rate: %d/%d (%.1f%%)\n", gamesWon, completed, 100*float64(gamesWon)/float64(completed))
	fmt.Printf("games with >=1 goal committed: %d/%d (%.1f%%) -- Stage 2 gate requires >=20%%\n", gamesWithCommit, completed, commitRate)
	fmt.Printf("commitments: %d total, %d completed, %d abandoned, %d still active at game end\n",
		totalCommitted, totalCompleted, totalAbandoned, totalStillActive)

	if decisionsGoalActive == 0 {
		fmt.Println("\nno decisions were made with a goal active -- changed-decision rate is undefined (n/a)")
		return
	}
	changedRate := 100 * float64(decisionsChanged) / float64(decisionsGoalActive)
	fmt.Printf("\ndecisions: %d total, %d with a goal active, %d changed (%.1f%% of goal-active decisions) -- Stage 2 gate requires >=10%%\n",
		decisionsTotal, decisionsGoalActive, decisionsChanged, changedRate)

	fmt.Println("changed decisions by phase:")
	phases := make([]risk.Phase, 0, len(changedByPhase))
	for p := range changedByPhase {
		phases = append(phases, p)
	}
	sort.Slice(phases, func(i, j int) bool { return phases[i] < phases[j] })
	for _, p := range phases {
		fmt.Printf("  %-12s %d\n", p, changedByPhase[p])
	}

	gate := commitRate >= 20 && changedRate >= 10
	fmt.Printf("\nStage 1 -> Stage 2 gate: %v (commit rate %.1f%% >= 20%%, changed rate %.1f%% >= 10%%)\n", gate, commitRate, changedRate)
}
