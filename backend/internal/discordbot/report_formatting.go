package discordbot

import (
	"fmt"
	"strings"

	"backend/internal/reporting"
)

// maxDiscordMessage is a conservative limit below Discord's 2000-character cap.
const maxDiscordMessage = 1900

// formatLastRolls renders a slice of recent combat rolls as a Discord message.
func formatLastRolls(rolls []reporting.RecentCombatRoll) string {
	if len(rolls) == 0 {
		return "No combat events found for this game yet."
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "🎲 **Last %d Combat Roll(s)**\n\n", len(rolls))

	for _, r := range rolls {
		atk := joinInts(r.AttackerDice)
		def := joinInts(r.DefenderDice)
		line := fmt.Sprintf(
			"#%d **%s** → **%s**: %s → %s\nAttack [%s] vs Defense [%s] — Losses: attacker %d, defender %d",
			r.GameSequence,
			r.AttackerDisplayName, r.DefenderDisplayName,
			r.SourceTerritoryID, r.TargetTerritoryID,
			atk, def,
			r.AttackerLosses, r.DefenderLosses,
		)
		if r.Captured {
			line += " ✅ captured"
		}
		line += "\n\n"

		if sb.Len()+len(line) > maxDiscordMessage {
			sb.WriteString("*(output truncated)*")
			break
		}
		sb.WriteString(line)
	}
	return strings.TrimSpace(sb.String())
}

// formatDiceReport renders an aggregate dice-statistics report.
func formatDiceReport(r reporting.DiceReport) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "🎲 **Dice Report**\n\n")
	fmt.Fprintf(&sb, "Combat rolls: %d\n", r.CombatRolls)
	fmt.Fprintf(&sb, "Attacker dice: %d\n", r.AttackerDice.Total)
	fmt.Fprintf(&sb, "Defender dice: %d\n\n", r.DefenderDice.Total)

	sb.WriteString("**Attacker faces**\n")
	writeFaceDistribution(&sb, r.AttackerDice)
	sb.WriteString("\n")

	sb.WriteString("**Defender faces**\n")
	writeFaceDistribution(&sb, r.DefenderDice)
	sb.WriteString("\n")

	fmt.Fprintf(&sb, "Attacker losses: %d\n", r.AttackerLosses)
	fmt.Fprintf(&sb, "Defender losses: %d\n", r.DefenderLosses)

	if r.CombatRolls > 0 {
		capRate := float64(r.Captures) / float64(r.CombatRolls) * 100.0
		fmt.Fprintf(&sb, "Captures: %d / %d (%.1f%%)\n", r.Captures, r.CombatRolls, capRate)
	} else {
		sb.WriteString("Captures: 0\n")
	}

	if r.SkippedEvents > 0 {
		fmt.Fprintf(&sb, "Skipped events: %d\n", r.SkippedEvents)
	}

	sb.WriteString("\n⚠️ *These are descriptive results — not proof of RNG bias or fairness.*")
	return sb.String()
}

// formatPlayerReport renders a single-player attack statistics report.
func formatPlayerReport(r reporting.PlayerCombatReport) string {
	if r.AttackRolls == 0 {
		name := r.PlayerDisplayName
		if name == "" {
			name = r.PlayerID
		}
		return fmt.Sprintf("No attack rolls found for player %s in this game.", name)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "⚔️ **Combat Stats — %s**\n\n", r.PlayerDisplayName)
	fmt.Fprintf(&sb, "Attack rolls: %d\n", r.AttackRolls)
	fmt.Fprintf(&sb, "Territories captured: %d (%.1f%%)\n\n", r.TerritoriesCaptured, r.CaptureRate)
	fmt.Fprintf(&sb, "Attacker dice rolled: %d\n", r.AttackerDiceRolled)
	fmt.Fprintf(&sb, "Average dice per attack: %.2f\n\n", r.AverageAttackerDice)
	fmt.Fprintf(&sb, "Armies lost while attacking: %d\n", r.AttackerLosses)
	fmt.Fprintf(&sb, "Defender armies destroyed: %d\n\n", r.DefenderLossesInflicted)
	fmt.Fprintf(&sb, "Average source armies: %.1f\n", r.AverageSourceArmiesBefore)
	fmt.Fprintf(&sb, "Average target armies: %.1f\n", r.AverageTargetArmiesBefore)
	fmt.Fprintf(&sb, "Average army advantage: %.1f", r.AverageArmyAdvantage)
	return sb.String()
}

func writeFaceDistribution(sb *strings.Builder, dist reporting.FaceDistribution) {
	for face := 1; face <= 6; face++ {
		count := dist.Counts[face]
		var pct float64
		if dist.Total > 0 {
			pct = float64(count) / float64(dist.Total) * 100.0
		}
		fmt.Fprintf(sb, "%d: %d (%.1f%%)", face, count, pct)
		if face < 6 {
			sb.WriteString("  ")
		}
	}
	sb.WriteString("\n")
}

func joinInts(vals []int) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = fmt.Sprintf("%d", v)
	}
	return strings.Join(parts, ", ")
}
