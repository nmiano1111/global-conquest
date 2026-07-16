package simulation

import (
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

func sampleMilestone() Milestone {
	return Milestone{Type: MilestoneCapture, Turn: 3, Seat: 1, PlayerID: "p1", Detail: "captured Kamchatka"}
}

func sampleEntry() Entry {
	return Entry{
		CommandIndex: 7,
		Turn:         3,
		Seat:         1,
		PlayerID:     "p1",
		StrategyID:   "scored-v1",
		Phase:        string(risk.PhaseAttack),
		Command:      bot.Command{Action: bot.ActionAttack, From: "Alaska", To: "Kamchatka"},
		Explanation:  bot.Explanation{Score: 4.5, Features: []bot.Feature{{Name: "army_advantage", Value: 4.5}}},
		Fingerprint:  StateFingerprint(12345),
		DomainEvent:  &risk.DomainEvent{Type: risk.EventTypeCombatRollResolved},
	}
}

func TestRecorderLevelAccessor(t *testing.T) {
	r := NewRecorder(TraceDecision)
	if r.Level() != TraceDecision {
		t.Fatalf("expected Level() to report TraceDecision, got %s", r.Level())
	}
}

func TestRecorderNoneRecordsNothing(t *testing.T) {
	r := NewRecorder(TraceNone)
	r.RecordMilestone(sampleMilestone())
	r.RecordEntry(sampleEntry())
	if len(r.Milestones()) != 0 {
		t.Fatalf("expected no milestones at TraceNone, got %d", len(r.Milestones()))
	}
	if len(r.Entries()) != 0 {
		t.Fatalf("expected no entries at TraceNone, got %d", len(r.Entries()))
	}
}

func TestRecorderSummaryRecordsMilestonesOnly(t *testing.T) {
	r := NewRecorder(TraceSummary)
	r.RecordMilestone(sampleMilestone())
	r.RecordEntry(sampleEntry())
	if len(r.Milestones()) != 1 {
		t.Fatalf("expected 1 milestone at TraceSummary, got %d", len(r.Milestones()))
	}
	if len(r.Entries()) != 0 {
		t.Fatalf("expected no entries at TraceSummary, got %d", len(r.Entries()))
	}
}

func TestRecorderDecisionRecordsEntriesWithoutFullOnlyFields(t *testing.T) {
	r := NewRecorder(TraceDecision)
	r.RecordMilestone(sampleMilestone())
	r.RecordEntry(sampleEntry())

	if len(r.Milestones()) != 1 {
		t.Fatalf("expected 1 milestone at TraceDecision, got %d", len(r.Milestones()))
	}
	entries := r.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry at TraceDecision, got %d", len(entries))
	}
	got := entries[0]
	if got.Command.Action != bot.ActionAttack || got.Explanation.Score != 4.5 {
		t.Fatalf("expected the core decision data to survive, got %+v", got)
	}
	if got.Fingerprint != 0 {
		t.Fatalf("expected Fingerprint to be stripped at TraceDecision, got %v", got.Fingerprint)
	}
	if got.DomainEvent != nil {
		t.Fatalf("expected DomainEvent to be stripped at TraceDecision, got %+v", got.DomainEvent)
	}
}

func TestRecorderFullRecordsEverything(t *testing.T) {
	r := NewRecorder(TraceFull)
	r.RecordEntry(sampleEntry())

	entries := r.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry at TraceFull, got %d", len(entries))
	}
	got := entries[0]
	if got.Fingerprint != StateFingerprint(12345) {
		t.Fatalf("expected Fingerprint to survive at TraceFull, got %v", got.Fingerprint)
	}
	if got.DomainEvent == nil || got.DomainEvent.Type != risk.EventTypeCombatRollResolved {
		t.Fatalf("expected DomainEvent to survive at TraceFull, got %+v", got.DomainEvent)
	}
}

func TestRecorderPreservesOrder(t *testing.T) {
	r := NewRecorder(TraceFull)
	for i := 0; i < 5; i++ {
		e := sampleEntry()
		e.CommandIndex = i
		r.RecordEntry(e)
		m := sampleMilestone()
		m.Turn = i
		r.RecordMilestone(m)
	}
	entries := r.Entries()
	milestones := r.Milestones()
	for i := 0; i < 5; i++ {
		if entries[i].CommandIndex != i {
			t.Fatalf("expected entries in recorded order, entry %d has CommandIndex %d", i, entries[i].CommandIndex)
		}
		if milestones[i].Turn != i {
			t.Fatalf("expected milestones in recorded order, milestone %d has Turn %d", i, milestones[i].Turn)
		}
	}
}

// TestRecorderNeverTouchesGameState is a narrower, recorder-only version
// of the design doc's "trace collection must never affect gameplay"
// requirement -- the stronger claim (identical Result across trace
// levels for the same seed) needs a full RunOne loop to verify and
// belongs to the simulator/cross-cutting test suite, but this confirms
// the recorder itself is purely additive bookkeeping with no path back
// into game state.
func TestRecorderNeverTouchesGameState(t *testing.T) {
	g, err := risk.NewClassicAutoStartGame([]string{"p0", "p1", "p2"}, NewDeterministicRNG(1))
	if err != nil {
		t.Fatalf("build game: %v", err)
	}
	before := Fingerprint(g)

	r := NewRecorder(TraceFull)
	for i := 0; i < 10; i++ {
		r.RecordMilestone(sampleMilestone())
		e := sampleEntry()
		e.Fingerprint = Fingerprint(g) // recording the game's fingerprint, not mutating it
		r.RecordEntry(e)
	}

	if after := Fingerprint(g); after != before {
		t.Fatalf("expected recording activity to never change game state, fingerprint changed from %v to %v", before, after)
	}
}
