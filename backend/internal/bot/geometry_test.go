package bot

import (
	"slices"
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

func TestMostValuablePositiveOwnedContinentTieBreaksByContinentOrder(t *testing.T) {
	g, _ := newTestGame(t)
	// North America and Europe both have bonus 5 -- continentOrder lists
	// north_america first (it appears first in g.Board.Order), so it must
	// win the tie.
	for _, terr := range g.Board.Continents["north_america"].Territories {
		g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 1}
	}
	for _, terr := range g.Board.Continents["europe"].Territories {
		g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 1}
	}

	if !ownsAnyPositiveContinent(g, 0) {
		t.Fatalf("expected player 0 to own at least one positive-bonus continent")
	}
	got, ok := mostValuablePositiveOwnedContinent(g, 0)
	if !ok {
		t.Fatalf("expected a continent to be found")
	}
	if got != "north_america" {
		t.Fatalf("expected north_america to win the bonus-5 tie via continent order, got %s", got)
	}
}

func TestOwnsAnyPositiveContinentFalseWhenNoneFullyOwned(t *testing.T) {
	g, _ := newTestGame(t)
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1} // one North America territory, not the whole continent

	if ownsAnyPositiveContinent(g, 0) {
		t.Fatalf("expected false when no continent is fully owned")
	}
}

func TestEasiestContinentToTakePicksHighestRatio(t *testing.T) {
	g, _ := newTestGame(t)
	// South America: own Venezuela(5); Peru/Brazil/Argentina default to
	// owner=1/armies=1 -> own=5, enemy=3, ratio ~1.67.
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 0, Armies: 5}
	// Australia: own Indonesia(1); the rest default -> own=1, enemy=3,
	// ratio ~0.33.
	g.Territories["Indonesia"] = risk.TerritoryState{Owner: 0, Armies: 1}

	got, ok := easiestContinentToTake(g, 0)
	if !ok {
		t.Fatalf("expected a continent to be found")
	}
	if got != "south_america" {
		t.Fatalf("expected south_america (higher own:enemy ratio), got %s", got)
	}
}

func TestBiggestArmyTerritoryPicksMostArmies(t *testing.T) {
	g, _ := newTestGame(t)
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 10}

	got, ok := biggestArmyTerritory(g, 0)
	if !ok || got != "Argentina" {
		t.Fatalf("expected Argentina (10 armies), got %s (ok=%v)", got, ok)
	}
}

func TestBiggestArmyTerritoryNoneWhenNothingOwned(t *testing.T) {
	g, _ := newTestGame(t)
	if _, ok := biggestArmyTerritory(g, 0); ok {
		t.Fatalf("expected ok=false when player owns nothing")
	}
}

func TestWeakestEnemyNeighborInContinentRestrictsToContinent(t *testing.T) {
	g, _ := newTestGame(t)
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 10}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 1, Armies: 5}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 1, Armies: 2}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1} // weakest overall, but in Asia not North America

	weakest, ok := weakestEnemyNeighborInContinent(g, "Alaska", 0, "north_america")
	if !ok {
		t.Fatalf("expected an enemy neighbor within north_america")
	}
	if weakest != "Alberta" {
		t.Fatalf("expected Alberta (weakest within north_america; Kamchatka excluded), got %s", weakest)
	}
}

func TestClusterBorderExcludesInteriorRoot(t *testing.T) {
	g, _ := newTestGame(t)
	// Madagascar's only neighbors (South Africa, East Africa) are both
	// owned, so it's interior to the cluster despite being the BFS root.
	g.Territories["Madagascar"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["South Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["East Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}

	got := clusterBorder(g, "Madagascar", 0)
	want := []risk.Territory{"East Africa", "South Africa"}
	if !slices.Equal(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestClusterBorderNilWhenRootNotOwned(t *testing.T) {
	g, _ := newTestGame(t)
	// Alaska defaults to owner=1 in newTestGame.
	if got := clusterBorder(g, "Alaska", 0); got != nil {
		t.Fatalf("expected nil when root isn't owned by pi, got %v", got)
	}
}

func TestCheapestRouteToContinentPrefersLowerCostPathOverFewerHops(t *testing.T) {
	g, _ := newTestGame(t)
	// South America's continent borders are Venezuela (-> Central America)
	// and Brazil (-> North Africa). Nothing is owned directly adjacent to
	// either border; the only owned territory is Congo, two hops from
	// Brazil through a lightly-defended North Africa (cost 2). Central
	// America and everything past it stay at the default owner=1/armies=1
	// and lead nowhere owned, so that side can never resolve to a
	// candidate cheaper than 2.
	g.Territories["North Africa"] = risk.TerritoryState{Owner: 1, Armies: 2}
	g.Territories["Congo"] = risk.TerritoryState{Owner: 0, Armies: 1}

	got, ok := cheapestRouteToContinent(g, 0, "south_america")
	if !ok {
		t.Fatalf("expected a route to be found")
	}
	if got != "Congo" {
		t.Fatalf("expected Congo (cost 2 via Brazil -> North Africa), got %s", got)
	}
}

func TestCheapestRouteToContinentFalseWhenNothingOwned(t *testing.T) {
	g, _ := newTestGame(t)
	if _, ok := cheapestRouteToContinent(g, 0, "south_america"); ok {
		t.Fatalf("expected ok=false when player owns nothing reachable")
	}
}

func TestBestFortifyDestinationPicksHighestEnemyNeighborCount(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	g.Territories["Madagascar"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["South Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["East Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}

	actions := risk.LegalFortifications(g, p0)
	best, score, ok := bestFortifyDestination(g, 0, actions)
	if !ok {
		t.Fatalf("expected a fortify destination to be found")
	}
	if best.From != "South Africa" || best.To != "East Africa" {
		t.Fatalf("expected South Africa -> East Africa, got %s -> %s", best.From, best.To)
	}
	if score != 4 {
		t.Fatalf("expected score 4 (East Africa's enemy neighbor count), got %d", score)
	}
}

func TestBestFortifyDestinationFalseWhenNoActions(t *testing.T) {
	g, _ := newTestGame(t)
	if _, _, ok := bestFortifyDestination(g, 0, nil); ok {
		t.Fatalf("expected ok=false for empty actions")
	}
}
