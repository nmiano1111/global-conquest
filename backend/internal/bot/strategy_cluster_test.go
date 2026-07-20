package bot

import (
	"context"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

func TestClusterRootPicksOwnedContinentTerritory(t *testing.T) {
	g, _ := newTestGame(t)
	for _, terr := range g.Board.Continents["south_america"].Territories {
		g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 1}
	}

	root, ok := clusterRoot(g, 0)
	if !ok {
		t.Fatalf("expected a root to be found")
	}
	if root != "Venezuela" {
		t.Fatalf("expected Venezuela (canonical-first territory of south_america), got %s", root)
	}
}

func TestClusterRootFallsBackToBiggestArmyWhenNoContinentOwned(t *testing.T) {
	g, _ := newTestGame(t)
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 10}

	root, ok := clusterRoot(g, 0)
	if !ok {
		t.Fatalf("expected a root to be found")
	}
	if root != "Argentina" {
		t.Fatalf("expected Argentina (biggest army, no continent owned), got %s", root)
	}
}

func TestClusterStrategySetupReinforcePicksWeakestClusterBorder(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseSetupReinforce
	g.SetupReserves[0] = 5
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 0, Armies: 1}

	strat := NewClusterStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceInitialArmy {
		t.Fatalf("expected place_initial_army, got %s", cmd.Action)
	}
	if cmd.Territory != "Northwest Territory" {
		t.Fatalf("expected Northwest Territory (weakest cluster-border territory), got %s", cmd.Territory)
	}
}

func TestClusterStrategyNeverVoluntarilyTradesCards(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Players[0].Cards = []risk.Card{
		{Territory: "Alaska", Symbol: risk.Infantry},
		{Territory: "Peru", Symbol: risk.Cavalry},
		{Territory: "Egypt", Symbol: risk.Artillery},
	}

	strat := NewClusterStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceReinforcement {
		t.Fatalf("expected place_reinforcement (Cluster never voluntarily trades), got %s", cmd.Action)
	}
}

func TestClusterStrategyReinforceDumpsAllArmiesOnWeakestBorderOfOwnedContinent(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 6
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Peru"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 2}

	strat := NewClusterStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceReinforcement {
		t.Fatalf("expected place_reinforcement, got %s", cmd.Action)
	}
	if cmd.Territory != "Brazil" {
		t.Fatalf("expected Brazil (weakest cluster-border territory; Peru/Argentina are interior), got %s", cmd.Territory)
	}
	if cmd.Armies != 6 {
		t.Fatalf("expected every pending reinforcement dumped in one command, got %d", cmd.Armies)
	}
}

func TestClusterStrategyReinforcePlacesOnMostInContinentEnemyNeighborWhenOwningSomeOfTargetContinent(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 4
	g.Territories["Peru"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 2}
	// Venezuela/Brazil left at the default owner=1/armies=1; South America
	// is the only continent with any owned presence, so it's uniquely
	// favorable regardless of the exact ratio.

	strat := NewClusterStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceReinforcement {
		t.Fatalf("expected place_reinforcement, got %s", cmd.Action)
	}
	if cmd.Territory != "Peru" {
		t.Fatalf("expected Peru (borders 2 in-continent enemies vs Argentina's 1), got %s", cmd.Territory)
	}
}

func TestClusterStrategyAttackEasyExpand(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 4}

	strat := NewClusterStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack {
		t.Fatalf("expected attack, got %s", cmd.Action)
	}
	if cmd.From != "Alaska" || cmd.To != "Kamchatka" {
		t.Fatalf("expected Alaska -> Kamchatka (easy-expand: sole beatable enemy), got %s -> %s", cmd.From, cmd.To)
	}
}

func TestClusterStrategyAttackFillOutKillsIsland(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 0, Armies: 10}
	g.Territories["Mongolia"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Territories["Japan"] = risk.TerritoryState{Owner: 1, Armies: 1} // fully surrounded by owned Kamchatka+Mongolia

	strat := NewClusterStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack {
		t.Fatalf("expected attack, got %s", cmd.Action)
	}
	if cmd.From != "Kamchatka" || cmd.To != "Japan" {
		t.Fatalf("expected Kamchatka -> Japan (fill-out: Japan is an island), got %s -> %s", cmd.From, cmd.To)
	}
}

func TestClusterStrategyAttackConsolidatesTwoSoleEnemyContributors(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Peru"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: 1, Armies: 3} // beats either Peru or Argentina alone, not their combined 4

	strat := NewClusterStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack {
		t.Fatalf("expected attack, got %s", cmd.Action)
	}
	if cmd.From != "Peru" || cmd.To != "Brazil" {
		t.Fatalf("expected Peru -> Brazil (consolidate: Peru+Argentina's combined armies beat Brazil, neither alone does), got %s -> %s", cmd.From, cmd.To)
	}
}

func TestClusterStrategyAttackSplitUpWhenOutnumberingCombinedEnemies(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 0, Armies: 1}
	// Northwest Territory and Kamchatka stay at the default owner=1/armies=1.

	strat := NewClusterStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack {
		t.Fatalf("expected attack, got %s", cmd.Action)
	}
	if cmd.From != "Alaska" || cmd.To != "Northwest Territory" {
		t.Fatalf("expected Alaska -> Northwest Territory (split-up: 3 armies outnumber NWT+Kamchatka's combined 2 by >1.2x), got %s -> %s", cmd.From, cmd.To)
	}
}

func TestClusterStrategyAttackEndsWhenNoQualifyingCandidate(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}

	strat := NewClusterStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionEndAttack {
		t.Fatalf("expected end_attack when no candidate qualifies, got %s", cmd.Action)
	}
}

func TestShouldGoHogWildWhenArmiesExceedAllEnemiesCombined(t *testing.T) {
	g, _ := newTestGame(t)
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1000}

	if !shouldGoHogWild(g, 0) {
		t.Fatalf("expected hogwild when player 0's armies exceed everyone else's combined")
	}
}

func TestShouldGoHogWildFalseWhenNotDominant(t *testing.T) {
	g, _ := newTestGame(t)
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}

	if shouldGoHogWild(g, 0) {
		t.Fatalf("expected no hogwild for an ordinary, non-dominant position")
	}
}

func TestAttackAsMuchAsPossibleFindsAMatchOutsideTheRootTerritory(t *testing.T) {
	g, _ := newTestGame(t)
	// Two disconnected owned territories: Alaska (surrounded on all
	// sides, can't beat anything) and Madagascar (a genuine easy-expand
	// match via South Africa).
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Madagascar"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["South Africa"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["East Africa"] = risk.TerritoryState{Owner: 1, Armies: 1}

	cmd, ok := attackAsMuchAsPossible(g, 0)
	if !ok {
		t.Fatalf("expected a match to be found")
	}
	if cmd.From != "Madagascar" || cmd.To != "East Africa" {
		t.Fatalf("expected Madagascar -> East Africa, got %s -> %s", cmd.From, cmd.To)
	}
}

func TestClusterStrategyOccupyLeavesMinimumWhenFromHasWeakerContinentMatchup(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseOccupy
	// Alberta is adjacent to Alaska, Northwest Territory, Ontario, and
	// Western United States; Alaska only reaches Northwest Territory
	// besides Alberta itself. From = Alberta (the more connected side, so
	// its own extra neighbors -- Ontario here -- can beat anything Alaska
	// has access to, including their shared Northwest Territory).
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 0, Armies: 10}
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1} // just conquered
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 1, Armies: 5}
	g.Territories["Ontario"] = risk.TerritoryState{Owner: 1, Armies: 1}
	g.Territories["Western United States"] = risk.TerritoryState{Owner: 1, Armies: 5}
	g.Occupy = &risk.OccupyState{From: "Alberta", To: "Alaska", MinMove: 2, MaxMove: 8}

	strat := NewClusterStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionOccupy {
		t.Fatalf("expected occupy, got %s", cmd.Action)
	}
	if cmd.Armies != 2 {
		t.Fatalf("expected the legal minimum (2): Alberta's weakest North America neighbor Ontario (1) beats Alaska's Northwest Territory (5), got %d", cmd.Armies)
	}
}

func TestClusterStrategyOccupySendsMaximumWhenToHasWeakerContinentMatchup(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseOccupy
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 10}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 1, Armies: 5}
	g.Territories["Ontario"] = risk.TerritoryState{Owner: 1, Armies: 1}
	g.Territories["Western United States"] = risk.TerritoryState{Owner: 1, Armies: 5}
	g.Occupy = &risk.OccupyState{From: "Alaska", To: "Alberta", MinMove: 2, MaxMove: 8}

	strat := NewClusterStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionOccupy {
		t.Fatalf("expected occupy, got %s", cmd.Action)
	}
	if cmd.Armies != 8 {
		t.Fatalf("expected the legal maximum (8): Alberta's weakest North America neighbor (1) beats Alaska's (5), got %d", cmd.Armies)
	}
}

func TestClusterStrategyFortifyPrefersMostEnemyNeighborDestination(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	g.Territories["Madagascar"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["South Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["East Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}

	strat := NewClusterStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionFortify {
		t.Fatalf("expected fortify, got %s", cmd.Action)
	}
	if cmd.From != "South Africa" || cmd.To != "East Africa" {
		t.Fatalf("expected South Africa -> East Africa, got %s -> %s", cmd.From, cmd.To)
	}
}

func TestClusterStrategyFortifyEndsTurnWhenNoDestinationFacesAnEnemy(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 3}
	}

	strat := NewClusterStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionEndTurn {
		t.Fatalf("expected end_turn when no fortification destination faces a threat, got %s", cmd.Action)
	}
}

func TestClusterStrategySetupReinforceRoutesTowardEasiestContinentWhenNoneOwned(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseSetupReinforce
	g.SetupReserves[0] = 5
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 1}

	strat := NewClusterStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceInitialArmy {
		t.Fatalf("expected place_initial_army, got %s", cmd.Action)
	}
	// Neither continent is owned outright; placeToTakeContinent picks
	// north_america as easiest (Alaska's 3 armies vs 8 enemy beats
	// Argentina's 1 vs south_america's 3), landing on Alaska, its only
	// owned member -- the fix for the gap clusterOrTakeContinentPlacement
	// closes (setupReinforce previously never reached this branch).
	if cmd.Territory != "Alaska" {
		t.Fatalf("expected Alaska, got %s", cmd.Territory)
	}
}
