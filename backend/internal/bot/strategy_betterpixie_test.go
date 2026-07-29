package bot

import (
	"context"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

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
