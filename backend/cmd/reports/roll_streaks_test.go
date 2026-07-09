package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"backend/internal/reporting"
)

func sampleReport() reporting.RollStreakReport {
	lossStreak := reporting.Streak{
		ID:                      "game-123:tucker:attacking_loss:144-148",
		GameID:                  "game-123",
		GameName:                "Greasy Weasel",
		AttackerID:              "tucker",
		AttackerName:            "Tucker",
		Type:                    reporting.StreakAttackingLoss,
		Length:                  5,
		StartSeq:                144,
		EndSeq:                  148,
		DefendersInvolved:       []string{"Nick", "Dan"},
		AttackerTerritories:     []string{"ukraine", "middle_east"},
		DefenderTerritories:     []string{"afghanistan", "india"},
		AttackerArmiesLost:      10,
		DefenderArmiesLost:      0,
		NetArmyDeltaForAttacker: -10,
		RollTrace:               "2-0, 2-0, 2-0, 2-0, 2-0",
		Rolls: []reporting.StreakRoll{
			{GameSequence: 144, AttackerTerritoryID: "ukraine", DefenderTerritoryID: "afghanistan"},
			{GameSequence: 148, AttackerTerritoryID: "middle_east", DefenderTerritoryID: "india"},
		},
	}
	return reporting.RollStreakReport{
		GameID:         "game-123",
		GameName:       "Greasy Weasel",
		PartialHistory: true,
		Warnings:       []string{"this game has partial event history. Streaks only reflect captured rolls after event logging began."},
		SummaryByAttacker: []reporting.PlayerStreakSummary{
			{
				PlayerID: "tucker", PlayerName: "Tucker", GameID: "game-123", GameName: "Greasy Weasel",
				AttackRollsCaptured: 6, AttackerLossCount: 5, SplitCount: 1,
				LossStreakCount2Plus: 1, LongestLossStreak: 5, LongestLossStreakID: lossStreak.ID,
			},
		},
		AttackingLossStreaks: []reporting.Streak{lossStreak},
	}
}

func TestWriteMarkdownReport_ContainsRequiredSections(t *testing.T) {
	var buf bytes.Buffer
	if err := writeMarkdownReport(&buf, sampleReport(), 5); err != nil {
		t.Fatalf("writeMarkdownReport: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"# Roll Streak Report — Greasy Weasel",
		"Note: this game has partial event history",
		"## Definitions",
		"## Summary by Attacker",
		"## Individual Attacking Loss Streaks",
		"## Individual Attacking Win Streaks",
		"## Individual Attack Droughts",
		"## Diagnostics",
		"L1. Tucker — 5 attacking losses",
		"Territories: ukraine → afghanistan, middle_east → india",
		"Rolls: 2-0, 2-0, 2-0, 2-0, 2-0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown output missing %q\n--- full output ---\n%s", want, out)
		}
	}
}

func TestWriteMarkdownReport_TopLimitsStreaksShown(t *testing.T) {
	report := sampleReport()
	extra := report.AttackingLossStreaks[0]
	extra.ID = "game-123:tucker:attacking_loss:200-201"
	extra.Length = 2
	extra.StartSeq, extra.EndSeq = 200, 201
	report.AttackingLossStreaks = append(report.AttackingLossStreaks, extra)

	var buf bytes.Buffer
	if err := writeMarkdownReport(&buf, report, 1); err != nil {
		t.Fatalf("writeMarkdownReport: %v", err)
	}
	if !strings.Contains(buf.String(), "1 more not shown") {
		t.Errorf("expected truncation notice with top=1 and 2 streaks, got:\n%s", buf.String())
	}
}

func TestWriteJSONReport_ValidShape(t *testing.T) {
	var buf bytes.Buffer
	if err := writeJSONReport(&buf, sampleReport()); err != nil {
		t.Fatalf("writeJSONReport: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}

	for _, key := range []string{"game_id", "game_name", "partial_history", "warnings", "summary_by_attacker", "streaks"} {
		if _, ok := parsed[key]; !ok {
			t.Errorf("missing top-level key %q", key)
		}
	}

	streaks, ok := parsed["streaks"].(map[string]any)
	if !ok {
		t.Fatalf("streaks is not an object: %v", parsed["streaks"])
	}
	for _, key := range []string{"attacking_loss", "attacking_win", "attack_drought"} {
		if _, ok := streaks[key]; !ok {
			t.Errorf("missing streaks.%s", key)
		}
	}

	lossStreaks, ok := streaks["attacking_loss"].([]any)
	if !ok || len(lossStreaks) != 1 {
		t.Fatalf("expected 1 attacking_loss streak, got %v", streaks["attacking_loss"])
	}
	first := lossStreaks[0].(map[string]any)
	if first["streak_id"] != "game-123:tucker:attacking_loss:144-148" {
		t.Errorf("unexpected streak_id: %v", first["streak_id"])
	}
	if first["streak_type"] != "attacking_loss" {
		t.Errorf("unexpected streak_type: %v", first["streak_type"])
	}
}

func TestWriteJSONReport_EmptyStreaksAreEmptyArraysNotNull(t *testing.T) {
	var buf bytes.Buffer
	if err := writeJSONReport(&buf, reporting.RollStreakReport{GameID: "g1", GameName: "Empty"}); err != nil {
		t.Fatalf("writeJSONReport: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "null") {
		t.Errorf("expected empty arrays, not null, in JSON output:\n%s", out)
	}
}
