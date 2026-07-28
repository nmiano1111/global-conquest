package bot

import (
	"context"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// borderClusterGame builds the scenario TestGroupReinforcer*/
// TestCandidateReinforcements* share: p0 owns Alaska, Northwest Territory,
// Alberta, Ontario, and Western United States. Per risk.ClassicBoard's
// adjacency data, Alberta's own neighbors (Alaska, Northwest Territory,
// Ontario, Western United States) are ALL owned by p0 too -- Alberta has
// zero enemy-bordering neighbors, making it the one interior territory in
// this cluster. Every other owned territory borders at least one
// enemy-owned territory (Kamchatka/Greenland/Quebec/Eastern United
// States/Central America, all left at newTestGame's default owner=1).
func borderClusterGame(t *testing.T) (*risk.Game, string) {
	t.Helper()
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Ontario"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Western United States"] = risk.TerritoryState{Owner: 0, Armies: 1}
	return g, p0
}

func TestCandidateReinforcementsFiltersToEnemyBorderingTerritories(t *testing.T) {
	g, p0 := borderClusterGame(t)
	pi := playerIndex(g, p0)
	value := singleFeatureBoardValue(t, "my_army_fraction", 1.0)

	got := candidateReinforcements(g, p0, pi, value, 0)
	for _, t2 := range got {
		if t2 == "Alberta" {
			t.Fatalf("expected Alberta (zero enemy-bordering neighbors) to be filtered out, got %v", got)
		}
	}
	if len(got) != 4 {
		t.Fatalf("expected the 4 bordering territories (Alaska, Northwest Territory, Ontario, Western United States), got %d: %v", len(got), got)
	}
}

func TestCandidateReinforcementsFallsBackWhenNoneAreBordering(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	// Own literally every territory -- no territory can border an enemy.
	for _, t2 := range risk.ClassicBoard().Order {
		g.Territories[t2] = risk.TerritoryState{Owner: 0, Armies: 1}
	}
	pi := playerIndex(g, p0)
	value := singleFeatureBoardValue(t, "my_army_fraction", 1.0)

	got := candidateReinforcements(g, p0, pi, value, 0)
	want := len(risk.ClassicBoard().Order)
	if len(got) != want {
		t.Fatalf("expected the fallback to the full unfiltered set (%d territories) when none border an enemy, got %d", want, len(got))
	}
}

func TestCandidateReinforcementsCapsToTopTpByScore(t *testing.T) {
	g, p0 := borderClusterGame(t)
	pi := playerIndex(g, p0)
	// Reward Ontario specifically -- it should be the top-ranked
	// candidate once capped to tp=1.
	value := singleFeatureBoardValue(t, "territory_Ontario_army_fraction", 10.0)

	got := candidateReinforcements(g, p0, pi, value, 1)
	if len(got) != 1 || got[0] != "Ontario" {
		t.Fatalf("expected tp=1 to cap to just the top-scoring bordering territory (Ontario), got %v", got)
	}
}

// TestGroupReinforcerPrefersEnemyBorderingOverHigherScoringInterior
// confirms the Tp filter actually changes the decision: the shared model
// rewards Alberta (the one interior territory in borderClusterGame) far
// more than any bordering territory, but GroupReinforcer must never
// choose it, since candidateReinforcements excludes non-bordering
// territories whenever any bordering ones exist.
func TestGroupReinforcerPrefersEnemyBorderingOverHigherScoringInterior(t *testing.T) {
	g, p0 := borderClusterGame(t)

	bvs := NewBoardValueStrategy(singleFeatureBoardValue(t, "territory_Alberta_army_fraction", 10.0))
	bvs.ReinforceSearchDepth = 1

	cmd, _, err := bvs.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceReinforcement || cmd.Territory == "Alberta" {
		t.Fatalf("expected a bordering territory, never the higher-scoring interior Alberta, got %+v", cmd)
	}
}

// TestGroupReinforcerDeeperSearchAnticipatesFurtherGroups confirms real
// multi-step chaining: with PendingReinforcements=6 and the default
// Gp=3, two groups are needed to exhaust the pool. A reward purely on
// Alaska's own army fraction means the correct full plan concentrates
// both groups there (Alaska: 1 -> 4 -> 7), so Depth=2's returned score
// (evaluated after both groups) must exceed Depth=1's (evaluated after
// only the first group) on the identical starting state -- proving
// Depth=2 actually looked past the first group, not just reported the
// same 1-ply value twice.
func TestGroupReinforcerDeeperSearchAnticipatesFurtherGroups(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 6
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 0, Armies: 1}
	pi := playerIndex(g, p0)
	value := singleFeatureBoardValue(t, "territory_Alaska_army_fraction", 10.0)

	depth1 := &GroupReinforcer{Depth: 1}
	_, _, score1, ok1 := depth1.Search(g, p0, pi, value)
	if !ok1 {
		t.Fatal("depth=1 Search: expected ok=true")
	}

	depth2 := &GroupReinforcer{Depth: 2}
	_, _, score2, ok2 := depth2.Search(g, p0, pi, value)
	if !ok2 {
		t.Fatal("depth=2 Search: expected ok=true")
	}

	if score2 <= score1 {
		t.Fatalf("expected depth=2's score (%v, after both groups) to exceed depth=1's (%v, after only the first group)", score2, score1)
	}
}

func TestGroupReinforcerNoLegalReinforcementReturnsNotOk(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	// p0 owns nothing (newTestGame's default owner=1 everywhere), so
	// risk.LegalReinforcements is empty.
	pi := playerIndex(g, p0)
	value := singleFeatureBoardValue(t, "my_army_fraction", 1.0)

	gr := &GroupReinforcer{Depth: 2}
	_, _, _, ok := gr.Search(g, p0, pi, value)
	if ok {
		t.Fatal("expected ok=false when there is no legal reinforcement at all")
	}
}
