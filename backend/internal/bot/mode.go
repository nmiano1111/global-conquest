package bot

// ExecutionMode distinguishes a live interactive game (paced for human
// spectators/opponents) from a headless run (no artificial delays). Phase 1
// only implements a flat delay for live mode; action-specific pacing is a
// later milestone.
type ExecutionMode string

const (
	ExecutionLive       ExecutionMode = "live"
	ExecutionSimulation ExecutionMode = "simulation"
)
