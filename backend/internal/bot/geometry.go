package bot

import "github.com/nmiano1111/global-conquest/backend/internal/risk"

// This file provides read-only board-geometry query helpers over *risk.Game
// -- the same category as risk/legal_actions.go, never mutating state. It
// exists to support Lux Delux-inspired bot personas (see
// project-docs/bot_player/proposals/Lux_Delux_AI_Research_Notes.md and
// Lux_Port_Notes.md): AngryStrategy is the first, needing only the two
// helpers below, but later personas (Pixie, Cluster, Quo, Killbot, Boscoe)
// are expected to extend this file with continent-ownership, cluster, and
// path-cost queries as each one needs them.

// enemyNeighborCount counts the distinct territories adjacent to t that are
// not owned by pi -- Lux's Country.getNumberEnemyNeighbors(), a territory
// count, not an army sum (contrast strategy_basic.go's adjacentEnemyArmies,
// which sums armies).
func enemyNeighborCount(g *risk.Game, t risk.Territory, pi int) int {
	count := 0
	for _, other := range g.Board.Order {
		if other == t || !g.Board.IsAdjacent(t, other) {
			continue
		}
		if g.Territories[other].Owner != pi {
			count++
		}
	}
	return count
}

// weakestEnemyNeighbor returns the territory adjacent to t not owned by pi
// with the fewest armies, tie-broken by canonical board order. ok is false
// if t has no enemy neighbor.
func weakestEnemyNeighbor(g *risk.Game, t risk.Territory, pi int) (weakest risk.Territory, ok bool) {
	bestArmies := 0
	for _, other := range g.Board.Order {
		if other == t || !g.Board.IsAdjacent(t, other) {
			continue
		}
		ts := g.Territories[other]
		if ts.Owner == pi {
			continue
		}
		if !ok || ts.Armies < bestArmies {
			weakest, bestArmies, ok = other, ts.Armies, true
		}
	}
	return weakest, ok
}
