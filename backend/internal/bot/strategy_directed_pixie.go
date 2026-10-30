package bot

import (
	"fmt"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// Directive is a strategic instruction a director-style Strategy hands
// DirectedPixieStrategy for one decision. The zero value is a no-op: an
// executor given an empty Directive behaves identically to plain
// BetterPixieStrategy, verified by TestDirectedPixieEmptyDirectiveMatchesPlainBetterPixie.
type Directive struct {
	// WantedContinents, when non-empty, is UNIONED with (never replaces)
	// whatever the executor's own betterPixieWantedContinents would
	// independently choose -- a director adds a priority, it doesn't
	// override the executor's own judgment about what else to defend or
	// pursue. Once the executor owns anything, betterPixieWantedContinents'
	// own "budget" branch already supports wanting multiple continents at
	// once, so unioning in the director's pick lets that existing
	// multi-continent balancing keep working instead of being bypassed --
	// this is the direct fix for goal-killbot-v2's diagnosed tunnel-vision
	// cost (concentrating reinforcement on one staging territory left the
	// rest of the goal's continent thinner, see design doc §19.10).
	WantedContinents map[risk.Continent]bool
	// ArmyCap, when set for a continent in WantedContinents, is the
	// strategic layer's own computed army-count requirement for that
	// continent's border -- reinforce stops treating the continent as
	// "needing help" once its border territory's current armies meet
	// this cap, even if betterPixieContinentNeedsHelp's own generic,
	// fixed threshold (pixieBorderForce = 20, unrelated to any specific
	// tactical requirement) would still say yes. Added after measurement
	// (design doc §20.7) showed that WITHOUT this cap, unioning a goal
	// continent into WantedContinents made reinforcement concentrate on
	// it indefinitely -- pixieBorderForce's generic 20-army bar has no
	// relationship to what the strategic layer actually computed as
	// sufficient, so ceding the stopping decision to it entirely
	// worsened the exact tunnel-vision cost this pass set out to fix,
	// rather than reducing it. A continent absent from ArmyCap (or the
	// whole map nil) is uncapped -- the original union-only behavior,
	// still the right default for continents the executor wants on its
	// own, which the director has no basis to cap.
	ArmyCap map[risk.Continent]int
	// FocusTerritory, when non-empty, is a specific territory fortify
	// should prefer moving armies toward -- the only lever fortify has,
	// since (confirmed by reading betterPixieFortifyDestination in full)
	// it never consults "wanted continents" at all, unlike reinforce/
	// attack/occupy.
	FocusTerritory risk.Territory
}

// unionWanted returns a new map containing every continent set in either
// a or b -- never mutates either argument.
func unionWanted(a, b map[risk.Continent]bool) map[risk.Continent]bool {
	out := make(map[risk.Continent]bool, len(a)+len(b))
	for c := range a {
		out[c] = true
	}
	for c := range b {
		out[c] = true
	}
	return out
}

// DirectedPixieStrategy is BetterPixieStrategy's directive-taking sibling
// -- a genuine fork, not a modification of BetterPixieStrategy itself
// (which stays completely untouched, so every "dumb" bot built on it --
// Cluster/Quo/Boscoe/Pixie/Killbot/KillbotV2/both prior goal-killbot
// passes -- is unaffected). Its phase methods take an explicit Directive
// parameter instead of matching the plain Strategy interface, since it's
// never registered or used standalone -- only a director-style Strategy
// (SymbolicDirectorStrategy) composes and calls it directly. No mutable
// per-call state, so one instance is safe to share across concurrent
// games exactly like every other stateless strategy in the registry.
//
// Every helper function BetterPixieStrategy's own phase methods use
// (betterPixieContinentNeedsHelp, placeToTakeSpecificContinent,
// placeToTakeContinent, placeNearWeakestEnemyCluster,
// bestByEnemyNeighborCount, betterPixieAttackInContinent,
// takeOutContinentCheck, attackForCard, attackAsMuchAsPossible,
// betterPixieFortifyDestination, bestFortifyDestination, etc.) is a
// standalone, package-level function, not a method on
// *BetterPixieStrategy -- so this fork only needs to reimplement the 4
// short top-level phase methods (each with exactly one line changed,
// where `wanted` gets computed), reusing every other helper unchanged.
type DirectedPixieStrategy struct {
	// cmp mirrors BetterPixieStrategy.cmp exactly -- same
	// attackComparator type, same WithAttackComparator-style option,
	// same rawArmyComparator default.
	cmp attackComparator
}

// DirectedPixieOption configures a DirectedPixieStrategy at construction.
type DirectedPixieOption func(*DirectedPixieStrategy)

// WithDirectedAttackComparator overrides the attackComparator
// NewDirectedPixieStrategy otherwise defaults to rawArmyComparator --
// mirrors BetterPixieStrategy's own WithAttackComparator.
func WithDirectedAttackComparator(cmp attackComparator) DirectedPixieOption {
	return func(d *DirectedPixieStrategy) { d.cmp = cmp }
}

// NewDirectedPixieStrategy creates a DirectedPixieStrategy.
func NewDirectedPixieStrategy(opts ...DirectedPixieOption) *DirectedPixieStrategy {
	d := &DirectedPixieStrategy{cmp: rawArmyComparator}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// directedPixiePlacement mirrors betterPixiePlacement (strategy_betterpixie.go)
// exactly, except wanted is a parameter instead of being computed
// internally via betterPixieWantedContinents, and a continent present in
// armyCap is skipped by the "needs help" tier once its border territory's
// current armies meet the cap -- see Directive.ArmyCap's own doc comment
// for why this exists (betterPixieContinentNeedsHelp's generic threshold
// has no relationship to any specific tactical requirement, so without a
// cap this tier never naturally stops helping a not-yet-fully-owned
// continent). Every downstream call is reused unchanged.
func directedPixiePlacement(g *risk.Game, pi int, actions []risk.ReinforcementAction, numberOfArmies int, wanted map[risk.Continent]bool, armyCap map[risk.Continent]int) (risk.Territory, int) {
	for _, cont := range continentOrder(g) {
		if !wanted[cont] || !betterPixieContinentNeedsHelp(g, pi, cont) {
			continue
		}
		t := placeToTakeSpecificContinent(g, pi, actions, cont)
		if cap, ok := armyCap[cont]; ok && g.Territories[t].Armies >= cap {
			continue
		}
		return t, 1
	}
	if len(wanted) == 0 {
		return placeToTakeContinent(g, pi, actions), numberOfArmies
	}
	if t, chunk, ok := placeNearWeakestEnemyCluster(g, pi, numberOfArmies); ok {
		return t, chunk
	}
	get := func(a risk.ReinforcementAction) risk.Territory { return a.Territory }
	return bestByEnemyNeighborCount(g, pi, actions, get), numberOfArmies
}

// reinforce mirrors BetterPixieStrategy.reinforce exactly, except wanted
// is betterPixieWantedContinents unioned with dir.WantedContinents
// instead of betterPixieWantedContinents alone, and dir.ArmyCap can stop
// the "needs help" tier from re-selecting an already-sufficiently-reinforced
// directed continent.
func (d *DirectedPixieStrategy) reinforce(g *risk.Game, playerID string, dir Directive) (Command, error) {
	if cmd, ok := voluntaryCardTurnIn(g, playerID); ok {
		return cmd, nil
	}
	actions := risk.LegalReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal reinforcement for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	wanted := unionWanted(betterPixieWantedContinents(g, pi, g.PendingReinforcements), dir.WantedContinents)
	t, armies := directedPixiePlacement(g, pi, actions, g.PendingReinforcements, wanted, dir.ArmyCap)
	return Command{Action: ActionPlaceReinforcement, Territory: string(t), Armies: armies}, nil
}

// attack mirrors BetterPixieStrategy.attack exactly, except wanted is
// betterPixieWantedContinents unioned with dir.WantedContinents instead
// of betterPixieWantedContinents alone.
func (d *DirectedPixieStrategy) attack(g *risk.Game, playerID string, dir Directive) (Command, error) {
	pi := playerIndex(g, playerID)
	wanted := unionWanted(betterPixieWantedContinents(g, pi, 0), dir.WantedContinents)
	for _, cont := range continentOrder(g) {
		if wanted[cont] {
			if cmd, ok := betterPixieAttackInContinent(g, pi, cont, wanted, d.cmp); ok {
				return cmd, nil
			}
		}
	}
	if cmd, ok := takeOutContinentCheck(g, pi); ok {
		return cmd, nil
	}
	if cmd, ok := attackForCard(g, pi); ok {
		return cmd, nil
	}
	if shouldGoHogWild(g, pi) {
		if cmd, ok := attackAsMuchAsPossible(g, pi, d.cmp); ok {
			return cmd, nil
		}
	}
	return Command{Action: ActionEndAttack}, nil
}

// occupy mirrors BetterPixieStrategy.occupy exactly, except wanted (only
// consulted in the tie-break branch reached when both sides have nonzero
// enemy neighbors) is betterPixieWantedContinents unioned with
// dir.WantedContinents instead of betterPixieWantedContinents alone.
func (d *DirectedPixieStrategy) occupy(g *risk.Game, playerID string, dir Directive) (Command, error) {
	actions := risk.LegalOccupations(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal occupation for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	minArmies, maxArmies := actions[0].Armies, actions[len(actions)-1].Armies
	halfArmies := (minArmies + maxArmies) / 2
	from, to := g.Occupy.From, g.Occupy.To

	aEnemies, dEnemies := enemyNeighborCount(g, from, pi), enemyNeighborCount(g, to, pi)
	switch {
	case aEnemies == 0 && dEnemies != 0:
		return Command{Action: ActionOccupy, Armies: maxArmies}, nil
	case aEnemies != 0 && dEnemies == 0:
		return Command{Action: ActionOccupy, Armies: minArmies}, nil
	case dEnemies == 0:
		return Command{Action: ActionOccupy, Armies: halfArmies}, nil
	}

	wanted := unionWanted(betterPixieWantedContinents(g, pi, 0), dir.WantedContinents)
	tc := territoryContinent(g)
	wantedEnemyNeighbors := func(t risk.Territory) []risk.Territory {
		var out []risk.Territory
		for other := range g.Board.Adjacent[t] {
			if g.Territories[other].Owner != pi && wanted[tc[other]] {
				out = append(out, other)
			}
		}
		return out
	}
	aList, dList := wantedEnemyNeighbors(from), wantedEnemyNeighbors(to)

	switch {
	case len(aList) == 0 && len(dList) != 0:
		return Command{Action: ActionOccupy, Armies: maxArmies}, nil
	case len(aList) != 0 && len(dList) == 0:
		return Command{Action: ActionOccupy, Armies: minArmies}, nil
	case len(aList) == 0:
		return Command{Action: ActionOccupy, Armies: halfArmies}, nil
	}

	seen := make(map[risk.Territory]bool)
	var combined []risk.Territory
	for _, t := range append(append([]risk.Territory(nil), aList...), dList...) {
		if !seen[t] {
			seen[t] = true
			combined = append(combined, t)
		}
	}
	componentOf := make(map[risk.Territory]int)
	for ci, comp := range allNonOwnedComponents(g, pi) {
		for _, t := range comp {
			componentOf[t] = ci
		}
	}
	first, allSame := -1, true
	for _, t := range combined {
		if first == -1 {
			first = componentOf[t]
		} else if componentOf[t] != first {
			allSame = false
			break
		}
	}
	if allSame {
		return Command{Action: ActionOccupy, Armies: maxArmies}, nil
	}
	return Command{Action: ActionOccupy, Armies: halfArmies}, nil
}

// fortify is the one phase with genuinely new logic -- plain
// BetterPixieStrategy's fortify never consults "wanted continents" at
// all (confirmed by reading betterPixieFortifyDestination in full: it
// scores purely on continent-border-distance and weakest-enemy-neighbor),
// so dir.WantedContinents has nothing to union into here. When
// dir.FocusTerritory is set, prefer a legal fortification move whose
// destination is adjacent to it (reusing bestFortifyDestination,
// geometry.go, unchanged -- the same filter+score shape
// GoalKillbotV2Strategy.fortify already used); if no such candidate
// exists, fall through to plain betterPixieFortifyDestination, the same
// graceful-fallback shape reinforce/attack/occupy already have via
// unionWanted's zero-value no-op.
func (d *DirectedPixieStrategy) fortify(g *risk.Game, playerID string, dir Directive) (Command, error) {
	pi := playerIndex(g, playerID)
	actions := risk.LegalFortifications(g, playerID)

	if dir.FocusTerritory != "" {
		var candidates []risk.FortificationAction
		for _, a := range actions {
			if _, adj := g.Board.Adjacent[a.To][dir.FocusTerritory]; adj {
				candidates = append(candidates, a)
			}
		}
		if best, _, ok := bestFortifyDestination(g, pi, candidates); ok {
			return Command{Action: ActionFortify, From: string(best.From), To: string(best.To), Armies: best.MaxArmies}, nil
		}
	}

	best, ok := betterPixieFortifyDestination(g, pi, actions)
	if !ok {
		return Command{Action: ActionEndTurn}, nil
	}
	return Command{Action: ActionFortify, From: string(best.From), To: string(best.To), Armies: best.MaxArmies}, nil
}
