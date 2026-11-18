package service

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
	"github.com/nmiano1111/global-conquest/backend/internal/store"
)

// testBoard: A-B-C in a line; continent x = {A, B}, continent y = {C}.
func testBoard() risk.Board {
	adj := map[risk.Territory]map[risk.Territory]struct{}{
		"A": {"B": {}},
		"B": {"A": {}, "C": {}},
		"C": {"B": {}},
	}
	return risk.Board{
		Continents: map[risk.Continent]risk.ContinentInfo{
			"x": {Bonus: 2, Territories: []risk.Territory{"A", "B"}},
			"y": {Bonus: 1, Territories: []risk.Territory{"C"}},
		},
		Adjacent: adj,
		Order:    []risk.Territory{"A", "B", "C"},
	}
}

type terr = map[string]risk.TerritoryState

func replayEvent(t *testing.T, seq int64, action, phase string, territories terr, extra map[string]any) store.GameReplayEvent {
	t.Helper()
	payload := map[string]any{
		"phase":       phase,
		"players":     []map[string]any{{"user_id": "p0"}, {"user_id": "p1"}},
		"territories": territories,
	}
	for k, v := range extra {
		payload[k] = v
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return store.GameReplayEvent{GameSequence: seq, ActorPlayerID: "p0", ActionType: action, Payload: b}
}

func attackExtra(from, to string, att, def []int, attLoss, defLoss int, conquered bool) map[string]any {
	return map[string]any{
		"action_from": from, "action_to": to,
		"result": map[string]any{
			"attacker_rolls": att, "defender_rolls": def,
			"attacker_loss": attLoss, "defender_loss": defLoss, "conquered": conquered,
		},
	}
}

func testDecisionEvents(t *testing.T) []store.GameReplayEvent {
	return []store.GameReplayEvent{
		replayEvent(t, 1, "place_reinforcement", "attack", terr{"A": {Owner: 0, Armies: 5}, "B": {Owner: 1, Armies: 2}, "C": {Owner: 1, Armies: 1}}, nil),
		replayEvent(t, 2, "attack", "attack", terr{"A": {Owner: 0, Armies: 5}, "B": {Owner: 1, Armies: 1}, "C": {Owner: 1, Armies: 1}},
			attackExtra("A", "B", []int{6, 3, 2}, []int{4, 3}, 1, 1, false)),
		replayEvent(t, 3, "attack", "occupy", terr{"A": {Owner: 0, Armies: 5}, "B": {Owner: 0, Armies: 0}, "C": {Owner: 1, Armies: 1}},
			attackExtra("A", "B", []int{6, 3, 2}, []int{1}, 0, 1, true)),
		replayEvent(t, 4, "occupy", "attack", terr{"A": {Owner: 0, Armies: 2}, "B": {Owner: 0, Armies: 3}, "C": {Owner: 1, Armies: 1}}, nil),
		replayEvent(t, 5, "end_attack", "fortify", terr{"A": {Owner: 0, Armies: 2}, "B": {Owner: 0, Armies: 3}, "C": {Owner: 1, Armies: 1}}, nil),
	}
}

func TestExtractAttackDecisions(t *testing.T) {
	rows := extractAttackDecisions(testBoard(), map[int]string{0: "alice", 1: "bob"}, testDecisionEvents(t))
	if len(rows) != 3 {
		t.Fatalf("got %d decisions, want 3", len(rows))
	}

	first := rows[0]
	if first.Kind != "attack" || *first.Continued || first.PlayerKey != "alice" || first.Seat != 0 {
		t.Fatalf("first decision = %+v", first)
	}
	if *first.AttackerArmies != 5 || *first.DefenderArmies != 2 || *first.DefenderSeat != 1 {
		t.Errorf("armies = %d vs %d (def seat %d), want 5 vs 2 (seat 1)", *first.AttackerArmies, *first.DefenderArmies, *first.DefenderSeat)
	}
	if want := risk.ForecastAttack(5, 2).WinProbability; math.Abs(*first.WinProb-want) > 1e-9 {
		t.Errorf("win prob = %v, want %v", *first.WinProb, want)
	}
	// Only A->B is legal (C is not adjacent to A); taking B completes x for
	// alice, and bob is the only (so both strongest and weakest) opponent.
	if first.Options != 1 || !*first.CompletesContinent || *first.BreaksContinent || !*first.TargetsLeader || !*first.TargetsWeakest {
		t.Errorf("features = %+v", first)
	}
	if first.OptionsCompletesContinent != 1 || first.OptionsBreaksContinent != 0 {
		t.Errorf("option counts = %+v", first)
	}
	if *first.AttackerDice != 3 || *first.DefenderDice != 2 || *first.ExpectedDefenderLoss <= 0 {
		t.Errorf("dice = %d v %d, expected def loss %v", *first.AttackerDice, *first.DefenderDice, *first.ExpectedDefenderLoss)
	}

	second := rows[1]
	if second.Kind != "attack" || !*second.Continued || !*second.Captured || *second.DefenderArmies != 1 {
		t.Errorf("second decision = %+v", second)
	}

	stop := rows[2]
	if stop.Kind != "stop" || stop.Options != 1 || stop.WinProb != nil {
		t.Fatalf("stop decision = %+v", stop)
	}
	// From B (3 armies) into C (1, bob's whole continent y).
	if want := risk.ForecastAttack(3, 1).WinProbability; math.Abs(*stop.BestWinProb-want) > 1e-9 || stop.OptionsBreaksContinent != 1 {
		t.Errorf("stop best = %v breaks = %d", *stop.BestWinProb, stop.OptionsBreaksContinent)
	}
}

func TestExpectedRollLosses(t *testing.T) {
	// Standard one-roll tables: 1v1 the attacker wins 15/36; 3v2 the
	// defender loses 2 with p 2890/7776 and 1 with p 2611/7776, so expects
	// (2*2890 + 2611)/7776 ≈ 1.0791 losses.
	if got := expectedRollLosses[1][1][1]; math.Abs(got-15.0/36) > 1e-9 {
		t.Errorf("1v1 expected defender loss = %v, want %v", got, 15.0/36)
	}
	e := expectedRollLosses[3][2]
	if math.Abs(e[0]+e[1]-2) > 1e-9 || math.Abs(e[1]-(2.0*2890+2611)/7776) > 1e-9 {
		t.Errorf("3v2 expected losses = %v", e)
	}
}

func TestBuildDecisionStats(t *testing.T) {
	decisions := extractAttackDecisions(testBoard(), map[int]string{0: "alice"}, testDecisionEvents(t))
	when := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var rows []store.AttackDecisionRow
	for _, d := range decisions {
		rows = append(rows, store.AttackDecisionRow{AttackDecision: d, GameID: "g1", CompletedAt: when})
	}
	s := buildDecisionStats(rows)
	if s.Games != 1 || s.Rolls != 2 || s.Attacks != 1 || s.Stops != 1 {
		t.Fatalf("counts = games %d rolls %d attacks %d stops %d", s.Games, s.Rolls, s.Attacks, s.Stops)
	}
	if *s.AvgRollsPerAttack != 2 || *s.AbandonedPct != 0 || *s.MaxDicePct != 100 {
		t.Errorf("rolls/attack %v abandoned %v max dice %v", *s.AvgRollsPerAttack, *s.AbandonedPct, *s.MaxDicePct)
	}
	b := s.OddsBuckets[winProbBucket(risk.ForecastAttack(5, 2).WinProbability)]
	if b.Attacks != 1 || b.ChosenPct != 100 || b.AvailablePct != 100 || *b.Preference != 1 || *b.CapturePct != 100 {
		t.Errorf("odds bucket = %+v", b)
	}
	if f := s.Features[0]; f.Key != "completes_continent" || f.Chosen != 1 || f.ChosenPct != 100 {
		t.Errorf("completes feature = %+v", f)
	}
	// Stopped with a ~75%+ attack (3 vs 1) still available.
	best := risk.ForecastAttack(3, 1).WinProbability
	if s.StopBuckets[winProbBucket(best)+1].Stops != 1 {
		t.Errorf("stop buckets = %+v (best %v)", s.StopBuckets, best)
	}
	if len(s.Series) != 1 || s.Series[0].Attacks != 1 || s.Series[0].CumKillLuckPct == nil {
		t.Errorf("series = %+v", s.Series)
	}
}

func TestBuildDecisionStatsEmpty(t *testing.T) {
	s := buildDecisionStats(nil)
	if s.Games != 0 || s.AvgWinProb != nil || len(s.OddsBuckets) != numWinProbBuckets || s.Series == nil {
		t.Errorf("empty stats = %+v", s)
	}
}
