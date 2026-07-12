package risk

import (
	"encoding/json"
	"testing"
)

// snapshot returns a JSON copy of the game for later mutation-detection
// comparisons.
func snapshot(t *testing.T, g *Game) []byte {
	t.Helper()
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	return raw
}

func assertUnchanged(t *testing.T, g *Game, before []byte) {
	t.Helper()
	after := snapshot(t, g)
	if string(before) != string(after) {
		t.Fatalf("expected game state unchanged by legal-action query\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestLegalSetupReinforcements(t *testing.T) {
	g := mustGame(t)
	claimAllRoundRobin(t, g)
	if g.Phase != PhaseSetupReinforce {
		t.Fatalf("expected setup reinforce, got %s", g.Phase)
	}
	p0 := g.Players[0].ID

	before := snapshot(t, g)
	actions := LegalSetupReinforcements(g, p0)
	assertUnchanged(t, g, before)

	if len(actions) == 0 {
		t.Fatalf("expected at least one legal setup reinforcement action")
	}
	for _, a := range actions {
		ts := g.Territories[a.Territory]
		if ts.Owner != 0 {
			t.Fatalf("action territory %s not owned by player 0 (owner=%d)", a.Territory, ts.Owner)
		}
	}

	// A player with no reserves left has no legal actions.
	g.SetupReserves[0] = 0
	if got := LegalSetupReinforcements(g, p0); len(got) != 0 {
		t.Fatalf("expected no legal actions once reserves are exhausted, got %d", len(got))
	}
}

func TestLegalCardTurnIns(t *testing.T) {
	g := mustGame(t)
	p0 := g.Players[0].ID
	g.Players[0].Cards = []Card{
		{Territory: "Alaska", Symbol: Infantry},
		{Territory: "Peru", Symbol: Cavalry},
		{Territory: "Egypt", Symbol: Artillery},
		{Territory: "Ural", Symbol: Infantry},
	}

	before := snapshot(t, g)
	sets := LegalCardTurnIns(g, p0)
	assertUnchanged(t, g, before)

	if len(sets) == 0 {
		t.Fatalf("expected at least one legal set (one of each symbol is valid)")
	}
	for _, s := range sets {
		cards := []Card{g.Players[0].Cards[s.Indices[0]], g.Players[0].Cards[s.Indices[1]], g.Players[0].Cards[s.Indices[2]]}
		if !isValidSet(cards) {
			t.Fatalf("LegalCardTurnIns returned an invalid set: %+v", s)
		}
	}

	// No cards at all -> no legal sets.
	g.Players[0].Cards = nil
	if got := LegalCardTurnIns(g, p0); len(got) != 0 {
		t.Fatalf("expected no legal sets with empty hand, got %d", len(got))
	}
}

func TestCardTurnInRequired(t *testing.T) {
	g := mustGame(t)
	p0 := g.Players[0].ID
	if CardTurnInRequired(g, p0) {
		t.Fatalf("expected turn-in not required with no cards")
	}
	g.Players[0].Cards = make([]Card, 5)
	if !CardTurnInRequired(g, p0) {
		t.Fatalf("expected turn-in required with 5 cards")
	}
}

func TestLegalReinforcements(t *testing.T) {
	g := mustGame(t)
	for _, terr := range g.Board.Order {
		g.Territories[terr] = TerritoryState{Owner: 1, Armies: 1}
	}
	g.Territories["Alaska"] = TerritoryState{Owner: 0, Armies: 3}
	g.Territories["Peru"] = TerritoryState{Owner: 0, Armies: 2}
	g.Phase = PhaseReinforce
	g.PendingReinforcements = 3
	p0 := g.Players[0].ID

	before := snapshot(t, g)
	actions := LegalReinforcements(g, p0)
	assertUnchanged(t, g, before)

	if len(actions) != 2 {
		t.Fatalf("expected 2 legal reinforcement territories, got %d", len(actions))
	}
	for _, a := range actions {
		if g.Territories[a.Territory].Owner != 0 {
			t.Fatalf("action territory %s not owned by player 0", a.Territory)
		}
	}

	// No pending reinforcements -> no legal actions.
	g.PendingReinforcements = 0
	if got := LegalReinforcements(g, p0); len(got) != 0 {
		t.Fatalf("expected no legal actions with zero pending reinforcements, got %d", len(got))
	}

	// Forced card trade (5+ cards) blocks reinforcement placement.
	g.PendingReinforcements = 3
	g.Players[0].Cards = make([]Card, 5)
	if got := LegalReinforcements(g, p0); len(got) != 0 {
		t.Fatalf("expected no legal actions with 5+ cards pending mandatory trade, got %d", len(got))
	}
}

func TestLegalAttacks(t *testing.T) {
	g := mustGame(t)
	for _, terr := range g.Board.Order {
		g.Territories[terr] = TerritoryState{Owner: 1, Armies: 1}
	}
	g.Territories["Alaska"] = TerritoryState{Owner: 0, Armies: 5} // borders Kamchatka(enemy), Northwest Territory(enemy), Alberta(enemy)
	g.Territories["Peru"] = TerritoryState{Owner: 0, Armies: 1}   // only 1 army: cannot attack
	g.Phase = PhaseAttack
	p0 := g.Players[0].ID

	before := snapshot(t, g)
	actions := LegalAttacks(g, p0)
	assertUnchanged(t, g, before)

	if len(actions) == 0 {
		t.Fatalf("expected legal attacks from Alaska")
	}
	for _, a := range actions {
		if a.From == "Peru" {
			t.Fatalf("Peru has only 1 army and must not appear as an attack source")
		}
		src := g.Territories[a.From]
		if src.Owner != 0 {
			t.Fatalf("attack source %s not owned by player 0", a.From)
		}
		dst := g.Territories[a.To]
		if dst.Owner == 0 || dst.Owner < 0 {
			t.Fatalf("attack target %s has invalid owner %d", a.To, dst.Owner)
		}
		if !g.Board.IsAdjacent(a.From, a.To) {
			t.Fatalf("attack target %s is not adjacent to source %s", a.To, a.From)
		}
		if a.MaxAttackerDice != min(3, src.Armies-1) {
			t.Fatalf("unexpected MaxAttackerDice for %s: got %d", a.From, a.MaxAttackerDice)
		}
	}

	// Not the attack phase -> no legal attacks.
	g.Phase = PhaseFortify
	if got := LegalAttacks(g, p0); len(got) != 0 {
		t.Fatalf("expected no legal attacks outside attack phase, got %d", len(got))
	}
}

func TestLegalOccupations(t *testing.T) {
	g := mustGame(t)
	p0 := g.Players[0].ID
	g.Territories["Alaska"] = TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Kamchatka"] = TerritoryState{Owner: 0, Armies: 2}
	g.Phase = PhaseOccupy
	g.Occupy = &OccupyState{From: "Alaska", To: "Kamchatka", MinMove: 2, MaxMove: 4}

	before := snapshot(t, g)
	actions := LegalOccupations(g, p0)
	assertUnchanged(t, g, before)

	if len(actions) != 3 {
		t.Fatalf("expected 3 legal occupation amounts (2,3,4), got %d", len(actions))
	}
	for i, a := range actions {
		if a.Armies != g.Occupy.MinMove+i {
			t.Fatalf("expected ascending occupation amounts, got %d at index %d", a.Armies, i)
		}
	}

	// Wrong phase -> nothing.
	g.Phase = PhaseAttack
	if got := LegalOccupations(g, p0); len(got) != 0 {
		t.Fatalf("expected no legal occupations outside occupy phase, got %d", len(got))
	}
}

func TestLegalFortifications(t *testing.T) {
	g := mustGame(t)
	for _, terr := range g.Board.Order {
		g.Territories[terr] = TerritoryState{Owner: 1, Armies: 1}
	}
	// Alaska - Northwest Territory - Alberta all owned by player 0 and
	// mutually connected, so Alaska can fortify into either neighbor.
	g.Territories["Alaska"] = TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Northwest Territory"] = TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Alberta"] = TerritoryState{Owner: 0, Armies: 1}
	g.Phase = PhaseFortify
	p0 := g.Players[0].ID

	before := snapshot(t, g)
	actions := LegalFortifications(g, p0)
	assertUnchanged(t, g, before)

	if len(actions) == 0 {
		t.Fatalf("expected legal fortifications from Alaska")
	}
	for _, a := range actions {
		if g.Territories[a.From].Owner != 0 || g.Territories[a.To].Owner != 0 {
			t.Fatalf("fortification %+v involves a non-owned territory", a)
		}
		if !g.isContiguous(a.From, a.To, 0) {
			t.Fatalf("fortification %+v is not contiguous through owned territory", a)
		}
		if a.MaxArmies != g.Territories[a.From].Armies-1 {
			t.Fatalf("unexpected MaxArmies for %+v", a)
		}
	}

	// A territory with only 1 army cannot be a fortification source.
	for _, a := range actions {
		if a.From == "Northwest Territory" || a.From == "Alberta" {
			t.Fatalf("territory with 1 army must not appear as fortification source: %+v", a)
		}
	}

	// Already fortified this turn -> nothing.
	g.HasFortified = true
	if got := LegalFortifications(g, p0); len(got) != 0 {
		t.Fatalf("expected no legal fortifications after HasFortified, got %d", len(got))
	}
}
