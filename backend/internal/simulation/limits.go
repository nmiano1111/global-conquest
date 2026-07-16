package simulation

import (
	"fmt"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// LimitBreach describes which safety limit was exceeded and why. The
// simulator wraps this into a full Failure, adding the seed/player/
// strategy/command context it already has in scope at the point of
// dispatch -- context the tracker itself has no way to know.
type LimitBreach struct {
	Type    FailureType
	Message string
}

// LimitTracker enforces Limits incrementally as a simulation progresses.
// Construct one per simulation (it is not safe to share across
// simulations) and call Observe once after every successfully dispatched
// command.
type LimitTracker struct {
	limits Limits

	commands int

	firstObserved   bool
	lastPlayer      int
	lastPhase       risk.Phase
	noProgressCount int

	stateCounts map[StateFingerprint]int
}

// NewLimitTracker returns a tracker enforcing the given limits, which
// must already be valid (see Limits.Validate) -- this constructor doesn't
// re-validate them.
func NewLimitTracker(limits Limits) *LimitTracker {
	return &LimitTracker{
		limits:      limits,
		stateCounts: make(map[StateFingerprint]int),
	}
}

// Observe records one more dispatched command against g's resulting state
// and reports whether any limit has now been exceeded (nil if not). Call
// it once per successfully dispatched command only -- a rejected command
// is already a hard FailureEngineRejectedCommand handled by the
// dispatcher/simulator directly, never a limit concern.
//
// Checks run cheapest-first: the two plain counters (MaxCommands,
// MaxTurns), then the cheap no-progress counter (no CurrentPlayer/Phase
// change), and only then the more expensive state-fingerprint check
// (statehash.go) -- both of the latter two report
// FailureRepeatedStateDetected, since they're two different-granularity
// views of the same underlying problem: the game isn't moving forward.
func (lt *LimitTracker) Observe(g *risk.Game) *LimitBreach {
	lt.commands++
	if lt.commands > lt.limits.MaxCommands {
		return &LimitBreach{
			Type:    FailureCommandLimitReached,
			Message: fmt.Sprintf("exceeded MaxCommands (%d)", lt.limits.MaxCommands),
		}
	}

	if g.TurnNumber > lt.limits.MaxTurns {
		return &LimitBreach{
			Type:    FailureTurnLimitReached,
			Message: fmt.Sprintf("exceeded MaxTurns (%d)", lt.limits.MaxTurns),
		}
	}

	if lt.firstObserved && g.CurrentPlayer == lt.lastPlayer && g.Phase == lt.lastPhase {
		lt.noProgressCount++
	} else {
		lt.noProgressCount = 0
	}
	lt.lastPlayer = g.CurrentPlayer
	lt.lastPhase = g.Phase
	lt.firstObserved = true
	if lt.noProgressCount >= lt.limits.MaxCommandsWithoutProgress {
		return &LimitBreach{
			Type:    FailureRepeatedStateDetected,
			Message: fmt.Sprintf("no CurrentPlayer/Phase change for %d consecutive commands", lt.noProgressCount),
		}
	}

	fp := Fingerprint(g)
	lt.stateCounts[fp]++
	if lt.stateCounts[fp] >= lt.limits.MaxRepeatedStates {
		return &LimitBreach{
			Type:    FailureRepeatedStateDetected,
			Message: fmt.Sprintf("identical game state recurred %d times", lt.stateCounts[fp]),
		}
	}

	return nil
}
