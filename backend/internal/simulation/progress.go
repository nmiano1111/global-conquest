package simulation

import "time"

// progressInterval throttles how often RunOne calls an onProgress
// callback -- time-based rather than every-N-commands, since command
// throughput varies enormously by matchup (a few hundred per second for
// simple decisions, far fewer once ForecastAttack's cost grows with army
// count in a long game). ~10 updates/second is smooth for a terminal
// display without meaningfully affecting simulation throughput.
const progressInterval = 100 * time.Millisecond

// ProgressUpdate is reported periodically during RunOne so a caller (e.g.
// a CLI) can show live progress without polling or blocking on the
// result. There is no "percent complete" here deliberately: game length
// varies enormously by matchup (some strategy pairings deadlock for many
// thousands of commands before hitting a safety limit — see
// Limits.MaxDuration), so there is no meaningful total to measure
// against, only how far the game has gotten so far.
type ProgressUpdate struct {
	Commands int
	Turn     int
	Elapsed  time.Duration
}
