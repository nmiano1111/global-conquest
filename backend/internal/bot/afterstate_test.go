package bot

import (
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
	"github.com/nmiano1111/global-conquest/backend/internal/tdstate"
)

func TestCopyGameStateTerritoriesIndependent(t *testing.T) {
	g, _ := newTestGame(t)
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}

	c := copyGameState(g)
	c.Territories["Alaska"] = risk.TerritoryState{Owner: 1, Armies: 99}

	if g.Territories["Alaska"].Owner != 0 || g.Territories["Alaska"].Armies != 3 {
		t.Fatalf("expected the original game's Alaska to be unaffected by mutating the copy, got %+v", g.Territories["Alaska"])
	}
}

func TestCopyGameStatePlayerCardsIndependent(t *testing.T) {
	g, _ := newTestGame(t)
	g.Players[0].Cards = []risk.Card{{Territory: "Alaska", Symbol: risk.Infantry}}

	c := copyGameState(g)
	c.Players[0].Cards = append(c.Players[0].Cards, risk.Card{Territory: "Iceland", Symbol: risk.Cavalry})

	if len(g.Players[0].Cards) != 1 {
		t.Fatalf("expected the original game's Cards slice to be unaffected by mutating the copy, got %d cards", len(g.Players[0].Cards))
	}
}

func TestCopyGameStateOccupyIndependent(t *testing.T) {
	g, _ := newTestGame(t)
	g.Occupy = &risk.OccupyState{From: "Alaska", To: "Kamchatka", MinMove: 1, MaxMove: 5}

	c := copyGameState(g)
	c.Occupy.MaxMove = 999

	if g.Occupy.MaxMove != 5 {
		t.Fatalf("expected the original game's Occupy to be unaffected by mutating the copy, got MaxMove=%d", g.Occupy.MaxMove)
	}
}

func TestCopyGameStateSetupReservesAndDeckIndependent(t *testing.T) {
	g, _ := newTestGame(t)
	g.SetupReserves[0] = 5
	g.Deck = []risk.Card{{Territory: "Alaska", Symbol: risk.Infantry}}
	g.Discard = []risk.Card{{Territory: "Iceland", Symbol: risk.Cavalry}}

	c := copyGameState(g)
	c.SetupReserves[0] = 999
	c.Deck = append(c.Deck, risk.Card{Territory: "Peru", Symbol: risk.Artillery})
	c.Discard = append(c.Discard, risk.Card{Territory: "Egypt", Symbol: risk.Wild})

	if g.SetupReserves[0] != 5 {
		t.Errorf("expected the original SetupReserves to be unaffected, got %d", g.SetupReserves[0])
	}
	if len(g.Deck) != 1 {
		t.Errorf("expected the original Deck to be unaffected, got %d cards", len(g.Deck))
	}
	if len(g.Discard) != 1 {
		t.Errorf("expected the original Discard to be unaffected, got %d cards", len(g.Discard))
	}
}

func TestReinforceAfterstateAppliesWithoutMutatingOriginal(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 2}

	after := reinforceAfterstate(g, p0, "Alaska", 3)

	if after.Territories["Alaska"].Armies != 5 {
		t.Fatalf("expected the afterstate's Alaska to have 5 armies, got %d", after.Territories["Alaska"].Armies)
	}
	if g.Territories["Alaska"].Armies != 2 {
		t.Fatalf("expected the original game's Alaska to be unaffected, got %d armies", g.Territories["Alaska"].Armies)
	}
}

func TestOccupyAfterstateAppliesWithoutMutatingOriginal(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseOccupy
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 10}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Occupy = &risk.OccupyState{From: "Alaska", To: "Kamchatka", MinMove: 1, MaxMove: 9}

	after := occupyAfterstate(g, p0, 4)

	if after.Territories["Kamchatka"].Armies != 5 {
		t.Fatalf("expected the afterstate's Kamchatka to have 5 armies (1+4), got %d", after.Territories["Kamchatka"].Armies)
	}
	if after.Territories["Alaska"].Armies != 6 {
		t.Fatalf("expected the afterstate's Alaska to have 6 armies (10-4), got %d", after.Territories["Alaska"].Armies)
	}
	if g.Territories["Alaska"].Armies != 10 || g.Territories["Kamchatka"].Armies != 1 {
		t.Fatalf("expected the original game to be unaffected, got Alaska=%d Kamchatka=%d", g.Territories["Alaska"].Armies, g.Territories["Kamchatka"].Armies)
	}
}

func TestFortifyAfterstateAppliesWithoutMutatingOriginal(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	g.Territories["Madagascar"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["South Africa"] = risk.TerritoryState{Owner: 0, Armies: 1}

	after := fortifyAfterstate(g, p0, "Madagascar", "South Africa", 3)

	if after.Territories["Madagascar"].Armies != 2 || after.Territories["South Africa"].Armies != 4 {
		t.Fatalf("expected the afterstate to move 3 armies from Madagascar to South Africa, got Madagascar=%d SouthAfrica=%d",
			after.Territories["Madagascar"].Armies, after.Territories["South Africa"].Armies)
	}
	if g.Territories["Madagascar"].Armies != 5 || g.Territories["South Africa"].Armies != 1 {
		t.Fatalf("expected the original game to be unaffected, got Madagascar=%d SouthAfrica=%d",
			g.Territories["Madagascar"].Armies, g.Territories["South Africa"].Armies)
	}
}

func TestAttackAfterstateBlendWeightsByWinProbability(t *testing.T) {
	g, _ := newTestGame(t)
	// A hugely favorable attack: win probability should be very close to 1,
	// so the blend should land very close to the "conquered" encoding.
	a := risk.AttackAction{From: "Alaska", To: "Kamchatka", SourceArmies: 30, TargetArmies: 1, MaxAttackerDice: 3}
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 30}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1}

	forecast := ForecastAttack(a.SourceArmies, a.TargetArmies)
	if forecast.WinProbability < 0.95 {
		t.Fatalf("expected this matchup to be hugely favorable, got WinProbability=%v", forecast.WinProbability)
	}

	blended := attackAfterstateBlend(g, 0, a)

	conquered := copyGameState(g)
	conquered.Territories[a.From] = risk.TerritoryState{Owner: 0, Armies: max(1, 30-round(forecast.ExpectedAttackerLosses)-a.MaxAttackerDice)}
	conquered.Territories[a.To] = risk.TerritoryState{Owner: 0, Armies: a.MaxAttackerDice}
	wantClose := tdstate.Encode(conquered, 0).Flatten()

	if len(blended) != len(wantClose) {
		t.Fatalf("expected blended feature vector length %d, got %d", len(wantClose), len(blended))
	}
	var maxDiff float64
	for i := range blended {
		diff := blended[i] - wantClose[i]
		if diff < 0 {
			diff = -diff
		}
		if diff > maxDiff {
			maxDiff = diff
		}
	}
	if maxDiff > 0.1 {
		t.Errorf("expected the blend to land close to the 'conquered' encoding for a near-certain win, max diff = %v", maxDiff)
	}
}
