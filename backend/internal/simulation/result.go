package simulation

import (
	"fmt"
	"time"
)

// Result is the structured outcome of one simulation, returned whether the
// game completed normally or failed partway through -- Seats/Turns/
// Commands/etc. reflect whatever actually happened before a Failure (if
// any) cut the run short, so a failed run still carries useful diagnostic
// data rather than nothing.
type Result struct {
	Seed        int64
	PlayerCount int
	Seats       []SeatResult

	// WinnerSeat is -1 and WinnerPlayerID/WinnerStrategy are empty when
	// the simulation didn't reach a winner (incomplete or failed).
	WinnerSeat     int
	WinnerPlayerID string
	WinnerStrategy string

	Turns    int
	Commands int

	// CombatRolls counts Attack calls (dice-roll exchanges), a distinct
	// count from Captures (territories conquered) and Eliminations
	// (players knocked out) -- one Attack call can be none, one, or both
	// of the latter two.
	CombatRolls  int
	Captures     int
	Eliminations int
	CardTurnIns  int

	// Duration is wall-clock machine-performance data, not gameplay data
	// -- exclude it from any determinism comparison between two runs of
	// the same Config.
	Duration time.Duration

	// Completed is true only if the simulation reached risk.PhaseGameOver
	// normally. Failure is nil exactly when Completed is true.
	Completed bool
	Failure   *Failure
}

// NewResult returns a Result with WinnerSeat set to -1 (no winner yet)
// rather than Go's zero value 0, which would otherwise be misread as
// "seat 0 won." Callers building a Result incrementally over the course
// of a simulation should start from this rather than a bare Result{}.
func NewResult(seed int64, playerCount int) Result {
	return Result{
		Seed:        seed,
		PlayerCount: playerCount,
		WinnerSeat:  -1,
	}
}

// SeatResult summarizes one seat's outcome. Several fields have no direct
// risk.Game equivalent (risk.Game.playerTerritoryCount is unexported, and
// the engine has no notion of elimination order at all) -- the simulator
// is responsible for computing and tracking these itself as the game
// progresses, not reading them off engine state after the fact.
type SeatResult struct {
	Seat       int
	PlayerID   string
	StrategyID string
	Eliminated bool

	// FinishOrder is 1 for the winner. Among eliminated seats, the seat
	// eliminated LAST (survived longest) places 2nd, down to the seat
	// eliminated FIRST placing last (PlayerCount) -- surviving longer is
	// always at least as good a finish. 0 if the simulation ended before
	// finishing order could be determined.
	FinishOrder int

	FinalTerritories int
	FinalArmies      int

	Captures          int
	EliminationsMade  int
	CombatLossesTaken int
}

// FailureType classifies why a simulation didn't complete. See the
// constants below for the full set and how each should be handled.
type FailureType string

const (
	// FailureInvalidStrategyID marks a seat's strategy ID not present in
	// the StrategyRegistry the Simulator was given. In practice this is
	// caught by Config.Validate before a game is ever constructed, so it
	// surfaces as a plain error from RunOne, not a populated Result --
	// this constant exists for taxonomy completeness (e.g. a future
	// tournament runner classifying a batch of Config-validation
	// failures under the same scheme), not because RunOne itself
	// produces one.
	FailureInvalidStrategyID FailureType = "invalid_strategy_id"

	// FailureStrategyError means Strategy.NextCommand itself returned a
	// non-nil error -- always a strategy bug (e.g. an internal
	// "no legal action" case the strategy's own logic should have
	// prevented from being reachable).
	FailureStrategyError FailureType = "strategy_error"

	// FailureEngineRejectedCommand means the dispatcher's call into
	// risk.Game returned ErrInvalidMove/ErrOutOfTurn/ErrInvalidPhase.
	// Since the simulator is single-threaded and re-reads authoritative
	// state before every decision, this can only mean a strategy (or the
	// dispatcher itself) produced an illegal command against the exact
	// state it just observed -- always a bug, never retried.
	FailureEngineRejectedCommand FailureType = "engine_rejected_command"

	// FailureCommandLimitReached means Limits.MaxCommands was hit.
	FailureCommandLimitReached FailureType = "command_limit_reached"

	// FailureTurnLimitReached means Limits.MaxTurns was hit.
	FailureTurnLimitReached FailureType = "turn_limit_reached"

	// FailureRepeatedStateDetected means the loop-detection fingerprint
	// (statehash.go) recurred Limits.MaxRepeatedStates times, or the
	// cheap no-progress counter (Limits.MaxCommandsWithoutProgress) was
	// exceeded.
	FailureRepeatedStateDetected FailureType = "repeated_state_detected"

	// FailureDurationLimitReached means Limits.MaxDuration was hit -- a
	// wall-clock backstop distinct from the count-based limits above,
	// for scenarios where the state keeps genuinely changing (so no
	// repeated-state check fires) but per-command cost grows without the
	// game converging, e.g. two bots endlessly reinforcing a shared
	// border without ever attacking.
	FailureDurationLimitReached FailureType = "duration_limit_reached"

	// FailureContextCanceled means ctx.Done() fired -- caller-initiated,
	// not a bug.
	FailureContextCanceled FailureType = "context_canceled"

	// FailureInternalInvariant means the simulator itself observed
	// something that should be impossible (e.g. CurrentPlayer out of
	// range) -- a simulator bug, not a strategy bug.
	FailureInternalInvariant FailureType = "internal_invariant_violated"
)

// Failure explains why a simulation didn't complete, with enough context
// to reproduce the exact failing run. It implements error so it can be
// returned directly as RunOne's error value.
type Failure struct {
	Type         FailureType
	Message      string
	Phase        string
	PlayerID     string
	StrategyID   string
	Command      string
	CommandIndex int
	Turn         int

	// Seed is always populated -- reproducing a failure only requires
	// this plus the original Config.
	Seed int64
}

func (f *Failure) Error() string {
	return fmt.Sprintf("simulation: %s (seed=%d turn=%d phase=%s command=%d): %s",
		f.Type, f.Seed, f.Turn, f.Phase, f.CommandIndex, f.Message)
}
