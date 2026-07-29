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
// rival to eliminate whose territory is reachable as a single cluster
// (targetHasSingleReachableCluster -- Lux's Vulture.placeToKill gate,
// see geometry.go), the owned territory that starts the cheapest attack
// route toward them (cheapestAttackHopToPlayer's from); otherwise
// delegates entirely to the backer's own setupReinforce -- Lux's
// placeInitialArmies delegates straight to placeArmies, which itself
// falls through to backer.placeArmies whenever toKillPlayer is unset.
// This phase has no turn-scoped commitment to record (setup happens
// before any real turn cycle exists, and ApplyGameAction's
// "place_initial_army" case never consults Command.KillTarget), so it
// just recomputes the routing decision live each call, same as before
// Part B.
func (k *KillbotStrategy) setupReinforce(g *risk.Game, playerID string) (Command, error) {
	pi := playerIndex(g, playerID)
	if target, ok := killTarget(g, pi); ok && targetHasSingleReachableCluster(g, target) {
		if from, _, _, ok := cheapestAttackHopToPlayer(g, pi, target); ok {
			return Command{Action: ActionPlaceInitialArmy, Territory: string(from)}, nil
		}
	}
	return k.backer.setupReinforce(g, playerID)
}

// reinforce trades cards whenever a legal set exists (voluntaryCardTurnIn,
// shared with Quo/Boscoe -- Killbot's Lux cardsPhase delegates straight to
// its backer's cardsPhase, BetterPixie's own voluntary cash), then either
// commits to a kill this turn or delegates the whole placement decision
// to the backer -- BetterPixie's placement genuinely needs multiple
// chunked calls across a turn (see strategy_betterpixie.go's
// betterPixiePlacement), so this can't collapse to "pick one territory,
// dump everything on it" the way the plain-Pixie-backed version could.
//
// Committing (Lux's Vulture.setToKillPlayer + placeToKill, called once
// per turn from placeArmies) requires: a target killTarget still finds
// (recomputed fresh, since this engine has no turn-cached toKillPlayer
// field), whose territory is reachable as a single cluster
// (targetHasSingleReachableCluster), and enough armies -- current plus
// this whole pending batch -- to beat the route's cost (Lux's own
// "placeOnCountry.getArmies() + numberOfArmies > ..." gate, previously
// missing from this port entirely). Command.KillTarget records the
// commitment as a side effect of this same PlaceReinforcement call (see
// risk.Game.SetKillPlan) -- attack() then reads it back from
// g.KillPlans instead of re-deciding.
func (k *KillbotStrategy) reinforce(g *risk.Game, playerID string) (Command, error) {
	if cmd, ok := voluntaryCardTurnIn(g, playerID); ok {
		return cmd, nil
	}
	pi := playerIndex(g, playerID)
	if target, ok := killTarget(g, pi); ok && targetHasSingleReachableCluster(g, target) {
		if from, _, cost, ok := cheapestAttackHopToPlayer(g, pi, target); ok {
			src := g.Territories[from]
			if src.Armies+g.PendingReinforcements > cost {
				return Command{
					Action:     ActionPlaceReinforcement,
					Territory:  string(from),
					Armies:     g.PendingReinforcements,
					KillTarget: g.Players[target].ID,
				}, nil
			}
		}
	}
	return k.backer.reinforce(g, playerID)
}

// attack is a hard binary branch matching Lux's Vulture.attackPhase
// exactly: if this turn's reinforce call committed to a kill (g.KillPlans,
// set via SetKillPlan as a side effect of that PlaceReinforcement call --
// see reinforce), walk the route and never fall through to the backer at
// all, only a bare shouldGoHogWild/attackAsMuchAsPossible afterward
// (Vulture.attackPhase calls attackHogWild() unconditionally after either
// branch, not only as a last resort the way ClusterStrategy/QuoStrategy/
// BoscoeStrategy gate it behind their own fallback failing); otherwise
// delegate the whole decision to the backer, which already ends with its
// own shouldGoHogWild/attackAsMuchAsPossible. The commitment is treated
// as spent (falling through to the backer for the rest of this call)
// once the target is eliminated -- cheapestAttackHopToPlayer would
// naturally stop finding a route at that point anyway (the target no
// longer owns anything to search from), but the explicit check avoids
// getting stuck choosing only between hogwild and ending the attack
// phase for the rest of the turn once the kill is already done.
func (k *KillbotStrategy) attack(g *risk.Game, playerID string) (Command, error) {
	pi := playerIndex(g, playerID)

	if plan, ok := g.KillPlans[pi]; ok && plan.Committed && !g.Players[plan.Target].Eliminated {
		if from, to, cost, ok := cheapestAttackHopToPlayer(g, pi, plan.Target); ok {
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
		if shouldGoHogWild(g, pi) {
			if cmd, ok := attackAsMuchAsPossible(g, pi); ok {
				return cmd, nil
			}
		}
		return Command{Action: ActionEndAttack}, nil
	}

	return k.backer.attack(g, playerID)
}

// occupy forces the maximum legal army count whenever this player has an
// active, uneliminated kill commitment, instead of delegating to the
// backer -- Lux's Vulture.attackAlongRoute sets
// (backer).moveInMemory = 1000000 before every attack along the kill
// route, forcing the backer's own moveArmiesIn to move everyone in via
// its memoryMoveArmiesInTest short-circuit, so the pursuing stack is
// never diluted mid-chase. moveInMemory itself was already noted
// (Lux_Port_Notes.md's Pixie addendum) as not needing a port -- true for
// Cluster/Pixie's own single-call use of it, but that decision predates
// Killbot's specific repurposing of the same flag for this route-wide
// effect, which needs its own explicit handling here instead.
// Simplification: this forces max armies for any conquest while a kill
// is committed, not only conquests along the specific routed hop (Lux's
// own attackHogWild, called only after the route completes or fails,
// does not get this treatment) -- committed-mode hogwild conquests only
// happen when the routed hop itself couldn't fire this turn, so treating
// them the same way is a deliberate, minor over-application, not a
// meaningfully different posture.
func (k *KillbotStrategy) occupy(g *risk.Game, playerID string) (Command, error) {
	pi := playerIndex(g, playerID)
	if plan, ok := g.KillPlans[pi]; ok && plan.Committed && !g.Players[plan.Target].Eliminated {
		actions := risk.LegalOccupations(g, playerID)
		if len(actions) == 0 {
			return Command{}, fmt.Errorf("bot: no legal occupation for player %s", playerID)
		}
		return Command{Action: ActionOccupy, Armies: actions[len(actions)-1].Armies}, nil
	}
	return k.backer.occupy(g, playerID)
}

// fortify delegates straight to the backer -- Lux's Vulture.fortifyPhase
// also delegates straight to the backer, with no kill-specific override.
func (k *KillbotStrategy) fortify(g *risk.Game, playerID string) (Command, error) {
	return k.backer.fortify(g, playerID)
}
