package tdstate

import (
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

type fixedRNG struct{}

func (fixedRNG) IntN(int) int { return 0 }

// newTestGame builds a 3-player classic game with a fixed shuffle, then
// blanks out territory ownership so each test can set up its own
// scenario -- mirrors internal/bot's identical test helper (can't import
// it directly: it's unexported in a different package).
func newTestGame(t *testing.T) *risk.Game {
	t.Helper()
	g, err := risk.NewClassicGame([]string{"p1", "p2", "p3"}, fixedRNG{})
	if err != nil {
		t.Fatalf("new game: %v", err)
	}
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 1, Armies: 1}
	}
	return g
}

func TestFlattenLengthMatchesFeatureNames(t *testing.T) {
	g := newTestGame(t)
	features := Encode(g, 0)
	flat := features.Flatten()
	names := FeatureNames(g.Board)
	if len(flat) != len(names) {
		t.Fatalf("Flatten() produced %d values, FeatureNames() produced %d names", len(flat), len(names))
	}
}

func TestEncodeIsMinePerspective(t *testing.T) {
	g := newTestGame(t)
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 5}

	fromP0 := Encode(g, 0)
	fromP1 := Encode(g, 1)

	alaskaIdx := indexOf(t, g.Board.Order, "Alaska")
	if !fromP0.Territories[alaskaIdx].IsMine {
		t.Error("expected Alaska to be IsMine from player 0's perspective")
	}
	if fromP1.Territories[alaskaIdx].IsMine {
		t.Error("expected Alaska to NOT be IsMine from player 1's perspective (owned by player 0)")
	}
}

func TestEncodeArmyFraction(t *testing.T) {
	g := newTestGame(t)
	// 42 territories, default armies=1 each except Alaska=5 -- total = 41 + 5 = 46.
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 5}

	f := Encode(g, 0)
	alaskaIdx := indexOf(t, g.Board.Order, "Alaska")
	want := 5.0 / 46.0
	if diff := f.Territories[alaskaIdx].ArmyFraction - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("Alaska ArmyFraction = %v, want %v", f.Territories[alaskaIdx].ArmyFraction, want)
	}
}

func TestEncodeContinentOneHotAndBorder(t *testing.T) {
	g := newTestGame(t)
	f := Encode(g, 0)
	continents := sortedContinents(g.Board)

	alaskaIdx := indexOf(t, g.Board.Order, "Alaska")
	naIdx := indexOfContinent(t, continents, "north_america")

	onehot := f.Territories[alaskaIdx].Continent
	trueCount := 0
	for i, v := range onehot {
		if v {
			trueCount++
			if i != naIdx {
				t.Errorf("expected Alaska's one-hot continent to be north_america (index %d), got index %d set", naIdx, i)
			}
		}
	}
	if trueCount != 1 {
		t.Errorf("expected exactly one continent flagged for Alaska, got %d", trueCount)
	}
	// Alaska borders Kamchatka (asia) -- a real inter-continent border.
	if !f.Territories[alaskaIdx].IsContinentBorder {
		t.Error("expected Alaska to be flagged as a continent border (borders Kamchatka in asia)")
	}
}

func TestEncodeGlobalFeatures(t *testing.T) {
	g := newTestGame(t)
	g.Phase = risk.PhaseSetupReinforce
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Iceland"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Players[0].Cards = []risk.Card{{Territory: "Alaska", Symbol: risk.Infantry}, {Territory: "Iceland", Symbol: risk.Infantry}}

	f := Encode(g, 0)

	// player 0 owns Alaska(5) + Iceland(3) = 8 armies; total = 40*1 + 5 + 3 = 48.
	wantArmyFraction := 8.0 / 48.0
	if diff := f.Global.MyArmyFraction - wantArmyFraction; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("MyArmyFraction = %v, want %v", f.Global.MyArmyFraction, wantArmyFraction)
	}

	wantTerritoryFraction := 2.0 / 42.0
	if diff := f.Global.MyTerritoryFraction - wantTerritoryFraction; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("MyTerritoryFraction = %v, want %v", f.Global.MyTerritoryFraction, wantTerritoryFraction)
	}

	wantCardFraction := 2.0 / cardFractionCap
	if diff := f.Global.CardFraction - wantCardFraction; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("CardFraction = %v, want %v", f.Global.CardFraction, wantCardFraction)
	}

	if !f.Global.IsMyTurn {
		t.Error("expected IsMyTurn true for player 0 (CurrentPlayer defaults to 0 after NewClassicGame)")
	}

	phaseIdx := indexOfPhase(t, risk.PhaseSetupReinforce)
	if !f.Global.Phase[phaseIdx] {
		t.Error("expected the setup_reinforce phase bit set (NewClassicGame's starting phase)")
	}
}

func TestEncodeEnemyThreatFractionSumsAdjacentEnemyArmiesOnly(t *testing.T) {
	g := newTestGame(t)
	// Alaska's neighbors are Northwest Territory, Alberta, Kamchatka
	// (board.go). Mine (own, shouldn't count), an enemy (should count),
	// and unowned/vacated (Owner < 0, shouldn't count).
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 0, Armies: 9} // mine
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 1, Armies: 7}             // enemy
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: -1, Armies: 0}          // unowned

	f := Encode(g, 0)
	alaskaIdx := indexOf(t, g.Board.Order, "Alaska")

	totalArmies := 0
	for _, ts := range g.Territories {
		totalArmies += ts.Armies
	}
	want := 7.0 / float64(totalArmies)
	got := f.Territories[alaskaIdx].EnemyThreatFraction
	if diff := got - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("Alaska EnemyThreatFraction = %v, want %v (only Alberta's 7 armies should count)", got, want)
	}
}

func TestEncodeDefenceZeroWhenNoContinentOwned(t *testing.T) {
	g := newTestGame(t)
	// newTestGame's default owner (1) everywhere means p0 owns nothing.
	f := Encode(g, 0)
	if f.Global.Defence != 0 {
		t.Errorf("Defence = %v, want 0 (p0 owns no continent)", f.Global.Defence)
	}
}

func TestEncodeDefenceUsesWeakestFrontierTerritory(t *testing.T) {
	g := newTestGame(t)
	// Australia's only external link is Indonesia -> Siam (board.go); the
	// other 3 Australia territories only border each other. Owning all of
	// Australia while Siam stays enemy-owned makes Indonesia the sole
	// frontier territory for that continent.
	g.Territories["Indonesia"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Territories["New Guinea"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Western Australia"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Eastern Australia"] = risk.TerritoryState{Owner: 0, Armies: 5}
	// Siam left at newTestGame's default: Owner 1, Armies 1.

	f := Encode(g, 0)

	totalArmies := 0
	for _, ts := range g.Territories {
		totalArmies += ts.Armies
	}
	want := 3.0 / float64(totalArmies)
	if diff := f.Global.Defence - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("Defence = %v, want %v (Indonesia's 3 armies is the sole frontier)", f.Global.Defence, want)
	}
}

func TestEncodeDefenceZeroWhenContinentHasNoFrontier(t *testing.T) {
	g := newTestGame(t)
	// Same Australia setup, but Siam is ALSO mine now -- no external
	// enemy border anywhere in the continent, so it contributes nothing
	// despite being fully owned.
	g.Territories["Indonesia"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Territories["New Guinea"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Western Australia"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Eastern Australia"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Siam"] = risk.TerritoryState{Owner: 0, Armies: 2}

	f := Encode(g, 0)
	if f.Global.Defence != 0 {
		t.Errorf("Defence = %v, want 0 (Australia has no frontier once Siam is also mine)", f.Global.Defence)
	}
}

func TestEncodeDefenceCapsAtDefenceCap(t *testing.T) {
	g := newTestGame(t)
	// Shrink the rest of the board so Indonesia's own armies dominate the
	// total, pushing the raw ratio above defenceCap (0.2).
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 1, Armies: 1}
	}
	g.Territories["Indonesia"] = risk.TerritoryState{Owner: 0, Armies: 20}
	g.Territories["New Guinea"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Western Australia"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Eastern Australia"] = risk.TerritoryState{Owner: 0, Armies: 5}

	f := Encode(g, 0)
	if f.Global.Defence != defenceCap {
		t.Errorf("Defence = %v, want the cap %v", f.Global.Defence, defenceCap)
	}
}

func TestEncodeStrongestEnemyIgnoresEliminatedAndSelf(t *testing.T) {
	g := newTestGame(t)
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 1, Armies: 20} // strong
	g.Territories["Iceland"] = risk.TerritoryState{Owner: 2, Armies: 3} // weaker
	g.Players[1].Eliminated = true                                     // strongest is eliminated -- should be ignored

	f := Encode(g, 0)
	// total armies = 40*1 + 20 + 3 = 63. Strongest non-eliminated enemy is
	// player 2 with Iceland's 3 armies (player 1's 20 is ignored).
	total := 63.0
	want := 3.0 / total
	if diff := f.Global.StrongestEnemyArmyFraction - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("StrongestEnemyArmyFraction = %v, want %v (eliminated player 1 should be excluded)", f.Global.StrongestEnemyArmyFraction, want)
	}
}

func indexOf(t *testing.T, order []risk.Territory, target risk.Territory) int {
	t.Helper()
	for i, x := range order {
		if x == target {
			return i
		}
	}
	t.Fatalf("%s not found in board order", target)
	return -1
}

func indexOfContinent(t *testing.T, continents []risk.Continent, target risk.Continent) int {
	t.Helper()
	for i, x := range continents {
		if x == target {
			return i
		}
	}
	t.Fatalf("%s not found in sorted continents %v", target, continents)
	return -1
}

func indexOfPhase(t *testing.T, target risk.Phase) int {
	t.Helper()
	for i, p := range allPhases {
		if p == target {
			return i
		}
	}
	t.Fatalf("%s not found in allPhases", target)
	return -1
}
