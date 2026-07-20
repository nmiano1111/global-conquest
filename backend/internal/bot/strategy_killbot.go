package bot

import (
	"context"
	"fmt"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// StrategyKillbotV1 is the registry identifier for KillbotStrategy, the
// sixth and final Lux Delux-inspired persona (see
// project-docs/bot_player/proposals/Lux_Delux_AI_Research_Notes.md and
// Lux_Port_Notes.md). Killbot extends Vulture in Lux's hierarchy, backed
// by BetterPixie for its non-kill fallback behavior; per the Phase-3
// scoping decision (plain Pixie is the persona the paper's roster
// actually names), that fallback reuses PixieStrategy's already-shipped
// logic directly rather than re-deriving BetterPixie's marginally more
// sophisticated variant. Killbot's own distinguishing mechanic:
// opportunistic elimination hunting -- when it has roughly 2x the
// (card-adjusted) armies of the weakest living rival it can plausibly
// reach, it routes reinforcements and attacks toward eliminating that
// specific player.
const StrategyKillbotV1 = "killbot-v1"

// KillbotStrategy implements StrategyKillbotV1.
type KillbotStrategy struct {
	// backer is Killbot's Pixie-backed fallback for occupy, mirroring
	// Lux's own backer field (Vulture holds a LuxAgent backer and
	// delegates moveArmiesIn straight to it).
	backer *PixieStrategy
}

// NewKillbotStrategy creates a KillbotStrategy.
func NewKillbotStrategy() *KillbotStrategy { return &KillbotStrategy{backer: NewPixieStrategy()} }

// NextCommand dispatches on g.Phase, mirroring the other Lux-inspired
// strategies' shape. It always returns a zero-value Explanation, since
// Killbot has no scoring model to report.
func (k *KillbotStrategy) NextCommand(_ context.Context, g *risk.Game, playerID string) (Command, Explanation, error) {
	cmd, err := k.nextCommand(g, playerID)
	return cmd, Explanation{}, err
}

func (k *KillbotStrategy) nextCommand(g *risk.Game, playerID string) (Command, error) {
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
		return Command{}, fmt.Errorf("bot: %s has no move for phase %q", StrategyKillbotV1, g.Phase)
	}
}

// killbotPlacementTerritory picks where KillbotStrategy places
// reinforcements: if killTarget finds a rival to eliminate, the owned
// territory that starts the cheapest attack route toward them
// (cheapestAttackHopToPlayer's from -- Lux's placeToKill, simplified per
// Lux_Port_Notes.md's Killbot addendum); otherwise pixiePlacementTerritory
// (Lux's own fallback to the backer's placeArmies).
func killbotPlacementTerritory(g *risk.Game, pi int, actions []risk.ReinforcementAction) risk.Territory {
	if target, ok := killTarget(g, pi); ok {
		if from, _, _, ok := cheapestAttackHopToPlayer(g, pi, target); ok {
			return from
		}
	}
	return pixiePlacementTerritory(g, pi, actions)
}

// setupReinforce places the one initial army via killbotPlacementTerritory
// -- Lux's placeInitialArmies delegates straight to placeArmies.
func (k *KillbotStrategy) setupReinforce(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalSetupReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal setup reinforcement for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	reinforceActions := make([]risk.ReinforcementAction, len(actions))
	for i, a := range actions {
		reinforceActions[i] = risk.ReinforcementAction{Territory: a.Territory}
	}
	best := killbotPlacementTerritory(g, pi, reinforceActions)
	return Command{Action: ActionPlaceInitialArmy, Territory: string(best)}, nil
}

// reinforce trades cards whenever a legal set exists (voluntaryCardTurnIn,
// shared with Quo/Boscoe -- Killbot's Lux cardsPhase delegates straight to
// its backer's cardsPhase, BetterPixie's own voluntary cash), then places
// via killbotPlacementTerritory for the whole pending batch.
func (k *KillbotStrategy) reinforce(g *risk.Game, playerID string) (Command, error) {
	if cmd, ok := voluntaryCardTurnIn(g, playerID); ok {
		return cmd, nil
	}

	actions := risk.LegalReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal reinforcement for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	best := killbotPlacementTerritory(g, pi, actions)
	return Command{Action: ActionPlaceReinforcement, Territory: string(best), Armies: g.PendingReinforcements}, nil
}

// attack tries killTarget's route first (Lux's Vulture.attackPhase, which
// tries attackToKillPlayer before falling back to the backer's own
// attackPhase), then Pixie's continent-scoped attack sequence, then
// unconditionally shouldGoHogWild/attackAsMuchAsPossible -- Lux's
// Vulture.attackPhase calls attackHogWild() after either branch, not
// only as a last resort the way ClusterStrategy/QuoStrategy/
// BoscoeStrategy gate it behind their own fallback failing.
func (k *KillbotStrategy) attack(g *risk.Game, playerID string) (Command, error) {
	pi := playerIndex(g, playerID)

	if target, ok := killTarget(g, pi); ok {
		if from, to, cost, ok := cheapestAttackHopToPlayer(g, pi, target); ok {
			src := g.Territories[from]
			// src.Armies must exceed both the route's cost and 1
			// (risk.Game.Attack's own hard minimum to attack at all --
			// cost can be 0 when from directly borders target, so the
			// cost check alone doesn't imply this; see
			// Lux_Port_Notes.md's Boscoe addendum for the identical fix
			// attackToKillPlayer needed).
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

// occupy delegates straight to the backer -- Lux's Vulture.moveArmiesIn
// calls backer.moveArmiesIn(...) unconditionally, with no kill-specific
// override at all.
func (k *KillbotStrategy) occupy(g *risk.Game, playerID string) (Command, error) {
	return k.backer.occupy(g, playerID)
}

// fortify uses bestFortifyDestination (shared with every other
// Lux-inspired strategy so far) -- Lux's Vulture.fortifyPhase also
// delegates straight to the backer, whose own fortifyPhase collapses to
// this same shared helper under this engine's one-fortify-per-turn limit.
func (k *KillbotStrategy) fortify(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalFortifications(g, playerID)
	pi := playerIndex(g, playerID)
	best, bestScore, ok := bestFortifyDestination(g, pi, actions)
	if !ok || bestScore == 0 {
		return Command{Action: ActionEndTurn}, nil
	}
	return Command{Action: ActionFortify, From: string(best.From), To: string(best.To), Armies: best.MaxArmies}, nil
}
