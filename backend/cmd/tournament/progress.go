package main

import (
	"os"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"

	"github.com/nmiano1111/global-conquest/backend/internal/simulation"
)

// progressReporter shows one live progress bar per tournament on stderr,
// via mpb: a single-tournament run gets one bar, a --config batch run
// gets one per entry, each on its own line, updating concurrently as
// results arrive from that tournament's own goroutine.
//
// Always writes to stderr, never stdout -- stdout is the aggregate
// summary output (text or JSON, possibly redirected). Suppressed
// entirely when stderr isn't a terminal: mpb itself has no such
// detection built in and would happily emit cursor-control sequences
// into a redirected/piped stream, so that check stays here, matching
// this project's established convention (cmd/simulate's spinner, this
// package's prior single-bar reporter).
type progressReporter struct {
	enabled bool
	p       *mpb.Progress
}

// newProgressReporter constructs the shared bar container (if enabled).
// Call newBar once per tournament -- once for single-tournament mode, once
// per entry for batch mode -- before starting that tournament's
// goroutine, then wait() once every tournament has been launched.
func newProgressReporter() *progressReporter {
	if !isTerminal(os.Stderr) {
		return &progressReporter{enabled: false}
	}
	return &progressReporter{enabled: true, p: mpb.New(mpb.WithOutput(os.Stderr))}
}

// tournamentBar wraps one *mpb.Bar (or nothing, if progress reporting is
// disabled) behind an onResult-shaped update method, so call sites don't
// need to branch on whether reporting is enabled.
type tournamentBar struct {
	enabled   bool
	bar       *mpb.Bar
	completed int
}

// newBar adds one labeled bar sized to total games. Safe to call from the
// CLI's own goroutine before each tournament's worker goroutine starts --
// mpb.Progress.AddBar itself is safe for that; only the returned bar is
// then owned by exactly one tournament's goroutine from here on.
func (p *progressReporter) newBar(name string, total int) *tournamentBar {
	if !p.enabled {
		return &tournamentBar{enabled: false}
	}
	bar := p.p.AddBar(int64(total),
		mpb.PrependDecorators(
			decor.Name(name, decor.WC{W: len(name) + 1, C: decor.DindentRight}),
		),
		mpb.AppendDecorators(
			decor.CountersNoUnit("%d / %d", decor.WCSyncWidth),
			decor.Percentage(decor.WCSyncSpace),
			decor.Elapsed(decor.ET_STYLE_MMSS, decor.WCSyncSpace),
		),
	)
	return &tournamentBar{enabled: true, bar: bar}
}

// update is called once per completed/failed game, from tournament.Run's
// own single consuming goroutine for that tournament (see
// internal/tournament.Run's doc comment on onResult) -- no locking needed
// here for that reason; a given bar is only ever touched by its own
// tournament's goroutine.
func (b *tournamentBar) update(_ simulation.Result) {
	b.completed++
	if !b.enabled {
		return
	}
	b.bar.SetCurrent(int64(b.completed))
}

// done releases this bar. Safe to call whether the tournament ran to
// completion or was cut short (e.g. context cancellation) -- Abort is a
// no-op on an already-complete bar, and on one that never reached its
// total it lets that bar's last-drawn state stand rather than forcing it
// to 100%, which is the more honest rendering of a batch that got cut
// short.
func (b *tournamentBar) done() {
	if !b.enabled {
		return
	}
	b.bar.Abort(false)
}

// wait blocks until every bar added via newBar has completed or been
// released via done(), leaving the cursor below the last bar line, ready
// for the aggregate output that follows.
func (p *progressReporter) wait() {
	if !p.enabled {
		return
	}
	p.p.Wait()
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
