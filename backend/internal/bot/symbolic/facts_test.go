package symbolic

import (
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// fixedRNG always returns 0, keeping player order equal to input order --
// mirrors internal/bot's own strategy_basic_test.go fixture; duplicated
// here since this package deliberately has no internal/bot dependency.
type fixedRNG struct{}

func (fixedRNG) IntN(int) int { return 0 }

// newTestGame builds a real 3-player classic game with every territory
// defaulted to player 1 at 1 army, mirroring internal/bot's own
// newTestGame fixture (strategy_basic_test.go) so individual tests only
// need to set up what they care about.
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

// TestSelfNearCompletionFindsSelfNearCompletion sets South America (bonus
// 2: Venezuela/Peru/Brazil/Argentina) to pi owning 3 of 4, player 1
// owning the last -- Argentina is the sole missing territory of pi's own
// near-complete continent.
func TestSelfNearCompletionFindsSelfNearCompletion(t *testing.T) {
	g := newTestGame(t)
	pi := 0
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: pi, Armies: 5}
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 5}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 5}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 1, Armies: 5}

	facts := SelfNearCompletion(g, pi)
	if len(facts) != 1 {
		t.Fatalf("expected exactly one self-near-completion fact, got %+v", facts)
	}
	fact := facts[0]
	if fact.Continent != "south_america" || fact.MissingTerr != "Argentina" || fact.Bonus != 2 {
		t.Fatalf("unexpected fact: %+v", fact)
	}
}

// TestSelfNearCompletionOmitsWhenNoSingleOwnerHoldsAllButOne splits South
// America 2-2 between pi and player 1 -- no single owner holds 3 of 4, so
// nothing should be reported.
func TestSelfNearCompletionOmitsWhenNoSingleOwnerHoldsAllButOne(t *testing.T) {
	g := newTestGame(t)
	pi := 0
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: pi, Armies: 5}
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 5}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: 1, Armies: 5}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 1, Armies: 5}

	if facts := SelfNearCompletion(g, pi); len(facts) != 0 {
		t.Fatalf("expected no completion facts for a 2-2 split, got %+v", facts)
	}
}

// TestSelfNearCompletionOmitsRivalNearCompletion confirms this package's
// self-only scoping directly: a rival (player 1) owning 3 of South
// America's 4 territories, with pi owning the missing one, must NOT be
// reported -- unlike this package's reverted first version, there is no
// enemy-completion fact type at all.
func TestSelfNearCompletionOmitsRivalNearCompletion(t *testing.T) {
	g := newTestGame(t)
	pi := 0
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 1, Armies: 5}
	g.Territories["Peru"] = risk.TerritoryState{Owner: 1, Armies: 5}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: 1, Armies: 5}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 5}

	if facts := SelfNearCompletion(g, pi); len(facts) != 0 {
		t.Fatalf("expected no facts reported for a rival's near-complete continent, got %+v", facts)
	}
}
