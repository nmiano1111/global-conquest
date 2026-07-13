package reporting

import (
	"fmt"
	"slices"
	"sort"
)

// RollResult classifies a single combat roll from the attacker's perspective.
type RollResult int

const (
	// RollAttackerWin indicates the attacker inflicted more losses on the defender than it took itself.
	RollAttackerWin RollResult = iota
	// RollAttackerLoss indicates the attacker took more losses than it inflicted on the defender.
	RollAttackerLoss
	// RollSplit indicates the attacker and defender took an equal number of losses.
	RollSplit
)

// StreakType identifies which of the three streak definitions a Streak represents.
type StreakType string

const (
	// StreakAttackingLoss identifies a run of consecutive rolls in which the attacker lost more armies than it inflicted.
	StreakAttackingLoss StreakType = "attacking_loss"
	// StreakAttackingWin identifies a run of consecutive rolls in which the attacker inflicted more losses than it took.
	StreakAttackingWin StreakType = "attacking_win"
	// StreakAttackDrought identifies a run of consecutive rolls in which the attacker failed to win outright (losses or splits).
	StreakAttackDrought StreakType = "attack_drought"
)

// StreakThresholds configures the minimum length required for a run of rolls
// to be reported as a streak of each type.
type StreakThresholds struct {
	// MinLossStreakLength is the minimum number of consecutive attacker-loss rolls required to report a loss streak.
	MinLossStreakLength int
	// MinWinStreakLength is the minimum number of consecutive attacker-win rolls required to report a win streak.
	MinWinStreakLength int
	// MinDroughtLength is the minimum number of consecutive non-win rolls required to report an attack drought.
	MinDroughtLength int
}

// DefaultStreakThresholds returns the report's default thresholds:
// loss/win streaks at 2+, droughts at 3+.
func DefaultStreakThresholds() StreakThresholds {
	return StreakThresholds{MinLossStreakLength: 2, MinWinStreakLength: 2, MinDroughtLength: 3}
}

// classifyRoll returns the roll result from the attacker's perspective.
func classifyRoll(ev CombatEvent) RollResult {
	switch {
	case ev.DefenderLosses > ev.AttackerLosses:
		return RollAttackerWin
	case ev.AttackerLosses > ev.DefenderLosses:
		return RollAttackerLoss
	default:
		return RollSplit
	}
}

// matchesStreakType reports whether a roll result continues a streak of the given type.
func matchesStreakType(result RollResult, t StreakType) bool {
	switch t {
	case StreakAttackingLoss:
		return result == RollAttackerLoss
	case StreakAttackingWin:
		return result == RollAttackerWin
	case StreakAttackDrought:
		return result != RollAttackerWin
	default:
		return false
	}
}

// StreakRoll is a single roll's display-relevant detail within a Streak.
type StreakRoll struct {
	// GameSequence is the game-relative sequence number of the roll.
	GameSequence int64 `json:"event_seq"`
	// OccurredAt is the RFC3339 timestamp at which the roll was resolved.
	OccurredAt string `json:"created_at"`
	// DefenderPlayerID is the ID of the defending player.
	DefenderPlayerID string `json:"defender_id"`
	// DefenderDisplayName is the defending player's display name.
	DefenderDisplayName string `json:"defender_name"`
	// AttackerTerritoryID is the ID of the territory the attack was launched from.
	AttackerTerritoryID string `json:"attacker_territory"`
	// DefenderTerritoryID is the ID of the territory being attacked.
	DefenderTerritoryID string `json:"defender_territory"`
	// AttackerDice holds the face values rolled by the attacker.
	AttackerDice []int `json:"attack_dice"`
	// DefenderDice holds the face values rolled by the defender.
	DefenderDice []int `json:"defend_dice"`
	// AttackerLosses is the number of armies the attacker lost in this roll.
	AttackerLosses int `json:"attacker_losses"`
	// DefenderLosses is the number of armies the defender lost in this roll.
	DefenderLosses int `json:"defender_losses"`
	// Captured reports whether this roll resulted in the target territory being captured.
	Captured bool `json:"captured"`
}

// Streak is one qualifying run of consecutive matching rolls by a single
// attacker within a single game.
type Streak struct {
	// ID uniquely identifies this streak, derived from the game, attacker, type, and sequence range.
	ID string `json:"streak_id"`
	// GameID identifies the game this streak occurred in.
	GameID string `json:"game_id"`
	// GameName is the display name of the game this streak occurred in.
	GameName string `json:"game_name"`
	// AttackerID is the ID of the player who accumulated this streak.
	AttackerID string `json:"attacker_id"`
	// AttackerName is the display name of the player who accumulated this streak.
	AttackerName string `json:"attacker_name"`
	// Type identifies which streak definition this run satisfies.
	Type StreakType `json:"streak_type"`
	// Length is the number of consecutive matching rolls in this streak.
	Length int `json:"streak_length"`
	// StartSeq is the game sequence number of the streak's first roll.
	StartSeq int64 `json:"start_event_seq"`
	// EndSeq is the game sequence number of the streak's last roll.
	EndSeq int64 `json:"end_event_seq"`
	// StartTime is the RFC3339 timestamp of the streak's first roll.
	StartTime string `json:"start_time"`
	// EndTime is the RFC3339 timestamp of the streak's last roll.
	EndTime string `json:"end_time"`

	// DefendersInvolved lists the display names of every distinct defender faced during the streak, in first-seen order.
	DefendersInvolved []string `json:"defenders_involved"`
	// AttackerTerritories lists the distinct source territory IDs the attacker attacked from during the streak, in first-seen order.
	AttackerTerritories []string `json:"attacker_territories"`
	// DefenderTerritories lists the distinct target territory IDs attacked during the streak, in first-seen order.
	DefenderTerritories []string `json:"defender_territories"`

	// AttackerArmiesLost is the total number of armies the attacker lost across all rolls in the streak.
	AttackerArmiesLost int `json:"attacker_armies_lost"`
	// DefenderArmiesLost is the total number of armies defenders lost across all rolls in the streak.
	DefenderArmiesLost int `json:"defender_armies_lost"`
	// NetArmyDeltaForAttacker is DefenderArmiesLost minus AttackerArmiesLost for the streak.
	NetArmyDeltaForAttacker int `json:"net_army_delta_for_attacker"`
	// CapturesDuringStreak is the number of territory captures that occurred during the streak.
	CapturesDuringStreak int `json:"captures_during_streak"`

	// RollTrace is a human-readable, comma-separated "attackerLosses-defenderLosses" summary of each roll in the streak.
	RollTrace string `json:"roll_trace"`
	// Rolls holds the display detail for each individual roll in the streak, in chronological order.
	Rolls []StreakRoll `json:"rolls"`
}

// PlayerStreakSummary aggregates one attacker's roll and streak statistics
// for a single game.
type PlayerStreakSummary struct {
	// PlayerID is the ID of the attacking player this summary covers.
	PlayerID string `json:"player_id"`
	// PlayerName is the attacking player's display name.
	PlayerName string `json:"player_name"`
	// GameID identifies the game this summary covers.
	GameID string `json:"game_id"`
	// GameName is the display name of the game this summary covers.
	GameName string `json:"game_name"`
	// AttackRollsCaptured is the total number of combat rolls this player initiated as attacker.
	AttackRollsCaptured int `json:"attack_rolls_captured"`

	// AttackerWinCount is the number of rolls this player won outright as attacker.
	AttackerWinCount int `json:"attacker_win_count"`
	// AttackerLossCount is the number of rolls this player lost outright as attacker.
	AttackerLossCount int `json:"attacker_loss_count"`
	// SplitCount is the number of rolls that ended in an even split of losses.
	SplitCount int `json:"split_count"`

	// LossStreakCount2Plus is the number of qualifying attacker-loss streaks (length >= the configured MinLossStreakLength) this player accumulated.
	LossStreakCount2Plus int `json:"loss_streak_count_2_plus"`
	// LongestLossStreak is the length of this player's longest qualifying loss streak.
	LongestLossStreak int `json:"longest_loss_streak"`
	// LongestLossStreakID is the Streak.ID of this player's longest loss streak.
	LongestLossStreakID string `json:"longest_loss_streak_id"`

	// WinStreakCount2Plus is the number of qualifying attacker-win streaks (length >= the configured MinWinStreakLength) this player accumulated.
	WinStreakCount2Plus int `json:"win_streak_count_2_plus"`
	// LongestWinStreak is the length of this player's longest qualifying win streak.
	LongestWinStreak int `json:"longest_win_streak"`
	// LongestWinStreakID is the Streak.ID of this player's longest win streak.
	LongestWinStreakID string `json:"longest_win_streak_id"`

	// AttackDroughtCount3Plus is the number of qualifying attack droughts (length >= the configured MinDroughtLength) this player accumulated.
	AttackDroughtCount3Plus int `json:"attack_drought_count_3_plus"`
	// LongestAttackDrought is the length of this player's longest qualifying attack drought.
	LongestAttackDrought int `json:"longest_attack_drought"`
	// LongestAttackDroughtID is the Streak.ID of this player's longest attack drought.
	LongestAttackDroughtID string `json:"longest_attack_drought_id"`

	// LossStreaksPer20Attacks is LossStreakCount2Plus normalized to a rate per 20 attack rolls.
	LossStreaksPer20Attacks float64 `json:"loss_streaks_per_20_attacks"`
	// WinStreaksPer20Attacks is WinStreakCount2Plus normalized to a rate per 20 attack rolls.
	WinStreaksPer20Attacks float64 `json:"win_streaks_per_20_attacks"`
	// DroughtsPer20Attacks is AttackDroughtCount3Plus normalized to a rate per 20 attack rolls.
	DroughtsPer20Attacks float64 `json:"droughts_per_20_attacks"`
}

// RollStreakReport is the full output of streak detection for one game.
type RollStreakReport struct {
	// GameID identifies the game this report covers.
	GameID string `json:"game_id"`
	// GameName is the display name of the game this report covers.
	GameName string `json:"game_name"`
	// PartialHistory reports whether the game's combat event log begins after the game itself started, meaning streaks may be incomplete.
	PartialHistory bool `json:"partial_history"`
	// Warnings lists data-quality issues detected while building the report.
	Warnings []string `json:"warnings"`

	// SummaryByAttacker holds one aggregated summary per attacking player, sorted by notability.
	SummaryByAttacker []PlayerStreakSummary `json:"summary_by_attacker"`

	// AttackingLossStreaks holds every qualifying attacker-loss streak across all players, sorted by length descending.
	AttackingLossStreaks []Streak `json:"-"`
	// AttackingWinStreaks holds every qualifying attacker-win streak across all players, sorted by length descending.
	AttackingWinStreaks []Streak `json:"-"`
	// AttackDroughts holds every qualifying attack-drought streak across all players, sorted by length descending.
	AttackDroughts []Streak `json:"-"`
}

// rollTimeFormat is used for StreakRoll/Streak time fields (RFC3339, UTC).
const rollTimeFormat = "2006-01-02T15:04:05Z07:00"

// BuildRollStreakReport is the pure, testable core of the roll streak report.
// It takes already-decoded combat events (see decodeCombatEvent), the count of
// events skipped during decoding, player display names, and streak thresholds,
// and returns per-player summaries, individual streak details, and data-quality
// warnings. It does not touch the database, Discord, or any output format.
func BuildRollStreakReport(
	gameID, gameName string,
	partialHistory bool,
	events []CombatEvent,
	skipped int,
	names map[string]string,
	thresholds StreakThresholds,
) RollStreakReport {
	report := RollStreakReport{
		GameID:         gameID,
		GameName:       gameName,
		PartialHistory: partialHistory,
	}

	// Defensively restrict to this game's events even if the caller passed a
	// mixed-game slice; streaks must never bridge across games.
	sorted := make([]CombatEvent, 0, len(events))
	for _, ev := range events {
		if ev.GameID == gameID {
			sorted = append(sorted, ev)
		}
	}
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].GameSequence < sorted[j].GameSequence })

	report.Warnings = append(report.Warnings, detectDataWarnings(sorted)...)
	if skipped > 0 {
		report.Warnings = append(report.Warnings, fmt.Sprintf(
			"%d event(s) were skipped during decoding (missing IDs, invalid dice, or negative losses) and are excluded from this report",
			skipped,
		))
	}
	if partialHistory {
		report.Warnings = append(report.Warnings,
			"this game has partial event history. Streaks only reflect captured rolls after event logging began.")
	}

	// Group by attacker, preserving first-seen order for stable output.
	var attackerOrder []string
	byAttacker := make(map[string][]CombatEvent)
	for _, ev := range sorted {
		if ev.AttackerPlayerID == "" {
			continue
		}
		if _, ok := byAttacker[ev.AttackerPlayerID]; !ok {
			attackerOrder = append(attackerOrder, ev.AttackerPlayerID)
		}
		byAttacker[ev.AttackerPlayerID] = append(byAttacker[ev.AttackerPlayerID], ev)
	}

	for _, attackerID := range attackerOrder {
		attackerEvents := byAttacker[attackerID]
		attackerName := playerDisplayName(names, attackerID)

		summary := PlayerStreakSummary{
			PlayerID:            attackerID,
			PlayerName:          attackerName,
			GameID:              gameID,
			GameName:            gameName,
			AttackRollsCaptured: len(attackerEvents),
		}

		for _, ev := range attackerEvents {
			switch classifyRoll(ev) {
			case RollAttackerWin:
				summary.AttackerWinCount++
			case RollAttackerLoss:
				summary.AttackerLossCount++
			case RollSplit:
				summary.SplitCount++
			}
		}

		lossStreaks := detectStreaks(attackerEvents, StreakAttackingLoss, thresholds.MinLossStreakLength, gameID, gameName, attackerID, attackerName, names)
		winStreaks := detectStreaks(attackerEvents, StreakAttackingWin, thresholds.MinWinStreakLength, gameID, gameName, attackerID, attackerName, names)
		droughts := detectStreaks(attackerEvents, StreakAttackDrought, thresholds.MinDroughtLength, gameID, gameName, attackerID, attackerName, names)

		summary.LossStreakCount2Plus = len(lossStreaks)
		summary.WinStreakCount2Plus = len(winStreaks)
		summary.AttackDroughtCount3Plus = len(droughts)

		if s := longestStreak(lossStreaks); s != nil {
			summary.LongestLossStreak = s.Length
			summary.LongestLossStreakID = s.ID
		}
		if s := longestStreak(winStreaks); s != nil {
			summary.LongestWinStreak = s.Length
			summary.LongestWinStreakID = s.ID
		}
		if s := longestStreak(droughts); s != nil {
			summary.LongestAttackDrought = s.Length
			summary.LongestAttackDroughtID = s.ID
		}

		if summary.AttackRollsCaptured > 0 {
			n := float64(summary.AttackRollsCaptured)
			summary.LossStreaksPer20Attacks = float64(summary.LossStreakCount2Plus) / n * 20
			summary.WinStreaksPer20Attacks = float64(summary.WinStreakCount2Plus) / n * 20
			summary.DroughtsPer20Attacks = float64(summary.AttackDroughtCount3Plus) / n * 20
		}

		report.SummaryByAttacker = append(report.SummaryByAttacker, summary)
		report.AttackingLossStreaks = append(report.AttackingLossStreaks, lossStreaks...)
		report.AttackingWinStreaks = append(report.AttackingWinStreaks, winStreaks...)
		report.AttackDroughts = append(report.AttackDroughts, droughts...)
	}

	sortStreaks(report.AttackingLossStreaks)
	sortStreaks(report.AttackingWinStreaks)
	sortStreaks(report.AttackDroughts)
	sortPlayerSummaries(report.SummaryByAttacker)

	return report
}

// detectStreaks scans one attacker's chronologically ordered events and returns
// every contiguous run of rolls matching streakType that meets minLength.
func detectStreaks(
	events []CombatEvent,
	streakType StreakType,
	minLength int,
	gameID, gameName, attackerID, attackerName string,
	names map[string]string,
) []Streak {
	var streaks []Streak
	var run []CombatEvent

	flush := func() {
		if len(run) >= minLength {
			streaks = append(streaks, buildStreak(run, gameID, gameName, attackerID, attackerName, streakType, names))
		}
		run = nil
	}

	for _, ev := range events {
		if matchesStreakType(classifyRoll(ev), streakType) {
			run = append(run, ev)
			continue
		}
		flush()
	}
	flush()

	return streaks
}

// buildStreak converts a contiguous run of matching events into a Streak.
func buildStreak(
	run []CombatEvent,
	gameID, gameName, attackerID, attackerName string,
	streakType StreakType,
	names map[string]string,
) Streak {
	first, last := run[0], run[len(run)-1]

	s := Streak{
		ID:           fmt.Sprintf("%s:%s:%s:%d-%d", gameID, attackerID, streakType, first.GameSequence, last.GameSequence),
		GameID:       gameID,
		GameName:     gameName,
		AttackerID:   attackerID,
		AttackerName: attackerName,
		Type:         streakType,
		Length:       len(run),
		StartSeq:     first.GameSequence,
		EndSeq:       last.GameSequence,
		StartTime:    first.OccurredAt.Format(rollTimeFormat),
		EndTime:      last.OccurredAt.Format(rollTimeFormat),
	}

	seenDefenders := make(map[string]struct{})
	seenAttackerTerr := make(map[string]struct{})
	seenDefenderTerr := make(map[string]struct{})
	var traceParts []string

	for _, ev := range run {
		defenderName := playerDisplayName(names, ev.DefenderPlayerID)
		if _, ok := seenDefenders[defenderName]; !ok {
			seenDefenders[defenderName] = struct{}{}
			s.DefendersInvolved = append(s.DefendersInvolved, defenderName)
		}
		if _, ok := seenAttackerTerr[ev.SourceTerritoryID]; !ok {
			seenAttackerTerr[ev.SourceTerritoryID] = struct{}{}
			s.AttackerTerritories = append(s.AttackerTerritories, ev.SourceTerritoryID)
		}
		if _, ok := seenDefenderTerr[ev.TargetTerritoryID]; !ok {
			seenDefenderTerr[ev.TargetTerritoryID] = struct{}{}
			s.DefenderTerritories = append(s.DefenderTerritories, ev.TargetTerritoryID)
		}

		s.AttackerArmiesLost += ev.AttackerLosses
		s.DefenderArmiesLost += ev.DefenderLosses
		if ev.TerritoryCaptured {
			s.CapturesDuringStreak++
		}

		traceParts = append(traceParts, fmt.Sprintf("%d-%d", ev.AttackerLosses, ev.DefenderLosses))

		s.Rolls = append(s.Rolls, StreakRoll{
			GameSequence:        ev.GameSequence,
			OccurredAt:          ev.OccurredAt.Format(rollTimeFormat),
			DefenderPlayerID:    ev.DefenderPlayerID,
			DefenderDisplayName: defenderName,
			AttackerTerritoryID: ev.SourceTerritoryID,
			DefenderTerritoryID: ev.TargetTerritoryID,
			AttackerDice:        ev.AttackerDice,
			DefenderDice:        ev.DefenderDice,
			AttackerLosses:      ev.AttackerLosses,
			DefenderLosses:      ev.DefenderLosses,
			Captured:            ev.TerritoryCaptured,
		})
	}

	s.NetArmyDeltaForAttacker = s.DefenderArmiesLost - s.AttackerArmiesLost
	s.RollTrace = joinTrace(traceParts)

	return s
}

func joinTrace(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}

func longestStreak(streaks []Streak) *Streak {
	if len(streaks) == 0 {
		return nil
	}
	longest := streaks[0]
	for _, s := range streaks[1:] {
		if s.Length > longest.Length {
			longest = s
		}
	}
	return &longest
}

// sortStreaks orders a slice of streaks by length DESC, then start time ASC.
func sortStreaks(streaks []Streak) {
	sort.SliceStable(streaks, func(i, j int) bool {
		if streaks[i].Length != streaks[j].Length {
			return streaks[i].Length > streaks[j].Length
		}
		if streaks[i].StartTime != streaks[j].StartTime {
			return streaks[i].StartTime < streaks[j].StartTime
		}
		return streaks[i].StartSeq < streaks[j].StartSeq
	})
}

// sortPlayerSummaries orders summaries by longest drought, then longest loss
// streak, then loss-streak count, then rolls captured — all DESC.
func sortPlayerSummaries(summaries []PlayerStreakSummary) {
	sort.SliceStable(summaries, func(i, j int) bool {
		a, b := summaries[i], summaries[j]
		if a.LongestAttackDrought != b.LongestAttackDrought {
			return a.LongestAttackDrought > b.LongestAttackDrought
		}
		if a.LongestLossStreak != b.LongestLossStreak {
			return a.LongestLossStreak > b.LongestLossStreak
		}
		if a.LossStreakCount2Plus != b.LossStreakCount2Plus {
			return a.LossStreakCount2Plus > b.LossStreakCount2Plus
		}
		return a.AttackRollsCaptured > b.AttackRollsCaptured
	})
}

// detectDataWarnings surfaces suspicious event data that passed decoding but
// still merits a diagnostics note: duplicate sequences, zero-zero outcomes,
// and dice/loss-comparison mismatches.
func detectDataWarnings(sorted []CombatEvent) []string {
	var warnings []string
	seenSeq := make(map[int64]int)

	for _, ev := range sorted {
		if ev.GameSequence <= 0 {
			warnings = append(warnings, fmt.Sprintf(
				"event %s has a missing or invalid game_sequence (%d)", ev.ID, ev.GameSequence))
		}
		seenSeq[ev.GameSequence]++

		if ev.AttackerLosses == 0 && ev.DefenderLosses == 0 {
			warnings = append(warnings, fmt.Sprintf(
				"event %s (game_sequence %d) has attacker_losses=0 and defender_losses=0, which is not a valid combat outcome",
				ev.ID, ev.GameSequence))
		}

		if len(ev.Comparisons) > 0 {
			expected := len(ev.Comparisons)
			actual := ev.AttackerLosses + ev.DefenderLosses
			if actual != expected {
				warnings = append(warnings, fmt.Sprintf(
					"event %s (game_sequence %d) has attacker_losses+defender_losses (%d) != len(comparisons) (%d)",
					ev.ID, ev.GameSequence, actual, expected))
			}
		}
	}

	var dupSeqs []int64
	for seq, count := range seenSeq {
		if count > 1 {
			dupSeqs = append(dupSeqs, seq)
		}
	}
	slices.Sort(dupSeqs)
	for _, seq := range dupSeqs {
		warnings = append(warnings, fmt.Sprintf("game_sequence %d appears %d times (duplicate event sequence)", seq, seenSeq[seq]))
	}

	return warnings
}
