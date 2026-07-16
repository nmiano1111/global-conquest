package main

import (
	"fmt"
	"os"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/simulation"
)

// spinnerFrames cycles a braille-dot spinner, the same style used by most
// modern CLI tools (npm, cargo, etc.) for indeterminate-length work.
var spinnerFrames = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

// progressReporter shows a live-updating status line on stderr while a
// simulation runs. There's no "percent complete" here on purpose: game
// length varies enormously by matchup (see the README's convergence
// note), so there's no meaningful total to measure against -- only how
// far the game has gotten so far.
//
// Always writes to stderr, never stdout: stdout is the actual result
// output (text or JSON, possibly redirected by a pipeline), and mixing a
// carriage-return-driven status line into that stream would corrupt it.
// Suppressed entirely when stderr isn't a terminal, so redirected/piped
// runs and CI logs never see carriage-return spam.
type progressReporter struct {
	enabled bool
	frame   int
}

func newProgressReporter() *progressReporter {
	return &progressReporter{enabled: isTerminal(os.Stderr)}
}

func (p *progressReporter) update(u simulation.ProgressUpdate) {
	if !p.enabled {
		return
	}
	spin := spinnerFrames[p.frame%len(spinnerFrames)]
	p.frame++
	fmt.Fprintf(os.Stderr, "\r%c turn %d, %d commands, %s elapsed...\033[K",
		spin, u.Turn, u.Commands, u.Elapsed.Round(100*time.Millisecond))
}

// done clears the status line so it doesn't linger above the final
// result output.
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
