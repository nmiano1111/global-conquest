package main

import (
	"fmt"
	"sort"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// printReport aggregates every game's gameSummaryV2 and prints the full
// desirable/feasible/committed/securing/completed/abandoned lifecycle
// funnel plus decision-changed rates and game-length/timeout data --
// see the design doc's Instrumentation Funnel section for how this maps
// onto the brief's 10 diagnostic questions.
func printReport(all []gameSummaryV2, elapsed time.Duration) {
	var completed, gamesWon, gamesWithCommit int
	var totalCommitted, totalEnteredSecuring, totalCompleted, totalAbandoned int
	var stillActiveAtEnd, stillSecuringAtEnd int
	var totalTurnsGoalPresent, totalTurnsGoalActiveObserved, totalTurnsGoalFeasible int
	var decisionsTotal, decisionsGoalActive, decisionsChanged int
	var sumGameTurns int
	changedByPhase := map[risk.Phase]int{}
	failureTypes := map[string]int{}

	for _, s := range all {
		if s.FailureType != "" {
			failureTypes[s.FailureType]++
		}
		if !s.Completed {
			continue
		}
		completed++
		sumGameTurns += s.GameTurns
		if s.GameWon {
			gamesWon++
		}
		if s.GoalCommittedCount > 0 {
			gamesWithCommit++
		}
		totalCommitted += s.GoalCommittedCount
		totalEnteredSecuring += s.GoalEnteredSecuringCount
		totalCompleted += s.GoalCompletedCount
		totalAbandoned += s.GoalAbandonedCount
		if s.GoalStillActiveAtEnd {
			stillActiveAtEnd++
		}
		if s.GoalStillSecuringAtEnd {
			stillSecuringAtEnd++
		}
		totalTurnsGoalPresent += s.TurnsGoalPresent
		totalTurnsGoalActiveObserved += s.TurnsGoalActiveObserved
		totalTurnsGoalFeasible += s.TurnsGoalFeasible
		decisionsTotal += s.DecisionsTotal
		decisionsGoalActive += s.DecisionsGoalActive
		decisionsChanged += s.DecisionsChanged
		for phase, n := range s.ChangedByPhase {
			changedByPhase[phase] += n
		}
	}

	fmt.Printf("goalinstrumentv2: %d games (%d completed, %d failed) in %s\n",
		len(all), completed, len(all)-completed, elapsed.Round(10*time.Millisecond))
	if len(failureTypes) > 0 {
		fmt.Print("failures:")
		types := make([]string, 0, len(failureTypes))
		for ft := range failureTypes {
			types = append(types, ft)
		}
		sort.Strings(types)
		for _, ft := range types {
			fmt.Printf(" %s: %d", ft, failureTypes[ft])
		}
		fmt.Println()
	}
	if completed == 0 {
		fmt.Println("no completed games -- nothing to report")
		return
	}

	fmt.Printf("\ngoal-killbot-v2 win rate: %d/%d (%.1f%%)\n", gamesWon, completed, 100*float64(gamesWon)/float64(completed))
	fmt.Printf("avg game length: %.1f turns\n", float64(sumGameTurns)/float64(completed))

	fmt.Printf("\ngoals committed: %d\n", totalCommitted)
	fmt.Printf("goals entering securing (continent completed): %d\n", totalEnteredSecuring)
	fmt.Printf("goals completed (reached securing, then cleared): %d\n", totalCompleted)
	fmt.Printf("goals abandoned (cleared from active, never secured): %d\n", totalAbandoned)
	fmt.Printf("still active at game end: %d, still securing at game end: %d\n", stillActiveAtEnd, stillSecuringAtEnd)

	fmt.Printf("\nturns with a goal present (active or securing): %d\n", totalTurnsGoalPresent)
	if totalTurnsGoalActiveObserved > 0 {
		feasibleRate := 100 * float64(totalTurnsGoalFeasible) / float64(totalTurnsGoalActiveObserved)
		fmt.Printf("turns Active (not yet securing): %d, still feasible: %d (%.1f%%)\n",
			totalTurnsGoalActiveObserved, totalTurnsGoalFeasible, feasibleRate)
	}

	if decisionsGoalActive == 0 {
		fmt.Println("\nno decisions were made with a goal active -- changed-decision rate is undefined (n/a)")
		return
	}
	changedRate := 100 * float64(decisionsChanged) / float64(decisionsGoalActive)
	fmt.Printf("\ndecisions: %d total, %d with a goal active, %d changed (%.1f%% of goal-active decisions)\n",
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

	commitRate := 100 * float64(gamesWithCommit) / float64(completed)
	fmt.Printf("\ngames with >=1 goal committed: %d/%d (%.1f%%)\n", gamesWithCommit, completed, commitRate)
	fmt.Printf("goals committed per completed game (avg, including re-commitments): %.2f\n", float64(totalCommitted)/float64(completed))
}
