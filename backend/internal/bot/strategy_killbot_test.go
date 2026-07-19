package bot

import (
	"context"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

func TestKillbotStrategyTradesCardsVoluntarily(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Players[0].Cards = []risk.Card{
		{Territory: "Alaska", Symbol: risk.Infantry},
		{Territory: "Peru", Symbol: risk.Cavalry},
		{Territory: "Egypt", Symbol: risk.Artillery},
	}

	strat := NewKillbotStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionTradeCards {
		t.Fatalf("expected trade_cards (Killbot trades any legal set, via BetterPixie's own voluntary cash policy), got %s", cmd.Action)
	}
}

func TestKillbotStrategyReinforcePlacesTowardKillTarget(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 5
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 1, Armies: 1000}
	}
	g.Territories["Central America"] = risk.TerritoryState{Owner: 1, Armies: 2}
	g.Territories["Western United States"] = risk.TerritoryState{Owner: 0, Armies: 100}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 2, Armies: 1}

	strat := NewKillbotStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceReinforcement {
		t.Fatalf("expected place_reinforcement, got %s", cmd.Action)
	}
	if cmd.Territory != "Western United States" {
		t.Fatalf("expected Western United States (routing toward the kill target via Central America), got %s", cmd.Territory)
	}
	if cmd.Armies != 5 {
		t.Fatalf("expected every pending reinforcement dumped in one command, got %d", cmd.Armies)
	}
}

func TestKillbotStrategyAttackFiresKillBranch(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 1, Armies: 1000}
	}
	g.Territories["Central America"] = risk.TerritoryState{Owner: 1, Armies: 2}
	g.Territories["Western United States"] = risk.TerritoryState{Owner: 0, Armies: 100}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 2, Armies: 1}

	strat := NewKillbotStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack {
		t.Fatalf("expected attack, got %s", cmd.Action)
	}
	if cmd.From != "Western United States" || cmd.To != "Central America" {
		t.Fatalf("expected Western United States -> Central America (kill-target branch), got %s -> %s", cmd.From, cmd.To)
	}
}

func TestKillbotStrategyAttackFallsBackToPixieStyleWhenNoKillTarget(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Peru"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: 1, Armies: 3}

	strat := NewKillbotStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack {
		t.Fatalf("expected attack, got %s", cmd.Action)
	}
	if cmd.From != "Venezuela" || cmd.To != "Brazil" {
		t.Fatalf("expected Venezuela -> Brazil (Pixie-style fallback: no rival is weak enough to trigger the kill branch), got %s -> %s", cmd.From, cmd.To)
	}
}

func TestKillbotStrategyAttackEndsWhenNothingQualifies(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}

	strat := NewKillbotStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionEndAttack {
		t.Fatalf("expected end_attack, got %s", cmd.Action)
	}
}

func TestKillbotStrategyOccupyDelegatesToPixieLogic(t *testing.T) {
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

	strat := NewKillbotStrategy()
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

func TestKillbotStrategyFortifyPrefersMostEnemyNeighborDestination(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	g.Territories["Madagascar"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["South Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["East Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}

	strat := NewKillbotStrategy()
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
