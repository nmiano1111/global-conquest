package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/nmiano1111/global-conquest/backend/internal/simulation"
	"github.com/nmiano1111/global-conquest/backend/internal/tdstate"
)

// trainingRow is one living player's perspective of the board at one
// completed turn boundary -- the TD(λ) trainer groups rows by
// (GameID, PlayerID), sorts by Turn, and treats the resulting sequence as
// one episode (see analytics/src/global_conquest_analytics/td_fit.py).
type trainingRow struct {
	// GameID identifies which game this row came from -- see
	// computeGameID; safe across separate cmd/tdtraindata invocations the
	// same way cmd/traindata's identically-named helper is.
	GameID     string
	Seed       int64
	PlayerID   string
	StrategyID string
	Turn       int
	// Won is the eventual outcome for PlayerID specifically -- the same
	// value on every row sharing a (GameID, PlayerID), included on each
	// row for simplicity (TD(λ) only actually needs it at the sequence's
	// last row, but repeating it costs nothing and keeps every row
	// self-contained).
	Won bool
	// Features is tdstate.Encode(...).Flatten()'s output -- a fixed-order
	// float64 vector. Column names for this order are written once, not
	// per row, to featureNamesPath(output) (see output.go).
	Features []float64
}

// computeGameID deterministically identifies one game, safe to use across
// separate cmd/tdtraindata invocations -- identical in spirit to
// cmd/traindata's own computeGameID (same inputs determine a game's exact
// trajectory), duplicated rather than imported since cmd/traindata is a
// separate, independently self-contained binary.
func computeGameID(strategies []string, gameMode string, seed int64) string {
	return fmt.Sprintf("%s@%s@%d", strings.Join(strategies, ","), gameMode, seed)
}

// runOneGame plays one game via sim.RunOne, capturing one trainingRow per
// living player at every completed turn boundary (see
// simulation.Config.OnTurnBoundary). Returns ok=false (and no rows) for a
// game that didn't complete -- no reliable win/loss label exists for it,
// matching cmd/traindata's identical policy.
func runOneGame(sim *simulation.Simulator, cfg simulation.Config, gameID string) (rows []trainingRow, ok bool) {
	type pending struct {
		playerID   string
		strategyID string
		turn       int
		features   []float64
	}
	var buffered []pending

	cfg.OnTurnBoundary = func(tb simulation.TurnBoundary) {
		for i, p := range tb.Game.Players {
			if p.Eliminated {
				continue
			}
			buffered = append(buffered, pending{
				playerID:   p.ID,
				strategyID: p.Strategy,
				turn:       tb.Turn,
				features:   tdstate.Encode(tb.Game, i).Flatten(),
			})
		}
	}

	result, _, _ := sim.RunOne(context.Background(), cfg, nil)
	if !result.Completed {
		return nil, false
	}

	rows = make([]trainingRow, len(buffered))
	for i, p := range buffered {
		rows[i] = trainingRow{
			GameID:     gameID,
			Seed:       cfg.Seed,
			PlayerID:   p.playerID,
			StrategyID: p.strategyID,
			Turn:       p.turn,
			Won:        p.playerID == result.WinnerPlayerID,
			Features:   p.features,
		}
	}
	return rows, true
}
