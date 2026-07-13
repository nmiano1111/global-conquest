package bot

import (
	"context"
	"testing"

	"backend/internal/risk"
)

// newTestGame builds a 3-player classic game with a fixed shuffle (so
// player order and the initial deck are deterministic), then blanks out
// territory ownership so each test can set up its own scenario.
func newTestGame(t *testing.T) (*risk.Game, string) {
	t.Helper()
	g, err := risk.NewClassicGame([]string{"p1", "p2", "p3"}, fixedRNG{})
	if err != nil {
		t.Fatalf("new game: %v", err)
	}
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 1, Armies: 1}
	}
	return g, g.Players[0].ID
}

// fixedRNG always returns 0, keeping player order equal to input order.
type fixedRNG struct{}

func (fixedRNG) IntN(int) int { return 0 }

func TestBasicStrategySetupReinforcePrefersThreatenedBorder(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseSetupReinforce
	g.SetupReserves[0] = 5

	// Alaska borders three enemy territories (Northwest Territory, Alberta,
	// Kamchatka), each with 1 army = 3 total. Argentina only borders Peru
	// and Brazil (2 enemy armies).
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 5} // heavily defended neighbor

	strat := NewBasicStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceInitialArmy {
		t.Fatalf("expected place_initial_army, got %s", cmd.Action)
	}
	if cmd.Territory != "Alaska" {
		t.Fatalf("expected Alaska (heavier enemy threat), got %s", cmd.Territory)
	}
}

func TestBasicStrategyCardTurnInMandatory(t *testing.T) {
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

	strat := NewBasicStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionTradeCards {
		t.Fatalf("expected trade_cards with 5 cards in hand, got %s", cmd.Action)
	}
}

func TestBasicStrategyCardTurnInOptional(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Players[0].Cards = []risk.Card{
		{Territory: "Alaska", Symbol: risk.Infantry},
		{Territory: "Peru", Symbol: risk.Cavalry},
		{Territory: "Egypt", Symbol: risk.Artillery},
	}

	strat := NewBasicStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionTradeCards {
		t.Fatalf("expected an optional legal set to be turned in immediately, got %s", cmd.Action)
	}
}

func TestBasicStrategyNoLegalSetPlacesReinforcementInstead(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Players[0].Cards = []risk.Card{
		{Territory: "Alaska", Symbol: risk.Infantry},
		{Territory: "Peru", Symbol: risk.Infantry},
	} // fewer than 3 cards: no possible set

	strat := NewBasicStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceReinforcement {
		t.Fatalf("expected place_reinforcement with no legal set, got %s", cmd.Action)
	}
}

func TestBasicStrategyCardTurnInDeterministicAmongMultipleSets(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	// Two wilds mean almost any trio forms a valid set; the lowest-index
	// combination (0,1,2) must always be chosen.
	g.Players[0].Cards = []risk.Card{
		{Symbol: risk.Wild},
		{Territory: "Peru", Symbol: risk.Cavalry},
		{Territory: "Egypt", Symbol: risk.Artillery},
		{Territory: "Ural", Symbol: risk.Infantry},
	}

	strat := NewBasicStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionTradeCards {
		t.Fatalf("expected trade_cards, got %s", cmd.Action)
	}
	if cmd.CardIndices != [3]int{0, 1, 2} {
		t.Fatalf("expected deterministic lowest-index set, got %v", cmd.CardIndices)
	}
}

func TestBasicStrategyReinforcePrefersBorderWeakestRelativeToThreat(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 4
	// Alaska: 1 own army vs Kamchatka's 5 enemy armies -> very weak.
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 5}
	// Argentina: 4 own armies vs Peru/Brazil's 1 army each -> not threatened.
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 4}

	strat := NewBasicStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceReinforcement {
		t.Fatalf("expected place_reinforcement, got %s", cmd.Action)
	}
	if cmd.Territory != "Alaska" {
		t.Fatalf("expected Alaska (weaker relative to threat), got %s", cmd.Territory)
	}
	if cmd.Armies != 4 {
		t.Fatalf("expected all 4 pending reinforcements placed at once, got %d", cmd.Armies)
	}
}

func TestBasicStrategyAttackThresholdEnforced(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	// Alaska(3) vs Kamchatka(2): 3 < 2+2, not eligible.
	// Alaska's neighbors are Northwest Territory, Alberta, and Kamchatka;
	// own the first two so Kamchatka is the only attackable neighbor.
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 2}

	strat := NewBasicStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionEndAttack {
		t.Fatalf("expected end_attack when no attack meets the +2 threshold, got %s", cmd.Action)
	}
}

func TestBasicStrategyAttackPrefersWeakestTargetAndMaxDice(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	// Alaska's neighbors are Northwest Territory, Alberta, and Kamchatka;
	// own Northwest Territory so only Kamchatka(3) and Alberta(1) remain
	// attackable. Both meet the +2 threshold (source=6); Alberta should be
	// preferred as the weaker target.
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 6}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 3}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 1, Armies: 1}

	strat := NewBasicStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack {
		t.Fatalf("expected attack, got %s", cmd.Action)
	}
	if cmd.From != "Alaska" || cmd.To != "Alberta" {
		t.Fatalf("expected Alaska -> Alberta (weakest target), got %s -> %s", cmd.From, cmd.To)
	}
	if cmd.AttackerDice != 3 {
		t.Fatalf("expected max legal attacker dice (3) for 6 armies, got %d", cmd.AttackerDice)
	}
}

func TestBasicStrategyOccupyUsesMinimum(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseOccupy
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Occupy = &risk.OccupyState{From: "Alaska", To: "Kamchatka", MinMove: 2, MaxMove: 4}

	strat := NewBasicStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionOccupy {
		t.Fatalf("expected occupy, got %s", cmd.Action)
	}
	if cmd.Armies != 2 {
		t.Fatalf("expected minimum legal occupation (2), got %d", cmd.Armies)
	}
}

func TestBasicStrategyFortifyMovesTowardBorder(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	// Madagascar's only neighbors are South Africa and East Africa; owning
	// both makes Madagascar interior (zero adjacent enemy armies). East
	// Africa borders four enemy territories (Egypt, North Africa, Congo,
	// Middle East, all left at the default owner=1/armies=1) vs South
	// Africa's one (Congo), so East Africa is the clear best destination.
	g.Territories["Madagascar"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["South Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["East Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}

	strat := NewBasicStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionFortify {
		t.Fatalf("expected fortify, got %s", cmd.Action)
	}
	if cmd.From != "Madagascar" || cmd.To != "East Africa" {
		t.Fatalf("expected Madagascar -> East Africa, got %s -> %s", cmd.From, cmd.To)
	}
	if cmd.Armies != 4 {
		t.Fatalf("expected max legal fortify amount (armies-1=4), got %d", cmd.Armies)
	}
}

func TestBasicStrategyNoUsefulFortificationEndsTurn(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	// Every territory on the board is owned by player 0: no enemy exists
	// anywhere, so no fortification can face a threat.
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 3}
	}

	strat := NewBasicStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionEndTurn {
		t.Fatalf("expected end_turn when no fortification faces a threat, got %s", cmd.Action)
	}
}

func TestBasicStrategyAlreadyFortifiedEndsTurn(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	g.HasFortified = true
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 0, Armies: 1}

	strat := NewBasicStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionEndTurn {
		t.Fatalf("expected end_turn after already fortifying, got %s", cmd.Action)
	}
}
