// Command pairedstats computes matchup-keyed head-to-head statistics
// between two strategies across a pair of mirrored-seat tournament runs
// -- nothing in internal/tournament computes this today (StrategyStats'
// aggregator is purely independent per-strategy running sums, confirmed
// by reading it in full), so this is new, self-contained functionality.
//
// Intended use: run cmd/tournament twice with the SAME seed range but
// the two strategies of interest swapped between config indices (see
// project-docs/bot_player/phase_3_continuous_improvement/
// 12_Symbolic_Strategic_Reasoning_Layer.md's Tournament Statistics
// section for why this cancels seat/turn-order bias with zero engine
// changes -- turn-order/territory-layout permutation depends only on
// player count and seed, never on which strategy occupies which seat),
// each with --raw-output to a JSONL file of simulation.Result rows, then
// compare them here.
//
// Usage:
//
//	go run ./cmd/pairedstats --a seatA.jsonl --b seatB.jsonl \
//	  --strategy-x goal-killbot-v1 --strategy-y killbot-v2
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/nmiano1111/global-conquest/backend/internal/simulation"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("pairedstats", flag.ExitOnError)
	pathA := fs.String("a", "", "First run's --raw-output JSONL file (required)")
	pathB := fs.String("b", "", "Second run's --raw-output JSONL file, same seed range, strategies x/y swapped seats (required)")
	strategyX := fs.String("strategy-x", "", "First strategy ID to compare (required)")
	strategyY := fs.String("strategy-y", "", "Second strategy ID to compare (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pathA == "" || *pathB == "" || *strategyX == "" || *strategyY == "" {
		return fmt.Errorf("--a, --b, --strategy-x, and --strategy-y are all required")
	}

	a, err := readBySeed(*pathA)
	if err != nil {
		return fmt.Errorf("read --a: %w", err)
	}
	b, err := readBySeed(*pathB)
	if err != nil {
		return fmt.Errorf("read --b: %w", err)
	}

	report(a, b, *strategyX, *strategyY)
	return nil
}

// readBySeed reads one JSONL file of simulation.Result rows (as written
// by cmd/tournament's --raw-output), keyed by Seed -- duplicate seeds in
// one file are unexpected (a well-formed run has exactly one row per
// seed) and the later row silently wins, matching Go map-literal
// semantics rather than erroring, since this is a diagnostic tool, not a
// correctness-critical one.
func readBySeed(path string) (map[int64]simulation.Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := make(map[int64]simulation.Result)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var r simulation.Result
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, err
		}
		out[r.Seed] = r
	}
	return out, sc.Err()
}

// seatFor finds which seat strategyID occupies in result, if any.
func seatFor(r simulation.Result, strategyID string) (int, bool) {
	for _, s := range r.Seats {
		if s.StrategyID == strategyID {
			return s.Seat, true
		}
	}
	return 0, false
}
