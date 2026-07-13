package main

import (
	"encoding/json"
	"io"

	"github.com/nmiano1111/global-conquest/backend/internal/reporting"
)

// jsonReport mirrors reporting.RollStreakReport but nests the three streak
// slices under a single "streaks" object, per the report's documented JSON shape.
type jsonReport struct {
	GameID            string                          `json:"game_id"`
	GameName          string                          `json:"game_name"`
	PartialHistory    bool                            `json:"partial_history"`
	Warnings          []string                        `json:"warnings"`
	SummaryByAttacker []reporting.PlayerStreakSummary `json:"summary_by_attacker"`
	Streaks           jsonStreaks                     `json:"streaks"`
}

type jsonStreaks struct {
	AttackingLoss []reporting.Streak `json:"attacking_loss"`
	AttackingWin  []reporting.Streak `json:"attacking_win"`
	AttackDrought []reporting.Streak `json:"attack_drought"`
}

func writeJSONReport(w io.Writer, r reporting.RollStreakReport) error {
	out := jsonReport{
		GameID:            r.GameID,
		GameName:          r.GameName,
		PartialHistory:    r.PartialHistory,
		Warnings:          emptyIfNil(r.Warnings),
		SummaryByAttacker: emptySummariesIfNil(r.SummaryByAttacker),
		Streaks: jsonStreaks{
			AttackingLoss: emptyStreaksIfNil(r.AttackingLossStreaks),
			AttackingWin:  emptyStreaksIfNil(r.AttackingWinStreaks),
			AttackDrought: emptyStreaksIfNil(r.AttackDroughts),
		},
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func emptyIfNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func emptyStreaksIfNil(s []reporting.Streak) []reporting.Streak {
	if s == nil {
		return []reporting.Streak{}
	}
	return s
}

func emptySummariesIfNil(s []reporting.PlayerStreakSummary) []reporting.PlayerStreakSummary {
	if s == nil {
		return []reporting.PlayerStreakSummary{}
	}
	return s
}
