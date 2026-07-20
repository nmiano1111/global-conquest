package bot

import (
	"context"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

func TestPixieStrategySetupReinforceFallsBackToPlaceToTakeContinent(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseSetupReinforce
	g.SetupReserves[0] = 5
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 1}

	strat := NewPixieStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceInitialArmy {
		t.Fatalf("expected place_initial_army, got %s", cmd.Action)
	}
	// Neither continent is "wanted" yet (too little owned presence in
	// either), so placement falls through to placeToTakeContinent, which
	// picks north_america as the easiest continent to take (Alaska's 3
	// armies against 8 enemy armies beats Argentina's 1 against
	// south_america's 3) and then Alaska as its only owned member.
	if cmd.Territory != "Alaska" {
		t.Fatalf("expected Alaska, got %s", cmd.Territory)
	}
}

func TestPixieStrategyNeverVoluntarilyTradesCards(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Players[0].Cards = []risk.Card{
		{Territory: "Alaska", Symbol: risk.Infantry},
		{Territory: "Peru", Symbol: risk.Cavalry},
		{Territory: "Egypt", Symbol: risk.Artillery},
	}

	strat := NewPixieStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceReinforcement {
		t.Fatalf("expected place_reinforcement (Pixie never voluntarily trades), got %s", cmd.Action)
	}
}

func TestPixieStrategyReinforcePlacesOnWeakestBorderOfNeedyWantedContinent(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 6
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 0, Armies: 5} // weak border, needs help
	g.Territories["Peru"] = risk.TerritoryState{Owner: 0, Armies: 50}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: 0, Armies: 50}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 50}

	strat := NewPixieStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceReinforcement {
		t.Fatalf("expected place_reinforcement, got %s", cmd.Action)
	}
	if cmd.Territory != "Venezuela" {
		t.Fatalf("expected Venezuela (weak border, below pixieBorderForce), got %s", cmd.Territory)
	}
	if cmd.Armies != 6 {
		t.Fatalf("expected every pending reinforcement dumped in one command, got %d", cmd.Armies)
	}
}

func TestPixieStrategyReinforceSpreadsNearEnemiesWhenWantedContinentIsHealthy(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 4
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 0, Armies: 50}
	g.Territories["Peru"] = risk.TerritoryState{Owner: 0, Armies: 50}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: 0, Armies: 50}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 50}
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1} // 3 enemy neighbors, most of any owned territory

	strat := NewPixieStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceReinforcement {
		t.Fatalf("expected place_reinforcement, got %s", cmd.Action)
	}
	if cmd.Territory != "Alaska" {
		t.Fatalf("expected Alaska (south_america is wanted but healthy, falls back to spreading near enemies), got %s", cmd.Territory)
	}
}

func TestPixieStrategyAttackWithinWantedContinent(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	// Venezuela is kept modest (5, not e.g. 20): a big enough stack there
	// also makes north_america "wanted" via playerArmiesAdjoiningContinent
	// (Venezuela borders Central America), and north_america sorts before
	// south_america in continentOrder, which would attack there first.
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Peru"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: 1, Armies: 3}

	strat := NewPixieStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack {
		t.Fatalf("expected attack, got %s", cmd.Action)
	}
	if cmd.From != "Venezuela" || cmd.To != "Brazil" {
		t.Fatalf("expected Venezuela -> Brazil (attack within wanted south_america), got %s -> %s", cmd.From, cmd.To)
	}
}

func TestAttackForCardPicksBestRatioAcrossWholeBoard(t *testing.T) {
	g, _ := newTestGame(t)
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 1, Armies: 2} // ratio 1.5
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 10}
	g.Territories["Peru"] = risk.TerritoryState{Owner: 1, Armies: 1} // ratio 10, much better

	cmd, ok := attackForCard(g, 0)
	if !ok {
		t.Fatalf("expected a match to be found")
	}
	if cmd.From != "Argentina" || cmd.To != "Peru" {
		t.Fatalf("expected Argentina -> Peru (best ratio 10 vs Alaska -> Northwest Territory's 1.5), got %s -> %s", cmd.From, cmd.To)
	}
}

func TestAttackForCardFalseWhenConqueredThisTurn(t *testing.T) {
	g, _ := newTestGame(t)
	g.ConqueredThisTurn = true
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 100}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 1, Armies: 1}

	if _, ok := attackForCard(g, 0); ok {
		t.Fatalf("expected ok=false once a conquest already happened this turn")
	}
}

func TestAttackForCardFalseWhenNoRatioExceedsOne(t *testing.T) {
	g, _ := newTestGame(t)
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 1, Armies: 5}

	if _, ok := attackForCard(g, 0); ok {
		t.Fatalf("expected ok=false when no matchup exceeds a 1:1 ratio")
	}
}

func TestPixieStrategyAttackEndsWhenNothingQualifies(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}

	strat := NewPixieStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionEndAttack {
		t.Fatalf("expected end_attack, got %s", cmd.Action)
	}
}

func TestPixieStrategyOccupyLeavesMinimumWhenFromMoreThreatened(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseOccupy
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 10}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Yakutsk"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Irkutsk"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Mongolia"] = risk.TerritoryState{Owner: 0, Armies: 1}
	// Alaska faces Northwest Territory + Alberta (2 enemies); Kamchatka
	// faces only Japan now that Yakutsk/Irkutsk/Mongolia are also owned.
	g.Occupy = &risk.OccupyState{From: "Alaska", To: "Kamchatka", MinMove: 2, MaxMove: 4}

	strat := NewPixieStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionOccupy {
		t.Fatalf("expected occupy, got %s", cmd.Action)
	}
	if cmd.Armies != 2 {
		t.Fatalf("expected the legal minimum (2): Alaska(2 enemies) more threatened than Kamchatka(1), got %d", cmd.Armies)
	}
}

func TestPixieStrategyOccupyHalfWhenBothSidesTiedWithEnemies(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseOccupy
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 10}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Yakutsk"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Irkutsk"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Mongolia"] = risk.TerritoryState{Owner: 0, Armies: 1}
	// Alaska's only remaining enemy is Northwest Territory (1); Kamchatka's
	// only remaining enemy is Japan (1) -- tied at 1 each.
	g.Occupy = &risk.OccupyState{From: "Alaska", To: "Kamchatka", MinMove: 2, MaxMove: 8}

	strat := NewPixieStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionOccupy {
		t.Fatalf("expected occupy, got %s", cmd.Action)
	}
	if cmd.Armies != 5 {
		t.Fatalf("expected the legal midpoint (5): both sides tied with 1 enemy each, got %d", cmd.Armies)
	}
}

func TestPixieStrategyOccupyMinWhenFromContinentIsWantedButTosIsNot(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseOccupy
	g.Territories["Indonesia"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["New Guinea"] = risk.TerritoryState{Owner: 0, Armies: 10}
	g.Territories["Western Australia"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Eastern Australia"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Peru"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 1} // just conquered
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 1, Armies: 10} // keeps south_america from being wanted
	g.Occupy = &risk.OccupyState{From: "New Guinea", To: "Argentina", MinMove: 1, MaxMove: 9}

	strat := NewPixieStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionOccupy {
		t.Fatalf("expected occupy, got %s", cmd.Action)
	}
	if cmd.Armies != 1 {
		t.Fatalf("expected the legal minimum (1): New Guinea's continent (australia) is wanted, Argentina's (south_america) isn't, got %d", cmd.Armies)
	}
}

func TestPixieStrategyFortifyPrefersMostEnemyNeighborDestination(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	g.Territories["Madagascar"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["South Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["East Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}

	strat := NewPixieStrategy()
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

func TestPixieStrategyFortifyEndsTurnWhenNoDestinationFacesAnEnemy(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 3}
	}

	strat := NewPixieStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionEndTurn {
		t.Fatalf("expected end_turn when no fortification destination faces a threat, got %s", cmd.Action)
	}
}
