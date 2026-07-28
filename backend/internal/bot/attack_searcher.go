package bot

import "github.com/nmiano1111/global-conquest/backend/internal/risk"

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
type AttackSearcher interface {
	// Search returns the first action of the best attack (or attack
	// sequence) found from g's current attack-phase state for the acting
	// player (playerID/pi identify them), and the score to gate against
	// the current state's own score -- ok is false only when there is no
	// legal attack to consider at all.
	Search(g *risk.Game, playerID string, pi int, value ValueFunction) (a risk.AttackAction, score float64, ok bool)
}
