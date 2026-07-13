package bot

import (
	"context"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

func TestAdjacentEnemyTerritoryCount(t *testing.T) {
	g, _ := newTestGame(t)
	// Alaska's neighbors are Northwest Territory, Alberta, Kamchatka --
	// own Alberta, leaving 2 enemy neighbors.
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 0, Armies: 1}
	if got := adjacentEnemyTerritoryCount(g, "Alaska", 0); got != 2 {
		t.Fatalf("expected 2 enemy neighbors (Northwest Territory, Kamchatka), got %d", got)
	}
}

func TestContinentReinforceValueDefendsBorderOfOwnedContinent(t *testing.T) {
	g, _ := newTestGame(t)
	for _, terr := range []risk.Territory{"Indonesia", "New Guinea", "Western Australia", "Eastern Australia"} {
		g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 2}
	}
	// Australia (bonus 2) fully owned: Indonesia is its only border (Siam
	// sits outside the continent); the other three are fully interior.
	if v := continentReinforceValue(g, 0, "Indonesia"); v != 2.0 {
		t.Fatalf("expected Indonesia to get the full continent bonus (2.0), got %v", v)
	}
	if v := continentReinforceValue(g, 0, "New Guinea"); v != 0 {
		t.Fatalf("expected interior New Guinea to get 0, got %v", v)
	}
}

func TestContinentReinforceValuePushesTowardFrontier(t *testing.T) {
	g, _ := newTestGame(t)
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Peru"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: 0, Armies: 1}
	// Argentina still enemy: Peru/Brazil border it (frontiers of a
	// continent 1 territory from completion); Venezuela doesn't.
	if v := continentReinforceValue(g, 0, "Peru"); v != 1.0 {
		t.Fatalf("expected Peru's push value 1.0 (bonus 2 / (missing 1 + 1)), got %v", v)
	}
	if v := continentReinforceValue(g, 0, "Venezuela"); v != 0 {
		t.Fatalf("expected non-frontier Venezuela to get 0, got %v", v)
	}
}

func TestScoredStrategyReinforceCardTurnInMandatory(t *testing.T) {
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

	strat := NewScoredStrategy(DefaultWeights)
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionTradeCards {
		t.Fatalf("expected trade_cards with 5 cards in hand, got %s", cmd.Action)
	}
}

// TestScoredStrategyReinforceBatchesRatherThanDumping: with 9 pending
// reinforcements and a single legal candidate, this call should place
// max(1, 9/3) = 3, not all 9.
func TestScoredStrategyReinforceBatchesRatherThanDumping(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 9
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}

	strat := NewScoredStrategy(DefaultWeights)
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceReinforcement || cmd.Territory != "Alaska" {
		t.Fatalf("expected place_reinforcement at Alaska, got %+v", cmd)
	}
	if cmd.Armies != 3 {
		t.Fatalf("expected a batch of 3 (9/3), not the full 9, got %d", cmd.Armies)
	}
}

// TestScoredStrategyReinforceBatchFloorIsOne: with 2 pending, max(1, 2/3)
// must floor to 1, not 0.
func TestScoredStrategyReinforceBatchFloorIsOne(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 2
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}

	strat := NewScoredStrategy(DefaultWeights)
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Armies != 1 {
		t.Fatalf("expected the batch floor of 1 when only 2 are pending, got %d", cmd.Armies)
	}
}

// TestScoredStrategyReinforcePrefersHeavilyThreatenedWeakBorder reuses the
// same fixture as basic-v1's equivalent test: Alaska is both more
// threatened and more under-defended than Argentina.
func TestScoredStrategyReinforcePrefersHeavilyThreatenedWeakBorder(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 4
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 5}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 4}

	strat := NewScoredStrategy(DefaultWeights)
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceReinforcement || cmd.Territory != "Alaska" {
		t.Fatalf("expected place_reinforcement at Alaska (far more threatened and weaker), got %+v", cmd)
	}
}

// TestScoredStrategyReinforcePrefersContinentDefenseBorder mirrors the
// attack-phase continent test: Australia fully owned, Indonesia the only
// border.
func TestScoredStrategyReinforcePrefersContinentDefenseBorder(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	for _, terr := range []risk.Territory{"Indonesia", "New Guinea", "Western Australia", "Eastern Australia"} {
		g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 2}
	}

	strat := NewScoredStrategy(DefaultWeights)
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Territory != "Indonesia" {
		t.Fatalf("expected Indonesia (Australia's only border, defending a completed continent), got %s", cmd.Territory)
	}
}

// TestScoredStrategyReinforceSpreadsAcrossBordersOverTurn simulates
// successive calls, manually applying each committed placement the way the
// engine would, and checks that reinforcements eventually shift to a
// second, untouched border once the first stops being the clear top pick
// (ReinforceWeakness and ReinforceConcentrationPenalty both erode a
// territory's own score as its armies grow, while an untouched candidate's
// score stays fixed). Only Alaska and Iceland are owned -- deliberately
// not owning any of their neighbors, since owning a "helper" neighbor to
// isolate one enemy count would itself become a competing legal
// reinforcement candidate.
//
// Hand-derived (both start at armies=1): Alaska's neighbors are Northwest
// Territory, Alberta (both enemy, armies 1 each) and Kamchatka (armies 6)
// -- threat 8, 3 enemy neighbors, a north_america frontier (missing 8 of 9)
// worth 5/9. Score_Alaska(a) = 2*8 + 4.5 + 2*(5/9) - 1.3a = 21.6111 - 1.3a.
// Iceland's neighbors are Greenland, Great Britain, Scandinavia (all enemy,
// armies 1 each; Greenland is north_america, not europe) -- threat 3, 3
// enemy neighbors, a europe frontier (missing 6 of 7) worth 5/7.
// Score_Iceland(b) = 2*3 + 4.5 + 2*(5/7) - 1.3b = 11.9286 - 1.3b. Alaska
// starts far ahead and wins 5 straight batches (3,2,1,1,1 -- exhausting all
// but the last of the 9 pending armies) before its own growing weakness/
// concentration cost finally drops it below Iceland's untouched score on
// the 6th and final call.
func TestScoredStrategyReinforceSpreadsAcrossBordersOverTurn(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 9

	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 6}
	g.Territories["Iceland"] = risk.TerritoryState{Owner: 0, Armies: 1}
	// Northwest Territory, Alberta, Greenland, Great Britain, Scandinavia
	// all stay at their default owner=1, armies=1 -- deliberately enemy,
	// not owned, so they never become competing candidates.

	strat := NewScoredStrategy(DefaultWeights)

	type step struct {
		wantTerritory string
		wantArmies    int
	}
	steps := []step{
		{"Alaska", 3},
		{"Alaska", 2},
		{"Alaska", 1},
		{"Alaska", 1},
		{"Alaska", 1},
		{"Iceland", 1},
	}
	for i, want := range steps {
		cmd, _, err := strat.NextCommand(context.Background(), g, p0)
		if err != nil {
			t.Fatalf("step %d: NextCommand: %v", i, err)
		}
		if cmd.Territory != want.wantTerritory || cmd.Armies != want.wantArmies {
			t.Fatalf("step %d: expected %s +%d, got %s +%d", i, want.wantTerritory, want.wantArmies, cmd.Territory, cmd.Armies)
		}
		ts := g.Territories[risk.Territory(cmd.Territory)]
		ts.Armies += cmd.Armies
		g.Territories[risk.Territory(cmd.Territory)] = ts
		g.PendingReinforcements -= cmd.Armies
	}
	if g.PendingReinforcements != 0 {
		t.Fatalf("expected all 9 pending reinforcements consumed, %d remain", g.PendingReinforcements)
	}
}

func TestScoredStrategySetupReinforceReturnsLegalPlacement(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseSetupReinforce
	g.SetupReserves[0] = 5
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 5}

	strat := NewScoredStrategy(DefaultWeights)
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
