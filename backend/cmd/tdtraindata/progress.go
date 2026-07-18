package main

import (
	"os"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// progressReporter shows a single live progress bar on stderr while
// tdtraindata runs, tracking games completed (not rows written) against
// the requested total -- copy-adapted from cmd/traindata's identical
// helper, not shared; each cmd/* binary in this project is independently
// self-contained.
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
		mpb.PrependDecorators(decor.Name("tdtraindata", decor.WC{W: 12, C: decor.DindentRight})),
		mpb.AppendDecorators(
			decor.CountersNoUnit("%d / %d", decor.WCSyncWidth),
			decor.Percentage(decor.WCSyncSpace),
			decor.Elapsed(decor.ET_STYLE_MMSS, decor.WCSyncSpace),
		),
	)
	return &progressReporter{enabled: true, p: p, bar: bar}
}

func (p *progressReporter) update(completed int) {
	if !p.enabled {
		return
	}
	p.bar.SetCurrent(int64(completed))
}

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
