package bot

import (
	"context"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

func TestAngryStrategySetupReinforcePrefersMostEnemyNeighborTerritoriesNotArmySum(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseSetupReinforce
	g.SetupReserves[0] = 5
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 1}
	// Alaska's neighbors (Northwest Territory, Alberta, Kamchatka) stay at
	// the default owner=1/armies=1 -> 3 enemy neighbor territories, sum=3.
	// Argentina's neighbors (Peru, Brazil) have far more armies each, but
	// there are only 2 of them -> 2 enemy neighbor territories, sum=200.
	// Angry must pick Alaska: it counts neighbor territories, not armies.
	g.Territories["Peru"] = risk.TerritoryState{Owner: 1, Armies: 100}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: 1, Armies: 100}

	strat := NewAngryStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceInitialArmy {
		t.Fatalf("expected place_initial_army, got %s", cmd.Action)
	}
	if cmd.Territory != "Alaska" {
		t.Fatalf("expected Alaska (3 enemy neighbor territories beats Argentina's 2, despite much lower army sum), got %s", cmd.Territory)
	}
}

func TestAngryStrategyNeverVoluntarilyTradesCards(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Players[0].Cards = []risk.Card{
		{Territory: "Alaska", Symbol: risk.Infantry},
		{Territory: "Peru", Symbol: risk.Cavalry},
		{Territory: "Egypt", Symbol: risk.Artillery},
	} // a legal set exists, but Angry never cashes voluntarily

	strat := NewAngryStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceReinforcement {
		t.Fatalf("expected place_reinforcement (Angry never voluntarily trades), got %s", cmd.Action)
	}
}

func TestAngryStrategyTradesCardsWhenMandatory(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 0
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Players[0].Cards = []risk.Card{
		{Territory: "Alaska", Symbol: risk.Infantry},
		{Territory: "Peru", Symbol: risk.Cavalry},
		{Territory: "Egypt", Symbol: risk.Artillery},
		{Territory: "Ural", Symbol: risk.Infantry},
		{Territory: "Siam", Symbol: risk.Infantry},
	}

	strat := NewAngryStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionTradeCards {
		t.Fatalf("expected trade_cards with 5 cards in hand, got %s", cmd.Action)
	}
}

func TestAngryStrategyReinforceDumpsAllArmiesOnBestTerritoryInOneCommand(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 7
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 2}    // 3 enemy neighbor territories
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 2} // 2 enemy neighbor territories

	strat := NewAngryStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceReinforcement {
		t.Fatalf("expected place_reinforcement, got %s", cmd.Action)
	}
	if cmd.Territory != "Alaska" {
		t.Fatalf("expected Alaska (more enemy neighbor territories), got %s", cmd.Territory)
	}
	if cmd.Armies != 7 {
		t.Fatalf("expected every pending reinforcement dumped in one command, got %d", cmd.Armies)
	}
}

func TestAngryStrategyAttackPicksFirstQualifyingPairNotBestMatchup(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	// Alaska(5) vs its only enemy neighbor Kamchatka(4): a bare 1-army edge,
	// which is enough for Angry to attack.
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 4}
	// Argentina(10) vs Peru/Brazil(1 each, default) is a far better matchup,
	// but Argentina comes after Alaska in canonical board order, so Angry
	// (which takes the first qualifying pair, not the best one) must still
	// pick Alaska -> Kamchatka.
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 10}

	strat := NewAngryStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack {
		t.Fatalf("expected attack, got %s", cmd.Action)
	}
	if cmd.From != "Alaska" || cmd.To != "Kamchatka" {
		t.Fatalf("expected Alaska -> Kamchatka (first qualifying pair in board order), got %s -> %s", cmd.From, cmd.To)
	}
	if cmd.AttackerDice != 3 {
		t.Fatalf("expected max legal attacker dice (3) for 5 armies, got %d", cmd.AttackerDice)
	}
}

func TestAngryStrategyAttackEndsWhenNoQualifyingPair(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 3} // tied, not a strict edge

	strat := NewAngryStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionEndAttack {
		t.Fatalf("expected end_attack when no owned territory outnumbers its weakest enemy neighbor, got %s", cmd.Action)
	}
}

func TestAngryStrategyOccupyLeavesMinimumWhenFromMoreThreatened(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseOccupy
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 0, Armies: 1}
	// Own three of Kamchatka's other neighbors so only Japan remains an
	// enemy there (count=1), while Alaska still faces Northwest Territory
	// and Alberta as enemies (count=2) -> Alaska is more threatened.
	g.Territories["Yakutsk"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Irkutsk"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Mongolia"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Occupy = &risk.OccupyState{From: "Alaska", To: "Kamchatka", MinMove: 2, MaxMove: 4}

	strat := NewAngryStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionOccupy {
		t.Fatalf("expected occupy, got %s", cmd.Action)
	}
	if cmd.Armies != 2 {
		t.Fatalf("expected the legal minimum (2) left in the more-threatened From, got %d", cmd.Armies)
	}
}

func TestAngryStrategyOccupySendsMaximumOtherwise(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseOccupy
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 0, Armies: 1}
	// Alaska faces 2 enemies (Northwest Territory, Alberta); Kamchatka
	// faces 4 (Yakutsk, Irkutsk, Mongolia, Japan) -> To is more threatened.
	g.Occupy = &risk.OccupyState{From: "Alaska", To: "Kamchatka", MinMove: 2, MaxMove: 4}

	strat := NewAngryStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionOccupy {
		t.Fatalf("expected occupy, got %s", cmd.Action)
	}
	if cmd.Armies != 4 {
		t.Fatalf("expected the legal maximum (4) sent to the more-threatened To, got %d", cmd.Armies)
	}
}

func TestAngryStrategyFortifyPrefersMostEnemyNeighborDestination(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	g.Territories["Madagascar"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["South Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["East Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}
	// East Africa borders 4 enemy territories (Egypt, North Africa, Congo,
	// Middle East, all left at the default owner=1) -- the best fortify
	// destination. Both Madagascar and South Africa can reach it; South
	// Africa wins the tie by coming first in canonical board order.

	strat := NewAngryStrategy()
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
	if cmd.Armies != 1 {
		t.Fatalf("expected max legal fortify amount from South Africa (armies-1=1), got %d", cmd.Armies)
	}
}

func TestAngryStrategyFortifyEndsTurnWhenNoDestinationFacesAnEnemy(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 3}
	}

	strat := NewAngryStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionEndTurn {
		t.Fatalf("expected end_turn when no fortification destination faces a threat, got %s", cmd.Action)
	}
}

func TestAngryStrategyAlreadyFortifiedEndsTurn(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	g.HasFortified = true
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 0, Armies: 1}

	strat := NewAngryStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionEndTurn {
		t.Fatalf("expected end_turn after already fortifying, got %s", cmd.Action)
	}
}
