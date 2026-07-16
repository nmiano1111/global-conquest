package simulation

import (
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

func fingerprintFixture(t *testing.T) *risk.Game {
	t.Helper()
	g, err := risk.NewClassicAutoStartGame([]string{"p0", "p1", "p2", "p3"}, NewDeterministicRNG(1))
	if err != nil {
		t.Fatalf("build fixture: %v", err)
	}
	return g
}

func TestFingerprintStableAcrossRepeatedCalls(t *testing.T) {
	g := fingerprintFixture(t)
	first := Fingerprint(g)
	for i := 0; i < 50; i++ {
		if got := Fingerprint(g); got != first {
			t.Fatalf("call %d: fingerprint changed on an unmodified game (%v vs %v) -- likely leaking Go's randomized map iteration order", i, got, first)
		}
	}
}

func TestFingerprintIdenticalForIdenticallyConstructedGames(t *testing.T) {
	g1, err := risk.NewClassicAutoStartGame([]string{"p0", "p1", "p2", "p3"}, NewDeterministicRNG(7))
	if err != nil {
		t.Fatalf("build g1: %v", err)
	}
	g2, err := risk.NewClassicAutoStartGame([]string{"p0", "p1", "p2", "p3"}, NewDeterministicRNG(7))
	if err != nil {
		t.Fatalf("build g2: %v", err)
	}
	if Fingerprint(g1) != Fingerprint(g2) {
		t.Fatalf("expected identical fingerprints for two games built from the same seed")
	}
}

func TestFingerprintChangesOnArmyCount(t *testing.T) {
	g := fingerprintFixture(t)
	before := Fingerprint(g)
	ts := g.Territories["Alaska"]
	ts.Armies++
	g.Territories["Alaska"] = ts
	if Fingerprint(g) == before {
		t.Fatalf("expected fingerprint to change after an army-count change")
	}
}

func TestFingerprintChangesOnOwnership(t *testing.T) {
	g := fingerprintFixture(t)
	before := Fingerprint(g)
	ts := g.Territories["Alaska"]
	ts.Owner = (ts.Owner + 1) % len(g.Players)
	g.Territories["Alaska"] = ts
	if Fingerprint(g) == before {
		t.Fatalf("expected fingerprint to change after an ownership change")
	}
}

func TestFingerprintChangesOnPhase(t *testing.T) {
	g := fingerprintFixture(t)
	before := Fingerprint(g)
	g.Phase = risk.PhaseFortify
	if Fingerprint(g) == before {
		t.Fatalf("expected fingerprint to change after a phase change")
	}
}

func TestFingerprintChangesOnCurrentPlayer(t *testing.T) {
	g := fingerprintFixture(t)
	before := Fingerprint(g)
	g.CurrentPlayer = (g.CurrentPlayer + 1) % len(g.Players)
	if Fingerprint(g) == before {
		t.Fatalf("expected fingerprint to change after CurrentPlayer changes")
	}
}

func TestFingerprintChangesOnPendingReinforcements(t *testing.T) {
	g := fingerprintFixture(t)
	before := Fingerprint(g)
	g.PendingReinforcements += 3
	if Fingerprint(g) == before {
		t.Fatalf("expected fingerprint to change after PendingReinforcements changes")
	}
}

func TestFingerprintChangesOnFlags(t *testing.T) {
	cases := []struct {
		name  string
		apply func(*risk.Game)
	}{
		{"HasFortified", func(g *risk.Game) { g.HasFortified = !g.HasFortified }},
		{"ForcedCardTrade", func(g *risk.Game) { g.ForcedCardTrade = !g.ForcedCardTrade }},
		{"ConqueredThisTurn", func(g *risk.Game) { g.ConqueredThisTurn = !g.ConqueredThisTurn }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := fingerprintFixture(t)
			before := Fingerprint(g)
			tc.apply(g)
			if Fingerprint(g) == before {
				t.Fatalf("expected fingerprint to change after toggling %s", tc.name)
			}
		})
	}
}

func TestFingerprintChangesOnCardCount(t *testing.T) {
	g := fingerprintFixture(t)
	before := Fingerprint(g)
	g.Players[0].Cards = append(g.Players[0].Cards, risk.Card{Territory: "Alaska", Symbol: risk.Infantry})
	if Fingerprint(g) == before {
		t.Fatalf("expected fingerprint to change after a player's card count changes")
	}
}

func TestFingerprintChangesOnOccupyState(t *testing.T) {
	g := fingerprintFixture(t)
	before := Fingerprint(g)
	g.Occupy = &risk.OccupyState{From: "Alaska", To: "Kamchatka", MinMove: 1, MaxMove: 3}
	if Fingerprint(g) == before {
		t.Fatalf("expected fingerprint to change when Occupy state is set")
	}

	afterSet := Fingerprint(g)
	g.Occupy.MaxMove = 4
	if Fingerprint(g) == afterSet {
		t.Fatalf("expected fingerprint to change when Occupy.MaxMove changes")
	}
}

// TestFingerprintIgnoresTurnNumber is the key test for this file: a
// genuine stuck loop spans multiple turns with the same board position
// recurring, so TurnNumber (which only ever increases) must not be part
// of the fingerprint, or a real loop would never be detected.
func TestFingerprintIgnoresTurnNumber(t *testing.T) {
	g := fingerprintFixture(t)
	before := Fingerprint(g)
	g.TurnNumber += 50
	if Fingerprint(g) != before {
		t.Fatalf("expected fingerprint to ignore TurnNumber, but it changed")
	}
}

// TestFingerprintIgnoresSetsTraded mirrors the TurnNumber case: SetsTraded
// also only ever increases, so it must not affect the fingerprint either.
func TestFingerprintIgnoresSetsTraded(t *testing.T) {
	g := fingerprintFixture(t)
	before := Fingerprint(g)
	g.SetsTraded += 3
	if Fingerprint(g) != before {
		t.Fatalf("expected fingerprint to ignore SetsTraded, but it changed")
	}
}
