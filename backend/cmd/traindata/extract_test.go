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

func exploredEntry(phase, playerID string, features ...bot.Feature) simulation.Entry {
	return simulation.Entry{
		CommandIndex: 5,
		Turn:         3,
		Seat:         1,
		PlayerID:     playerID,
		StrategyID:   bot.StrategyScoredV1Exploring,
		Phase:        phase,
		Explanation:  bot.Explanation{Features: features, Explored: true},
	}
}

// entryWithCandidates builds an entry as bot.NewExploringScoredStrategy's
// recordCandidates=true path would -- Explanation.AllCandidates populated
// with every legal candidate, chosen marking which one was actually
// picked (its Features also mirrored onto Explanation.Features, matching
// what explanationFor always does regardless of recording).
func entryWithCandidates(phase, playerID string, chosen int, candidates ...[]bot.Feature) simulation.Entry {
	all := make([]bot.Candidate, len(candidates))
	for i, features := range candidates {
		all[i] = bot.Candidate{Features: features, Chosen: i == chosen}
	}
	return simulation.Entry{
		CommandIndex: 5,
		Turn:         3,
		Seat:         1,
		PlayerID:     playerID,
		StrategyID:   bot.StrategyScoredV1Exploring,
		Phase:        phase,
		Explanation:  bot.Explanation{Features: candidates[chosen], AllCandidates: all},
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
			rows := rowsFromEntries(1, "test-game", []simulation.Entry{entry(tc.phase, "p0", tc.feature)}, "p0")
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
	rows := rowsFromEntries(1, "test-game", []simulation.Entry{e}, "p0")
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

	rows := rowsFromEntries(1, "test-game", []simulation.Entry{reinforceE, fortifyE}, "p0")
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

	rows := rowsFromEntries(1, "test-game", []simulation.Entry{attackEnd, fortifyEnd}, "p0")
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
	rows := rowsFromEntries(1, "test-game", []simulation.Entry{e}, "p0")
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows for an empty Explanation, got %d", len(rows))
	}
}

func TestRowsFromEntriesSkipsCardTurnInEntries(t *testing.T) {
	// strategy_scored_cards.go records Explanation{Score: 1, Features:
	// []Feature{{Name: reason, Value: 1}}} -- an arbitrary reason string,
	// never one of phaseFeatures' known names.
	e := entry("reinforce", "p0", bot.Feature{Name: "mandatory_trade", Value: 1})
	rows := rowsFromEntries(1, "test-game", []simulation.Entry{e}, "p0")
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows for a card-trade-in-shaped entry, got %d", len(rows))
	}
}

func TestRowsFromEntriesSkipsIncompleteGames(t *testing.T) {
	w := bot.DefaultWeights
	e := entry("attack", "p0", bot.Feature{Name: "army_advantage", Value: w.ArmyAdvantage})
	rows := rowsFromEntries(1, "test-game", []simulation.Entry{e}, "") // no winner -- game didn't complete
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows when winnerPlayerID is empty, got %d", len(rows))
	}
}

func TestRowsFromEntriesLabelsWinLoss(t *testing.T) {
	w := bot.DefaultWeights
	winner := entry("attack", "p0", bot.Feature{Name: "army_advantage", Value: w.ArmyAdvantage})
	loser := entry("attack", "p1", bot.Feature{Name: "army_advantage", Value: w.ArmyAdvantage})

	rows := rowsFromEntries(7, "test-game-7", []simulation.Entry{winner, loser}, "p0")
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
	if rows[0].GameID != "test-game-7" || rows[1].GameID != "test-game-7" {
		t.Errorf("expected GameID to be carried through onto every row, got %q and %q", rows[0].GameID, rows[1].GameID)
	}
}

func TestRowsFromEntriesPropagatesExplored(t *testing.T) {
	w := bot.DefaultWeights
	explored := exploredEntry("attack", "p0", bot.Feature{Name: "army_advantage", Value: w.ArmyAdvantage * 4})
	notExplored := entry("attack", "p1", bot.Feature{Name: "army_advantage", Value: w.ArmyAdvantage * 4})

	rows := rowsFromEntries(1, "test-game", []simulation.Entry{explored, notExplored}, "p0")
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if !rows[0].Explored {
		t.Errorf("expected the exploring strategy's row to have Explored == true")
	}
	if rows[1].Explored {
		t.Errorf("expected the plain-scored strategy's row to have Explored == false")
	}
}

func TestRowsFromEntriesEmitsOneRowPerCandidateWhenAllCandidatesPopulated(t *testing.T) {
	w := bot.DefaultWeights
	e := entryWithCandidates("reinforce", "p0", 1,
		[]bot.Feature{{Name: "enemy_threat", Value: w.ReinforceEnemyThreat * 2}},
		[]bot.Feature{{Name: "enemy_threat", Value: w.ReinforceEnemyThreat * 9}},
		[]bot.Feature{{Name: "enemy_threat", Value: w.ReinforceEnemyThreat * 5}},
	)
	rows := rowsFromEntries(1, "test-game", []simulation.Entry{e}, "p0")
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows (one per candidate), got %d: %+v", len(rows), rows)
	}

	wantRaw := map[int]float64{0: 2, 1: 9, 2: 5}
	chosenCount := 0
	for i, r := range rows {
		if got := r.Features["enemy_threat"]; got < wantRaw[i]-1e-9 || got > wantRaw[i]+1e-9 {
			t.Errorf("candidate %d: enemy_threat raw = %v, want %v", i, got, wantRaw[i])
		}
		if r.CommandIndex != e.CommandIndex || r.GameID != "test-game" || r.PlayerID != "p0" {
			t.Errorf("candidate %d: expected shared decision metadata carried through, got %+v", i, r)
		}
		if r.Chosen {
			chosenCount++
			if i != 1 {
				t.Errorf("expected only candidate index 1 to be marked Chosen, got it at index %d", i)
			}
		}
	}
	if chosenCount != 1 {
		t.Errorf("expected exactly 1 row marked Chosen, got %d", chosenCount)
	}
}

func TestRowsFromEntriesSkipsUnrecognizedCandidatesButKeepsOthers(t *testing.T) {
	w := bot.DefaultWeights
	e := entryWithCandidates("attack", "p0", 0,
		[]bot.Feature{{Name: "army_advantage", Value: w.ArmyAdvantage * 3}},
		[]bot.Feature{{Name: "end_phase_bias", Value: 0}}, // the only-excluded-feature case, like the real end_attack sentinel
	)
	rows := rowsFromEntries(1, "test-game", []simulation.Entry{e}, "p0")
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (the end_phase_bias-only candidate skipped), got %d: %+v", len(rows), rows)
	}
	if !rows[0].Chosen {
		t.Errorf("expected the surviving row to be the chosen candidate")
	}
}

func TestRowsFromEntriesChosenOnlyFallbackWithoutAllCandidates(t *testing.T) {
	w := bot.DefaultWeights
	e := entry("attack", "p0", bot.Feature{Name: "army_advantage", Value: w.ArmyAdvantage})
	rows := rowsFromEntries(1, "test-game", []simulation.Entry{e}, "p0")
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if !rows[0].Chosen {
		t.Errorf("expected the sole row from a non-recording entry to be marked Chosen")
	}
}

func TestComputeGameIDDeterministicAndCollisionFree(t *testing.T) {
	a := computeGameID([]string{"basic-v1", "scored-v1"}, "auto_start", 1)
	aAgain := computeGameID([]string{"basic-v1", "scored-v1"}, "auto_start", 1)
	if a != aAgain {
		t.Errorf("expected the same config+seed to always produce the same GameID, got %q vs %q", a, aAgain)
	}

	differentStrategies := computeGameID([]string{"scored-v1", "scored-v1"}, "auto_start", 1)
	differentMode := computeGameID([]string{"basic-v1", "scored-v1"}, "manual", 1)
	differentSeed := computeGameID([]string{"basic-v1", "scored-v1"}, "auto_start", 2)

	for _, other := range []string{differentStrategies, differentMode, differentSeed} {
		if a == other {
			t.Errorf("expected a different strategies/game-mode/seed to produce a different GameID, both were %q", a)
		}
	}
}
