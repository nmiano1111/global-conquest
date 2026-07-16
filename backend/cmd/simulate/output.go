package main

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/nmiano1111/global-conquest/backend/internal/simulation"
)

// writeText renders a compact human-readable summary: a header line, the
// winner (or failure) line, and a per-seat table. This is deliberately
// the minimum useful text format for milestone 1 -- see the simulation
// framework design doc's Output Formats section.
func writeText(w io.Writer, result simulation.Result) error {
	fmt.Fprintf(w, "seed %d · %d players · %d turns · %d commands\n",
		result.Seed, result.PlayerCount, result.Turns, result.Commands)

	if result.Completed {
		fmt.Fprintf(w, "winner: seat %d (%s)\n\n", result.WinnerSeat, result.WinnerStrategy)
	} else if result.Failure != nil {
		fmt.Fprintf(w, "did not complete: %s\n\n", result.Failure.Error())
	}

	// Note: tabwriter.AlignRight has a real rendering bug where the last
	// two columns lose their separating space entirely (confirmed with a
	// standalone repro) -- left-aligned (the default, flags=0) renders
	// correctly, so that's what this uses despite the numeric columns
	// technically reading better right-aligned.
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "seat\tstrategy\tterritories\tarmies\tcaptures\telims")
	for _, seat := range result.Seats {
		fmt.Fprintf(tw, "%d\t%s\t%d\t%d\t%d\t%d\n",
			seat.Seat, seat.StrategyID, seat.FinalTerritories, seat.FinalArmies, seat.Captures, seat.EliminationsMade)
	}
	return tw.Flush()
}

// jsonReport combines Result with whatever trace data the recorder
// accumulated. Result itself never embeds trace data (see recorder.go's
// package comment on Simulator.RunOne) -- this is where the two get
// joined for output, kept out of the simulation package itself since it's
// a CLI-shaped concern, not a simulation one.
type jsonReport struct {
	Result     simulation.Result      `json:"result"`
	Milestones []simulation.Milestone `json:"milestones,omitempty"`
	Decisions  []simulation.Entry     `json:"decisions,omitempty"`
}

// writeJSON marshals a full report: the Result plus the trace level's
// milestones/decisions (both empty at TraceNone, populated as the level
// increases -- see recorder.go).
func writeJSON(w io.Writer, result simulation.Result, recorder *simulation.Recorder) error {
	report := jsonReport{
		Result:     result,
		Milestones: recorder.Milestones(),
		Decisions:  recorder.Entries(),
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}
