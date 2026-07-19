package bot

import (
	"context"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

func TestSweepForwardSucceedsWhenInitialFrontierIsAlreadySingle(t *testing.T) {
	g, _ := newTestGame(t)
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 0, Armies: 1}
	// Alaska's only remaining neighbor, Kamchatka, is enemy-owned -- the
	// initial frontier is already a single territory, so the sweep is
	// trivially worthwhile without needing any folding.

	cmd, ok := sweepForward(g, 0, "Alaska")
	if !ok {
		t.Fatalf("expected the sweep to be worthwhile")
	}
	if cmd.Action != ActionAttack || cmd.From != "Alaska" || cmd.To != "Kamchatka" {
		t.Fatalf("expected attack Alaska -> Kamchatka, got %+v", cmd)
	}
}

func TestSweepForwardCollapsesThroughMultipleFolds(t *testing.T) {
	g, _ := newTestGame(t)
	g.Territories["New Guinea"] = risk.TerritoryState{Owner: 0, Armies: 10}
	// Indonesia, Western Australia, Eastern Australia stay at the default
	// owner=1/armies=1: Western/Eastern Australia each become fully
	// enveloped and drop out, Indonesia folds forward into Siam, landing
	// on a single remaining contact point.

	cmd, ok := sweepForward(g, 0, "New Guinea")
	if !ok {
		t.Fatalf("expected the sweep to be worthwhile")
	}
	if cmd.Action != ActionAttack || cmd.From != "New Guinea" || cmd.To != "Indonesia" {
		t.Fatalf("expected attack New Guinea -> Indonesia, got %+v", cmd)
	}
}

func TestSweepForwardFalseWhenFrontierStaysWide(t *testing.T) {
	g, _ := newTestGame(t)
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 10}
	// Peru/Brazil fold outward, but North Africa's high connectivity
	// keeps the frontier from ever collapsing to a single point.

	if _, ok := sweepForward(g, 0, "Argentina"); ok {
		t.Fatalf("expected the sweep not to be worthwhile (frontier never collapses to one)")
	}
}

func TestSweepForwardFalseWhenNoUnownedNeighbors(t *testing.T) {
	g, _ := newTestGame(t)
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 3}
	}
	if _, ok := sweepForward(g, 0, "Alaska"); ok {
		t.Fatalf("expected false when sweep has no unowned neighbors")
	}
}

func TestSweepForwardFalseWhenSweepCannotAttack(t *testing.T) {
	g, _ := newTestGame(t)
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 0, Armies: 1}

	if _, ok := sweepForward(g, 0, "Alaska"); ok {
		t.Fatalf("expected false when the sweeping territory has only 1 army")
	}
}

func TestVoluntaryCardTurnInTradesOptionalSet(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Players[0].Cards = []risk.Card{
		{Territory: "Alaska", Symbol: risk.Infantry},
		{Territory: "Peru", Symbol: risk.Cavalry},
		{Territory: "Egypt", Symbol: risk.Artillery},
	}

	cmd, ok := voluntaryCardTurnIn(g, p0)
	if !ok {
		t.Fatalf("expected a legal set to be found")
	}
	if cmd.Action != ActionTradeCards {
		t.Fatalf("expected trade_cards, got %s", cmd.Action)
	}
}

func TestVoluntaryCardTurnInFalseWithoutASet(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Players[0].Cards = []risk.Card{
		{Territory: "Alaska", Symbol: risk.Infantry},
		{Territory: "Peru", Symbol: risk.Infantry},
	}
	if _, ok := voluntaryCardTurnIn(g, p0); ok {
		t.Fatalf("expected false with fewer than 3 cards / no valid set")
	}
}

func TestQuoStrategyTradesCardsVoluntarily(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Players[0].Cards = []risk.Card{
		{Territory: "Alaska", Symbol: risk.Infantry},
		{Territory: "Peru", Symbol: risk.Cavalry},
		{Territory: "Egypt", Symbol: risk.Artillery},
	}

	strat := NewQuoStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionTradeCards {
		t.Fatalf("expected trade_cards (Quo trades any legal set, unlike Angry/Cluster/Pixie), got %s", cmd.Action)
	}
}

func TestQuoStrategyReinforceDumpsAllArmiesOnWeakestBorderOfOwnedContinent(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 6
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Peru"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 2}

	strat := NewQuoStrategy()
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

func TestQuoStrategyAttackUsesClusterStagesFirst(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 4}

	strat := NewQuoStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack {
		t.Fatalf("expected attack, got %s", cmd.Action)
	}
	if cmd.From != "Alaska" || cmd.To != "Kamchatka" {
		t.Fatalf("expected Alaska -> Kamchatka (easy-expand fires before the sweep stage), got %s -> %s", cmd.From, cmd.To)
	}
}

func TestQuoStrategyAttackFallsThroughToSweep(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	g.Territories["New Guinea"] = risk.TerritoryState{Owner: 0, Armies: 10}

	strat := NewQuoStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack {
		t.Fatalf("expected attack, got %s", cmd.Action)
	}
	if cmd.From != "New Guinea" || cmd.To != "Indonesia" {
		t.Fatalf("expected New Guinea -> Indonesia (via the sweep stage), got %s -> %s", cmd.From, cmd.To)
	}
}

func TestQuoStrategyAttackSkipsSweepWhenAlreadyConqueredThisTurn(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	g.ConqueredThisTurn = true
	g.Territories["New Guinea"] = risk.TerritoryState{Owner: 0, Armies: 10}

	strat := NewQuoStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	// The sweep stage is skipped once a conquest already happened this
	// turn (same gate suppresses attackForCard); nothing else qualifies
	// here (not dominant enough for hogwild), so the turn ends.
	if cmd.Action != ActionEndAttack {
		t.Fatalf("expected end_attack, got %s", cmd.Action)
	}
}

func TestQuoStrategyOccupyLeavesMinimumWhenFromHasWeakerContinentMatchup(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseOccupy
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 0, Armies: 10}
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 1, Armies: 5}
	g.Territories["Ontario"] = risk.TerritoryState{Owner: 1, Armies: 1}
	g.Territories["Western United States"] = risk.TerritoryState{Owner: 1, Armies: 5}
	g.Occupy = &risk.OccupyState{From: "Alberta", To: "Alaska", MinMove: 2, MaxMove: 8}

	strat := NewQuoStrategy()
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

func TestQuoStrategyFortifyPrefersMostEnemyNeighborDestination(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	g.Territories["Madagascar"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["South Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["East Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}

	strat := NewQuoStrategy()
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
