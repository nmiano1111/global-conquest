package bot

import (
	"context"
	"fmt"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// StrategyKillbotV2 is the registry identifier for KillbotV2Strategy, an
// experimental variant of StrategyKillbotV1 carrying three targeted fixes
// on top of it. Rather than editing the shared code StrategyKillbotV1's
// BetterPixie backer relies on (also used by Cluster/Quo/Boscoe/Pixie's
// own placement/attack paths, and changing StrategyKillbotV1's live
// behavior out from under any in-progress game), this embeds
// KillbotStrategy and overrides only the specific decisions below --
// occupy and fortify delegate straight through unchanged.
//
//  1. Locked-in reinforcement (guardPlacement): placeToTakeSpecificContinent
//     (strategy_cluster.go, used by the BetterPixie backer) counts enemy
//     neighbors *within the target continent only* when picking where to
//     reinforce a continent it already owns outright but whose border is
//     weak (betterPixieContinentNeedsHelp's "own it but a border needs
//     help" branch). Once every territory in that continent is pi-owned,
//     there is no non-owned territory left inside it to find, so every
//     candidate scores 0 enemies and the tie-break collapses to
//     "whichever territory sorts first in canonical board order" -- a
//     pick with no relationship to which territory is actually the weak,
//     attackable border. The result: StrategyKillbotV1 can keep dumping
//     reinforcements onto the same arbitrary, often fully interior,
//     territory turn after turn -- one with zero real enemy neighbors,
//     unable to ever attack no matter how many armies pile up. Fixed by
//     redirecting to the best legal territory that does have an
//     attackable neighbor (bestByEnemyNeighborCount) whenever the
//     underlying pick has none.
//  2. Board-relative hogwild threshold (killbotV2ShouldGoHogWild):
//     shouldGoHogWild's `own > 1500` absolute cap (strategy_cluster.go)
//     is a constant tuned for Lux Delux's own ~42-territory classic map
//     and doesn't scale to this project's custom, differently-sized maps
//     (see CLAUDE.md's Custom Maps feature). Replaced with a per-territory
//     army threshold that lands at roughly the same absolute cap on a
//     42-territory board (continuity with StrategyKillbotV1's tuning) but
//     scales correctly for smaller or larger custom maps.
//  3. Stalled kill commitment falls through, not ends attack (attack):
//     StrategyKillbotV1, once committed to a kill, never falls back to
//     the backer's ordinary attack repertoire this turn -- if the route
//     can't fire (target's cluster now unreachable, insufficient armies)
//     and hogwild doesn't apply either, it unconditionally ends the
//     attack phase (matching Lux's own Vulture.attackPhase). That
//     forfeits whatever ordinary, unrelated attacks might still be
//     perfectly good this turn. KillbotV2Strategy instead falls through
//     to the backer's normal attack() in that case.
//  4. Real win probability replaces the cost check (reinforce, attack):
//     StrategyKillbotV1 decides whether to commit to, and later fire, a
//     kill-route attack via `src.Armies > cost` (reinforce) and
//     `src.Armies > cost` again (attack) -- but cost
//     (cheapestAttackHopToPlayer, geometry.go) only accumulates the army
//     counts of *third-party* territories crossed en route to reach pi's
//     own border; cheapestRouteSearch treats every one of the target's
//     own territories as a free, zero-cost starting point. So whenever pi
//     directly borders the target -- cheapestAttackHopToPlayer's
//     overwhelmingly common case -- cost is 0 regardless of how many
//     armies the target actually has there, and StrategyKillbotV1
//     commits/attacks unconditionally, never actually checking whether
//     the fight is winnable. Both decisions now call risk.ForecastAttack
//     (internal/risk/combat_odds.go, exact dice-probability math already
//     used elsewhere in this package) against the real defending
//     territory and require killbotV2MinAttackWinProbability before
//     committing or firing.
//  5. Win probability applied broadly, via a pluggable comparator, not
//     just KillbotV2Strategy's own kill-route decision: the shared
//     attack-stage helpers underneath the BetterPixie backer
//     (attackEasyExpand, attackFillOut, attackConsolidate,
//     attackInContinent, betterPixieAttackInContinent's pass-1 check, and
//     attackAsMuchAsPossible) all take an attackComparator now
//     (geometry.go) instead of hard-coding `attacker > defender`.
//     KillbotV2Strategy constructs its own BetterPixieStrategy (the
//     backer field below) via WithAttackComparator(winProbabilityComparator(...)),
//     completely separate from the embedded KillbotStrategy's own
//     raw-comparator backer -- so essentially every attack decision this
//     strategy makes, not just the kill-hunting one, is win-probability
//     aware. StrategyKillbotV1 and every other persona
//     (Cluster/Quo/Boscoe/Pixie/Turtle/KillbotOld) pass rawArmyComparator
//     explicitly at each of these call sites, so their live behavior is
//     unchanged by this threading.
//  6. Lower kill-hunting advantage ratio (killbotV2KillAdvantageRatio):
//     killTarget's default 2x requirement (killTargetDefaultAdvantageRatio,
//     geometry.go -- StrategyKillbotV1 still uses it unchanged) is a
//     high bar in a real multiplayer game. Lowered to 1.5x so kill-hunting
//     commits more readily -- though follow-up measurement (see point 8's
//     doc comment) found neither this ratio nor
//     targetHasSingleReachableCluster's cluster-size cap were actually
//     the long-game bottleneck: sweeping the cap from 20 up to 42
//     (effectively uncapped) across a 150-game tournament left avg turns
//     and avg card sets traded flat to very slightly worse. Kept anyway
//     as a real, harmless improvement (verified via
//     TestKillbotV2CommitsAtLowerAdvantageRatioThanV1), just not the fix
//     for game length it was originally guessed to be.
//  7. Every threshold above is admin-tunable (effectiveBotTuning,
//     tuning.go): KillbotV2Strategy reads risk.Game.BotTuning (snapshotted
//     once at game creation from the admin's current config, see
//     internal/service's BotTuningConfig) instead of fixed package
//     constants, falling back to DefaultBotTuning() -- today's values,
//     below -- for any game with no snapshot. Because the same shared
//     KillbotV2Strategy instance serves every game, the BetterPixie
//     backer (win-probability-configured, point 5) can no longer be a
//     fixed field either -- backerFor builds one fresh per decision from
//     whichever tuning that specific game was created with.
//  8. Endgame stalemate breaker (killbotV2ShouldGoHogWild's alive-count
//     check, plus attack()'s restructuring below): shouldGoHogWild's
//     `own > total-own` trigger requires an outright majority of every
//     army on the board -- a bar two roughly matched survivors in a
//     late-game 1v1 can never clear, no matter how long they grind, since
//     neither ever pulls decisively ahead while both keep reinforcing
//     every turn. Measured directly: a 300-game mixed-persona tournament
//     had 83 games (27.7%) fail to resolve within the safety limits at
//     all, and 58 of those 83 (70%) were stuck in exactly this shape --
//     two survivors, thousands of combat rolls, hundreds of captures
//     traded back and forth, no elimination. Not killbot-v2-specific
//     either (one observed standoff was quo-v1 vs. pixie-v1), but
//     killbot-v2 is the only persona free to fix its own hogwild trigger
//     without touching the shared shouldGoHogWild every other persona
//     still uses unmodified. killbotV2ShouldGoHogWild now also triggers
//     once few enough rivals remain (killbotV2EndgameAliveThreshold)
//     regardless of army share -- but two follow-on fixes in attack()
//     were needed before this actually helped: the check was originally
//     only reached inside the "kill committed" branch, unreachable in
//     exactly the standoffs it targets (a close 1v1 rarely has an active
//     kill commitment at all), and attackAsMuchAsPossible was still being
//     called with the win-probability comparator, which silently defeats
//     hogwild's whole point of attacking aggressively despite normal
//     caution. Fixed: the check now runs unconditionally, and passes
//     rawArmyComparator. Re-measured after both fixes: killbot-v2's win
//     rate in that same mixed-persona tournament rose from 58.8% to
//     62.6%, and killbot-v2-involved 2-player standoffs dropped from
//     roughly 50 to 16 (the remaining ~28 of the tournament's 70
//     still-failing games are standoffs between two *other* personas,
//     outside what a killbot-v2-only fix can reach).
const StrategyKillbotV2 = "killbot-v2"

// killbotV2HogWildArmiesPerTerritory is DefaultBotTuning's
// HogwildArmiesPerTerritory: killbotV2ShouldGoHogWild's per-territory army
// threshold. 36 lands at 1512 armies on a 42-territory board --
// StrategyKillbotV1's flat 1500 cap, near-exactly -- while scaling with
// the board's actual territory count for custom maps.
const killbotV2HogWildArmiesPerTerritory = 36

// killbotV2EndgameAliveThreshold is killbotV2ShouldGoHogWild's endgame
// stalemate-breaker threshold (point 8 above): once this many or fewer
// players remain in the game, killbot-v2 goes hogwild regardless of army
// share -- a 2-player standoff was, by a wide margin, the dominant shape
// of the 27.7% of a mixed-persona tournament sample that never resolved
// at all. 2 targets that dominant case directly without reaching into
// earlier-game states (3+ players still alive) that weren't observed to
// be a problem.
const killbotV2EndgameAliveThreshold = 2

// killbotV2MinAttackWinProbability is DefaultBotTuning's
// MinAttackWinProbability: the minimum risk.ForecastAttack win probability
// KillbotV2 requires before committing to, or firing, a kill-route attack
// -- see doc comment above (point 4). 0.5 requires being actually favored
// to win, not merely "not obviously suicidal."
const killbotV2MinAttackWinProbability = 0.5

// killbotV2KillAdvantageRatio is DefaultBotTuning's KillAdvantageRatio,
// replacing killTargetDefaultAdvantageRatio (2.0) as the army-advantage
// KillbotV2 requires before committing to a kill hunt -- see doc comment
// above (point 6). 1.5 still requires a real material edge, just a
// smaller one, so kill-hunting engages more often without committing to
// fights killbotV2MinAttackWinProbability's own gate would decline anyway.
const killbotV2KillAdvantageRatio = 1.5

// attackBiasFunc, when set, gives a wrapping strategy an opinion on one
// specific candidate attack: how much to relax the normal minimum
// win-probability bar for that (from, to) pair this turn, plus a short
// reason string for debugging/explainability. Returns (0, "") for "no
// opinion." This exists because attackComparator (geometry.go) has no
// territory identity -- it can't express "relax the bar for this
// specific candidate," only "change the bar for every candidate this
// call scores" -- so a per-candidate bias needs its own hook rather than
// reusing the comparator mechanism. nil is a true no-op: every check
// below is skipped entirely, so a KillbotV2Strategy built without one
// (every existing caller) is byte-for-byte unchanged. First built and
// validated for a single-decision bias (see
// project-docs/bot_player/phase_3_continuous_improvement/
// 12_Symbolic_Strategic_Reasoning_Layer.md); re-added here to serve
// GoalKillbotStrategy's persistent-goal attack bias -- the hook itself
// is unchanged from that earlier, already-proven-safe version.
type attackBiasFunc func(g *risk.Game, pi int, from, to risk.Territory) (delta float64, reason string)

// KillbotV2Strategy implements StrategyKillbotV2.
type KillbotV2Strategy struct {
	*KillbotStrategy
	bias attackBiasFunc // nil by default -- see attackBiasFunc's doc comment
}

// WithAttackBias sets an optional per-candidate attack-bias hook,
// mirroring BetterPixieStrategy's WithAttackComparator option
// (strategy_betterpixie.go) in shape: a functional option a wrapping
// strategy supplies at construction, never touched by KillbotV2Strategy
// itself beyond storing and consulting it.
func WithAttackBias(b attackBiasFunc) func(*KillbotV2Strategy) {
	return func(k *KillbotV2Strategy) { k.bias = b }
}

// NewKillbotV2Strategy creates a KillbotV2Strategy. With no options, its
// behavior is identical to before opts existed -- opts is additive and
// every existing zero-arg call site keeps compiling unchanged.
func NewKillbotV2Strategy(opts ...func(*KillbotV2Strategy)) *KillbotV2Strategy {
	k := &KillbotV2Strategy{KillbotStrategy: NewKillbotStrategy()}
	for _, opt := range opts {
		opt(k)
	}
	return k
}

// backerFor builds KillbotV2Strategy's BetterPixieStrategy backer,
// configured with a win-probability attackComparator at tuning's
// threshold (point 5/7 above) -- deliberately separate from the embedded
// KillbotStrategy's own backer field (still on the default
// rawArmyComparator), which setupReinforce/occupy/fortify below still
// delegate to unchanged, since none of those phases make an attack-stage
// decision. Built fresh per decision rather than cached: the same shared
// KillbotV2Strategy instance serves every game, and different games can
// carry different tuning snapshots (point 7) -- BetterPixieStrategy is a
// trivial, stateless struct, so this costs nothing worth caching.
func (k *KillbotV2Strategy) backerFor(tuning risk.BotTuning) *BetterPixieStrategy {
	return NewBetterPixieStrategy(WithAttackComparator(winProbabilityComparator(tuning.MinAttackWinProbability)))
}

// NextCommand dispatches on g.Phase, mirroring KillbotStrategy's own
// shape. It always returns a zero-value Explanation, since KillbotV2 has
// no scoring model to report.
func (k *KillbotV2Strategy) NextCommand(_ context.Context, g *risk.Game, playerID string) (Command, Explanation, error) {
	cmd, err := k.nextCommand(g, playerID)
	return cmd, Explanation{}, err
}

func (k *KillbotV2Strategy) nextCommand(g *risk.Game, playerID string) (Command, error) {
	switch g.Phase {
	case risk.PhaseSetupReinforce:
		return k.setupReinforce(g, playerID)
	case risk.PhaseReinforce:
		return k.reinforce(g, playerID)
	case risk.PhaseAttack:
		return k.attack(g, playerID)
	case risk.PhaseOccupy:
		return k.KillbotStrategy.occupy(g, playerID)
	case risk.PhaseFortify:
		return k.KillbotStrategy.fortify(g, playerID)
	default:
		return Command{}, fmt.Errorf("bot: %s has no move for phase %q", StrategyKillbotV2, g.Phase)
	}
}

// setupReinforce delegates to KillbotStrategy then guards the chosen
// territory against the locked-in-territory bug (see doc comment above).
func (k *KillbotV2Strategy) setupReinforce(g *risk.Game, playerID string) (Command, error) {
	cmd, err := k.KillbotStrategy.setupReinforce(g, playerID)
	if err != nil {
		return cmd, err
	}
	actions := setupReinforceActionsAsReinforcements(risk.LegalSetupReinforcements(g, playerID))
	return k.guardPlacement(g, playerID, cmd, actions), nil
}

// reinforce mirrors KillbotStrategy.reinforce (strategy_killbot.go), with
// the commitment gate replaced by a real win-probability check (see doc
// comment above, point 4) and every resulting placement (whether
// committing or falling through to the backer) guarded against the
// locked-in-territory bug (point 1).
func (k *KillbotV2Strategy) reinforce(g *risk.Game, playerID string) (Command, error) {
	if cmd, ok := voluntaryCardTurnIn(g, playerID); ok {
		return cmd, nil
	}
	tuning := effectiveBotTuning(g)
	pi := playerIndex(g, playerID)
	if target, ok := killTarget(g, pi, tuning.KillAdvantageRatio); ok && targetHasSingleReachableCluster(g, target, tuning.KillClusterMaxSize) {
		if from, to, _, ok := cheapestAttackHopToPlayer(g, pi, target); ok {
			src := g.Territories[from]
			forecast := risk.ForecastAttack(src.Armies+g.PendingReinforcements, g.Territories[to].Armies)
			if forecast.WinProbability >= tuning.MinAttackWinProbability {
				cmd := Command{
					Action:     ActionPlaceReinforcement,
					Territory:  string(from),
					Armies:     g.PendingReinforcements,
					KillTarget: g.Players[target].ID,
				}
				return k.guardPlacement(g, playerID, cmd, risk.LegalReinforcements(g, playerID)), nil
			}
		}
	}

	cmd, err := k.backerFor(tuning).reinforce(g, playerID)
	if err != nil || cmd.Action != ActionPlaceReinforcement {
		return cmd, err
	}
	return k.guardPlacement(g, playerID, cmd, risk.LegalReinforcements(g, playerID)), nil
}

// guardPlacement redirects cmd's Territory to the best legal, currently
// attackable alternative if the underlying strategy chose a territory
// with zero attackable (non-owned) neighbors right now. A locked-in
// territory can never be the committed kill route
// (cheapestAttackHopToPlayer's from is always adjacent to its returned
// to), so this redirect never fires on a real kill commitment -- KillTarget
// is cleared anyway since a redirected command is no longer the one that
// route decision reasoned about.
func (k *KillbotV2Strategy) guardPlacement(g *risk.Game, playerID string, cmd Command, actions []risk.ReinforcementAction) Command {
	pi := playerIndex(g, playerID)
	if enemyNeighborCount(g, risk.Territory(cmd.Territory), pi) > 0 || len(actions) == 0 {
		return cmd
	}
	get := func(a risk.ReinforcementAction) risk.Territory { return a.Territory }
	cmd.Territory = string(bestByEnemyNeighborCount(g, pi, actions, get))
	cmd.KillTarget = ""
	return cmd
}

// killbotV2ShouldGoHogWild is shouldGoHogWild (strategy_cluster.go) with
// its absolute 1500-army cap replaced by armiesPerTerritory times the
// board's actual territory count (point 2), plus an endgame
// stalemate-breaker (point 8): once killbotV2EndgameAliveThreshold or
// fewer players remain, hogwild fires regardless of army share, since
// shouldGoHogWild's own `own > total-own` majority requirement can never
// be satisfied by either side of a genuinely close standoff.
func killbotV2ShouldGoHogWild(g *risk.Game, pi int, armiesPerTerritory int) bool {
	total, own := 0, 0
	for _, t := range g.Board.Order {
		ts := g.Territories[t]
		total += ts.Armies
		if ts.Owner == pi {
			own += ts.Armies
		}
	}
	if own > total-own {
		return true
	}
	if own > len(g.Board.Order)*armiesPerTerritory {
		return true
	}

	alive := 0
	for _, p := range g.Players {
		if !p.Eliminated {
			alive++
		}
	}
	return alive <= killbotV2EndgameAliveThreshold
}

// attack mirrors KillbotStrategy.attack (strategy_killbot.go), with four
// changes: a real win-probability check in place of the cost check
// (point 4), and falling through to the backer's normal attack() (rather
// than unconditionally ending the attack phase) once a committed kill
// route can't fire this turn (point 3) -- see doc comment above. The
// remaining two concern killbotV2ShouldGoHogWild specifically (points 2
// and 8): it is now checked unconditionally, not just while a kill is
// actively committed -- BetterPixieStrategy.attack() (backer.attack,
// reached whenever no kill is committed, or a committed one's route can't
// fire this turn either) calls the shared, unmodified shouldGoHogWild
// internally, which the endgame stalemate-breaker (point 8) specifically
// needs to bypass, so checking only inside the "committed" branch left it
// unreachable in exactly the standoffs it targets. And it now attacks
// with rawArmyComparator, not the backer's win-probability comparator:
// hogwild exists to force a resolution by attacking aggressively despite
// normal caution, which win-probability-gating attackAsMuchAsPossible's
// own candidate search would silently defeat.
func (k *KillbotV2Strategy) attack(g *risk.Game, playerID string) (Command, error) {
	tuning := effectiveBotTuning(g)
	pi := playerIndex(g, playerID)
	backer := k.backerFor(tuning)

	if plan, ok := g.KillPlans[pi]; ok && plan.Committed && !g.Players[plan.Target].Eliminated {
		if from, to, _, ok := cheapestAttackHopToPlayer(g, pi, plan.Target); ok {
			src := g.Territories[from]
			if src.Armies > 1 {
				forecast := risk.ForecastAttack(src.Armies, g.Territories[to].Armies)
				if forecast.WinProbability >= tuning.MinAttackWinProbability {
					return Command{Action: ActionAttack, From: string(from), To: string(to), AttackerDice: min(3, src.Armies-1)}, nil
				}
			}
		}
	}

	if k.bias != nil {
		if cmd, ok := k.biasedAttack(g, pi, playerID, tuning); ok {
			return cmd, nil
		}
	}

	if killbotV2ShouldGoHogWild(g, pi, tuning.HogwildArmiesPerTerritory) {
		if cmd, ok := attackAsMuchAsPossible(g, pi, rawArmyComparator); ok {
			return cmd, nil
		}
	}

	return backer.attack(g, playerID)
}

// biasedAttack scans every legal attack for the single candidate k.bias
// has the strongest opinion on (largest relaxation), firing it if the
// relaxed bar clears real risk.ForecastAttack math -- the same direct
// ForecastAttack pattern the committed-kill-route check above already
// uses, just against a lowered bar instead of the flat one. Picks the
// best candidate across all of risk.LegalAttacks, not the first match,
// consistent with this file's other "find the best candidate" helpers
// (killTarget, biggestArmyTerritoryWithEnemyNeighbor in geometry.go).
func (k *KillbotV2Strategy) biasedAttack(g *risk.Game, pi int, playerID string, tuning risk.BotTuning) (Command, bool) {
	bestDelta := 0.0
	var best risk.AttackAction
	found := false
	for _, a := range risk.LegalAttacks(g, playerID) {
		delta, _ := k.bias(g, pi, a.From, a.To)
		if delta > bestDelta {
			bestDelta, best, found = delta, a, true
		}
	}
	if !found {
		return Command{}, false
	}
	src := g.Territories[best.From]
	forecast := risk.ForecastAttack(src.Armies, g.Territories[best.To].Armies)
	if forecast.WinProbability < tuning.MinAttackWinProbability-bestDelta {
		return Command{}, false
	}
	return Command{Action: ActionAttack, From: string(best.From), To: string(best.To), AttackerDice: min(3, src.Armies-1)}, true
}
