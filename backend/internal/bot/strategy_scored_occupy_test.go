package bot

import (
	"context"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// TestScoredStrategyOccupyBalancesDefenseAndMomentum hand-derives the
// exact optimal armies count: sourceArmies=10, sourceThreat=6 (Northwest
// Territory=5 + Alberta=1, both enemy), destThreat=1 (only Japan left
// enemy among Kamchatka's neighbors; Yakutsk/Irkutsk/Mongolia are owned).
//
// score(armies) = 1.5*min(10-armies, 6) + 1.5*min(armies, 1) + 0.05*armies
//
// Momentum caps out at armies=1 (destThreat=1), so beyond that only
// defense_coverage and the small surplus term move. defense_coverage caps
// at remaining=6, i.e. armies=4 (10-4=6) -- past that point, every extra
// army costs 1.5 in defense_coverage but only gains 0.05 in surplus, so
// armies=4 is the strict maximum (verified by hand across the full legal
// range: scores are 10.55, 10.6, 10.65, 10.7, 9.25 for armies=1..5, then
// keep dropping).
func TestScoredStrategyOccupyBalancesDefenseAndMomentum(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseOccupy
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 10}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 1, Armies: 5}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 1, Armies: 1}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Yakutsk"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Irkutsk"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Mongolia"] = risk.TerritoryState{Owner: 0, Armies: 1}
	// Japan stays at its default owner=1, armies=1.
	g.Occupy = &risk.OccupyState{From: "Alaska", To: "Kamchatka", MinMove: 1, MaxMove: 9}

	strat := NewScoredStrategy(DefaultWeights)
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionOccupy {
		t.Fatalf("expected occupy, got %s", cmd.Action)
	}
	if cmd.Armies != 4 {
		t.Fatalf("expected the balanced optimum of 4 armies, got %d", cmd.Armies)
	}
}

// TestScoredStrategyOccupyPushesMaxWhenBothSidesAreSafe: with sourceThreat
// and destThreat both 0, defense_coverage and momentum are both pinned at
// 0 regardless of armies (min(x, 0) = 0), leaving only the small
// unconditional surplus term -- which strictly increases with armies, so
// the maximum legal amount wins.
func TestScoredStrategyOccupyPushesMaxWhenBothSidesAreSafe(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseOccupy
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 10}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Yakutsk"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Irkutsk"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Mongolia"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Japan"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Occupy = &risk.OccupyState{From: "Alaska", To: "Kamchatka", MinMove: 1, MaxMove: 9}

	strat := NewScoredStrategy(DefaultWeights)
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Armies != 9 {
		t.Fatalf("expected the full legal maximum (9) when neither side faces any threat, got %d", cmd.Armies)
	}
}
