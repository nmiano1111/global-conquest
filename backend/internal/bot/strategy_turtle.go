package bot

import (
	"context"
	"fmt"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
	"github.com/nmiano1111/global-conquest/backend/internal/tdstate"
)

// StrategyTurtleV1 is the registry identifier for TurtleStrategy -- unlike
// every other persona in this package, not a Lux Delux port (the paper's
// own six-AI roster, see
// project-docs/bot_player/proposals/Lux_Delux_AI_Research_Notes.md, has no
// defensive specialist). Added after two diagnostics found the actual gap
// behind the GCN bot's missing continent-defense behavior: tdstate.Encode's
// hand-built Defence feature correctly discriminates a defended border
// from a thin one, but a 30-game self-play trace across all six existing
// personas found continents held with a real defended border (Defence
// meaningfully above near-zero) only ~6% of the time they were held at
// all -- TD(lambda) can't learn a pattern its own teachers almost never
// demonstrate. Existing personas (ClusterStrategy and its
// Quo/Boscoe descendants) already reinforce their weakest continent
// border via clusterOrTakeContinentPlacement, but their attack phases
// keep expanding regardless of whether that border is actually safe, so
// whatever reinforcement just built up gets spent on offense the same
// turn. Turtle reuses every other persona's placement/occupy/card-timing
// logic unchanged and differs only in attack()/fortify(): once it owns a
// continent, it stops spending armies on expansion until the border is
// genuinely secure, and fortifies toward whichever border point is most
// exposed rather than just "has more enemy neighbors."
const StrategyTurtleV1 = "turtle-v1"

// turtleSecurityThreshold is the minimum tdstate Global.Defence value
// TurtleStrategy requires before resuming normal expansion. Defence
// measures the raw army count at pi's weakest owned frontier territory
// (not a ratio to any adjacent enemy), divided by total board armies,
// capped at 0.2 (internal/tdstate/encode.go's defenceCap).
//
// This value was tuned empirically against win rate, not raw Defence
// purity: TD(lambda) bootstraps every visited state toward that
// trajectory's own eventual outcome, so a "defended continent" pattern
// that mostly precedes a loss anyway teaches the network the opposite of
// the intended lesson. A stricter gate (0.05) drove mean self-play
// Defence up (0.0083 baseline -> 0.0353) but collapsed Turtle's win rate
// against killbot-v1/angry-v1 from ~32% (an effectively ungated
// threshold like 0.005) down to 5.9% -- too passive to ever reach the
// good outcomes that would validate the pattern. 0.01 turned out to beat
// that tradeoff on both axes at once: ~30% win rate (near the ungated
// ceiling) *and* a higher mean Defence (0.0401) than the stricter 0.05
// version -- a Turtle that actually wins survives longer and holds a
// bigger empire, which means more turns accumulating armies at its
// border than an overly cautious one that gets wiped out before it ever
// builds up much of anything.
const turtleSecurityThreshold = 0.01

// TurtleStrategy implements StrategyTurtleV1.
type TurtleStrategy struct{}

// NewTurtleStrategy creates a TurtleStrategy.
func NewTurtleStrategy() *TurtleStrategy { return &TurtleStrategy{} }

// NextCommand dispatches on g.Phase, mirroring the other Lux-inspired
// strategies' shape. It always returns a zero-value Explanation, since
// Turtle has no scoring model to report.
func (t *TurtleStrategy) NextCommand(_ context.Context, g *risk.Game, playerID string) (Command, Explanation, error) {
	cmd, err := t.nextCommand(g, playerID)
	return cmd, Explanation{}, err
}

func (t *TurtleStrategy) nextCommand(g *risk.Game, playerID string) (Command, error) {
	switch g.Phase {
	case risk.PhaseSetupReinforce:
		return t.setupReinforce(g, playerID)
	case risk.PhaseReinforce:
		return t.reinforce(g, playerID)
	case risk.PhaseAttack:
		return t.attack(g, playerID)
	case risk.PhaseOccupy:
		return t.occupy(g, playerID)
	case risk.PhaseFortify:
		return t.fortify(g, playerID)
	default:
		return Command{}, fmt.Errorf("bot: %s has no move for phase %q", StrategyTurtleV1, g.Phase)
	}
}

// setupReinforce places the one initial army via
// clusterOrTakeContinentPlacement -- identical to ClusterStrategy's own
// setupReinforce; Turtle's behavior only diverges once territories (and
// therefore a home continent) actually exist to defend.
func (t *TurtleStrategy) setupReinforce(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalSetupReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal setup reinforcement for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	best := clusterOrTakeContinentPlacement(g, pi, setupReinforceActionsAsReinforcements(actions))
	return Command{Action: ActionPlaceInitialArmy, Territory: string(best)}, nil
}

// reinforce cashes cards whenever possible (voluntaryCardTurnIn, shared
// with Quo/Boscoe -- more armies sooner means the border gets defended
// faster), then places via clusterOrTakeContinentPlacement, which already
// targets the weakest point of the best owned continent.
func (t *TurtleStrategy) reinforce(g *risk.Game, playerID string) (Command, error) {
	if cmd, ok := voluntaryCardTurnIn(g, playerID); ok {
		return cmd, nil
	}

	actions := risk.LegalReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal reinforcement for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	best := clusterOrTakeContinentPlacement(g, pi, actions)
	return Command{Action: ActionPlaceReinforcement, Territory: string(best), Armies: g.PendingReinforcements}, nil
}

// continentBorderSecure reports whether cont is safe enough for
// TurtleStrategy to resume normal expansion: either cont has no real
// front at all right now (no owned border territory has any enemy
// neighbor -- nothing to be threatened by, trivially secure), or
// tdstate.Encode's own Defence feature already clears
// turtleSecurityThreshold for pi's current position.
func continentBorderSecure(g *risk.Game, pi int, cont risk.Continent, threshold float64) bool {
	hasFront := false
	for _, border := range continentBorders(g, cont) {
		if g.Territories[border].Owner != pi {
			continue
		}
		if enemyNeighborCount(g, border, pi) > 0 {
			hasFront = true
			break
		}
	}
	if !hasFront {
		return true
	}
	return tdstate.Encode(g, pi).Global.Defence >= threshold
}

// attack expands like ClusterStrategy (easy-expand, fill-out, consolidate,
// split-up(1.2)) whenever Turtle has no home continent yet or its home
// continent's border is already secure -- but if it owns a continent
// whose border isn't secure, expansion is skipped entirely: only a truly
// free/safe attack (attackForCard) is taken, so reinforcements placed
// this turn aren't immediately spent on offense instead of defense.
// Turtle never calls shouldGoHogWild/attackAsMuchAsPossible -- permanent
// restraint once a continent is held is the whole point of this persona,
// not a temporary gate other personas eventually override.
func (t *TurtleStrategy) attack(g *risk.Game, playerID string) (Command, error) {
	pi := playerIndex(g, playerID)
	root, ok := clusterRoot(g, pi)
	if !ok {
		return Command{Action: ActionEndAttack}, nil
	}

	if home, hasHome := mostValuablePositiveOwnedContinent(g, pi); hasHome && !continentBorderSecure(g, pi, home, turtleSecurityThreshold) {
		if cmd, ok := attackForCard(g, pi); ok {
			return cmd, nil
		}
		return Command{Action: ActionEndAttack}, nil
	}

	stages := []func(*risk.Game, int, risk.Territory) (Command, bool){
		attackEasyExpand,
		attackFillOut,
		attackConsolidate,
		func(g *risk.Game, pi int, root risk.Territory) (Command, bool) { return attackSplitUp(g, pi, root, 1.2) },
	}
	for _, stage := range stages {
		if cmd, ok := stage(g, pi, root); ok {
			return cmd, nil
		}
	}
	if cmd, ok := attackForCard(g, pi); ok {
		return cmd, nil
	}
	return Command{Action: ActionEndAttack}, nil
}

// occupy is a thin wrapper over clusterOccupyDecision -- shared unchanged
// with Cluster/Boscoe/Quo.
func (t *TurtleStrategy) occupy(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalOccupations(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal occupation for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	armies := clusterOccupyDecision(g, pi, g.Occupy, actions)
	return Command{Action: ActionOccupy, Armies: armies}, nil
}

// turtleFortify finds the home continent's owned border territory with
// the lowest raw army count (a border territory with no enemy neighbor
// right now isn't a real front and is skipped entirely) -- mirroring
// tdstate.computeDefence's own isFrontier+minArmies logic exactly, since
// that raw count (not any ratio to adjacent enemy strength) is what
// Defence actually rewards -- then returns the legal fortification move
// targeting it, if one exists. risk.LegalFortifications allows any
// contiguous-owned-path move, not just direct adjacency, so this can
// pull reinforcements from deep in Turtle's interior toward its shell in
// a single move. ok is false if Turtle has no home continent yet, or no
// border territory is currently a real front, or no legal move reaches
// the weakest one.
func turtleFortify(g *risk.Game, pi int, actions []risk.FortificationAction) (best risk.FortificationAction, ok bool) {
	home, hasHome := mostValuablePositiveOwnedContinent(g, pi)
	if !hasHome {
		return risk.FortificationAction{}, false
	}

	var weakest risk.Territory
	weakestArmies := 0
	foundThreatened := false
	for _, border := range continentBorders(g, home) {
		ts := g.Territories[border]
		if ts.Owner != pi {
			continue
		}
		if enemyNeighborCount(g, border, pi) == 0 {
			continue
		}
		if !foundThreatened || ts.Armies < weakestArmies {
			weakest, weakestArmies, foundThreatened = border, ts.Armies, true
		}
	}
	if !foundThreatened {
		return risk.FortificationAction{}, false
	}

	for _, a := range actions {
		if a.To == weakest && (!ok || a.MaxArmies > best.MaxArmies) {
			best, ok = a, true
		}
	}
	return best, ok
}

// fortify tries turtleFortify first (shore up the home continent's most
// exposed border point), falling back to the shared bestFortifyDestination
// (used by every other persona) if there's no home continent yet or no
// move reaches its weakest point.
func (t *TurtleStrategy) fortify(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalFortifications(g, playerID)
	pi := playerIndex(g, playerID)

	if best, ok := turtleFortify(g, pi, actions); ok {
		return Command{Action: ActionFortify, From: string(best.From), To: string(best.To), Armies: best.MaxArmies}, nil
	}
	if best, bestScore, ok := bestFortifyDestination(g, pi, actions); ok && bestScore > 0 {
		return Command{Action: ActionFortify, From: string(best.From), To: string(best.To), Armies: best.MaxArmies}, nil
	}
	return Command{Action: ActionEndTurn}, nil
}
