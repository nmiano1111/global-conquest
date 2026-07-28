package bot

import "github.com/nmiano1111/global-conquest/backend/internal/risk"

// ReinforceSearcher decides which single territory/army-count reinforce()
// should place next, given the ValueFunction to score afterstates with --
// kept independent of ValueStrategy for the same reason AttackSearcher is
// (see its doc comment): search algorithm and value function stay two
// independently swappable axes. Implementations own their entire search
// strategy -- whether they consider one territory at a time or plan a
// multi-step group sequence internally (see GroupReinforcer). reinforce()
// calls Search once per decision and submits only the returned action --
// exactly like AttackSearcher.Search, any further placement in the same
// phase happens because bot.Runner reloads state and re-invokes
// NextCommand after the first is committed, not because Search itself
// submits more than one action.
type ReinforceSearcher interface {
	// Search returns the territory and army count to place next, and the
	// score to report via Explanation -- ok is false only when there is
	// no legal reinforcement to consider at all.
	Search(g *risk.Game, playerID string, pi int, value ValueFunction) (t risk.Territory, armies int, score float64, ok bool)
}
