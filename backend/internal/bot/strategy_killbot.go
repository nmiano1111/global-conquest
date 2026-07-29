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
// by BetterPixie for its non-kill fallback behavior -- confirmed directly
// in Killbot.java's own constructor (backer = new BetterPixie()), and
// ported as such (see the Lux_Port_Notes.md Killbot addendum for the
// history of the earlier plain-Pixie substitution this replaces).
// Killbot's own distinguishing mechanic: opportunistic elimination
// hunting -- when it has roughly 2x the (card-adjusted) armies of the
// weakest living rival it can plausibly reach, it routes reinforcements
// and attacks toward eliminating that specific player.
const StrategyKillbotV1 = "killbot-v1"

// KillbotStrategy implements StrategyKillbotV1.
type KillbotStrategy struct {
	// backer is Killbot's BetterPixie-backed fallback for every phase
	// that isn't actively pursuing a kill, mirroring Lux's own backer
	// field (Vulture holds a LuxAgent backer and delegates straight to
	// it whenever toKillPlayer is unset).
	backer *BetterPixieStrategy
}

// NewKillbotStrategy creates a KillbotStrategy.
func NewKillbotStrategy() *KillbotStrategy { return &KillbotStrategy{backer: NewBetterPixieStrategy()} }

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

// setupReinforce places the one initial army: if killTarget finds a
// rival to eliminate, the owned territory that starts the cheapest
// attack route toward them (cheapestAttackHopToPlayer's from -- Lux's
// placeToKill, simplified per Lux_Port_Notes.md's Killbot addendum);
// otherwise delegates entirely to the backer's own setupReinforce --
// Lux's placeInitialArmies delegates straight to placeArmies, which
// itself falls through to backer.placeArmies whenever toKillPlayer is
// unset.
func (k *KillbotStrategy) setupReinforce(g *risk.Game, playerID string) (Command, error) {
	pi := playerIndex(g, playerID)
	if target, ok := killTarget(g, pi); ok {
		if from, _, _, ok := cheapestAttackHopToPlayer(g, pi, target); ok {
			return Command{Action: ActionPlaceInitialArmy, Territory: string(from)}, nil
		}
	}
	return k.backer.setupReinforce(g, playerID)
}

// reinforce trades cards whenever a legal set exists (voluntaryCardTurnIn,
// shared with Quo/Boscoe -- Killbot's Lux cardsPhase delegates straight to
// its backer's cardsPhase, BetterPixie's own voluntary cash), then either
// places the whole pending batch on the cheapest route toward killTarget's
// rival, or delegates the whole placement decision to the backer's own
// reinforce -- BetterPixie's placement genuinely needs multiple chunked
// calls across a turn (see strategy_betterpixie.go's betterPixiePlacement),
// so this can't collapse to "pick one territory, dump everything on it"
// the way the plain-Pixie-backed version could.
func (k *KillbotStrategy) reinforce(g *risk.Game, playerID string) (Command, error) {
	if cmd, ok := voluntaryCardTurnIn(g, playerID); ok {
		return cmd, nil
	}
	pi := playerIndex(g, playerID)
	if target, ok := killTarget(g, pi); ok {
		if from, _, _, ok := cheapestAttackHopToPlayer(g, pi, target); ok {
			return Command{Action: ActionPlaceReinforcement, Territory: string(from), Armies: g.PendingReinforcements}, nil
		}
	}
	return k.backer.reinforce(g, playerID)
}

// attack tries killTarget's route first (Lux's Vulture.attackPhase, which
// tries attackToKillPlayer before falling back to the backer's own
// attackPhase entirely), then delegates the whole attack decision to the
// backer -- Vulture.attackPhase calls attackHogWild() unconditionally
// after either branch, not only as a last resort the way
// ClusterStrategy/QuoStrategy/BoscoeStrategy gate it behind their own
// fallback failing, but that distinction only matters while a kill is in
// progress (see Part B of the kill-commitment work, not yet landed) --
// the no-target fallback here is a full, single delegation to the
// backer's own attack, which already ends with its own
// shouldGoHogWild/attackAsMuchAsPossible.
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

	return k.backer.attack(g, playerID)
}

// occupy delegates straight to the backer -- Lux's Vulture.moveArmiesIn
// calls backer.moveArmiesIn(...) unconditionally, with no kill-specific
// override at all.
func (k *KillbotStrategy) occupy(g *risk.Game, playerID string) (Command, error) {
	return k.backer.occupy(g, playerID)
}

// fortify delegates straight to the backer -- Lux's Vulture.fortifyPhase
// also delegates straight to the backer, with no kill-specific override.
func (k *KillbotStrategy) fortify(g *risk.Game, playerID string) (Command, error) {
	return k.backer.fortify(g, playerID)
}
