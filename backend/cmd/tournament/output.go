package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/simulation"
	"github.com/nmiano1111/global-conquest/backend/internal/tournament"
)

// writeAggregateText renders a compact human-readable summary: a header
// line, a failures breakdown (if any), and a per-strategy table.
//
// Note: tabwriter.AlignRight has a real rendering bug where the last two
// columns lose their separating space entirely (confirmed with a
// standalone repro while building cmd/simulate) -- left-aligned (the
// default, flags=0) renders correctly, so that's what this uses too,
// despite the numeric columns technically reading better right-aligned.
func writeAggregateText(w io.Writer, cfg tournament.Config, agg tournament.Aggregate, elapsed time.Duration) error {
	seedEnd := cfg.SeedStart + int64(cfg.Games) - 1
	fmt.Fprintf(w, "tournament: %d games (%d completed, %d failed) · seeds %d-%d · avg %.1f turns, %.1f commands · %s elapsed\n",
		agg.TotalGames, agg.CompletedGames, agg.FailedGames, cfg.SeedStart, seedEnd,
		agg.AvgTurns, agg.AvgCommands, elapsed.Round(10*time.Millisecond))

	if agg.FailedGames > 0 {
		types := make([]string, 0, len(agg.Failures))
		for ft := range agg.Failures {
			types = append(types, string(ft))
		}
		sort.Strings(types)
		fmt.Fprint(w, "failures:")
		for _, ft := range types {
			fmt.Fprintf(w, " %s: %d", ft, agg.Failures[simulation.FailureType(ft)])
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w)

	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "strategy\tappearances\tcompleted\twins\twin rate\tavg finish\tavg captures\tavg elims")
	for _, s := range agg.Strategies {
		fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%.1f%%\t%.2f\t%.2f\t%.2f\n",
			s.StrategyID, s.Appearances, s.CompletedAppearances, s.Wins, s.WinRate*100,
			s.AvgFinishOrder, s.AvgCaptures, s.AvgEliminationsMade)
	}
	return tw.Flush()
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
