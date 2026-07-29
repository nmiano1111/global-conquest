package bot

import (
	"context"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

func TestContinentBorderSecureTrueWhenDefenceClearsThreshold(t *testing.T) {
	g, _ := newTestGame(t)
	for _, terr := range g.Board.Continents["south_america"].Territories {
		g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 10}
	}
	g.Territories["Central America"] = risk.TerritoryState{Owner: 1, Armies: 1} // Venezuela's only outside neighbor
	g.Territories["North Africa"] = risk.TerritoryState{Owner: 1, Armies: 1}    // Brazil's only outside neighbor

	if !continentBorderSecure(g, 0, "south_america", turtleSecurityThreshold) {
		t.Fatalf("expected secure: Defence at 10 armies/78 total is well above threshold")
	}
}

func TestContinentBorderSecureFalseWhenOneBorderIsThin(t *testing.T) {
	g, _ := newTestGame(t)
	for _, terr := range g.Board.Continents["south_america"].Territories {
		g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 10}
	}
	g.Territories["Central America"] = risk.TerritoryState{Owner: 1, Armies: 1}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["North Africa"] = risk.TerritoryState{Owner: 1, Armies: 1000}

	if continentBorderSecure(g, 0, "south_america", turtleSecurityThreshold) {
		t.Fatalf("expected insecure: Defence at 2 armies/~1069 total is far below threshold")
	}
}

func TestContinentBorderSecureTrueWhenBorderHasNoEnemyNeighbor(t *testing.T) {
	g, _ := newTestGame(t)
	for _, terr := range g.Board.Continents["south_america"].Territories {
		g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 10}
	}
	// pi also owns both outside neighbors -- no enemy adjacent to any
	// border territory at all.
	g.Territories["Central America"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["North Africa"] = risk.TerritoryState{Owner: 0, Armies: 1}

	if !continentBorderSecure(g, 0, "south_america", turtleSecurityThreshold) {
		t.Fatalf("expected secure: no border territory has any enemy neighbor to be threatened by")
	}
}

func TestTurtleStrategyAttackHoldsWhenHomeContinentInsecure(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	for _, terr := range g.Board.Continents["south_america"].Territories {
		g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 1}
	}
	g.Territories["Central America"] = risk.TerritoryState{Owner: 1, Armies: 1}
	g.Territories["North Africa"] = risk.TerritoryState{Owner: 1, Armies: 1000} // makes Brazil's border insecure

	strat := NewTurtleStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionEndAttack {
		t.Fatalf("expected end_attack (home continent insecure, no free attackForCard opportunity exists), got %s", cmd.Action)
	}
}

func TestTurtleStrategyAttackExpandsWhenHomeContinentSecure(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	for _, terr := range g.Board.Continents["south_america"].Territories {
		g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 10}
	}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 0, Armies: 10}
	g.Territories["Central America"] = risk.TerritoryState{Owner: 1, Armies: 1}
	g.Territories["North Africa"] = risk.TerritoryState{Owner: 1, Armies: 1}

	strat := NewTurtleStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack {
		t.Fatalf("expected attack (home continent secure, normal expansion resumes), got %s", cmd.Action)
	}
	if cmd.From != "Venezuela" || cmd.To != "Central America" {
		t.Fatalf("expected Venezuela -> Central America (easy-expand, sole enemy neighbor), got %s -> %s", cmd.From, cmd.To)
	}
}

func TestTurtleStrategyAttackExpandsWithNoHomeContinentYet(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 5}
	// Alaska also borders Northwest Territory and Alberta by default --
	// own those too so Kamchatka is its only remaining enemy neighbor
	// (enemyNeighborCount == 1, required for easy-expand to fire).
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1}

	strat := NewTurtleStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack || cmd.From != "Alaska" || cmd.To != "Kamchatka" {
		t.Fatalf("expected Alaska -> Kamchatka (normal cluster expansion, no home continent to gate on), got %s: %s -> %s", cmd.Action, cmd.From, cmd.To)
	}
}

func TestTurtleFortifyPicksMostExposedBorderTerritory(t *testing.T) {
	g, _ := newTestGame(t)
	for _, terr := range g.Board.Continents["south_america"].Territories {
		g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 1}
	}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: 0, Armies: 2} // most exposed: lowest raw army count among the two border territories
	g.Territories["Peru"] = risk.TerritoryState{Owner: 0, Armies: 10}  // best available source
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Territories["Central America"] = risk.TerritoryState{Owner: 1, Armies: 1}
	g.Territories["North Africa"] = risk.TerritoryState{Owner: 1, Armies: 8}
	g.Phase = risk.PhaseFortify

	actions := risk.LegalFortifications(g, g.Players[0].ID)
	best, ok := turtleFortify(g, 0, actions)
	if !ok {
		t.Fatalf("expected a fortification move to be found")
	}
	if best.To != "Brazil" {
		t.Fatalf("expected Brazil as the destination (lowest raw army count among border territories), got %s", best.To)
	}
	if best.From != "Peru" {
		t.Fatalf("expected Peru as the source (highest MaxArmies among moves into Brazil), got %s", best.From)
	}
	if best.MaxArmies != 9 {
		t.Fatalf("expected MaxArmies 9 (Peru's 10 armies - 1), got %d", best.MaxArmies)
	}
}

func TestTurtleFortifyFalseWithNoHomeContinent(t *testing.T) {
	g, _ := newTestGame(t)
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Phase = risk.PhaseFortify

	actions := risk.LegalFortifications(g, g.Players[0].ID)
	if _, ok := turtleFortify(g, 0, actions); ok {
		t.Fatalf("expected ok=false: pi owns no positive-bonus continent yet")
	}
}

func TestTurtleStrategyFortifyFallsBackToBestFortifyDestination(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	g.Territories["Madagascar"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["South Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["East Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}

	strat := NewTurtleStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionFortify {
		t.Fatalf("expected fortify, got %s", cmd.Action)
	}
	if cmd.From != "South Africa" || cmd.To != "East Africa" {
		t.Fatalf("expected South Africa -> East Africa (bestFortifyDestination fallback, no home continent), got %s -> %s", cmd.From, cmd.To)
	}
}

func TestTurtleStrategyReinforceTradesCardsVoluntarily(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Players[0].Cards = []risk.Card{
		{Territory: "Alaska", Symbol: risk.Infantry},
		{Territory: "Peru", Symbol: risk.Cavalry},
		{Territory: "Egypt", Symbol: risk.Artillery},
	}

	strat := NewTurtleStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionTradeCards {
		t.Fatalf("expected trade_cards (Turtle trades any legal set, like Quo/Boscoe), got %s", cmd.Action)
	}
}
