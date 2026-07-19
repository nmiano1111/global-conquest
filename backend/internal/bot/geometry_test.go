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

func TestPlayerArmiesAdjoiningContinentSumsOwnedNeighborsOutsideContinent(t *testing.T) {
	g, _ := newTestGame(t)
	// South America's continent borders are Venezuela (-> Central America)
	// and Brazil (-> North Africa).
	g.Territories["Central America"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["North Africa"] = risk.TerritoryState{Owner: 0, Armies: 3}

	got := playerArmiesAdjoiningContinent(g, 0, "south_america")
	if got != 8 {
		t.Fatalf("expected 8 (Central America 5 + North Africa 3), got %d", got)
	}
}

func TestPlayerArmiesAdjoiningContinentExcludesTerritoriesInsideContinent(t *testing.T) {
	g, _ := newTestGame(t)
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 0, Armies: 5} // inside south_america itself

	got := playerArmiesAdjoiningContinent(g, 0, "south_america")
	if got != 0 {
		t.Fatalf("expected 0 (Venezuela is inside south_america, not adjoining it), got %d", got)
	}
}

func TestPixieWantedContinentsIncludesContinentWonPurelyByAdjoiningStrength(t *testing.T) {
	g, _ := newTestGame(t)
	g.Territories["Central America"] = risk.TerritoryState{Owner: 0, Armies: 20}

	wanted := pixieWantedContinents(g, 0)
	if !wanted["south_america"] {
		t.Fatalf("expected south_america to be wanted purely from Central America's adjoining strength")
	}
	if !continentNeedsHelp(g, 0, "south_america") {
		t.Fatalf("expected south_america to need help: player owns nothing inside it yet")
	}
}

func TestPixieWantedContinentsExcludesWeaklyContestedContinent(t *testing.T) {
	g, _ := newTestGame(t)
	wanted := pixieWantedContinents(g, 0)
	if wanted["south_america"] {
		t.Fatalf("expected south_america not to be wanted with no owned presence in or near it")
	}
}

func TestContinentNeedsHelpFalseWhenFullyOwnedWithStrongBorders(t *testing.T) {
	g, _ := newTestGame(t)
	for _, terr := range g.Board.Continents["south_america"].Territories {
		g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 50} // well above pixieBorderForce
	}
	if continentNeedsHelp(g, 0, "south_america") {
		t.Fatalf("expected no help needed: fully owned with strong borders")
	}
}

func TestContinentNeedsHelpTrueWhenBorderIsWeak(t *testing.T) {
	g, _ := newTestGame(t)
	for _, terr := range g.Board.Continents["south_america"].Territories {
		g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 50}
	}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 0, Armies: 5} // a border, below pixieBorderForce(20)

	if !continentNeedsHelp(g, 0, "south_america") {
		t.Fatalf("expected help needed: Venezuela is a weak border")
	}
}

func TestPlayerIncomeBaseFormula(t *testing.T) {
	g, _ := newTestGame(t)
	// Own exactly 9 territories, no full continent: base = max(3, 9/3) = 3.
	owned := []risk.Territory{
		"Alaska", "Northwest Territory", "Greenland", "Alberta", "Ontario",
		"Quebec", "Western United States", "Eastern United States", "Iceland",
	}
	for _, terr := range owned {
		g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 1}
	}
	if got := playerIncome(g, 0); got != 3 {
		t.Fatalf("expected base income 3 (max(3, 9/3)), got %d", got)
	}
}

func TestPlayerIncomeAddsFullyOwnedContinentBonus(t *testing.T) {
	g, _ := newTestGame(t)
	for _, terr := range g.Board.Continents["south_america"].Territories {
		g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 1}
	}
	// 4 territories: base = max(3, 4/3=1) = 3; + south_america bonus 2 = 5.
	if got := playerIncome(g, 0); got != 5 {
		t.Fatalf("expected income 5 (base 3 + south_america bonus 2), got %d", got)
	}
}

func TestDominantPlayerToKillDetectsDefaultFullBoardOwnership(t *testing.T) {
	g, _ := newTestGame(t)
	// newTestGame's own baseline already has player index 1 owning every
	// territory -- the simplest possible "must be stopped" scenario.
	target, ok := dominantPlayerToKill(g, 0)
	if !ok {
		t.Fatalf("expected player 1 (owns literally everything by default) to be flagged dominant")
	}
	if target != 1 {
		t.Fatalf("expected target player 1, got %d", target)
	}
}

func TestDominantPlayerToKillFalseWhenNobodyDominates(t *testing.T) {
	g, _ := newTestGame(t)
	// Give player 0 the bulk of the board (excluded from the check
	// entirely) so neither player 1 nor player 2 crosses 50% of
	// anything: one sentinel territory per continent stays at the
	// default owner (1); player 2 owns nothing.
	sentinels := map[risk.Territory]bool{
		"Alaska": true, "Venezuela": true, "Iceland": true,
		"North Africa": true, "Ural": true, "Indonesia": true,
	}
	for _, terr := range g.Board.Order {
		if !sentinels[terr] {
			g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 1}
		}
	}
	if _, ok := dominantPlayerToKill(g, 0); ok {
		t.Fatalf("expected no dominant player: ownership is spread thin enough that nobody crosses 50%% of anything")
	}
}

func TestDominantPlayerToKillTieBreaksToHighestIndexAmongMultipleQualifiers(t *testing.T) {
	g, _ := newTestGame(t)
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 1}
	}
	// Player 1 crosses the armies threshold via one huge stack.
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1000}
	// Player 2 crosses the territory-count threshold via exactly half the
	// board (21 of 42 territories), each with 1 army.
	for _, terr := range g.Board.Order[:21] {
		g.Territories[terr] = risk.TerritoryState{Owner: 2, Armies: 1}
	}

	target, ok := dominantPlayerToKill(g, 0)
	if !ok {
		t.Fatalf("expected a dominant player to be found")
	}
	if target != 2 {
		t.Fatalf("expected player 2 (higher index, independently qualifies via territory share) to win the tie-break, got %d", target)
	}
}

func TestCheapestAttackHopToContinentTwoHopScenario(t *testing.T) {
	g, _ := newTestGame(t)
	// Same alternate-route hazard as
	// TestCheapestRouteToContinentWithCostReturnsAccumulatedCost -- block
	// everything else so only the intended path is cheap.
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 1, Armies: 1000}
	}
	g.Territories["Western United States"] = risk.TerritoryState{Owner: 0, Armies: 20}
	g.Territories["Central America"] = risk.TerritoryState{Owner: 1, Armies: 3}

	from, to, cost, ok := cheapestAttackHopToContinent(g, 0, "south_america")
	if !ok {
		t.Fatalf("expected a route to be found")
	}
	if from != "Western United States" {
		t.Fatalf("expected from=Western United States, got %s", from)
	}
	if to != "Central America" {
		t.Fatalf("expected to=Central America (the intermediate hop), got %s", to)
	}
	if cost != 3 {
		t.Fatalf("expected cost 3 (Central America's armies), got %d", cost)
	}
}

func TestCheapestAttackHopToContinentFalseWhenAlreadyOwningABorderTerritory(t *testing.T) {
	g, _ := newTestGame(t)
	// Venezuela is itself one of south_america's own border territories;
	// owning it directly means the search finds it immediately with no
	// predecessor to report.
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 0, Armies: 5}

	if _, _, _, ok := cheapestAttackHopToContinent(g, 0, "south_america"); ok {
		t.Fatalf("expected ok=false: Venezuela is already a south_america border territory, no hop to report")
	}
}

func TestCheapestRouteToContinentWithCostReturnsAccumulatedCost(t *testing.T) {
	g, _ := newTestGame(t)
	// Block every other path with a prohibitive army count, leaving only
	// the intended Venezuela -> Central America -> Western United States
	// route cheap -- on the real board, a long enough chain of
	// default-armies(1) hops can otherwise beat a single moderately
	// defended hop, so a bare "set Central America's armies" setup isn't
	// safe against alternate routes.
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 1, Armies: 1000}
	}
	g.Territories["Western United States"] = risk.TerritoryState{Owner: 0, Armies: 20}
	g.Territories["Central America"] = risk.TerritoryState{Owner: 1, Armies: 7}

	got, cost, ok := cheapestRouteToContinentWithCost(g, 0, "south_america")
	if !ok {
		t.Fatalf("expected a route to be found")
	}
	if got != "Western United States" {
		t.Fatalf("expected Western United States, got %s", got)
	}
	if cost != 7 {
		t.Fatalf("expected cost 7, got %d", cost)
	}
}
