package reporting

import (
	"encoding/json"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// decodeCombatEvent decodes and validates a single raw combat event row.
//
// Return semantics:
//   - (event, false, nil)  — valid event, include in calculations
//   - (zero,  true,  nil)  — malformed data; caller increments skip count and continues
//   - (zero,  false, err)  — unsupported event_version or schema_version; caller fails
func decodeCombatEvent(row rawCombatRow) (CombatEvent, bool, error) {
	if row.eventVersion != risk.EventVersionCombatRollResolved {
		return CombatEvent{}, false, ErrUnsupportedEventVersion{
			GameSequence: row.gameSequence,
			EventVersion: row.eventVersion,
		}
	}

	var p risk.CombatRollResolvedPayload
	if err := json.Unmarshal(row.payload, &p); err != nil {
		return CombatEvent{}, true, nil // skip malformed JSON
	}

	if p.SchemaVersion != risk.SchemaVersionCombatRollResolved {
		return CombatEvent{}, false, ErrUnsupportedSchemaVersion{
			GameSequence:  row.gameSequence,
			SchemaVersion: p.SchemaVersion,
		}
	}

	// Required identifiers.
	if p.AttackerPlayerID == "" || p.DefenderPlayerID == "" ||
		p.SourceTerritoryID == "" || p.TargetTerritoryID == "" {
		return CombatEvent{}, true, nil
	}

	// Dice arrays must be non-empty and in range 1–6.
	if len(p.AttackerDice) == 0 || len(p.DefenderDice) == 0 {
		return CombatEvent{}, true, nil
	}
	for _, d := range p.AttackerDice {
		if d < 1 || d > 6 {
			return CombatEvent{}, true, nil
		}
	}
	for _, d := range p.DefenderDice {
		if d < 1 || d > 6 {
			return CombatEvent{}, true, nil
		}
	}

	// Losses must be non-negative.
	if p.AttackerLosses < 0 || p.DefenderLosses < 0 {
		return CombatEvent{}, true, nil
	}

	return CombatEvent{
		ID:                 row.id,
		GameID:             row.gameID,
		GameSequence:       row.gameSequence,
		OccurredAt:         row.occurredAt,
		AttackerPlayerID:   p.AttackerPlayerID,
		DefenderPlayerID:   p.DefenderPlayerID,
		SourceTerritoryID:  p.SourceTerritoryID,
		TargetTerritoryID:  p.TargetTerritoryID,
		SourceArmiesBefore: p.SourceArmiesBefore,
		TargetArmiesBefore: p.TargetArmiesBefore,
		AttackerDice:       p.AttackerDice,
		DefenderDice:       p.DefenderDice,
		Comparisons:        p.Comparisons,
		AttackerLosses:     p.AttackerLosses,
		DefenderLosses:     p.DefenderLosses,
		TerritoryCaptured:  p.TerritoryCaptured,
	}, false, nil
}

// decodeAll decodes every raw row. Malformed rows increment the skipped counter.
// Returns an error immediately if any row carries an unsupported version.
func decodeAll(rows []rawCombatRow) (events []CombatEvent, skipped int, err error) {
	for _, row := range rows {
		ev, skip, decErr := decodeCombatEvent(row)
		if decErr != nil {
			return nil, 0, decErr
		}
		if skip {
			skipped++
			continue
		}
		events = append(events, ev)
	}
	return events, skipped, nil
}

// BuildDiceReport calculates aggregate dice statistics from decoded events.
func BuildDiceReport(gameID string, events []CombatEvent, skipped int) DiceReport {
	report := DiceReport{
		GameID:        gameID,
		SkippedEvents: skipped,
		AttackerDice:  FaceDistribution{Counts: make(map[int]int)},
		DefenderDice:  FaceDistribution{Counts: make(map[int]int)},
	}
	for _, ev := range events {
		report.CombatRolls++
		if ev.TerritoryCaptured {
			report.Captures++
		}
		report.AttackerLosses += ev.AttackerLosses
		report.DefenderLosses += ev.DefenderLosses
		for _, d := range ev.AttackerDice {
			report.AttackerDice.Counts[d]++
			report.AttackerDice.Total++
		}
		for _, d := range ev.DefenderDice {
			report.DefenderDice.Counts[d]++
			report.DefenderDice.Total++
		}
	}
	return report
}

// BuildPlayerReport calculates attack statistics for a single attacker.
// Only events where ev.AttackerPlayerID == playerID are counted.
func BuildPlayerReport(gameID, playerID string, events []CombatEvent, names map[string]string) PlayerCombatReport {
	report := PlayerCombatReport{
		GameID:            gameID,
		PlayerID:          playerID,
		PlayerDisplayName: playerDisplayName(names, playerID),
	}
	var totalSource, totalTarget float64
	for _, ev := range events {
		if ev.AttackerPlayerID != playerID {
			continue
		}
		report.AttackRolls++
		if ev.TerritoryCaptured {
			report.TerritoriesCaptured++
		}
		report.AttackerDiceRolled += len(ev.AttackerDice)
		report.AttackerLosses += ev.AttackerLosses
		report.DefenderLossesInflicted += ev.DefenderLosses
		totalSource += float64(ev.SourceArmiesBefore)
		totalTarget += float64(ev.TargetArmiesBefore)
	}
	if report.AttackRolls > 0 {
		n := float64(report.AttackRolls)
		report.AverageAttackerDice = float64(report.AttackerDiceRolled) / n
		report.AverageSourceArmiesBefore = totalSource / n
		report.AverageTargetArmiesBefore = totalTarget / n
		report.AverageArmyAdvantage = report.AverageSourceArmiesBefore - report.AverageTargetArmiesBefore
		report.CaptureRate = float64(report.TerritoriesCaptured) / n * 100.0
	}
	return report
}

// BuildRecentRolls converts events into display-ready roll entries.
// The caller must pass events already in the intended display order.
func BuildRecentRolls(events []CombatEvent, names map[string]string) []RecentCombatRoll {
	out := make([]RecentCombatRoll, 0, len(events))
	for _, ev := range events {
		out = append(out, RecentCombatRoll{
			GameSequence:        ev.GameSequence,
			OccurredAt:          ev.OccurredAt,
			AttackerPlayerID:    ev.AttackerPlayerID,
			DefenderPlayerID:    ev.DefenderPlayerID,
			AttackerDisplayName: playerDisplayName(names, ev.AttackerPlayerID),
			DefenderDisplayName: playerDisplayName(names, ev.DefenderPlayerID),
			SourceTerritoryID:   ev.SourceTerritoryID,
			TargetTerritoryID:   ev.TargetTerritoryID,
			AttackerDice:        ev.AttackerDice,
			DefenderDice:        ev.DefenderDice,
			AttackerLosses:      ev.AttackerLosses,
			DefenderLosses:      ev.DefenderLosses,
			Captured:            ev.TerritoryCaptured,
		})
	}
	return out
}

// uniquePlayerIDs collects all unique attacker and defender IDs from events.
func uniquePlayerIDs(events []CombatEvent) []string {
	seen := make(map[string]struct{})
	var ids []string
	for _, ev := range events {
		if _, ok := seen[ev.AttackerPlayerID]; !ok && ev.AttackerPlayerID != "" {
			seen[ev.AttackerPlayerID] = struct{}{}
			ids = append(ids, ev.AttackerPlayerID)
		}
		if _, ok := seen[ev.DefenderPlayerID]; !ok && ev.DefenderPlayerID != "" {
			seen[ev.DefenderPlayerID] = struct{}{}
			ids = append(ids, ev.DefenderPlayerID)
		}
	}
	return ids
}

// playerDisplayName returns the username for a player, falling back to a UUID
// prefix when the name is not available.
func playerDisplayName(names map[string]string, id string) string {
	if n, ok := names[id]; ok && n != "" {
		return n
	}
	if len(id) >= 8 {
		return id[:8]
	}
	return id
}
