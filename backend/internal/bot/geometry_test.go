package bot

import (
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

func TestEnemyNeighborCountCountsDistinctTerritoriesNotArmies(t *testing.T) {
	g, _ := newTestGame(t)
	// Alaska's neighbors are Northwest Territory, Alberta, and Kamchatka.
	// Own Northwest Territory; leave the other two at the default owner=1.
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 1, Armies: 50}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1}

	got := enemyNeighborCount(g, "Alaska", 0)
	if got != 2 {
		t.Fatalf("expected 2 enemy-owned neighbor territories regardless of army counts, got %d", got)
	}
}

func TestEnemyNeighborCountZeroWhenFullyEnveloped(t *testing.T) {
	g, _ := newTestGame(t)
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 1}
	}

	if got := enemyNeighborCount(g, "Alaska", 0); got != 0 {
		t.Fatalf("expected 0 enemy neighbors when every territory is owned by pi, got %d", got)
	}
}

func TestWeakestEnemyNeighborPicksFewestArmies(t *testing.T) {
	g, _ := newTestGame(t)
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 10}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 1, Armies: 5}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 1, Armies: 1}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 3}

	weakest, ok := weakestEnemyNeighbor(g, "Alaska", 0)
	if !ok {
		t.Fatalf("expected an enemy neighbor to be found")
	}
	if weakest != "Alberta" {
		t.Fatalf("expected Alberta (fewest armies), got %s", weakest)
	}
}

func TestWeakestEnemyNeighborTieBreaksByCanonicalOrder(t *testing.T) {
	g, _ := newTestGame(t)
	// Alaska's neighbors, in canonical board order, are:
	// Northwest Territory, Alberta, Kamchatka. Tie all three at 1 army.
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 10}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 1, Armies: 1}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 1, Armies: 1}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1}

	weakest, ok := weakestEnemyNeighbor(g, "Alaska", 0)
	if !ok {
		t.Fatalf("expected an enemy neighbor to be found")
	}
	if weakest != "Northwest Territory" {
		t.Fatalf("expected the canonical-order tie-break winner Northwest Territory, got %s", weakest)
	}
}

func TestWeakestEnemyNeighborNoneWhenFullyOwned(t *testing.T) {
	g, _ := newTestGame(t)
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 1}
	}

	if _, ok := weakestEnemyNeighbor(g, "Alaska", 0); ok {
		t.Fatalf("expected ok=false when every neighbor is owned by pi")
	}
}
