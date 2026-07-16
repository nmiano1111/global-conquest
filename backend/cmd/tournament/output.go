package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/fatih/color"

	"github.com/nmiano1111/global-conquest/backend/internal/simulation"
	"github.com/nmiano1111/global-conquest/backend/internal/tournament"
)

// useColor reports whether ANSI color codes should be emitted to w: only
// when w is the process's real stdout, that stdout is a live terminal, and
// the environment hasn't opted out (fatih/color's own NO_COLOR/dumb-terminal
// detection, consulted via the package-level color.NoColor). --output
// routes to a file, where embedded escape codes would just be noise for
// whatever later reads it.
func useColor(w io.Writer) bool {
	f, ok := w.(*os.File)
	return !color.NoColor && ok && f == os.Stdout && isTerminal(f)
}

// setColorEnabled overrides fatih/color's global NoColor flag for the
// duration of one render and returns a func restoring the prior value, so
// the override never leaks past this call.
func setColorEnabled(enabled bool) func() {
	prev := color.NoColor
	color.NoColor = !enabled
	return func() { color.NoColor = prev }
}

// rankByGameWinRate returns the best- and worst-performing strategy IDs by
// GameWinRate (not SeatWinRate -- GameWinRate answers "is this strategy
// identity actually better," unaffected by how many seats it occupies; see
// StrategyStats' doc comments), considering only strategies with at least
// one completed appearance. Returns two empty strings when there's nothing
// to rank (one strategy, or every strategy tied) so the caller skips
// highlighting rather than misleadingly marking an arbitrary strategy as
// both best and worst.
func rankByGameWinRate(stats []tournament.StrategyStats) (best, worst string) {
	if len(stats) < 2 {
		return "", ""
	}
	bestRate, worstRate := -1.0, 2.0
	for _, s := range stats {
		if s.CompletedAppearances == 0 {
			continue
		}
		if s.GameWinRate > bestRate {
			bestRate, best = s.GameWinRate, s.StrategyID
		}
		if s.GameWinRate < worstRate {
			worstRate, worst = s.GameWinRate, s.StrategyID
		}
	}
	if best == worst {
		return "", ""
	}
	return best, worst
}

// writeAggregateText renders a compact human-readable summary: a header
// line, a failures breakdown (if any), and a per-strategy table -- with the
// best/worst strategy by GameWinRate highlighted when color is enabled.
// The table reports both "seat win%" (SeatWinRate: given a seat is playing
// this strategy, how often does it win -- capped at 1/k for a strategy
// occupying k seats) and "game win%" (GameWinRate: what fraction of games
// did any seat playing this strategy win, the number that actually answers
// "is this strategy better"). See StrategyStats' doc comments in
// internal/tournament for why these two numbers can look surprisingly
// different for a mirror matchup.
//
// Note: tabwriter.AlignRight has a real rendering bug where the last two
// columns lose their separating space entirely (confirmed with a
// standalone repro while building cmd/simulate) -- left-aligned (the
// default, flags=0) renders correctly, so that's what this uses too,
// despite the numeric columns technically reading better right-aligned.
//
// Color is applied to the table only *after* tabwriter has finished
// aligning it, one whole rendered line at a time -- wrapping raw cell text
// in ANSI escape codes before tabwriter sees it would make tabwriter count
// those invisible bytes as visible width and misalign every column after
// the first colored cell.
func writeAggregateText(w io.Writer, cfg tournament.Config, agg tournament.Aggregate, elapsed time.Duration) error {
	restore := setColorEnabled(useColor(w))
	defer restore()

	label := color.New(color.FgCyan, color.Bold).Sprint("tournament:")
	completedStr := color.New(color.FgGreen).Sprintf("%d completed", agg.CompletedGames)
	failedText := fmt.Sprintf("%d failed", agg.FailedGames)
	if agg.FailedGames > 0 {
		failedText = color.New(color.FgRed).Sprint(failedText)
	} else {
		failedText = color.New(color.FgHiBlack).Sprint(failedText)
	}

	seedEnd := cfg.SeedStart + int64(cfg.Games) - 1
	fmt.Fprintf(w, "%s %d games (%s, %s) · seeds %d-%d · avg %.1f turns, %.1f commands · %s elapsed\n",
		label, agg.TotalGames, completedStr, failedText, cfg.SeedStart, seedEnd,
		agg.AvgTurns, agg.AvgCommands, elapsed.Round(10*time.Millisecond))

	if agg.FailedGames > 0 {
		types := make([]string, 0, len(agg.Failures))
		for ft := range agg.Failures {
			types = append(types, string(ft))
		}
		sort.Strings(types)
		fmt.Fprint(w, color.New(color.FgYellow).Sprint("failures:"))
		for _, ft := range types {
			fmt.Fprintf(w, " %s: %d", ft, agg.Failures[simulation.FailureType(ft)])
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w)

	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "strategy\tappearances\tcompleted\twins\tseat win%\tgame win%\tavg finish\tavg captures\tavg elims")
	for _, s := range agg.Strategies {
		fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%.1f%%\t%.1f%%\t%.2f\t%.2f\t%.2f\n",
			s.StrategyID, s.Appearances, s.CompletedAppearances, s.Wins, s.SeatWinRate*100, s.GameWinRate*100,
			s.AvgFinishOrder, s.AvgCaptures, s.AvgEliminationsMade)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) == 0 {
		return nil
	}
	fmt.Fprintln(w, color.New(color.Bold, color.Underline).Sprint(lines[0]))

	best, worst := rankByGameWinRate(agg.Strategies)
	for i, line := range lines[1:] {
		switch agg.Strategies[i].StrategyID {
		case best:
			fmt.Fprintln(w, color.New(color.FgGreen, color.Bold).Sprint(line))
		case worst:
			fmt.Fprintln(w, color.New(color.FgHiBlack).Sprint(line))
		default:
			fmt.Fprintln(w, line)
		}
	}
	return nil
}

// jsonAggregateReport pairs the Aggregate with the Config that produced it,
// so a saved report is self-describing and the exact batch is reproducible
// (same Strategies/SeedStart/Games/GameMode/Limits).
type jsonAggregateReport struct {
	Config    tournament.Config    `json:"config"`
	Aggregate tournament.Aggregate `json:"aggregate"`
}

func writeAggregateJSON(w io.Writer, cfg tournament.Config, agg tournament.Aggregate) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(jsonAggregateReport{Config: cfg, Aggregate: agg})
}

// rawWriter appends one compact JSON-encoded simulation.Result per line
// (JSONL) as each game completes -- the same field shape cmd/simulate's
// --format json emits under its "result" key, so a tool consuming both
// sees consistent field names. Deliberately no trace data: tournament games
// always run at simulation.TraceNone (see internal/tournament.Run's doc
// comment), so there's nothing else to include per line.
type rawWriter struct {
	f   *os.File
	enc *json.Encoder
}

func newRawWriter(path string) (*rawWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &rawWriter{f: f, enc: json.NewEncoder(f)}, nil
}

func (r *rawWriter) write(result simulation.Result) error {
	return r.enc.Encode(result)
}

func (r *rawWriter) close() error {
	return r.f.Close()
}
