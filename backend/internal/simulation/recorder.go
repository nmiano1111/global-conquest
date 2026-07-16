package simulation

import (
	"github.com/nmiano1111/global-conquest/backend/internal/bot"
	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// MilestoneType classifies a summary-level trace event.
type MilestoneType string

const (
	MilestoneTurnTransition MilestoneType = "turn_transition"
	MilestoneCapture        MilestoneType = "capture"
	MilestoneElimination    MilestoneType = "elimination"
	MilestoneCardTurnIn     MilestoneType = "card_turn_in"
)

// Milestone is one TraceSummary-and-above event: a small, fixed-shape
// record, never full game state.
type Milestone struct {
	Type     MilestoneType
	Turn     int
	Seat     int
	PlayerID string
	Detail   string // human-readable, e.g. "captured Kamchatka", "eliminated p2"
}

// Entry is one TraceDecision-and-above record: what a strategy chose and
// why, captured directly from Strategy.NextCommand's own return values
// before dispatch. This is only cheap to capture because the direct
// simulation loop already holds the Explanation in scope -- bot.Runner,
// by contrast, only ever logs it as a string and discards it.
//
// Fingerprint and DomainEvent are meaningful only at TraceFull; Recorder
// strips them at TraceDecision regardless of what a caller passes in (see
// RecordEntry), so a caller need not gate on level itself, other than to
// avoid the cost of computing a Fingerprint it won't end up storing.
type Entry struct {
	CommandIndex int
	Turn         int
	Seat         int
	PlayerID     string
	StrategyID   string
	Phase        string
	Command      bot.Command
	Explanation  bot.Explanation

	Fingerprint StateFingerprint
	DomainEvent *risk.DomainEvent
}

// Recorder accumulates trace data during a simulation, recording only
// what the configured TraceLevel calls for. No trace level ever stores a
// full risk.Game snapshot -- deliberately deferred, see the simulation
// framework design doc's Trace section. Construct one per simulation; it
// is not safe to share across simulations.
type Recorder struct {
	level      TraceLevel
	milestones []Milestone
	entries    []Entry
}

func NewRecorder(level TraceLevel) *Recorder {
	return &Recorder{level: level}
}

// Level reports the trace level this recorder was constructed with, so a
// caller can avoid expensive work (e.g. computing a StateFingerprint) it
// knows will be discarded below TraceFull.
func (r *Recorder) Level() TraceLevel {
	return r.level
}

// RecordMilestone appends m if the trace level is TraceSummary or above;
// a no-op at TraceNone.
func (r *Recorder) RecordMilestone(m Milestone) {
	if r.level == TraceNone {
		return
	}
	r.milestones = append(r.milestones, m)
}

// RecordEntry appends e if the trace level is TraceDecision or TraceFull;
// a no-op at TraceNone or TraceSummary. At TraceDecision (but not
// TraceFull), e's Fingerprint and DomainEvent are stripped before storing
// regardless of whether the caller populated them, so the recorder's
// behavior is authoritative -- a careless caller can't accidentally leak
// full-level data into a decision-level trace.
func (r *Recorder) RecordEntry(e Entry) {
	if r.level != TraceDecision && r.level != TraceFull {
		return
	}
	if r.level != TraceFull {
		e.Fingerprint = 0
		e.DomainEvent = nil
	}
	r.entries = append(r.entries, e)
}

// Milestones returns every recorded milestone, in the order recorded.
func (r *Recorder) Milestones() []Milestone {
	return r.milestones
}

// Entries returns every recorded decision/full entry, in the order
// recorded.
func (r *Recorder) Entries() []Entry {
	return r.entries
}
