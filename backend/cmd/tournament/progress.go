package main

import (
	"fmt"
	"os"

	"github.com/schollz/progressbar/v3"

	"github.com/nmiano1111/global-conquest/backend/internal/simulation"
)

// progressReporter shows a live progress bar on stderr while a tournament
// runs. Unlike cmd/simulate's indeterminate spinner (a single game's length
// isn't knowable ahead of time, see that package's README), a tournament's
// total game count is known up front, so this shows real progress against
// it rather than just an activity indicator.
//
// Always writes to stderr, never stdout -- stdout is the aggregate summary
// output (text or JSON, possibly redirected). Suppressed entirely when
// stderr isn't a terminal, so redirected/piped runs and CI logs never see
// the bar's carriage-return-driven redraws.
type progressReporter struct {
	enabled   bool
	bar       *progressbar.ProgressBar
	completed int
	failed    int
}

func newProgressReporter(total int) *progressReporter {
	enabled := isTerminal(os.Stderr)
	var bar *progressbar.ProgressBar
	if enabled {
		bar = progressbar.NewOptions(total,
			progressbar.OptionSetWriter(os.Stderr),
			progressbar.OptionEnableColorCodes(true),
			progressbar.OptionShowCount(),
			progressbar.OptionShowIts(),
			progressbar.OptionSetItsString("games"),
			progressbar.OptionSetPredictTime(true),
			progressbar.OptionSetElapsedTime(true),
			progressbar.OptionSetDescription("[cyan]running tournament[reset]"),
			progressbar.OptionSetTheme(progressbar.Theme{
				Saucer:        "[green]=[reset]",
				SaucerHead:    "[green]>[reset]",
				SaucerPadding: " ",
				BarStart:      "[",
				BarEnd:        "]",
			}),
		)
	}
	return &progressReporter{enabled: enabled, bar: bar}
}

// update is called once per completed/failed game, from tournament.Run's
// own single consuming goroutine (see internal/tournament.Run's doc
// comment on onResult) -- no locking needed here for that reason.
func (p *progressReporter) update(result simulation.Result) {
	p.completed++
	if !result.Completed {
		p.failed++
	}
	if !p.enabled {
		return
	}
	desc := "[cyan]running tournament[reset]"
	if p.failed > 0 {
		desc = fmt.Sprintf("[cyan]running tournament[reset] [red](%d failed)[reset]", p.failed)
	}
	p.bar.Describe(desc)
	_ = p.bar.Add(1)
}

// done finalizes the bar and drops to a new line so it doesn't collide
// with the aggregate output that follows.
func (p *progressReporter) done() {
	if !p.enabled {
		return
	}
	_ = p.bar.Finish()
	fmt.Fprintln(os.Stderr)
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
