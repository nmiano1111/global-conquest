package bot

import (
	"context"
	"testing"

	"backend/internal/risk"
)

// TestScoredStrategyCardTurnInDelaysWithNoUrgency: 3 cards (below the
// approaching-limit threshold), no border under pressure, no marginal
// continent/elimination attack anywhere -- the strategy should hold the
// cards and place reinforcements normally instead.
func TestScoredStrategyCardTurnInDelaysWithNoUrgency(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	// threat = Northwest Territory(1) + Alberta(1) + Kamchatka(1) = 3,
	// equal to (not greater than) armies -- not under pressure.
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Players[0].Cards = []risk.Card{
		{Territory: "Alaska", Symbol: risk.Infantry},
		{Territory: "Peru", Symbol: risk.Cavalry},
		{Territory: "Egypt", Symbol: risk.Artillery},
	}

	strat := NewScoredStrategy(DefaultWeights)
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceReinforcement {
		t.Fatalf("expected place_reinforcement (cards held for later), got %s", cmd.Action)
	}
}

// TestScoredStrategyCardTurnInApproachingLimit: 4 cards (one below the
// mandatory threshold) with no other urgency should still trade in now,
// to avoid being forced into a worse trade later.
func TestScoredStrategyCardTurnInApproachingLimit(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Players[0].Cards = []risk.Card{
		{Territory: "Alaska", Symbol: risk.Infantry},
		{Territory: "Peru", Symbol: risk.Cavalry},
		{Territory: "Egypt", Symbol: risk.Artillery},
		{Territory: "Ural", Symbol: risk.Infantry},
	}

	strat := NewScoredStrategy(DefaultWeights)
	cmd, expl, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionTradeCards {
		t.Fatalf("expected trade_cards when approaching the card limit, got %s", cmd.Action)
	}
	if len(expl.Features) != 1 || expl.Features[0].Name != "approaching_card_limit" {
		t.Fatalf("expected reason approaching_card_limit, got %+v", expl.Features)
	}
}

// TestScoredStrategyCardTurnInUnderPressure: only 3 cards, but Alaska is
// clearly outnumbered (threat 3 vs 1 army) -- trade in immediately for the
// defensive bonus rather than waiting.
func TestScoredStrategyCardTurnInUnderPressure(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Players[0].Cards = []risk.Card{
		{Territory: "Alaska", Symbol: risk.Infantry},
		{Territory: "Peru", Symbol: risk.Cavalry},
		{Territory: "Egypt", Symbol: risk.Artillery},
	}

	strat := NewScoredStrategy(DefaultWeights)
	cmd, expl, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionTradeCards {
		t.Fatalf("expected trade_cards while under pressure, got %s", cmd.Action)
	}
	if len(expl.Features) != 1 || expl.Features[0].Name != "under_pressure" {
		t.Fatalf("expected reason under_pressure, got %+v", expl.Features)
	}
}

// TestScoredStrategyCardTurnInEnablesContinent: player 0 owns 3 of South
// America's 4 territories; Argentina (the last) is defended by 1 army,
// attackable from either Peru or Brazil (both armies 2) at exactly the
// hand-verified 2-vs-1 win probability (15/36 ~ 0.4167, inside the
// marginal band) -- neither Peru nor Brazil is itself under pressure
// (their threat exactly equals their own armies), so this isolates the
// continent-completion trigger.
func TestScoredStrategyCardTurnInEnablesContinent(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Peru"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 1, Armies: 1}
	g.Players[0].Cards = []risk.Card{
		{Territory: "Alaska", Symbol: risk.Infantry},
		{Territory: "Ural", Symbol: risk.Cavalry},
		{Territory: "Japan", Symbol: risk.Artillery},
	}

	strat := NewScoredStrategy(DefaultWeights)
	cmd, expl, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionTradeCards {
		t.Fatalf("expected trade_cards to enable a continent completion, got %s", cmd.Action)
	}
	if len(expl.Features) != 1 || expl.Features[0].Name != "enables_continent" {
		t.Fatalf("expected reason enables_continent, got %+v", expl.Features)
	}
}

// TestScoredStrategyCardTurnInEnablesElimination: reassign the whole board
// to player 2 except a small p0 pocket (Alaska, Alberta) and Kamchatka,
// player 1's only remaining territory -- attackable from Alaska at the
// same hand-verified 2-vs-1 marginal probability. Alaska and Alberta are
// each given just enough armies that their own threat doesn't exceed
// them, isolating the elimination trigger from the pressure trigger.
func TestScoredStrategyCardTurnInEnablesElimination(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 2, Armies: 1}
	}
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1}
	g.Players[0].Cards = []risk.Card{
		{Territory: "Egypt", Symbol: risk.Infantry},
		{Territory: "Ural", Symbol: risk.Cavalry},
		{Territory: "Japan", Symbol: risk.Artillery},
	}

	strat := NewScoredStrategy(DefaultWeights)
	cmd, expl, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionTradeCards {
		t.Fatalf("expected trade_cards to enable an elimination, got %s", cmd.Action)
	}
	if len(expl.Features) != 1 || expl.Features[0].Name != "enables_elimination" {
		t.Fatalf("expected reason enables_elimination, got %+v", expl.Features)
	}
}

// TestBestCardSetPrefersTerritoryBonusMatch: among the two valid sets in
// this hand -- {Peru,Egypt,Ural} (indices 0,1,2, no owned territory) and
// {Egypt,Ural,Alaska} (indices 1,2,3, Alaska owned by p0) -- the second
// should win despite its higher indices, since it earns the once-per-turn
// +2 territory bonus.
func TestBestCardSetPrefersTerritoryBonusMatch(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 4}
	g.Players[0].Cards = []risk.Card{
		{Territory: "Peru", Symbol: risk.Cavalry},
		{Territory: "Egypt", Symbol: risk.Artillery},
		{Territory: "Ural", Symbol: risk.Infantry},
		{Territory: "Alaska", Symbol: risk.Cavalry},
	}

	strat := NewScoredStrategy(DefaultWeights)
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionTradeCards {
		t.Fatalf("expected trade_cards, got %s", cmd.Action)
	}
	if cmd.CardIndices != [3]int{1, 2, 3} {
		t.Fatalf("expected the set including Alaska (territory bonus match), got %v", cmd.CardIndices)
	}
}

// TestBestCardSetIgnoresMatchOnceTerritoryBonusUsed: same hand, but the
// territory bonus is already spent this turn -- falls back to the
// deterministic lowest-index set.
func TestBestCardSetIgnoresMatchOnceTerritoryBonusUsed(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	g.TerritoryBonusUsed = true
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 4}
	g.Players[0].Cards = []risk.Card{
		{Territory: "Peru", Symbol: risk.Cavalry},
		{Territory: "Egypt", Symbol: risk.Artillery},
		{Territory: "Ural", Symbol: risk.Infantry},
		{Territory: "Alaska", Symbol: risk.Cavalry},
	}

	strat := NewScoredStrategy(DefaultWeights)
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.CardIndices != [3]int{0, 1, 2} {
		t.Fatalf("expected the lowest-index set once the territory bonus is already used this turn, got %v", cmd.CardIndices)
	}
}
