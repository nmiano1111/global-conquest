package bot

import (
	"context"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

func TestPlacementToKillPlayerPicksCheaperOfTwoOwnedContinents(t *testing.T) {
	g, _ := newTestGame(t)
	// Target (player 2) fully owns south_america (cheap to reach) and
	// australia (far more expensive to reach). Block everything else so
	// only the two intended routes are viable candidates at all.
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 1, Armies: 1000}
	}
	for _, terr := range g.Board.Continents["south_america"].Territories {
		g.Territories[terr] = risk.TerritoryState{Owner: 2, Armies: 1000}
	}
	for _, terr := range g.Board.Continents["australia"].Territories {
		g.Territories[terr] = risk.TerritoryState{Owner: 2, Armies: 1000}
	}
	g.Territories["Western United States"] = risk.TerritoryState{Owner: 0, Armies: 20}
	g.Territories["Central America"] = risk.TerritoryState{Owner: 1, Armies: 2} // cheap route to south_america
	g.Territories["China"] = risk.TerritoryState{Owner: 0, Armies: 20}
	g.Territories["Siam"] = risk.TerritoryState{Owner: 1, Armies: 50} // expensive route to australia

	best, ok := placementToKillPlayer(g, 0, 2)
	if !ok {
		t.Fatalf("expected a placement route to be found")
	}
	if best != "Western United States" {
		t.Fatalf("expected Western United States (cheaper route to south_america, cost 2 vs 50), got %s", best)
	}
}

func TestAttackToKillPlayerTriesBiggestBonusContinentFirst(t *testing.T) {
	g, _ := newTestGame(t)
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 1, Armies: 1000}
	}
	for _, terr := range g.Board.Continents["south_america"].Territories {
		g.Territories[terr] = risk.TerritoryState{Owner: 2, Armies: 1000}
	}
	for _, terr := range g.Board.Continents["asia"].Territories {
		g.Territories[terr] = risk.TerritoryState{Owner: 2, Armies: 1000}
	}
	// South america's only cheap entry: Central America (cost 2).
	g.Territories["Central America"] = risk.TerritoryState{Owner: 1, Armies: 2}
	g.Territories["Western United States"] = risk.TerritoryState{Owner: 0, Armies: 20}
	// Asia's only cheap entry: East Africa (cost 3), via Middle East.
	g.Territories["East Africa"] = risk.TerritoryState{Owner: 1, Armies: 3}
	g.Territories["North Africa"] = risk.TerritoryState{Owner: 0, Armies: 20}

	cmd, ok := attackToKillPlayer(g, 0, 2)
	if !ok {
		t.Fatalf("expected an attack to be found")
	}
	if cmd.From != "North Africa" || cmd.To != "East Africa" {
		t.Fatalf("expected North Africa -> East Africa (asia, bonus 7, tried before south_america, bonus 2), got %s -> %s", cmd.From, cmd.To)
	}
}

func TestAttackToKillPlayerFalseWhenRouteExistsButNotEnoughArmies(t *testing.T) {
	g, _ := newTestGame(t)
	// Block every other path so Central America is unambiguously the
	// cheapest (and only) route -- see the geometry_test.go tests' own
	// comments on why an unprotected setup can let a long chain of
	// default-armies hops undercut an intended single expensive hop.
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 1, Armies: 1000}
	}
	for _, terr := range g.Board.Continents["south_america"].Territories {
		g.Territories[terr] = risk.TerritoryState{Owner: 2, Armies: 1000}
	}
	g.Territories["Central America"] = risk.TerritoryState{Owner: 1, Armies: 50}
	g.Territories["Western United States"] = risk.TerritoryState{Owner: 0, Armies: 10} // 10 <= 50, not viable

	if _, ok := attackToKillPlayer(g, 0, 2); ok {
		t.Fatalf("expected ok=false: the only route costs more than the owning territory's armies")
	}
}

func TestBoscoeStrategyTradesCardsVoluntarily(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Players[0].Cards = []risk.Card{
		{Territory: "Alaska", Symbol: risk.Infantry},
		{Territory: "Peru", Symbol: risk.Cavalry},
		{Territory: "Egypt", Symbol: risk.Artillery},
	}

	strat := NewBoscoeStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionTradeCards {
		t.Fatalf("expected trade_cards (Boscoe trades any legal set, like Quo), got %s", cmd.Action)
	}
}

func TestBoscoeStrategyReinforcePlacesToKillDominantPlayer(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 5
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 1, Armies: 1000}
	}
	g.Territories["Central America"] = risk.TerritoryState{Owner: 1, Armies: 2}
	g.Territories["Western United States"] = risk.TerritoryState{Owner: 0, Armies: 20}

	strat := NewBoscoeStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceReinforcement {
		t.Fatalf("expected place_reinforcement, got %s", cmd.Action)
	}
	if cmd.Territory != "Western United States" {
		t.Fatalf("expected Western United States (routing reinforcements toward the dominant player's cheapest owned continent), got %s", cmd.Territory)
	}
	if cmd.Armies != 5 {
		t.Fatalf("expected every pending reinforcement dumped in one command, got %d", cmd.Armies)
	}
}

func TestBoscoeStrategyAttackFiresKillDominantPlayerBranch(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 1, Armies: 1000}
	}
	g.Territories["Central America"] = risk.TerritoryState{Owner: 1, Armies: 2}
	g.Territories["Western United States"] = risk.TerritoryState{Owner: 0, Armies: 20}

	strat := NewBoscoeStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack {
		t.Fatalf("expected attack, got %s", cmd.Action)
	}
	if cmd.From != "Western United States" || cmd.To != "Central America" {
		t.Fatalf("expected Western United States -> Central America (kill-dominant-player branch), got %s -> %s", cmd.From, cmd.To)
	}
}

func TestBoscoeStrategyAttackUsesNormalClusterSequenceWhenNoDominantPlayer(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	// Give player 0 (pi, excluded from the dominance check) the bulk of
	// the board so neither remaining player crosses 50% of anything.
	sentinels := map[risk.Territory]bool{
		"Venezuela": true, "Iceland": true, "North Africa": true,
		"Ural": true, "Indonesia": true,
	}
	for _, terr := range g.Board.Order {
		if !sentinels[terr] {
			g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 1}
		}
	}
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 4}

	strat := NewBoscoeStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack {
		t.Fatalf("expected attack, got %s", cmd.Action)
	}
	if cmd.From != "Alaska" || cmd.To != "Kamchatka" {
		t.Fatalf("expected Alaska -> Kamchatka (normal easy-expand, no dominant player), got %s -> %s", cmd.From, cmd.To)
	}
}

func TestBoscoeStrategyOccupyLeavesMinimumWhenFromHasWeakerContinentMatchup(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseOccupy
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 0, Armies: 10}
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 1, Armies: 5}
	g.Territories["Ontario"] = risk.TerritoryState{Owner: 1, Armies: 1}
	g.Territories["Western United States"] = risk.TerritoryState{Owner: 1, Armies: 5}
	g.Occupy = &risk.OccupyState{From: "Alberta", To: "Alaska", MinMove: 2, MaxMove: 8}

	strat := NewBoscoeStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionOccupy {
		t.Fatalf("expected occupy, got %s", cmd.Action)
	}
	if cmd.Armies != 2 {
		t.Fatalf("expected the legal minimum (2), got %d", cmd.Armies)
	}
}

func TestBoscoeStrategyFortifyPrefersMostEnemyNeighborDestination(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	g.Territories["Madagascar"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["South Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["East Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}

	strat := NewBoscoeStrategy()
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
