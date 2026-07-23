package bot

import (
	"github.com/nmiano1111/global-conquest/backend/internal/risk"
	"github.com/nmiano1111/global-conquest/backend/internal/tdstate"
)

// copyGameState deep-copies exactly the mutable, reference-typed fields of
// g (Territories, Players including each player's own Cards slice,
// SetupReserves, Occupy, Deck, Discard) so a candidate move can be
// applied to the copy without mutating g. Board is static (never mutated
// by any risk.Game method) and is shared, not copied. The unexported rng
// field is carried over as-is by the struct copy -- callers must never
// exercise a code path on the copy that consumes it (see this file's
// package-level doc note in strategy_value.go for why that's a real
// constraint, and how ValueStrategy's design avoids it entirely).
func copyGameState(g *risk.Game) *risk.Game {
	g2 := *g

	g2.Territories = make(map[risk.Territory]risk.TerritoryState, len(g.Territories))
	for t, ts := range g.Territories {
		g2.Territories[t] = ts
	}

	g2.Players = make([]risk.PlayerState, len(g.Players))
	for i, p := range g.Players {
		p.Cards = append([]risk.Card(nil), p.Cards...)
		g2.Players[i] = p
	}

	g2.SetupReserves = make(map[int]int, len(g.SetupReserves))
	for k, v := range g.SetupReserves {
		g2.SetupReserves[k] = v
	}

	if g.Occupy != nil {
		occupyCopy := *g.Occupy
		g2.Occupy = &occupyCopy
	}

	g2.Deck = append([]risk.Card(nil), g.Deck...)
	g2.Discard = append([]risk.Card(nil), g.Discard...)

	return &g2
}

// reinforceAfterstate copies g and applies placing armies armies at t for
// playerID, via risk.Game.PlaceReinforcement directly (not
// internal/simulation.Dispatch -- simulation already depends on this
// package, so depending back on it would be an import cycle; calling the
// exported risk.Game method directly is simpler anyway). Never touches
// rng.
func reinforceAfterstate(g *risk.Game, playerID string, t risk.Territory, armies int) *risk.Game {
	c := copyGameState(g)
	_ = c.PlaceReinforcement(playerID, t, armies)
	return c
}

// occupyAfterstate copies g and applies moving armies armies into the
// just-conquered territory for playerID, via risk.Game.OccupyTerritory
// directly. Never touches rng.
func occupyAfterstate(g *risk.Game, playerID string, armies int) *risk.Game {
	c := copyGameState(g)
	_ = c.OccupyTerritory(playerID, armies)
	return c
}

// fortifyAfterstate copies g and applies moving armies armies from one
// owned, connected territory to another for playerID, via
// risk.Game.Fortify directly. Never touches rng.
func fortifyAfterstate(g *risk.Game, playerID string, from, to risk.Territory, armies int) *risk.Game {
	c := copyGameState(g)
	_ = c.Fortify(playerID, from, to, armies)
	return c
}

// attackAfterstateBlend computes a probability-weighted blend of two
// hypothetical afterstates for the given attack, at the *encoded feature*
// level (not the raw game state -- fractional army counts don't make
// sense on a real risk.Game): "conquered" (ownership of a.To transfers to
// pi, both territories' armies set from the forecast's expected losses)
// weighted by the forecast's win probability, and "held" (ownership
// unchanged, armies reduced by the same expected losses) weighted by the
// complement. This is deliberately an approximation, not an exact
// simulation of any one dice outcome -- matching
// 11_Learned_Board_Evaluation.md's own reasoning for why a smoother
// expected-value signal beats rolling one stochastic sample here. Never
// constructs a real risk.Game via risk.Game.Attack (which would consume
// rng); both hypothetical states are built by directly overwriting
// Territories entries on a copy.
func attackAfterstateBlend(g *risk.Game, pi int, a risk.AttackAction) []float64 {
	forecast := ForecastAttack(a.SourceArmies, a.TargetArmies)
	targetOwner := g.Territories[a.To].Owner

	attackerRemaining := max(1, a.SourceArmies-round(forecast.ExpectedAttackerLosses))
	defenderRemaining := max(1, a.TargetArmies-round(forecast.ExpectedDefenderLosses))

	conquered := copyGameState(g)
	conquered.Territories[a.From] = risk.TerritoryState{Owner: pi, Armies: max(1, attackerRemaining-a.MaxAttackerDice)}
	conquered.Territories[a.To] = risk.TerritoryState{Owner: pi, Armies: a.MaxAttackerDice}

	held := copyGameState(g)
	held.Territories[a.From] = risk.TerritoryState{Owner: pi, Armies: attackerRemaining}
	held.Territories[a.To] = risk.TerritoryState{Owner: targetOwner, Armies: defenderRemaining}

	conqueredFeatures := tdstate.Encode(conquered, pi).Flatten()
	heldFeatures := tdstate.Encode(held, pi).Flatten()

	p := forecast.WinProbability
	blended := make([]float64, len(conqueredFeatures))
	for i := range blended {
		blended[i] = p*conqueredFeatures[i] + (1-p)*heldFeatures[i]
	}
	return blended
}

func round(f float64) int {
	return int(f + 0.5)
}

// applyTerminalOutcome materializes one Attack Handler TerminalState (a
// concrete, real fight-to-a-conclusion result -- see attack_handler.go)
// into a copy of g. Unlike attackAfterstateBlend, which averages the
// "conquered" and "held" branches into a single feature vector for
// one-shot scoring, this produces an actual valid *risk.Game so a
// sequence search (attack_search.go) can keep reasoning from it as a
// real board state instead of chaining approximations of
// approximations.
//
// Deliberately does not model the real engine's PhaseOccupy sub-step:
// on conquest, occupyArmies moves into a.To immediately, matching
// attackAfterstateBlend's existing MaxAttackerDice heuristic rather than
// searching the real occupy choice space -- occupy()'s own
// LegalOccupations search stays authoritative once the real engine
// actually gets there. This keeps the sequence search scoped to "which
// attacks," not also "how many armies to occupy with," per
// Search_Integration_Roadmap_with_References.md's Phase 2 scoping.
func applyTerminalOutcome(g *risk.Game, pi int, a risk.AttackAction, outcome TerminalState, occupyArmies int) *risk.Game {
	c := copyGameState(g)
	if outcome.DefenderRemaining == 0 {
		c.Territories[a.To] = risk.TerritoryState{Owner: pi, Armies: occupyArmies}
		c.Territories[a.From] = risk.TerritoryState{Owner: pi, Armies: max(1, outcome.AttackerRemaining-occupyArmies)}
		return c
	}
	c.Territories[a.From] = risk.TerritoryState{Owner: pi, Armies: outcome.AttackerRemaining}
	c.Territories[a.To] = risk.TerritoryState{Owner: g.Territories[a.To].Owner, Armies: outcome.DefenderRemaining}
	return c
}
