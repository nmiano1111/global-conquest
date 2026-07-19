package bot

import (
	"context"
	"fmt"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// StrategyBoscoeV1 is the registry identifier for BoscoeStrategy, the
// fifth Lux Delux-inspired persona (see
// project-docs/bot_player/proposals/Lux_Delux_AI_Research_Notes.md and
// Lux_Port_Notes.md). Boscoe extends Yakool extends Cluster in Lux's own
// hierarchy; Boscoe itself only overrides cardsPhase (voluntary cash,
// like Quo) and attackFromCluster (restrained to easy-expand/fill-out/
// consolidate, never split-up). Everything else -- including the
// genuinely new mechanic, a "kill the dominant player" placement/attack
// override -- comes from Yakool, inherited untouched.
const StrategyBoscoeV1 = "boscoe-v1"

// BoscoeStrategy implements StrategyBoscoeV1.
type BoscoeStrategy struct{}

// NewBoscoeStrategy creates a BoscoeStrategy.
func NewBoscoeStrategy() *BoscoeStrategy { return &BoscoeStrategy{} }

// NextCommand dispatches on g.Phase, mirroring the other Lux-inspired
// strategies' shape. It always returns a zero-value Explanation, since
// Boscoe has no scoring model to report.
func (b *BoscoeStrategy) NextCommand(_ context.Context, g *risk.Game, playerID string) (Command, Explanation, error) {
	cmd, err := b.nextCommand(g, playerID)
	return cmd, Explanation{}, err
}

func (b *BoscoeStrategy) nextCommand(g *risk.Game, playerID string) (Command, error) {
	switch g.Phase {
	case risk.PhaseSetupReinforce:
		return b.setupReinforce(g, playerID)
	case risk.PhaseReinforce:
		return b.reinforce(g, playerID)
	case risk.PhaseAttack:
		return b.attack(g, playerID)
	case risk.PhaseOccupy:
		return b.occupy(g, playerID)
	case risk.PhaseFortify:
		return b.fortify(g, playerID)
	default:
		return Command{}, fmt.Errorf("bot: %s has no move for phase %q", StrategyBoscoeV1, g.Phase)
	}
}

// placementToKillPlayer finds, among every continent target fully owns,
// the cheapest weighted route from pi's own territory into it
// (cheapestRouteToContinentWithCost), returning the owned territory on
// the globally cheapest such route -- Lux's
// SmartAgentBase.placeArmiesToKillPlayer, which compares cost across
// every continent the target owns rather than stopping at the first one
// found.
func placementToKillPlayer(g *risk.Game, pi, target int) (risk.Territory, bool) {
	var best risk.Territory
	bestCost := 0
	found := false
	for _, cont := range continentOrder(g) {
		if !ownsContinent(g, target, cont) {
			continue
		}
		t, cost, ok := cheapestRouteToContinentWithCost(g, pi, cont)
		if !ok {
			continue
		}
		if !found || cost < bestCost {
			best, bestCost, found = t, cost, true
		}
	}
	return best, found
}

// boscoePlacementTerritory picks where BoscoeStrategy places
// reinforcements: if a dominant player must be stopped
// (dominantPlayerToKill), route toward the cheapest of their owned
// continents (placementToKillPlayer); otherwise, the same
// clusterOrTakeContinentPlacement ClusterStrategy/QuoStrategy use --
// Lux's Yakool.placeArmies, which falls through to Cluster's own
// placement logic once placeArmiesToKillDominantPlayer doesn't apply.
func boscoePlacementTerritory(g *risk.Game, pi int, actions []risk.ReinforcementAction) risk.Territory {
	if target, ok := dominantPlayerToKill(g, pi); ok {
		if t, ok := placementToKillPlayer(g, pi, target); ok {
			return t
		}
	}
	return clusterOrTakeContinentPlacement(g, pi, actions)
}

// setupReinforce places the one initial army via boscoePlacementTerritory
// -- Lux's placeInitialArmies delegates straight to placeArmies.
func (b *BoscoeStrategy) setupReinforce(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalSetupReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal setup reinforcement for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	best := boscoePlacementTerritory(g, pi, setupReinforceActionsAsReinforcements(actions))
	return Command{Action: ActionPlaceInitialArmy, Territory: string(best)}, nil
}

// reinforce trades cards whenever a legal set exists (voluntaryCardTurnIn,
// shared with QuoStrategy -- Boscoe's Lux cardsPhase also calls
// cashCardsIfPossible unconditionally), then places via
// boscoePlacementTerritory for the whole pending batch.
func (b *BoscoeStrategy) reinforce(g *risk.Game, playerID string) (Command, error) {
	if cmd, ok := voluntaryCardTurnIn(g, playerID); ok {
		return cmd, nil
	}

	actions := risk.LegalReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal reinforcement for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	best := boscoePlacementTerritory(g, pi, actions)
	return Command{Action: ActionPlaceReinforcement, Territory: string(best), Armies: g.PendingReinforcements}, nil
}

// attackToKillPlayer tries target's owned continents in descending bonus
// order (deterministic tie-break via continentOrder, never map
// iteration), returning the first attack hop from a continent with a
// currently viable route -- Lux's SmartAgentBase.attackToKillPlayer,
// which processes the target's continents biggest bonus first. No
// turn-scoped "already tried this continent" bookkeeping is needed:
// recomputing fresh from current state each NextCommand call naturally
// keeps hammering the same path until it succeeds or stops being viable,
// then moves to the next.
func attackToKillPlayer(g *risk.Game, pi, target int) (Command, bool) {
	var conts []risk.Continent
	for _, cont := range continentOrder(g) {
		if ownsContinent(g, target, cont) {
			conts = append(conts, cont)
		}
	}

	used := make([]bool, len(conts))
	for range conts {
		best := -1
		for i, cont := range conts {
			if used[i] {
				continue
			}
			if best == -1 || g.Board.Continents[cont].Bonus > g.Board.Continents[conts[best]].Bonus {
				best = i
			}
		}
		used[best] = true

		from, to, cost, ok := cheapestAttackHopToContinent(g, pi, conts[best])
		if !ok {
			continue
		}
		src := g.Territories[from]
		// src.Armies must exceed both the route's cost (Lux's own gate --
		// beats the defended path) and 1 (risk.Game.Attack's own hard
		// minimum to attack at all -- cost can be 0 when from directly
		// borders the target continent, so the cost check alone doesn't
		// imply this).
		if src.Armies <= 1 || src.Armies <= cost {
			continue
		}
		return Command{Action: ActionAttack, From: string(from), To: string(to), AttackerDice: min(3, src.Armies-1)}, true
	}
	return Command{}, false
}

// attack tries attackToKillPlayer first if a dominant player must be
// stopped, then falls through to Boscoe's own restrained cluster
// expansion (easy-expand/fill-out/consolidate only -- Lux's Boscoe.
// attackFromCluster override never calls attackSplitUp, unlike
// ClusterStrategy/QuoStrategy's normal sequence), then attackForCard,
// then the shared hogwild/stalemate fallback. attackAsMuchAsPossible is
// reused unmodified (including its own attackSplitUp stage): it's a
// SmartAgentBase method neither Yakool nor Boscoe overrides, so it always
// includes split-up once hogwild/stalemate conditions fire, regardless of
// Boscoe's restrained normal-attack sequence -- Lux_Port_Notes.md's
// Boscoe addendum has more on this distinction.
func (b *BoscoeStrategy) attack(g *risk.Game, playerID string) (Command, error) {
	pi := playerIndex(g, playerID)

	if target, ok := dominantPlayerToKill(g, pi); ok {
		if cmd, ok := attackToKillPlayer(g, pi, target); ok {
			return cmd, nil
		}
	}

	if root, ok := clusterRoot(g, pi); ok {
		for _, stage := range []func(*risk.Game, int, risk.Territory) (Command, bool){
			attackEasyExpand, attackFillOut, attackConsolidate,
		} {
			if cmd, ok := stage(g, pi, root); ok {
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

// occupy is a thin wrapper over clusterOccupyDecision -- Lux's Yakool/
// Boscoe don't override moveArmiesIn, inheriting Cluster's exact
// behavior.
func (b *BoscoeStrategy) occupy(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalOccupations(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal occupation for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	armies := clusterOccupyDecision(g, pi, g.Occupy, actions)
	return Command{Action: ActionOccupy, Armies: armies}, nil
}

// fortify uses bestFortifyDestination (shared with every other
// Lux-inspired strategy so far) -- Lux's Yakool/Boscoe don't override
// fortifyPhase either.
func (b *BoscoeStrategy) fortify(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalFortifications(g, playerID)
	pi := playerIndex(g, playerID)
	best, bestScore, ok := bestFortifyDestination(g, pi, actions)
	if !ok || bestScore == 0 {
		return Command{Action: ActionEndTurn}, nil
	}
	return Command{Action: ActionFortify, From: string(best.From), To: string(best.To), Armies: best.MaxArmies}, nil
}
