package bot

// ExecutionMode distinguishes a live interactive game (paced for human
// spectators/opponents) from a headless run (no artificial delays). Phase 1
// only implements a flat delay for live mode; action-specific pacing is a
// later milestone.
type ExecutionMode string

const (
	// ExecutionLive paces bot actions with bounded random delays so a game
	// is watchable by human spectators/opponents.
	ExecutionLive ExecutionMode = "live"
	// ExecutionSimulation runs bot actions back-to-back with no artificial
	// delays.
	ExecutionSimulation ExecutionMode = "simulation"
)
