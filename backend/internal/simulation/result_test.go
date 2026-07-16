package simulation

import (
	"strings"
	"testing"
)

var _ error = (*Failure)(nil)

func TestFailureErrorIncludesKeyContext(t *testing.T) {
	f := &Failure{
		Type:         FailureEngineRejectedCommand,
		Message:      "territory not owned by player",
		Phase:        "attack",
		PlayerID:     "p1",
		StrategyID:   "scored-v1",
		Command:      "attack",
		CommandIndex: 42,
		Turn:         7,
		Seed:         12345,
	}
	msg := f.Error()
	for _, want := range []string{"engine_rejected_command", "12345", "7", "attack", "42", "territory not owned by player"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected Error() to mention %q, got %q", want, msg)
		}
	}
}

func TestNewResultDefaultsWinnerSeatToNegativeOne(t *testing.T) {
	r := NewResult(99, 4)
	if r.WinnerSeat != -1 {
		t.Fatalf("expected WinnerSeat -1 for a fresh result, got %d", r.WinnerSeat)
	}
	if r.Seed != 99 || r.PlayerCount != 4 {
		t.Fatalf("expected Seed=99 PlayerCount=4, got Seed=%d PlayerCount=%d", r.Seed, r.PlayerCount)
	}
	if r.WinnerPlayerID != "" || r.WinnerStrategy != "" {
		t.Fatalf("expected empty winner identity on a fresh result, got %q/%q", r.WinnerPlayerID, r.WinnerStrategy)
	}
	if r.Completed || r.Failure != nil {
		t.Fatalf("expected a fresh result to be neither completed nor failed")
	}
}

func TestFailureTypeConstantsAreDistinct(t *testing.T) {
	all := []FailureType{
		FailureInvalidStrategyID,
		FailureStrategyError,
		FailureEngineRejectedCommand,
		FailureCommandLimitReached,
		FailureTurnLimitReached,
		FailureRepeatedStateDetected,
		FailureContextCanceled,
		FailureInternalInvariant,
	}
	seen := make(map[FailureType]bool, len(all))
	for _, ft := range all {
		if seen[ft] {
			t.Fatalf("duplicate FailureType value %q", ft)
		}
		seen[ft] = true
	}
	if len(seen) != 8 {
		t.Fatalf("expected 8 distinct failure types, got %d", len(seen))
	}
}
