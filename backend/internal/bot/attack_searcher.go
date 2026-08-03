package bot

import (
	"context"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// AttackSearcher decides which candidate attack (if any) ValueStrategy's
// attack phase should take, given the ValueFunction to score afterstates
// with -- kept independent of ValueStrategy so search algorithm and value
// function are two independently swappable axes (see ValueFunction's own
// doc comment for the model-swapping half of this). Implementations own
// their entire search strategy: how many plies deep, how many candidates
// wide, which terminal-state selection policy, and any memoization needed
// to stay fast (see SequenceSearcher's forecastCache). attack() calls
// Search once per decision and applies the identical margin gate to
// whatever it returns, regardless of which concrete searcher produced it.
//
// Three implementations exist: SinglePlySearcher (no lookahead, the
// original default), SequenceSearcher (fixed-ply-depth branching search),
// and AnytimeSearcher (wall-clock-budgeted iterative-deepening search,
// Phase 5 of project-docs/bot_player/proposals/
// Search_Integration_Roadmap_with_References.md). ctx is only consulted
// by AnytimeSearcher, to fold in an externally-imposed deadline and to
// notice cancellation -- SinglePlySearcher/SequenceSearcher ignore it
// entirely, leaving their existing, already-validated behavior unchanged.
type AttackSearcher interface {
	// Search returns the first action of the best attack (or attack
	// sequence) found from g's current attack-phase state for the acting
	// player (playerID/pi identify them), and the score to gate against
	// the current state's own score -- ok is false only when there is no
	// legal attack to consider at all.
	Search(ctx context.Context, g *risk.Game, playerID string, pi int, value ValueFunction) (a risk.AttackAction, score float64, ok bool)
}
