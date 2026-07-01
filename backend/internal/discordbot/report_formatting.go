package discordbot

import (
	"fmt"
	"strings"

	"backend/internal/reporting"
)

// maxCodeBlockBody is a conservative limit for content inside a code block,
// leaving room for the header, fence characters, and any footer.
const maxCodeBlockBody = 1800

// formatLastRolls renders recent combat rolls with a markdown header and a
// code-block body so the data stands out in the channel.
func formatLastRolls(rolls []reporting.RecentCombatRoll, gameName string) string {
	if len(rolls) == 0 {
		return "No combat events found for this game yet."
	}

	var body strings.Builder
	for _, r := range rolls {
		atk := joinInts(r.AttackerDice)
		def := joinInts(r.DefenderDice)
		captured := ""
		if r.Captured {
			captured = " [CAPTURED]"
		}
		line := fmt.Sprintf(
			"#%d %s → %s: %s → %s\n  Attack [%s] vs Defense [%s] — Losses: %d / %d%s\n\n",
			r.GameSequence,
			r.AttackerDisplayName, r.DefenderDisplayName,
			r.SourceTerritoryID, r.TargetTerritoryID,
			atk, def,
			r.AttackerLosses, r.DefenderLosses,
			captured,
		)
		if body.Len()+len(line) > maxCodeBlockBody {
			body.WriteString("(output truncated)\n")
			break
		}
		body.WriteString(line)
	}

	return fmt.Sprintf(
		"🎲 **Last %d Combat Roll(s)** · %s\n```\n%s```",
		len(rolls), gameName,
		body.String(),
	)
}

// formatDiceReport renders an aggregate dice-statistics report.
func formatDiceReport(r reporting.DiceReport, gameName string) string {
	var body strings.Builder
	fmt.Fprintf(&body, "Combat rolls : %d\n", r.CombatRolls)
	fmt.Fprintf(&body, "Attacker dice: %d total\n", r.AttackerDice.Total)
	fmt.Fprintf(&body, "Defender dice: %d total\n\n", r.DefenderDice.Total)

	body.WriteString("Attacker faces:\n")
	writeFaceDistribution(&body, r.AttackerDice)
	body.WriteString("\nDefender faces:\n")
	writeFaceDistribution(&body, r.DefenderDice)

	body.WriteString("\n")
	fmt.Fprintf(&body, "Attacker losses: %d\n", r.AttackerLosses)
	fmt.Fprintf(&body, "Defender losses: %d\n", r.DefenderLosses)

	if r.CombatRolls > 0 {
		capRate := float64(r.Captures) / float64(r.CombatRolls) * 100.0
		fmt.Fprintf(&body, "Captures       : %d / %d (%.1f%%)\n", r.Captures, r.CombatRolls, capRate)
	} else {
		body.WriteString("Captures       : 0\n")
	}

	if r.SkippedEvents > 0 {
		fmt.Fprintf(&body, "Skipped events : %d\n", r.SkippedEvents)
	}

	return fmt.Sprintf(
		"🎲 **Dice Report** · %s\n```\n%s```\n⚠️ *Descriptive results — not proof of RNG bias or fairness.*",
		gameName, body.String(),
	)
}

// formatPlayerReport renders a single-player attack statistics report.
func formatPlayerReport(r reporting.PlayerCombatReport, gameName string) string {
	if r.AttackRolls == 0 {
		name := r.PlayerDisplayName
		if name == "" {
			name = r.PlayerID
		}
		return fmt.Sprintf("No attack rolls found for player %s in this game.", name)
	}

	var body strings.Builder
	fmt.Fprintf(&body, "Attack rolls       : %d\n", r.AttackRolls)
	fmt.Fprintf(&body, "Territories captured: %d (%.1f%%)\n\n", r.TerritoriesCaptured, r.CaptureRate)
	fmt.Fprintf(&body, "Attacker dice rolled: %d\n", r.AttackerDiceRolled)
	fmt.Fprintf(&body, "Avg dice per attack : %.2f\n\n", r.AverageAttackerDice)
	fmt.Fprintf(&body, "Armies lost attacking: %d\n", r.AttackerLosses)
	fmt.Fprintf(&body, "Defender armies dest : %d\n\n", r.DefenderLossesInflicted)
	fmt.Fprintf(&body, "Avg source armies   : %.1f\n", r.AverageSourceArmiesBefore)
	fmt.Fprintf(&body, "Avg target armies   : %.1f\n", r.AverageTargetArmiesBefore)
	fmt.Fprintf(&body, "Avg army advantage  : %.1f\n", r.AverageArmyAdvantage)

	return fmt.Sprintf(
		"⚔️ **Combat Stats — %s** · %s\n```\n%s```",
		r.PlayerDisplayName, gameName,
		body.String(),
	)
}

func writeFaceDistribution(sb *strings.Builder, dist reporting.FaceDistribution) {
	for face := 1; face <= 6; face++ {
		count := dist.Counts[face]
		var pct float64
		if dist.Total > 0 {
			pct = float64(count) / float64(dist.Total) * 100.0
		}
		fmt.Fprintf(sb, "  %d: %4d  (%.1f%%)\n", face, count, pct)
	}
}

func joinInts(vals []int) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = fmt.Sprintf("%d", v)
	}
	return strings.Join(parts, ", ")
}
