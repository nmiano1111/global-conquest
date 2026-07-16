//go:build race

package simulation

// raceDetectorEnabled lets timing-sensitive tests scale their wall-clock
// budgets when built with `-race` (as `make test`'s TESTFLAGS always is).
// The race detector's instrumentation overhead -- easily 10x on
// memory-access-heavy code like combat resolution -- is real and expected,
// not a regression; a test asserting "this finishes quickly" needs a
// different bar under -race than a production CLI run does.
const raceDetectorEnabled = true
