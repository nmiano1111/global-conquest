package main

import (
	"fmt"
	"math"

	"github.com/nmiano1111/global-conquest/backend/internal/simulation"
)

// wilson95Z/wilsonScoreInterval95 duplicate internal/tournament's own
// unexported Wilson-interval code -- every cmd/* binary in this project
// is self-contained (see cmd/tdtraindata's identical rationale for
// duplicating computeGameID), and these two symbols are unexported in
// internal/tournament regardless, so there's no cheaper alternative to
// reusing the exact formula than copying it.
const wilson95Z = 1.959963984540054

type confidenceInterval struct{ Low, High float64 }

func wilsonScoreInterval95(wins, n int) confidenceInterval {
	if n == 0 {
		return confidenceInterval{}
	}
	p := float64(wins) / float64(n)
	nf := float64(n)
	z := wilson95Z
	denom := 1 + z*z/nf
	center := (p + z*z/(2*nf)) / denom
	margin := (z / denom) * math.Sqrt(p*(1-p)/nf+z*z/(4*nf*nf))
	return confidenceInterval{Low: math.Max(0, center-margin), High: math.Min(1, center+margin)}
}

// report finds every seed present (and completed) in both a and b,
// locates strategyX/strategyY's seats in each (their seat assignment is
// expected to be swapped between the two files, per this tool's whole
// purpose, but this code doesn't assume which file has which -- it just
// looks each strategy up fresh per result), and tallies head-to-head
// FinishOrder comparisons plus each strategy's own whole-game win rate
// across the combined, paired sample.
func report(a, b map[int64]simulation.Result, strategyX, strategyY string) {
	var pairedSeeds int
	var xBetter, yBetter, tied int
	var xWins, yWins, totalGames int
	var xFailures, yFailures int

	for seed, ra := range a {
		rb, ok := b[seed]
		if !ok {
			continue
		}
		pairedSeeds++

		for _, r := range []simulation.Result{ra, rb} {
			if !r.Completed {
				if _, ok := seatFor(r, strategyX); ok {
					xFailures++
				}
				if _, ok := seatFor(r, strategyY); ok {
					yFailures++
				}
				continue
			}
			totalGames++
			if xSeat, ok := seatFor(r, strategyX); ok && xSeat == r.WinnerSeat {
				xWins++
			}
			if ySeat, ok := seatFor(r, strategyY); ok && ySeat == r.WinnerSeat {
				yWins++
			}
		}

		if !ra.Completed || !rb.Completed {
			continue
		}
		xSeatA, okXA := seatFor(ra, strategyX)
		ySeatA, okYA := seatFor(ra, strategyY)
		if !okXA || !okYA {
			continue
		}
		xFinish, yFinish := ra.Seats[xSeatA].FinishOrder, ra.Seats[ySeatA].FinishOrder
		switch {
		case xFinish == 0 || yFinish == 0:
			// FinishOrder undetermined (e.g. a stalemate that still
			// technically completed) -- skip this seed's head-to-head
			// comparison rather than guess.
		case xFinish < yFinish:
			xBetter++
		case yFinish < xFinish:
			yBetter++
		default:
			tied++
		}
	}

	fmt.Printf("pairedstats: %d seeds present in both files\n\n", pairedSeeds)
	if totalGames == 0 {
		fmt.Println("no completed games in the paired sample -- nothing to report")
		return
	}

	xCI := wilsonScoreInterval95(xWins, totalGames)
	yCI := wilsonScoreInterval95(yWins, totalGames)
	fmt.Printf("%s: %d/%d wins (%.1f%%) 95%% CI [%.1f-%.1f], %d failures\n",
		strategyX, xWins, totalGames, 100*float64(xWins)/float64(totalGames), 100*xCI.Low, 100*xCI.High, xFailures)
	fmt.Printf("%s: %d/%d wins (%.1f%%) 95%% CI [%.1f-%.1f], %d failures\n",
		strategyY, yWins, totalGames, 100*float64(yWins)/float64(totalGames), 100*yCI.Low, 100*yCI.High, yFailures)

	headToHead := xBetter + yBetter + tied
	if headToHead == 0 {
		fmt.Println("\nno seeds had a determined head-to-head finish-order comparison")
		return
	}
	fmt.Printf("\nhead-to-head finish order (%d seeds, both games in a seed completed): %s better %d, %s better %d, tied %d\n",
		headToHead, strategyX, xBetter, strategyY, yBetter, tied)
	fmt.Println("\nnote: whole-game win rate at this sample size is only reliably powered to detect large effects -- see the design doc's Statistical Power section before treating a narrow gap here as decisive.")
}
