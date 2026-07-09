package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"backend/internal/reporting"
)

func runRollStreaks(args []string) error {
	fs := flag.NewFlagSet("roll-streaks", flag.ExitOnError)
	gameID := fs.String("game-id", "", "Game UUID (defaults to the most recently active game)")
	playerID := fs.String("player-id", "", "Restrict the report to a single attacker's player UUID")
	minLoss := fs.Int("min-loss-streak-length", reporting.DefaultStreakThresholds().MinLossStreakLength, "Minimum consecutive losses to count as a loss streak")
	minWin := fs.Int("min-win-streak-length", reporting.DefaultStreakThresholds().MinWinStreakLength, "Minimum consecutive wins to count as a win streak")
	minDrought := fs.Int("min-drought-length", reporting.DefaultStreakThresholds().MinDroughtLength, "Minimum consecutive non-wins to count as a drought")
	top := fs.Int("top", 5, "Number of streaks to show per category in Markdown output (0 = all)")
	format := fs.String("format", "markdown", "Output format: markdown|json")
	includePartial := fs.Bool("include-partial-games", false, "Required to proceed when the target game has partial event history")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *format != "markdown" && *format != "json" {
		return fmt.Errorf("invalid --format %q (want markdown or json)", *format)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	svc, closePool, err := newReportingService(ctx)
	if err != nil {
		return err
	}
	defer closePool()

	var resolvedID, resolvedName string
	if *gameID != "" {
		resolvedID, resolvedName = *gameID, *gameID
		// Best-effort: prefer the canonical name if this ID happens to be the latest game.
		if id, name, latestErr := svc.ResolveGame(ctx, ""); latestErr == nil && id == *gameID {
			resolvedName = name
		}
	} else {
		id, name, err := svc.ResolveGame(ctx, "")
		if err != nil {
			if errors.Is(err, reporting.ErrNoActiveGame) {
				return fmt.Errorf("no active game found; pass --game-id explicitly")
			}
			return fmt.Errorf("resolve game: %w", err)
		}
		resolvedID, resolvedName = id, name
	}

	thresholds := reporting.StreakThresholds{
		MinLossStreakLength: *minLoss,
		MinWinStreakLength:  *minWin,
		MinDroughtLength:    *minDrought,
	}

	report, err := svc.RollStreakReport(ctx, resolvedID, resolvedName, thresholds)
	if err != nil {
		if errors.Is(err, reporting.ErrNoEvents) {
			return fmt.Errorf("no combat events found for game %s", resolvedID)
		}
		return fmt.Errorf("build roll streak report: %w", err)
	}

	if report.PartialHistory && !*includePartial {
		return fmt.Errorf(
			"game %s has partial event history (streaks only reflect captured rolls after event logging began); "+
				"pass --include-partial-games to generate the report anyway", resolvedID)
	}

	if *playerID != "" {
		report = filterReportByPlayer(report, *playerID)
	}

	switch *format {
	case "json":
		return writeJSONReport(os.Stdout, report)
	default:
		return writeMarkdownReport(os.Stdout, report, *top)
	}
}

// filterReportByPlayer restricts a report to a single attacker, keeping the
// report shape intact (empty slices rather than nil where nothing matches).
func filterReportByPlayer(r reporting.RollStreakReport, playerID string) reporting.RollStreakReport {
	out := r
	out.SummaryByAttacker = nil
	for _, s := range r.SummaryByAttacker {
		if s.PlayerID == playerID {
			out.SummaryByAttacker = append(out.SummaryByAttacker, s)
		}
	}
	out.AttackingLossStreaks = filterStreaksByAttacker(r.AttackingLossStreaks, playerID)
	out.AttackingWinStreaks = filterStreaksByAttacker(r.AttackingWinStreaks, playerID)
	out.AttackDroughts = filterStreaksByAttacker(r.AttackDroughts, playerID)
	return out
}

func filterStreaksByAttacker(streaks []reporting.Streak, playerID string) []reporting.Streak {
	var out []reporting.Streak
	for _, s := range streaks {
		if s.AttackerID == playerID {
			out = append(out, s)
		}
	}
	return out
}
