package reporting

import (
	"strings"
	"testing"
	"time"
)

// --- test helpers ---

func mkRoll(seq int64, attackerID, defenderID string, attackerLosses, defenderLosses int) CombatEvent {
	return CombatEvent{
		ID:                "ev-" + itoa(seq),
		GameID:            "g1",
		GameSequence:      seq,
		OccurredAt:        time.Unix(seq, 0).UTC(),
		AttackerPlayerID:  attackerID,
		DefenderPlayerID:  defenderID,
		SourceTerritoryID: "alaska",
		TargetTerritoryID: "kamchatka",
		AttackerDice:      []int{6},
		DefenderDice:      []int{1},
		AttackerLosses:    attackerLosses,
		DefenderLosses:    defenderLosses,
		TerritoryCaptured: false,
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func defaultThresholds() StreakThresholds { return DefaultStreakThresholds() }

// --- Classification ---

func TestClassifyRoll_AttackerWin(t *testing.T) {
	ev := mkRoll(1, "a", "b", 0, 1)
	if got := classifyRoll(ev); got != RollAttackerWin {
		t.Errorf("want RollAttackerWin, got %v", got)
	}
}

func TestClassifyRoll_AttackerLoss(t *testing.T) {
	ev := mkRoll(1, "a", "b", 1, 0)
	if got := classifyRoll(ev); got != RollAttackerLoss {
		t.Errorf("want RollAttackerLoss, got %v", got)
	}
}

func TestClassifyRoll_Split(t *testing.T) {
	ev := mkRoll(1, "a", "b", 1, 1)
	if got := classifyRoll(ev); got != RollSplit {
		t.Errorf("want RollSplit, got %v", got)
	}
}

// --- Streak detection ---

func TestBuildRollStreakReport_TwoConsecutiveLossesCreateStreak(t *testing.T) {
	events := []CombatEvent{
		mkRoll(1, "a", "b", 1, 0),
		mkRoll(2, "a", "b", 1, 0),
	}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, defaultThresholds())
	if len(r.AttackingLossStreaks) != 1 {
		t.Fatalf("want 1 loss streak, got %d", len(r.AttackingLossStreaks))
	}
	if r.AttackingLossStreaks[0].Length != 2 {
		t.Errorf("want length 2, got %d", r.AttackingLossStreaks[0].Length)
	}
}

func TestBuildRollStreakReport_SingleLossBelowThreshold(t *testing.T) {
	events := []CombatEvent{
		mkRoll(1, "a", "b", 1, 0),
		mkRoll(2, "a", "b", 0, 1), // win breaks it
	}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, defaultThresholds())
	if len(r.AttackingLossStreaks) != 0 {
		t.Errorf("single loss should not pass min length 2, got %d streaks", len(r.AttackingLossStreaks))
	}
}

func TestBuildRollStreakReport_SplitBreaksStrictLossStreak(t *testing.T) {
	events := []CombatEvent{
		mkRoll(1, "a", "b", 1, 0),
		mkRoll(2, "a", "b", 1, 1), // split
		mkRoll(3, "a", "b", 1, 0),
	}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, defaultThresholds())
	if len(r.AttackingLossStreaks) != 0 {
		t.Errorf("split should break a strict loss streak, got %d streaks", len(r.AttackingLossStreaks))
	}
}

func TestBuildRollStreakReport_SplitContributesToDrought(t *testing.T) {
	events := []CombatEvent{
		mkRoll(1, "a", "b", 1, 0), // loss
		mkRoll(2, "a", "b", 1, 1), // split
		mkRoll(3, "a", "b", 1, 0), // loss
	}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, defaultThresholds())
	if len(r.AttackDroughts) != 1 {
		t.Fatalf("want 1 drought spanning loss+split+loss, got %d", len(r.AttackDroughts))
	}
	if r.AttackDroughts[0].Length != 3 {
		t.Errorf("want drought length 3, got %d", r.AttackDroughts[0].Length)
	}
}

func TestBuildRollStreakReport_AttackerWinBreaksDrought(t *testing.T) {
	events := []CombatEvent{
		mkRoll(1, "a", "b", 1, 0), // loss
		mkRoll(2, "a", "b", 1, 1), // split
		mkRoll(3, "a", "b", 0, 1), // win — breaks drought
		mkRoll(4, "a", "b", 1, 0), // loss
		mkRoll(5, "a", "b", 1, 1), // split
	}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, StreakThresholds{MinLossStreakLength: 2, MinWinStreakLength: 2, MinDroughtLength: 2})
	if len(r.AttackDroughts) != 2 {
		t.Fatalf("want 2 separate droughts split by the win, got %d", len(r.AttackDroughts))
	}
	for _, d := range r.AttackDroughts {
		if d.Length != 2 {
			t.Errorf("want each drought length 2, got %d", d.Length)
		}
	}
}

func TestBuildRollStreakReport_TwoConsecutiveWinsCreateStreak(t *testing.T) {
	events := []CombatEvent{
		mkRoll(1, "a", "b", 0, 1),
		mkRoll(2, "a", "b", 0, 1),
	}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, defaultThresholds())
	if len(r.AttackingWinStreaks) != 1 {
		t.Fatalf("want 1 win streak, got %d", len(r.AttackingWinStreaks))
	}
	if r.AttackingWinStreaks[0].Length != 2 {
		t.Errorf("want length 2, got %d", r.AttackingWinStreaks[0].Length)
	}
}

// A streak is scoped per attacker: two different attackers' losses at
// interleaved sequences must not merge into one streak or get attributed to
// the wrong attacker — each attacker's own consecutive rolls form their own streak.
func TestBuildRollStreakReport_StreaksDoNotCrossAttackerBoundaries(t *testing.T) {
	events := []CombatEvent{
		mkRoll(1, "a", "x", 1, 0),
		mkRoll(2, "b", "x", 1, 0),
		mkRoll(3, "a", "x", 1, 0),
		mkRoll(4, "b", "x", 1, 0),
	}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, defaultThresholds())
	if len(r.AttackingLossStreaks) != 2 {
		t.Fatalf("want 2 loss streaks (one per attacker), got %d", len(r.AttackingLossStreaks))
	}
	seenAttackers := map[string]bool{}
	for _, s := range r.AttackingLossStreaks {
		if s.Length != 2 {
			t.Errorf("want each attacker's streak length 2, got %d for %s", s.Length, s.AttackerID)
		}
		seenAttackers[s.AttackerID] = true
	}
	if !seenAttackers["a"] || !seenAttackers["b"] {
		t.Errorf("expected streaks attributed to both a and b, got %v", seenAttackers)
	}
}

func TestBuildRollStreakReport_StreaksDoNotCrossGameBoundaries(t *testing.T) {
	events := []CombatEvent{
		mkRoll(1, "a", "b", 1, 0),
		{GameID: "OTHER_GAME", GameSequence: 2, AttackerPlayerID: "a", DefenderPlayerID: "b", AttackerLosses: 1, DefenderLosses: 0, OccurredAt: time.Unix(2, 0)},
		mkRoll(3, "a", "b", 1, 0),
	}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, defaultThresholds())
	if len(r.AttackingLossStreaks) != 1 {
		t.Fatalf("want 1 loss streak built from g1's rolls only, got %d", len(r.AttackingLossStreaks))
	}
	s := r.AttackingLossStreaks[0]
	if s.Length != 2 {
		t.Errorf("want length 2 (the other game's roll must not extend it), got %d", s.Length)
	}
	for _, roll := range s.Rolls {
		if roll.GameSequence == 2 {
			t.Error("streak must not include the other game's event")
		}
	}
	if r.SummaryByAttacker[0].AttackRollsCaptured != 2 {
		t.Errorf("expected 2 rolls captured (g1 only, other game excluded), got %d", r.SummaryByAttacker[0].AttackRollsCaptured)
	}
}

func TestBuildRollStreakReport_StreaksOrderedByEventSequence(t *testing.T) {
	// Events supplied out of order; report must still detect the streak.
	events := []CombatEvent{
		mkRoll(3, "a", "b", 1, 0),
		mkRoll(1, "a", "b", 1, 0),
		mkRoll(2, "a", "b", 1, 0),
	}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, defaultThresholds())
	if len(r.AttackingLossStreaks) != 1 || r.AttackingLossStreaks[0].Length != 3 {
		t.Fatalf("expected a single length-3 streak after sorting, got %+v", r.AttackingLossStreaks)
	}
	s := r.AttackingLossStreaks[0]
	if s.StartSeq != 1 || s.EndSeq != 3 {
		t.Errorf("want start/end seq 1/3, got %d/%d", s.StartSeq, s.EndSeq)
	}
	for idx, roll := range s.Rolls {
		if roll.GameSequence != int64(idx+1) {
			t.Errorf("roll %d out of order: seq=%d", idx, roll.GameSequence)
		}
	}
}

func TestBuildRollStreakReport_EndOfInputStreakEmitted(t *testing.T) {
	// The run extends all the way to the last event; flush() must still catch it.
	events := []CombatEvent{
		mkRoll(1, "a", "b", 0, 1),
		mkRoll(2, "a", "b", 1, 0),
		mkRoll(3, "a", "b", 1, 0),
	}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, defaultThresholds())
	if len(r.AttackingLossStreaks) != 1 {
		t.Fatalf("want 1 loss streak at end of input, got %d", len(r.AttackingLossStreaks))
	}
	if r.AttackingLossStreaks[0].EndSeq != 3 {
		t.Errorf("want streak to end at seq 3, got %d", r.AttackingLossStreaks[0].EndSeq)
	}
}

// --- Summary aggregation ---

func TestBuildRollStreakReport_CountsStreaksPerPlayer(t *testing.T) {
	events := []CombatEvent{
		mkRoll(1, "a", "b", 1, 0),
		mkRoll(2, "a", "b", 1, 0),
		mkRoll(3, "a", "b", 0, 1),
		mkRoll(4, "a", "b", 1, 0),
		mkRoll(5, "a", "b", 1, 0),
	}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, defaultThresholds())
	if len(r.SummaryByAttacker) != 1 {
		t.Fatalf("want 1 attacker summary, got %d", len(r.SummaryByAttacker))
	}
	s := r.SummaryByAttacker[0]
	if s.LossStreakCount2Plus != 2 {
		t.Errorf("want 2 loss streaks, got %d", s.LossStreakCount2Plus)
	}
}

func TestBuildRollStreakReport_LongestStreakIDs(t *testing.T) {
	events := []CombatEvent{
		mkRoll(1, "a", "b", 1, 0),
		mkRoll(2, "a", "b", 1, 0),
		mkRoll(3, "a", "b", 1, 0),
		mkRoll(4, "a", "b", 0, 1),
		mkRoll(5, "a", "b", 1, 0),
		mkRoll(6, "a", "b", 1, 0),
	}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, defaultThresholds())
	s := r.SummaryByAttacker[0]
	wantID := "g1:a:attacking_loss:1-3"
	if s.LongestLossStreakID != wantID {
		t.Errorf("want longest streak id %q, got %q", wantID, s.LongestLossStreakID)
	}
	if s.LongestLossStreak != 3 {
		t.Errorf("want longest loss streak 3, got %d", s.LongestLossStreak)
	}
}

func TestBuildRollStreakReport_WLSTotals(t *testing.T) {
	events := []CombatEvent{
		mkRoll(1, "a", "b", 0, 1), // win
		mkRoll(2, "a", "b", 1, 0), // loss
		mkRoll(3, "a", "b", 1, 1), // split
	}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, defaultThresholds())
	s := r.SummaryByAttacker[0]
	if s.AttackerWinCount != 1 || s.AttackerLossCount != 1 || s.SplitCount != 1 {
		t.Errorf("want W/L/S 1/1/1, got %d/%d/%d", s.AttackerWinCount, s.AttackerLossCount, s.SplitCount)
	}
}

func TestBuildRollStreakReport_Per20AttackRates(t *testing.T) {
	events := make([]CombatEvent, 0, 20)
	for i := int64(1); i <= 20; i++ {
		// Two 2-loss streaks among the 20 rolls: seqs 1-2 and 5-6.
		if i == 1 || i == 2 || i == 5 || i == 6 {
			events = append(events, mkRoll(i, "a", "b", 1, 0))
		} else {
			events = append(events, mkRoll(i, "a", "b", 0, 1))
		}
	}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, defaultThresholds())
	s := r.SummaryByAttacker[0]
	if s.AttackRollsCaptured != 20 {
		t.Fatalf("want 20 rolls captured, got %d", s.AttackRollsCaptured)
	}
	if s.LossStreaksPer20Attacks != 2.0 {
		t.Errorf("want 2.0 loss streaks per 20, got %.2f", s.LossStreaksPer20Attacks)
	}
}

func TestBuildRollStreakReport_ZeroRollsRateIsZero(t *testing.T) {
	r := BuildRollStreakReport("g1", "Test Game", false, nil, 0, nil, defaultThresholds())
	if len(r.SummaryByAttacker) != 0 {
		t.Fatalf("want no summaries for empty input, got %d", len(r.SummaryByAttacker))
	}
}

func TestBuildRollStreakReport_PlayerWithZeroStreaks(t *testing.T) {
	events := []CombatEvent{
		mkRoll(1, "a", "b", 0, 1),
		mkRoll(2, "a", "b", 0, 1),
		mkRoll(3, "a", "b", 0, 1),
	}
	// Only win streaks exist; loss/drought counters must be zero, not crash.
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, defaultThresholds())
	s := r.SummaryByAttacker[0]
	if s.LossStreakCount2Plus != 0 || s.AttackDroughtCount3Plus != 0 {
		t.Errorf("expected zero loss/drought streaks, got loss=%d drought=%d", s.LossStreakCount2Plus, s.AttackDroughtCount3Plus)
	}
	if s.LongestLossStreakID != "" {
		t.Errorf("expected empty longest-loss-streak id, got %q", s.LongestLossStreakID)
	}
}

func TestBuildRollStreakReport_MissingPlayerNamesFallBackToID(t *testing.T) {
	events := []CombatEvent{
		mkRoll(1, "12345678-aaaa-bbbb-cccc-dddddddddddd", "b", 1, 0),
		mkRoll(2, "12345678-aaaa-bbbb-cccc-dddddddddddd", "b", 1, 0),
	}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, defaultThresholds())
	if r.SummaryByAttacker[0].PlayerName != "12345678" {
		t.Errorf("want fallback to UUID prefix, got %q", r.SummaryByAttacker[0].PlayerName)
	}
}

// --- Individual streak details ---

func TestBuildStreak_DefendersInvolvedCollected(t *testing.T) {
	events := []CombatEvent{
		mkRoll(1, "a", "b", 1, 0),
		mkRoll(2, "a", "c", 1, 0),
		mkRoll(3, "a", "b", 1, 0),
	}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, map[string]string{"b": "Bob", "c": "Carol"}, defaultThresholds())
	got := r.AttackingLossStreaks[0].DefendersInvolved
	if len(got) != 2 || got[0] != "Bob" || got[1] != "Carol" {
		t.Errorf("want [Bob Carol] in first-seen order, got %v", got)
	}
}

func TestBuildStreak_TerritoriesCollected(t *testing.T) {
	events := []CombatEvent{
		{GameID: "g1", GameSequence: 1, AttackerPlayerID: "a", DefenderPlayerID: "b", SourceTerritoryID: "ukraine", TargetTerritoryID: "afghanistan", AttackerLosses: 1, DefenderLosses: 0, OccurredAt: time.Unix(1, 0)},
		{GameID: "g1", GameSequence: 2, AttackerPlayerID: "a", DefenderPlayerID: "b", SourceTerritoryID: "middle_east", TargetTerritoryID: "india", AttackerLosses: 1, DefenderLosses: 0, OccurredAt: time.Unix(2, 0)},
	}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, defaultThresholds())
	s := r.AttackingLossStreaks[0]
	if len(s.AttackerTerritories) != 2 || s.AttackerTerritories[0] != "ukraine" || s.AttackerTerritories[1] != "middle_east" {
		t.Errorf("unexpected attacker territories: %v", s.AttackerTerritories)
	}
	if len(s.DefenderTerritories) != 2 || s.DefenderTerritories[0] != "afghanistan" || s.DefenderTerritories[1] != "india" {
		t.Errorf("unexpected defender territories: %v", s.DefenderTerritories)
	}
}

func TestBuildStreak_ArmyLossSumsAndNetDelta(t *testing.T) {
	events := []CombatEvent{
		mkRoll(1, "a", "b", 2, 0),
		mkRoll(2, "a", "b", 1, 1),
	}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, StreakThresholds{MinLossStreakLength: 99, MinWinStreakLength: 99, MinDroughtLength: 2})
	s := r.AttackDroughts[0]
	if s.AttackerArmiesLost != 3 {
		t.Errorf("want attacker armies lost 3, got %d", s.AttackerArmiesLost)
	}
	if s.DefenderArmiesLost != 1 {
		t.Errorf("want defender armies lost 1, got %d", s.DefenderArmiesLost)
	}
	if s.NetArmyDeltaForAttacker != -2 {
		t.Errorf("want net delta -2 (1-3), got %d", s.NetArmyDeltaForAttacker)
	}
}

func TestBuildStreak_CapturesCounted(t *testing.T) {
	events := []CombatEvent{
		{GameID: "g1", GameSequence: 1, AttackerPlayerID: "a", DefenderPlayerID: "b", AttackerLosses: 0, DefenderLosses: 1, TerritoryCaptured: true, OccurredAt: time.Unix(1, 0)},
		{GameID: "g1", GameSequence: 2, AttackerPlayerID: "a", DefenderPlayerID: "b", AttackerLosses: 0, DefenderLosses: 1, TerritoryCaptured: false, OccurredAt: time.Unix(2, 0)},
	}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, defaultThresholds())
	if r.AttackingWinStreaks[0].CapturesDuringStreak != 1 {
		t.Errorf("want 1 capture, got %d", r.AttackingWinStreaks[0].CapturesDuringStreak)
	}
}

func TestBuildStreak_RollTraceRendered(t *testing.T) {
	events := []CombatEvent{
		mkRoll(1, "a", "b", 2, 0),
		mkRoll(2, "a", "b", 1, 0),
		mkRoll(3, "a", "b", 1, 1),
	}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, StreakThresholds{MinLossStreakLength: 99, MinWinStreakLength: 99, MinDroughtLength: 3})
	want := "2-0, 1-0, 1-1"
	if r.AttackDroughts[0].RollTrace != want {
		t.Errorf("want roll trace %q, got %q", want, r.AttackDroughts[0].RollTrace)
	}
}

func TestBuildStreak_ID_Format(t *testing.T) {
	events := []CombatEvent{
		mkRoll(144, "tucker", "nick", 1, 0),
		mkRoll(145, "tucker", "nick", 1, 0),
	}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, defaultThresholds())
	want := "g1:tucker:attacking_loss:144-145"
	if r.AttackingLossStreaks[0].ID != want {
		t.Errorf("want streak id %q, got %q", want, r.AttackingLossStreaks[0].ID)
	}
}

// --- Partial history ---

func TestBuildRollStreakReport_PartialHistoryWarningPresent(t *testing.T) {
	events := []CombatEvent{mkRoll(1, "a", "b", 0, 1)}
	r := BuildRollStreakReport("g1", "Test Game", true, events, 0, nil, defaultThresholds())
	if !r.PartialHistory {
		t.Error("expected PartialHistory=true")
	}
	found := false
	for _, w := range r.Warnings {
		if strings.Contains(w, "partial event history") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a partial-history warning, got: %v", r.Warnings)
	}
}

func TestBuildRollStreakReport_NoPartialHistoryWarningWhenComplete(t *testing.T) {
	events := []CombatEvent{mkRoll(1, "a", "b", 0, 1)}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, defaultThresholds())
	if r.PartialHistory {
		t.Error("expected PartialHistory=false")
	}
	for _, w := range r.Warnings {
		if strings.Contains(w, "partial event history") {
			t.Errorf("did not expect a partial-history warning, got: %v", r.Warnings)
		}
	}
}

// --- Bad data warnings ---

func TestBuildRollStreakReport_ZeroZeroOutcomeWarns(t *testing.T) {
	events := []CombatEvent{mkRoll(1, "a", "b", 0, 0)}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, defaultThresholds())
	found := false
	for _, w := range r.Warnings {
		if strings.Contains(w, "not a valid combat outcome") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a 0-0 outcome warning, got: %v", r.Warnings)
	}
}

func TestBuildRollStreakReport_DuplicateSequenceWarns(t *testing.T) {
	events := []CombatEvent{
		mkRoll(1, "a", "b", 1, 0),
		mkRoll(1, "a", "b", 0, 1),
	}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, defaultThresholds())
	found := false
	for _, w := range r.Warnings {
		if strings.Contains(w, "duplicate event sequence") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a duplicate-sequence warning, got: %v", r.Warnings)
	}
}

func TestBuildRollStreakReport_SkippedEventsSurfacedAsWarningWithoutCorruptingOthers(t *testing.T) {
	events := []CombatEvent{
		mkRoll(1, "a", "b", 1, 0),
		mkRoll(2, "a", "b", 1, 0),
	}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 3, nil, defaultThresholds())
	found := false
	for _, w := range r.Warnings {
		if strings.Contains(w, "3 event(s) were skipped") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected skipped-events warning, got: %v", r.Warnings)
	}
	// The one attacker's valid streak must still be detected correctly.
	if len(r.AttackingLossStreaks) != 1 || r.AttackingLossStreaks[0].Length != 2 {
		t.Errorf("skipped-event warning should not corrupt unrelated streak detection: %+v", r.AttackingLossStreaks)
	}
}

func TestBuildRollStreakReport_ZeroZeroIncludedInRollTraceNotHidden(t *testing.T) {
	events := []CombatEvent{
		mkRoll(1, "a", "b", 1, 0),
		mkRoll(2, "a", "b", 0, 0),
	}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, StreakThresholds{MinLossStreakLength: 99, MinWinStreakLength: 99, MinDroughtLength: 2})
	want := "1-0, 0-0"
	if r.AttackDroughts[0].RollTrace != want {
		t.Errorf("0-0 roll must appear in trace, want %q got %q", want, r.AttackDroughts[0].RollTrace)
	}
}

// --- Sorting ---

func TestSortStreaks_LengthDescThenStartTimeAsc(t *testing.T) {
	events := []CombatEvent{
		mkRoll(1, "a", "b", 1, 0),
		mkRoll(2, "a", "b", 1, 0),
		mkRoll(3, "a", "b", 0, 1),
		mkRoll(4, "a", "b", 1, 0),
		mkRoll(5, "a", "b", 1, 0),
		mkRoll(6, "a", "b", 1, 0),
	}
	r := BuildRollStreakReport("g1", "Test Game", false, events, 0, nil, defaultThresholds())
	if len(r.AttackingLossStreaks) != 2 {
		t.Fatalf("want 2 loss streaks, got %d", len(r.AttackingLossStreaks))
	}
	if r.AttackingLossStreaks[0].Length < r.AttackingLossStreaks[1].Length {
		t.Errorf("streaks must be sorted length DESC: %+v", r.AttackingLossStreaks)
	}
}
