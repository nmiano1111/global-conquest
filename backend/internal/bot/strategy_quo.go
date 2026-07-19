package bot

import (
	"context"
	"fmt"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// StrategyQuoV1 is the registry identifier for QuoStrategy, the fourth
// Lux Delux-inspired persona (see
// project-docs/bot_player/proposals/Lux_Delux_AI_Research_Notes.md and
// Lux_Port_Notes.md). Quo extends Shaft extends Cluster in Lux's own
// hierarchy and overrides almost nothing from Cluster -- placement, root
// selection, occupy, and fortify are all reused verbatim from
// strategy_cluster.go. The two genuinely new pieces are Shaft's "forward
// sweep" attack stage (sweepForward) and Quo's always-cash-cards policy
// plus attackForCard (reused from strategy_pixie.go).
const StrategyQuoV1 = "quo-v1"

// QuoStrategy implements StrategyQuoV1.
type QuoStrategy struct{}

// NewQuoStrategy creates a QuoStrategy.
func NewQuoStrategy() *QuoStrategy { return &QuoStrategy{} }

// NextCommand dispatches on g.Phase, mirroring the other Lux-inspired
// strategies' shape. It always returns a zero-value Explanation, since Quo
// has no scoring model to report.
func (q *QuoStrategy) NextCommand(_ context.Context, g *risk.Game, playerID string) (Command, Explanation, error) {
	cmd, err := q.nextCommand(g, playerID)
	return cmd, Explanation{}, err
}

func (q *QuoStrategy) nextCommand(g *risk.Game, playerID string) (Command, error) {
	switch g.Phase {
	case risk.PhaseSetupReinforce:
		return q.setupReinforce(g, playerID)
	case risk.PhaseReinforce:
		return q.reinforce(g, playerID)
	case risk.PhaseAttack:
		return q.attack(g, playerID)
	case risk.PhaseOccupy:
		return q.occupy(g, playerID)
	case risk.PhaseFortify:
		return q.fortify(g, playerID)
	default:
		return Command{}, fmt.Errorf("bot: %s has no move for phase %q", StrategyQuoV1, g.Phase)
	}
}

// voluntaryCardTurnIn trades the first legal card set, whenever one
// exists -- Lux's Quo/Boscoe both call cashCardsIfPossible unconditionally
// from cardsPhase, unlike Angry/Cluster/Pixie's inherited forced-only
// default (risk.CardTurnInRequired).
func voluntaryCardTurnIn(g *risk.Game, playerID string) (Command, bool) {
	sets := risk.LegalCardTurnIns(g, playerID)
	if len(sets) == 0 {
		return Command{}, false
	}
	return Command{Action: ActionTradeCards, CardIndices: sets[0].Indices}, true
}

// setupReinforce places the one initial army via clusterPlacementTerritory
// -- Lux's Quo/Shaft don't override placeArmies, so this is identical to
// ClusterStrategy's own setupReinforce.
func (q *QuoStrategy) setupReinforce(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalSetupReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal setup reinforcement for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	best := clusterPlacementTerritory(g, pi, actions, func(a risk.SetupReinforcementAction) risk.Territory { return a.Territory })
	return Command{Action: ActionPlaceInitialArmy, Territory: string(best)}, nil
}

// reinforce trades cards whenever a legal set exists (voluntaryCardTurnIn,
// not the forced-only gate Angry/Cluster/Pixie use), then places via
// clusterPlacementTerritory -- again identical to ClusterStrategy's own
// placement logic, since Lux's Quo/Shaft don't override placeArmies.
func (q *QuoStrategy) reinforce(g *risk.Game, playerID string) (Command, error) {
	if cmd, ok := voluntaryCardTurnIn(g, playerID); ok {
		return cmd, nil
	}

	actions := risk.LegalReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal reinforcement for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	best := clusterPlacementTerritory(g, pi, actions, func(a risk.ReinforcementAction) risk.Territory { return a.Territory })
	return Command{Action: ActionPlaceReinforcement, Territory: string(best), Armies: g.PendingReinforcements}, nil
}

// attack extends ClusterStrategy's own easy-expand/fill-out/consolidate
// sequence with Shaft's forward-sweep stage (only once nothing has been
// conquered yet this turn -- Lux's !board.tookOverACountry()), then
// attackForCard, then the shared hogwild/stalemate fallback -- Lux's
// Quo.attackPhase, which re-derives Cluster's own root-selection rather
// than anything new.
func (q *QuoStrategy) attack(g *risk.Game, playerID string) (Command, error) {
	pi := playerIndex(g, playerID)
	root, ok := clusterRoot(g, pi)
	if !ok {
		return Command{Action: ActionEndAttack}, nil
	}

	for _, stage := range []func(*risk.Game, int, risk.Territory) (Command, bool){
		attackEasyExpand, attackFillOut, attackConsolidate,
	} {
		if cmd, ok := stage(g, pi, root); ok {
			return cmd, nil
		}
	}

	if !g.ConqueredThisTurn {
		for _, border := range clusterBorder(g, root, pi) {
			if cmd, ok := sweepForward(g, pi, border); ok {
				return cmd, nil
			}
		}
	}

	if cmd, ok := attackForCard(g, pi); ok {
		return cmd, nil
	}
	if shouldGoHogWild(g, pi) {
		if cmd, ok := attackAsMuchAsPossible(g, pi); ok {
			return cmd, nil
		}
	}
	return Command{Action: ActionEndAttack}, nil
}

// sweepForward analyzes whether attacking outward from sweep can collapse
// its frontier down to a single remaining contact point -- Lux's Shaft.
// sweepForwardBorder's dry-run pass, which (like this port) never
// simulates combat, only board topology, so it translates directly into a
// stateless function: the initial "seen" set is sweep's currently unowned
// neighbors (canonical order); repeatedly fold any frontier territory
// with exactly one not-yet-seen enemy neighbor into seen, until nothing
// more folds (Lux's advanceSweep loop, recomputed to a clean fixed point
// each call rather than replicating its live ArrayList mutate-during-
// iterate behavior -- see Lux_Port_Notes.md). If exactly one territory
// remains in the frontier, the sweep is worthwhile: attack sweep -> the
// canonical-order-first member of the original seen set (Lux's
// seen.get(0)); subsequent NextCommand calls continue the sequence as the
// board evolves.
func sweepForward(g *risk.Game, pi int, sweep risk.Territory) (Command, bool) {
	var seenOrder []risk.Territory
	seen := make(map[risk.Territory]bool)
	for _, n := range g.Board.Order {
		if !g.Board.IsAdjacent(sweep, n) {
			continue
		}
		if g.Territories[n].Owner != pi {
			seen[n] = true
			seenOrder = append(seenOrder, n)
		}
	}
	if len(seenOrder) == 0 {
		return Command{}, false
	}

	frontier := append([]risk.Territory(nil), seenOrder...)
	for {
		var next []risk.Territory
		folded := false
		for _, c := range frontier {
			var onlyUnseen risk.Territory
			count := 0
			for _, n := range g.Board.Order {
				if !g.Board.IsAdjacent(c, n) {
					continue
				}
				if g.Territories[n].Owner != pi && !seen[n] {
					count++
					onlyUnseen = n
				}
			}
			switch count {
			case 0:
				// c has no unseen enemies left -- drop it from the frontier.
			case 1:
				seen[onlyUnseen] = true
				next = append(next, onlyUnseen)
				folded = true
			default:
				next = append(next, c)
			}
		}
		frontier = next
		if !folded {
			break
		}
	}

	if len(frontier) != 1 {
		return Command{}, false
	}
	src := g.Territories[sweep]
	if src.Armies <= 1 {
		return Command{}, false
	}
	return Command{Action: ActionAttack, From: string(sweep), To: string(seenOrder[0]), AttackerDice: min(3, src.Armies-1)}, true
}

// occupy is a thin wrapper over clusterOccupyDecision -- Lux's Quo/Shaft
// don't override moveArmiesIn, inheriting Cluster's exact behavior.
func (q *QuoStrategy) occupy(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalOccupations(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal occupation for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	armies := clusterOccupyDecision(g, pi, g.Occupy, actions)
	return Command{Action: ActionOccupy, Armies: armies}, nil
}

// fortify uses bestFortifyDestination (shared with every other Lux-inspired
// strategy so far) -- Lux's Quo/Shaft don't override fortifyPhase either.
func (q *QuoStrategy) fortify(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalFortifications(g, playerID)
	pi := playerIndex(g, playerID)
	best, bestScore, ok := bestFortifyDestination(g, pi, actions)
	if !ok || bestScore == 0 {
		return Command{Action: ActionEndTurn}, nil
	}
	return Command{Action: ActionFortify, From: string(best.From), To: string(best.To), Armies: best.MaxArmies}, nil
}
