package main

import (
	"fmt"
	"os"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/simulation"
)

// progressUpdateInterval throttles how often the live status line
// re-prints -- matches cmd/simulate's spinner cadence.
const progressUpdateInterval = 100 * time.Millisecond

// progressReporter shows a live-updating "games complete" counter on
// stderr while a tournament runs. Unlike cmd/simulate's per-command
// spinner (no knowable total, see that package's README), a tournament's
// total game count is known up front, so this reports real progress
// (N/total) rather than an indeterminate spinner.
//
// Always writes to stderr, never stdout -- stdout is the aggregate
// summary output (text or JSON, possibly redirected). Suppressed entirely
// when stderr isn't a terminal, so redirected/piped runs and CI logs never
// see carriage-return spam.
type progressReporter struct {
	enabled     bool
	total       int
	completed   int
	failed      int
	start       time.Time
	lastPrinted time.Time
}

func newProgressReporter(total int) *progressReporter {
	return &progressReporter{enabled: isTerminal(os.Stderr), total: total, start: time.Now()}
}

// update is called once per completed/failed game, from tournament.Run's
// own single consuming goroutine -- see internal/tournament.Run's doc
// comment on onResult. No locking needed here for that reason.
func (p *progressReporter) update(result simulation.Result) {
	p.completed++
	if !result.Completed {
		p.failed++
	}
	if !p.enabled {
		return
	}
	now := time.Now()
	// Always flush the very last update so the counter doesn't look stuck
	// short of total on a fast tournament between throttled prints.
	if p.completed < p.total && now.Sub(p.lastPrinted) < progressUpdateInterval {
		return
	}
	p.lastPrinted = now
	fmt.Fprintf(os.Stderr, "\r%d/%d games complete (%d failed)... %s elapsed...\033[K",
		p.completed, p.total, p.failed, time.Since(p.start).Round(100*time.Millisecond))
}

// done clears the status line so it doesn't linger above the final output.
func (p *progressReporter) done() {
	if !p.enabled {
		return
	}
	fmt.Fprint(os.Stderr, "\r\033[K")
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
