package bot

import (
	"context"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// TestBetterPixieStrategyTradesCardsVoluntarily confirms reinforce()
// trades any legal card set, not just mandatory ones -- Lux's
// BetterPixie.cardsPhase calls cashCardsIfPossible, matching
// Killbot/Quo/Boscoe's own identical override.
func TestBetterPixieStrategyTradesCardsVoluntarily(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Players[0].Cards = []risk.Card{
		{Territory: "Alaska", Symbol: risk.Infantry},
		{Territory: "Peru", Symbol: risk.Cavalry},
		{Territory: "Egypt", Symbol: risk.Artillery},
	}

	strat := NewBetterPixieStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionTradeCards {
		t.Fatalf("expected trade_cards (BetterPixie trades any legal set), got %s", cmd.Action)
	}
}

// TestBetterPixieFortifyDrainsTowardContinentBorder confirms
// betterPixieFortifyDestination prefers moving armies toward Australia's
// one border (Indonesia, the continent's only territory adjacent to
// anything outside it) when pi owns the whole continent outright. Only
// Eastern Australia (distance 2 from the border) has more than 1 army,
// so it's the only legal source; among its three legal destinations
// (Indonesia at distance 0, New Guinea and Western Australia at distance
// 1), Indonesia -- the largest border-distance reduction -- should win.
func TestBetterPixieFortifyDrainsTowardContinentBorder(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	g.Territories["Indonesia"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["New Guinea"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Western Australia"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Eastern Australia"] = risk.TerritoryState{Owner: 0, Armies: 5}

	bp := NewBetterPixieStrategy()
	cmd, _, err := bp.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionFortify || cmd.From != "Eastern Australia" || cmd.To != "Indonesia" {
		t.Fatalf("expected fortify Eastern Australia -> Indonesia (the largest border-distance reduction), got %+v", cmd)
	}
}

// TestBetterPixieFortifyEndsTurnWhenNoLegalMove mirrors every other
// persona's own version of this test.
func TestBetterPixieFortifyEndsTurnWhenNoLegalMove(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	// p0 owns nothing, so risk.LegalFortifications is empty.
	bp := NewBetterPixieStrategy()
	cmd, _, err := bp.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionEndTurn {
		t.Fatalf("expected end_turn with no legal fortification, got %+v", cmd)
	}
}
