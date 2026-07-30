package bot

import (
	"context"
	"fmt"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// StrategyKillbotOldV1 is the registry identifier for KillbotOldStrategy,
// a frozen snapshot of KillbotStrategy as it existed before two fidelity
// fixes landed: swapping its backer from plain PixieStrategy to a real
// port of BetterPixie, and replacing its per-call killTarget recompute
// with turn-scoped commitment (risk.Game.KillPlans, gated by
// targetHasSingleReachableCluster). Kept side-by-side with the current
// killbot-v1 -- not as a fidelity claim (it's deliberately the *less*
// faithful version) -- so both remain available for comparison (e.g. as
// a distinct opponent persona in training-data lineups) without losing
// either behavior.
const StrategyKillbotOldV1 = "killbot-old-v1"

// KillbotOldStrategy implements StrategyKillbotOldV1.
type KillbotOldStrategy struct {
	// backer is Killbot's Pixie-backed fallback for occupy -- the
	// pre-fix version substituted plain PixieStrategy for Lux's real
	// BetterPixie backer (see StrategyKillbotOldV1's doc comment).
	backer *PixieStrategy
}

// NewKillbotOldStrategy creates a KillbotOldStrategy.
func NewKillbotOldStrategy() *KillbotOldStrategy {
	return &KillbotOldStrategy{backer: NewPixieStrategy()}
}

// NextCommand dispatches on g.Phase, mirroring the other Lux-inspired
// strategies' shape. It always returns a zero-value Explanation, since
// Killbot has no scoring model to report.
func (k *KillbotOldStrategy) NextCommand(_ context.Context, g *risk.Game, playerID string) (Command, Explanation, error) {
	cmd, err := k.nextCommand(g, playerID)
	return cmd, Explanation{}, err
}

func (k *KillbotOldStrategy) nextCommand(g *risk.Game, playerID string) (Command, error) {
	switch g.Phase {
	case risk.PhaseSetupReinforce:
		return k.setupReinforce(g, playerID)
	case risk.PhaseReinforce:
		return k.reinforce(g, playerID)
	case risk.PhaseAttack:
		return k.attack(g, playerID)
	case risk.PhaseOccupy:
		return k.occupy(g, playerID)
	case risk.PhaseFortify:
		return k.fortify(g, playerID)
	default:
		return Command{}, fmt.Errorf("bot: %s has no move for phase %q", StrategyKillbotOldV1, g.Phase)
	}
}

// killbotOldPlacementTerritory picks where KillbotOldStrategy places
// reinforcements: if killTarget finds a rival to eliminate, the owned
// territory that starts the cheapest attack route toward them
// (cheapestAttackHopToPlayer's from); otherwise pixiePlacementTerritory
// (the backer's own placement).
func killbotOldPlacementTerritory(g *risk.Game, pi int, actions []risk.ReinforcementAction) risk.Territory {
	if target, ok := killTarget(g, pi); ok {
		if from, _, _, ok := cheapestAttackHopToPlayer(g, pi, target); ok {
			return from
		}
	}
	return pixiePlacementTerritory(g, pi, actions)
}

// setupReinforce places the one initial army via
// killbotOldPlacementTerritory.
func (k *KillbotOldStrategy) setupReinforce(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalSetupReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal setup reinforcement for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	reinforceActions := make([]risk.ReinforcementAction, len(actions))
	for i, a := range actions {
		reinforceActions[i] = risk.ReinforcementAction{Territory: a.Territory}
	}
	best := killbotOldPlacementTerritory(g, pi, reinforceActions)
	return Command{Action: ActionPlaceInitialArmy, Territory: string(best)}, nil
}

// reinforce trades cards whenever a legal set exists, then places via
// killbotOldPlacementTerritory for the whole pending batch.
func (k *KillbotOldStrategy) reinforce(g *risk.Game, playerID string) (Command, error) {
	if cmd, ok := voluntaryCardTurnIn(g, playerID); ok {
		return cmd, nil
	}

	actions := risk.LegalReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal reinforcement for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	best := killbotOldPlacementTerritory(g, pi, actions)
	return Command{Action: ActionPlaceReinforcement, Territory: string(best), Armies: g.PendingReinforcements}, nil
}

// attack tries killTarget's route first (recomputed fresh every call,
// with no turn-scoped commitment and no fragmentation gate), then
// Pixie's continent-scoped attack sequence, then unconditionally
// shouldGoHogWild/attackAsMuchAsPossible.
func (k *KillbotOldStrategy) attack(g *risk.Game, playerID string) (Command, error) {
	pi := playerIndex(g, playerID)

	if target, ok := killTarget(g, pi); ok {
		if from, to, cost, ok := cheapestAttackHopToPlayer(g, pi, target); ok {
			src := g.Territories[from]
			if src.Armies > 1 && src.Armies > cost {
				return Command{Action: ActionAttack, From: string(from), To: string(to), AttackerDice: min(3, src.Armies-1)}, nil
			}
		}
	}

	wanted := pixieWantedContinents(g, pi)
	for _, cont := range continentOrder(g) {
		if !wanted[cont] {
			continue
		}
		if cmd, ok := attackInContinent(g, pi, cont); ok {
			return cmd, nil
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

// occupy delegates straight to the backer -- no kill-specific override.
func (k *KillbotOldStrategy) occupy(g *risk.Game, playerID string) (Command, error) {
	return k.backer.occupy(g, playerID)
}

// fortify uses bestFortifyDestination (shared with every other
// Lux-inspired strategy).
func (k *KillbotOldStrategy) fortify(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalFortifications(g, playerID)
	pi := playerIndex(g, playerID)
	best, bestScore, ok := bestFortifyDestination(g, pi, actions)
	if !ok || bestScore == 0 {
		return Command{Action: ActionEndTurn}, nil
	}
	return Command{Action: ActionFortify, From: string(best.From), To: string(best.To), Armies: best.MaxArmies}, nil
}
