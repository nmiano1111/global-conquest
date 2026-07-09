package main

import (
	"fmt"
	"io"
	"strings"

	"backend/internal/reporting"
)

// writeMarkdownReport renders the full roll streak report to w. top limits
// how many individual streaks are listed per section (0 = all); the summary
// table always lists every attacker.
func writeMarkdownReport(w io.Writer, r reporting.RollStreakReport, top int) error {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# Roll Streak Report — %s\n\n", r.GameName)

	if r.PartialHistory {
		sb.WriteString("Note: this game has partial event history. Streaks only reflect captured rolls after event logging began.\n\n")
	}

	sb.WriteString("## Definitions\n\n")
	sb.WriteString("- Attacking loss streak: 2+ consecutive rolls where the attacker loses more armies than the defender.\n")
	sb.WriteString("- Attacking win streak: 2+ consecutive rolls where the defender loses more armies than the attacker.\n")
	sb.WriteString("- Attack drought: 3+ consecutive rolls where the attacker does not win the roll.\n")
	sb.WriteString("- Roll trace format: attacker losses - defender losses.\n\n")

	sb.WriteString("## Summary by Attacker\n\n")
	writeSummaryTable(&sb, r.SummaryByAttacker)

	sb.WriteString("\n## Individual Attacking Loss Streaks\n\n")
	writeStreakList(&sb, "L", r.AttackingLossStreaks, top, "attacking losses")

	sb.WriteString("\n## Individual Attacking Win Streaks\n\n")
	writeStreakList(&sb, "W", r.AttackingWinStreaks, top, "attacking wins")

	sb.WriteString("\n## Individual Attack Droughts\n\n")
	writeStreakList(&sb, "D", r.AttackDroughts, top, "roll drought")

	if len(r.Warnings) > 0 {
		sb.WriteString("\n## Diagnostics\n\n")
		for _, warning := range r.Warnings {
			fmt.Fprintf(&sb, "- %s\n", warning)
		}
	}

	_, err := io.WriteString(w, sb.String())
	return err
}

func writeSummaryTable(sb *strings.Builder, summaries []reporting.PlayerStreakSummary) {
	if len(summaries) == 0 {
		sb.WriteString("_No attack rolls captured._\n")
		return
	}
	sb.WriteString("| Player | Rolls | W/L/S | Loss Streaks | Worst Loss | Win Streaks | Best Win | Droughts | Worst Drought | Loss Streaks / 20 | Win Streaks / 20 | Droughts / 20 |\n")
	sb.WriteString("|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, p := range summaries {
		fmt.Fprintf(sb, "| %s | %d | %d/%d/%d | %d | %d | %d | %d | %d | %d | %.2f | %.2f | %.2f |\n",
			p.PlayerName, p.AttackRollsCaptured,
			p.AttackerWinCount, p.AttackerLossCount, p.SplitCount,
			p.LossStreakCount2Plus, p.LongestLossStreak,
			p.WinStreakCount2Plus, p.LongestWinStreak,
			p.AttackDroughtCount3Plus, p.LongestAttackDrought,
			p.LossStreaksPer20Attacks, p.WinStreaksPer20Attacks, p.DroughtsPer20Attacks,
		)
	}
}

func writeStreakList(sb *strings.Builder, prefix string, streaks []reporting.Streak, top int, unitLabel string) {
	if len(streaks) == 0 {
		sb.WriteString("_None._\n")
		return
	}
	n := len(streaks)
	if top > 0 && top < n {
		n = top
	}
	for i, s := range streaks[:n] {
		fmt.Fprintf(sb, "%s%d. %s — %d %s\n", prefix, i+1, s.AttackerName, s.Length, unitLabel)
		fmt.Fprintf(sb, "    Events %d–%d · lost %d · killed %d · net %d\n",
			s.StartSeq, s.EndSeq, s.AttackerArmiesLost, s.DefenderArmiesLost, s.NetArmyDeltaForAttacker)
		fmt.Fprintf(sb, "    Against: %s\n", strings.Join(s.DefendersInvolved, ", "))
		fmt.Fprintf(sb, "    Territories: %s\n", territoryPairs(s))
		fmt.Fprintf(sb, "    Captures: %d\n", s.CapturesDuringStreak)
		fmt.Fprintf(sb, "    Rolls: %s\n\n", s.RollTrace)
	}
	if top > 0 && len(streaks) > n {
		fmt.Fprintf(sb, "_(%d more not shown — use --top 0 or JSON output for the full list)_\n\n", len(streaks)-n)
	}
}

// territoryPairs renders each roll's source → target territory as a
// deduplicated, order-preserved "source → target" list, e.g.
// "Ukraine → Afghanistan, Middle East → India".
func territoryPairs(s reporting.Streak) string {
	seen := make(map[string]struct{})
	var pairs []string
	for _, roll := range s.Rolls {
		pair := fmt.Sprintf("%s → %s", roll.AttackerTerritoryID, roll.DefenderTerritoryID)
		if _, ok := seen[pair]; ok {
			continue
		}
		seen[pair] = struct{}{}
		pairs = append(pairs, pair)
	}
	return strings.Join(pairs, ", ")
}
