// Package symbolic extracts explicit, named strategic facts from a
// risk.Game -- e.g. "this continent is one territory from being fully
// owned by pi" -- for a symbolic strategic reasoning layer sitting above
// killbot-v2's existing tactical heuristics (see
// project-docs/bot_player/phase_3_continuous_improvement/
// 12_Symbolic_Strategic_Reasoning_Layer.md for the full design and
// roadmap, including why this second version is scoped to self-completion
// only, dropping the first version's symmetric enemy-completion fact).
// This package depends only on internal/risk, never internal/bot, so
// internal/bot's strategies depend on this package and not the other way
// around -- the same dependency direction internal/bot/gcnmodel and
// internal/bot/tdstate already use.
//
// A handful of small board-query helpers below (territoryContinents,
// continentOrder, nearCompletion) deliberately duplicate the *shape* of
// equivalent unexported helpers in internal/bot/geometry.go
// (territoryContinent, continentOrder): those are package-private and
// genuinely inaccessible from here regardless of which package depends
// on which, so a small, deliberate reimplementation -- scoped to exactly
// what this package's facts need, not a full port of geometry.go -- is
// the only option, not a convenience shortcut.
package symbolic

import "github.com/nmiano1111/global-conquest/backend/internal/risk"

// ContinentCompletionFact describes one positive-bonus continent that is
// exactly one territory away from being fully owned by pi.
type ContinentCompletionFact struct {
	// Continent is the continent one territory from being fully owned by pi.
	Continent risk.Continent
	// MissingTerr is the sole territory in Continent not owned by pi.
	MissingTerr risk.Territory
	// Bonus is Continent's reinforcement bonus (always > 0 -- zero/negative
	// bonus continents are never reported, since there's no strategic
	// value in a persistent goal to complete or defend one).
	Bonus int
}

// SelfNearCompletion returns every positive-bonus continent pi owns all
// but exactly one territory of, in board order. Scoped to pi only --
// unlike this package's first version (reverted, see the design doc's
// retrospective section), a rival's own near-complete continent has no
// multi-turn plan of pi's to attach a persistent goal to, only a
// single-decision denial consideration -- out of scope for the
// persistent-goal experiment this package now serves.
func SelfNearCompletion(g *risk.Game, pi int) []ContinentCompletionFact {
	tc := territoryContinents(g.Board)
	var out []ContinentCompletionFact
	for _, cont := range continentOrder(g.Board, tc) {
		info := g.Board.Continents[cont]
		if info.Bonus <= 0 {
			continue
		}
		owner, missing, ok := nearCompletion(g, info.Territories)
		if !ok || owner != pi {
			continue
		}
		out = append(out, ContinentCompletionFact{Continent: cont, MissingTerr: missing, Bonus: info.Bonus})
	}
	return out
}

// territoryContinents maps every territory to its continent, built once
// from risk.Board.Continents (a map, so it carries no order of its own).
func territoryContinents(b risk.Board) map[risk.Territory]risk.Continent {
	m := make(map[risk.Territory]risk.Continent, len(b.Order))
	for cont, info := range b.Continents {
		for _, t := range info.Territories {
			m[t] = cont
		}
	}
	return m
}

// continentOrder returns every continent in the order its first
// territory appears in b.Order, for deterministic iteration over
// b.Continents (a map).
func continentOrder(b risk.Board, tc map[risk.Territory]risk.Continent) []risk.Continent {
	seen := make(map[risk.Continent]bool, len(b.Continents))
	order := make([]risk.Continent, 0, len(b.Continents))
	for _, t := range b.Order {
		cont := tc[t]
		if !seen[cont] {
			seen[cont] = true
			order = append(order, cont)
		}
	}
	return order
}

// nearCompletion reports whether territories (one continent's full
// member list) is owned by a single player except for exactly one
// remaining territory. ok is false if no single player holds exactly
// len(territories)-1 of them (already fully owned, too contested, or
// too early to be "near" complete).
func nearCompletion(g *risk.Game, territories []risk.Territory) (owner int, missing risk.Territory, ok bool) {
	if len(territories) < 2 {
		return 0, "", false
	}
	counts := make(map[int]int, len(territories))
	for _, t := range territories {
		counts[g.Territories[t].Owner]++
	}
	found := false
	for o, c := range counts {
		if o >= 0 && c == len(territories)-1 {
			owner, found = o, true
			break
		}
	}
	if !found {
		return 0, "", false
	}
	for _, t := range territories {
		if g.Territories[t].Owner != owner {
			return owner, t, true
		}
	}
	return 0, "", false // unreachable: counts guarantees exactly one mismatch
}
