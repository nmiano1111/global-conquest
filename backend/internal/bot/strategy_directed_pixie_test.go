package bot

import (
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// TestDirectedPixieEmptyDirectiveMatchesPlainBetterPixie confirms
// DirectedPixieStrategy given an empty Directive produces byte-identical
// commands to plain BetterPixieStrategy across all 4 phases -- the
// critical no-regression guarantee, mirroring the "no-op when unset" bar
// every prior pass's hook met (WithAttackBias, WithDecisionObserver(V2)).
func TestDirectedPixieEmptyDirectiveMatchesPlainBetterPixie(t *testing.T) {
	bp := NewBetterPixieStrategy()
	dp := NewDirectedPixieStrategy()

	t.Run("reinforce", func(t *testing.T) {
		g, p0 := newTestGame(t)
		g.Phase = risk.PhaseReinforce
		g.PendingReinforcements = 4
		pi := 0
		g.Territories["Alaska"] = risk.TerritoryState{Owner: pi, Armies: 3}
		g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 2}

		want, err := bp.reinforce(g, p0)
		if err != nil {
			t.Fatalf("BetterPixieStrategy.reinforce: %v", err)
		}
		got, err := dp.reinforce(g, p0, Directive{})
		if err != nil {
			t.Fatalf("DirectedPixieStrategy.reinforce: %v", err)
		}
		if got != want {
			t.Fatalf("expected identical commands, got dp=%+v bp=%+v", got, want)
		}
	})

	t.Run("attack", func(t *testing.T) {
		g, p0 := newTestGame(t)
		g.Phase = risk.PhaseAttack
		pi := 0
		g.Territories["Alaska"] = risk.TerritoryState{Owner: pi, Armies: 5}
		g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1}

		want, err := bp.attack(g, p0)
		if err != nil {
			t.Fatalf("BetterPixieStrategy.attack: %v", err)
		}
		got, err := dp.attack(g, p0, Directive{})
		if err != nil {
			t.Fatalf("DirectedPixieStrategy.attack: %v", err)
		}
		if got != want {
			t.Fatalf("expected identical commands, got dp=%+v bp=%+v", got, want)
		}
	})

	t.Run("occupy", func(t *testing.T) {
		g, p0 := newTestGame(t)
		g.Phase = risk.PhaseOccupy
		pi := 0
		g.Territories["Alaska"] = risk.TerritoryState{Owner: pi, Armies: 8}
		g.Territories["Kamchatka"] = risk.TerritoryState{Owner: pi, Armies: 0}
		g.Occupy = &risk.OccupyState{From: "Alaska", To: "Kamchatka", MinMove: 2, MaxMove: 7}

		want, err := bp.occupy(g, p0)
		if err != nil {
			t.Fatalf("BetterPixieStrategy.occupy: %v", err)
		}
		got, err := dp.occupy(g, p0, Directive{})
		if err != nil {
			t.Fatalf("DirectedPixieStrategy.occupy: %v", err)
		}
		if got != want {
			t.Fatalf("expected identical commands, got dp=%+v bp=%+v", got, want)
		}
	})

	t.Run("fortify", func(t *testing.T) {
		g, p0 := newTestGame(t)
		g.Phase = risk.PhaseFortify
		pi := 0
		g.Territories["Alaska"] = risk.TerritoryState{Owner: pi, Armies: 6}
		g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: pi, Armies: 1}
		g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1}

		want, err := bp.fortify(g, p0)
		if err != nil {
			t.Fatalf("BetterPixieStrategy.fortify: %v", err)
		}
		got, err := dp.fortify(g, p0, Directive{})
		if err != nil {
			t.Fatalf("DirectedPixieStrategy.fortify: %v", err)
		}
		if got != want {
			t.Fatalf("expected identical commands, got dp=%+v bp=%+v", got, want)
		}
	})
}

// TestDirectedPixiePlacementRespondsToWantedRegardlessOfSource confirms
// directedPixiePlacement steers toward whatever continent it's TOLD is
// wanted -- tested directly against the reusable core function (rather
// than through a full reinforce() call, which would also have to fight
// betterPixieWantedContinents' own, hard-to-predict "which continent do
// I want" math) with a hand-built wanted map, proving the mechanism
// itself works regardless of where wanted came from.
func TestDirectedPixiePlacementRespondsToWantedRegardlessOfSource(t *testing.T) {
	g, _ := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	pi := 0
	// South America: pi owns Peru/Brazil/Argentina (3/4), Venezuela enemy
	// -- betterPixieContinentNeedsHelp(south_america) is true (not fully
	// owned), so directedPixiePlacement's first branch fires as soon as
	// south_america is in wanted, regardless of what put it there.
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 3}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 1, Armies: 4}
	actions := risk.LegalReinforcements(g, "p1")

	t.Run("not wanted places the full pending pool, not a 1-army border top-up", func(t *testing.T) {
		_, armies := directedPixiePlacement(g, pi, actions, 5, map[risk.Continent]bool{}, nil)
		if armies == 1 {
			t.Fatalf("test setup invalid: expected the empty-wanted fallback to NOT independently land on the same 1-army border-help placement the next case tests, so that case is a real, distinguishing test")
		}
	})

	t.Run("wanted steers to the weak border", func(t *testing.T) {
		territory, armies := directedPixiePlacement(g, pi, actions, 5, map[risk.Continent]bool{"south_america": true}, nil)
		if territory != "Peru" || armies != 1 {
			t.Fatalf("expected wanted=south_america to steer 1 army to Peru (its weak border), got territory=%s armies=%d", territory, armies)
		}
	})
}

// TestDirectedPixiePlacementRespectsArmyCap confirms the whole reason
// Directive.ArmyCap exists: betterPixieContinentNeedsHelp's own generic
// threshold (pixieBorderForce = 20) never naturally stops "needing help"
// for a continent that isn't fully owned. Distinguishes on Armies==1
// (the "needs help" tier's own fixed signature -- placeToTakeSpecificContinent
// always places exactly 1 army), not on which territory gets picked:
// with the rest of the board forming one large non-owned component,
// placeNearWeakestEnemyCluster (a LATER fallback tier, never
// cap-aware, and correctly so -- it isn't goal-related) can
// independently pick the same territory once the needs-help tier is
// skipped, since Peru is also the strongest pi-owned territory bordering
// that component. That's a real, separate mechanism, not evidence the
// cap failed -- the cap's only job is to stop the "needs help" tier
// specifically, which the Armies==1 signature isolates cleanly.
func TestDirectedPixiePlacementRespectsArmyCap(t *testing.T) {
	g, _ := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	pi := 0
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 8}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 1, Armies: 8}
	actions := risk.LegalReinforcements(g, "p1")

	t.Run("uncapped fires the needs-help tier (1 army)", func(t *testing.T) {
		territory, armies := directedPixiePlacement(g, pi, actions, 5, map[risk.Continent]bool{"south_america": true}, nil)
		if territory != "Peru" || armies != 1 {
			t.Fatalf("test setup invalid: expected the uncapped case to place 1 army at Peru via the needs-help tier (proving the next case is a real, distinguishing test), got territory=%s armies=%d", territory, armies)
		}
	})

	t.Run("capped skips the needs-help tier once the requirement is met", func(t *testing.T) {
		_, armies := directedPixiePlacement(g, pi, actions, 5, map[risk.Continent]bool{"south_america": true}, map[risk.Continent]int{"south_america": 8})
		if armies == 1 {
			t.Fatalf("expected the army cap to stop the needs-help tier from firing once Peru's armies (8) meet the cap (8), got armies=%d (the needs-help tier's own signature)", armies)
		}
	})
}

// TestDirectedPixieFortifyPrefersFocusTerritory confirms a
// FocusTerritory directive steers fortify toward a border adjacent to
// it, even when plain BetterPixieStrategy's own fortify logic would pick
// something else.
func TestDirectedPixieFortifyPrefersFocusTerritory(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	pi := 0
	g.Territories["Peru"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 6}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: pi, Armies: 4}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 1, Armies: 4}

	dp := NewDirectedPixieStrategy()
	cmd, err := dp.fortify(g, p0, Directive{FocusTerritory: "Venezuela"})
	if err != nil {
		t.Fatalf("DirectedPixieStrategy.fortify: %v", err)
	}
	if cmd.Action != ActionFortify || cmd.To != "Brazil" {
		t.Fatalf("expected fortify to steer armies toward Brazil (adjacent to the focus territory, Venezuela), got %+v", cmd)
	}
}

// TestUnionWantedDoesNotMutateArguments confirms unionWanted returns a
// fresh map, never mutating either input.
func TestUnionWantedDoesNotMutateArguments(t *testing.T) {
	a := map[risk.Continent]bool{"north_america": true}
	b := map[risk.Continent]bool{"south_america": true}
	out := unionWanted(a, b)

	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("expected inputs untouched, got a=%+v b=%+v", a, b)
	}
	if !out["north_america"] || !out["south_america"] || len(out) != 2 {
		t.Fatalf("expected the union of both inputs, got %+v", out)
	}
}
