package main

import (
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
	"github.com/nmiano1111/global-conquest/backend/internal/simulation"
)

func entry(phase, playerID string, features ...bot.Feature) simulation.Entry {
	return simulation.Entry{
		CommandIndex: 5,
		Turn:         3,
		Seat:         1,
		PlayerID:     playerID,
		StrategyID:   bot.StrategyScoredV1,
		Phase:        phase,
		Explanation:  bot.Explanation{Features: features},
	}
}

func TestRowsFromEntriesRecoversRawSignalPerPhase(t *testing.T) {
	w := bot.DefaultWeights
	cases := []struct {
		phase   string
		feature bot.Feature
		wantRaw float64
	}{
		{"attack", bot.Feature{Name: "army_advantage", Value: w.ArmyAdvantage * 4}, 4},
		{"reinforce", bot.Feature{Name: "enemy_threat", Value: w.ReinforceEnemyThreat * 3}, 3},
		{"setup_reinforce", bot.Feature{Name: "weakness", Value: w.ReinforceWeakness * -2}, -2},
		{"occupy", bot.Feature{Name: "momentum", Value: w.OccupyMomentum * 5}, 5},
		{"fortify", bot.Feature{Name: "destination_threat", Value: w.FortifyDestinationThreat * 1.5}, 1.5},
	}
	for _, tc := range cases {
		t.Run(tc.phase, func(t *testing.T) {
			rows := rowsFromEntries(1, []simulation.Entry{entry(tc.phase, "p0", tc.feature)}, "p0")
			if len(rows) != 1 {
				t.Fatalf("expected 1 row, got %d", len(rows))
			}
			got, ok := rows[0].Features[tc.feature.Name]
			if !ok {
				t.Fatalf("expected feature %q present in row", tc.feature.Name)
			}
			if diff := got - tc.wantRaw; diff > 1e-9 || diff < -1e-9 {
				t.Fatalf("%s raw signal = %v, want %v", tc.feature.Name, got, tc.wantRaw)
			}
		})
	}
}

func TestRowsFromEntriesDefaultsAbsentBooleanFeatureToZero(t *testing.T) {
	w := bot.DefaultWeights
	// A real attack candidate that didn't complete a continent -- Explanation
	// omits "completes_continent" entirely, matching how strategy_scored.go
	// only appends it when the condition is true.
	e := entry("attack", "p0",
		bot.Feature{Name: "army_advantage", Value: w.ArmyAdvantage * 2},
		bot.Feature{Name: "capture_probability", Value: w.CaptureProbability * 0.5},
	)
	rows := rowsFromEntries(1, []simulation.Entry{e}, "p0")
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	for _, name := range []string{"completes_continent", "breaks_enemy_continent", "eliminates_player", "card_opportunity", "expected_loss_cost", "exposure_penalty"} {
		v, ok := rows[0].Features[name]
		if !ok {
			t.Errorf("expected %q present (defaulted) in the row's Features map, key missing entirely", name)
		}
		if v != 0 {
			t.Errorf("expected %q to default to 0 when absent from the candidate, got %v", name, v)
		}
	}
}

func TestRowsFromEntriesContinentValueUsesCorrectWeightPerPhase(t *testing.T) {
	w := bot.DefaultWeights
	reinforceE := entry("reinforce", "p0", bot.Feature{Name: "continent_value", Value: w.ReinforceContinentValue * 3})
	fortifyE := entry("fortify", "p0", bot.Feature{Name: "continent_value", Value: w.FortifyContinentValue * 3})

	rows := rowsFromEntries(1, []simulation.Entry{reinforceE, fortifyE}, "p0")
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if got := rows[0].Features["continent_value"]; got < 2.999 || got > 3.001 {
		t.Errorf("reinforce continent_value raw = %v, want 3 (divided by ReinforceContinentValue)", got)
	}
	if got := rows[1].Features["continent_value"]; got < 2.999 || got > 3.001 {
		t.Errorf("fortify continent_value raw = %v, want 3 (divided by FortifyContinentValue)", got)
	}
}

func TestRowsFromEntriesSkipsEndPhaseAndEndTurnOnlyEntries(t *testing.T) {
	attackEnd := entry("attack", "p0", bot.Feature{Name: "end_phase_bias", Value: 0})
	fortifyEnd := entry("fortify", "p0", bot.Feature{Name: "end_turn_bias", Value: 0})

	rows := rowsFromEntries(1, []simulation.Entry{attackEnd, fortifyEnd}, "p0")
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows for end-phase/end-turn-only entries, got %d: %+v", len(rows), rows)
	}
}

func TestRowsFromEntriesSkipsBasicV1EmptyExplanation(t *testing.T) {
	e := simulation.Entry{
		PlayerID:    "p0",
		StrategyID:  bot.StrategyBasicV1,
		Phase:       "attack",
		Explanation: bot.Explanation{}, // basic-v1 always returns the zero value
	}
	rows := rowsFromEntries(1, []simulation.Entry{e}, "p0")
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows for an empty Explanation, got %d", len(rows))
	}
}

func TestRowsFromEntriesSkipsCardTurnInEntries(t *testing.T) {
	// strategy_scored_cards.go records Explanation{Score: 1, Features:
	// []Feature{{Name: reason, Value: 1}}} -- an arbitrary reason string,
	// never one of phaseFeatures' known names.
	e := entry("reinforce", "p0", bot.Feature{Name: "mandatory_trade", Value: 1})
	rows := rowsFromEntries(1, []simulation.Entry{e}, "p0")
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows for a card-trade-in-shaped entry, got %d", len(rows))
	}
}

func TestRowsFromEntriesSkipsIncompleteGames(t *testing.T) {
	w := bot.DefaultWeights
	e := entry("attack", "p0", bot.Feature{Name: "army_advantage", Value: w.ArmyAdvantage})
	rows := rowsFromEntries(1, []simulation.Entry{e}, "") // no winner -- game didn't complete
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows when winnerPlayerID is empty, got %d", len(rows))
	}
}

func TestRowsFromEntriesLabelsWinLoss(t *testing.T) {
	w := bot.DefaultWeights
	winner := entry("attack", "p0", bot.Feature{Name: "army_advantage", Value: w.ArmyAdvantage})
	loser := entry("attack", "p1", bot.Feature{Name: "army_advantage", Value: w.ArmyAdvantage})

	rows := rowsFromEntries(7, []simulation.Entry{winner, loser}, "p0")
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if !rows[0].Won {
		t.Errorf("expected p0's row to be marked Won")
	}
	if rows[1].Won {
		t.Errorf("expected p1's row to be marked not Won")
	}
	if rows[0].Seed != 7 || rows[1].Seed != 7 {
		t.Errorf("expected Seed to be carried through onto every row")
	}
}
