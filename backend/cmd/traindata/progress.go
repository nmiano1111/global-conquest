package main

import (
	"os"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// progressReporter shows a single live progress bar on stderr while
// traindata runs, tracking games completed (not rows written) against the
// requested total -- same mpb-based approach as cmd/tournament/progress.go
// (copy-adapted, not shared; each cmd/* binary in this project is
// independently self-contained).
type progressReporter struct {
	enabled bool
	p       *mpb.Progress
	bar     *mpb.Bar
}

func newProgressReporter(total int) *progressReporter {
	if !isTerminal(os.Stderr) {
		return &progressReporter{enabled: false}
	}
	p := mpb.New(mpb.WithOutput(os.Stderr))
	bar := p.AddBar(int64(total),
		mpb.PrependDecorators(decor.Name("traindata", decor.WC{W: 10, C: decor.DindentRight})),
		mpb.AppendDecorators(
			decor.CountersNoUnit("%d / %d", decor.WCSyncWidth),
			decor.Percentage(decor.WCSyncSpace),
			decor.Elapsed(decor.ET_STYLE_MMSS, decor.WCSyncSpace),
		),
	)
	return &progressReporter{enabled: true, p: p, bar: bar}
}

// update sets the bar's current games-completed count. Called from run's
// own single results-consuming goroutine (see main.go) -- no locking
// needed here for that reason.
func (p *progressReporter) update(completed int) {
	if !p.enabled {
		return
	}
	p.bar.SetCurrent(int64(completed))
}

// done releases the bar and waits for it to finish rendering.
func (p *progressReporter) done() {
	if !p.enabled {
		return
	}
	p.bar.Abort(false)
	p.p.Wait()
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
