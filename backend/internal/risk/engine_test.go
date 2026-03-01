package risk

import (
	"encoding/json"
	"testing"
)

type seqRNG struct {
	v []int
	i int
}

func (s *seqRNG) IntN(n int) int {
	if len(s.v) == 0 {
		return 0
	}
	x := s.v[s.i%len(s.v)]
	s.i++
	if n <= 0 {
		return 0
	}
	if x < 0 {
		x = -x
	}
	return x % n
}

func mustGame(t *testing.T) *Game {
	t.Helper()
	g, err := NewClassicGame([]string{"p1", "p2", "p3"}, &seqRNG{v: []int{0}})
	if err != nil {
		t.Fatalf("new game: %v", err)
	}
	return g
}

func claimAllRoundRobin(t *testing.T, g *Game) {
	t.Helper()
	for i, terr := range g.Board.Order {
		p := g.Players[i%len(g.Players)].ID
		if err := g.ClaimTerritory(p, terr); err != nil {
			t.Fatalf("claim %q by %s: %v", terr, p, err)
		}
	}
}

func finishSetup(t *testing.T, g *Game) {
	t.Helper()
	for g.Phase == PhaseSetupReinforce {
		pid := g.Players[g.CurrentPlayer].ID
		var terr Territory
		for t, ts := range g.Territories {
			if ts.Owner == g.CurrentPlayer {
				terr = t
				break
			}
		}
		if terr == "" {
			t.Fatalf("player %s has no territory during setup reinforce", pid)
		}
		if err := g.PlaceInitialArmy(pid, terr); err != nil {
			t.Fatalf("place setup army: %v", err)
		}
	}
}

func TestClassicBoardValid(t *testing.T) {
	b := ClassicBoard()
	if err := b.Validate(); err != nil {
		t.Fatalf("board invalid: %v", err)
	}
	if len(b.Order) != 42 {
		t.Fatalf("expected 42 territories, got %d", len(b.Order))
	}
}

func TestSetupFlowToReinforce(t *testing.T) {
	g := mustGame(t)
	claimAllRoundRobin(t, g)
	if g.Phase != PhaseSetupReinforce {
		t.Fatalf("expected setup reinforce phase, got %s", g.Phase)
	}
	finishSetup(t, g)
	if g.Phase != PhaseReinforce {
		t.Fatalf("expected reinforce phase, got %s", g.Phase)
	}
	if g.PendingReinforcements < 3 {
		t.Fatalf("expected minimum reinforcements, got %d", g.PendingReinforcements)
	}
}

func TestAutoStartDistributesTerritoriesAndBeginsReinforce(t *testing.T) {
	g, err := NewClassicAutoStartGame([]string{"p1", "p2", "p3", "p4"}, &seqRNG{v: []int{0}})
	if err != nil {
		t.Fatalf("new auto start game: %v", err)
	}
	if g.Phase != PhaseReinforce {
		t.Fatalf("expected reinforce phase, got %s", g.Phase)
	}
	if g.PendingReinforcements < 3 {
		t.Fatalf("expected minimum pending reinforcements, got %d", g.PendingReinforcements)
	}

	counts := make(map[int]int, len(g.Players))
	for _, ts := range g.Territories {
		if ts.Owner < 0 {
			t.Fatalf("found unowned territory in auto-start state")
		}
		if ts.Armies < 1 {
			t.Fatalf("found territory with no armies in auto-start state")
		}
		counts[ts.Owner]++
	}
	minC, maxC := len(g.Board.Order), 0
	for i := range g.Players {
		c := counts[i]
		if c < minC {
			minC = c
		}
		if c > maxC {
			maxC = c
		}
		if g.SetupReserves[i] != 0 {
			t.Fatalf("expected setup reserves to be consumed, got %d", g.SetupReserves[i])
		}
	}
	if maxC-minC > 1 {
		t.Fatalf("expected near-even territory split, got min=%d max=%d", minC, maxC)
	}
}

func TestReinforcementIncludesContinentBonus(t *testing.T) {
	g := mustGame(t)
	for _, t := range g.Board.Order {
		g.Territories[t] = TerritoryState{Owner: 1, Armies: 1}
	}
	// p1 controls all North America (9 territories) and only those.
	for _, t := range g.Board.Continents["north_america"].Territories {
		g.Territories[t] = TerritoryState{Owner: 0, Armies: 2}
	}
	g.Phase = PhaseReinforce
	g.CurrentPlayer = 0
	g.startTurn()
	// floor(9/3)=3 + NA bonus 5.
	if g.PendingReinforcements != 8 {
		t.Fatalf("expected 8 reinforcements, got %d", g.PendingReinforcements)
	}
}

func TestAttackAndOccupy(t *testing.T) {
	g := mustGame(t)
	g.Phase = PhaseAttack
	g.CurrentPlayer = 0
	g.Territories["Alaska"] = TerritoryState{Owner: 0, Armies: 4}
	g.Territories["Kamchatka"] = TerritoryState{Owner: 1, Armies: 1}
	g.Territories["Peru"] = TerritoryState{Owner: 1, Armies: 1}

	// attacker roll: 6 (5%6+1), defender roll: 1 (0%6+1)
	g.rng = &seqRNG{v: []int{5, 0}}
	res, err := g.Attack("p1", "Alaska", "Kamchatka", 1, 1)
	if err != nil {
		t.Fatalf("attack: %v", err)
	}
	if !res.Conquered {
		t.Fatalf("expected conquest")
	}
	if g.Phase != PhaseOccupy || g.Occupy == nil {
		t.Fatalf("expected occupy phase")
	}
	if err := g.OccupyTerritory("p1", 1); err != nil {
		t.Fatalf("occupy: %v", err)
	}
	if g.Phase != PhaseAttack {
		t.Fatalf("expected attack phase after occupy, got %s", g.Phase)
	}
	if g.Territories["Kamchatka"].Owner != 0 {
		t.Fatalf("expected Kamchatka owned by attacker")
	}
}

func TestAttackAfterJSONRoundTripUsesDefaultRNG(t *testing.T) {
	g := mustGame(t)
	g.Phase = PhaseAttack
	g.CurrentPlayer = 0
	g.Territories["Alaska"] = TerritoryState{Owner: 0, Armies: 4}
	g.Territories["Kamchatka"] = TerritoryState{Owner: 1, Armies: 2}

	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal game: %v", err)
	}
	var restored Game
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatalf("unmarshal game: %v", err)
	}

	if _, err := restored.Attack("p1", "Alaska", "Kamchatka", 1, 1); err != nil {
		t.Fatalf("attack after round trip: %v", err)
	}
}

func TestTradeValueProgression(t *testing.T) {
	want := []int{4, 6, 8, 10, 12, 15, 20, 25}
	for i, v := range want {
		got := nextTradeValue(i + 1)
		if got != v {
			t.Fatalf("set %d: expected %d got %d", i+1, v, got)
		}
	}
}

func TestCardTradeAndTerritoryBonus(t *testing.T) {
	g := mustGame(t)
	g.Phase = PhaseReinforce
	g.CurrentPlayer = 0
	g.PendingReinforcements = 0
	g.Players[0].Cards = []Card{
		{Territory: "Alaska", Symbol: Infantry},
		{Territory: "Peru", Symbol: Cavalry},
		{Territory: "China", Symbol: Artillery},
	}
	g.Territories["Alaska"] = TerritoryState{Owner: 0, Armies: 3}

	got, err := g.TradeCards("p1", [3]int{0, 1, 2})
	if err != nil {
		t.Fatalf("trade cards: %v", err)
	}
	// First trade is 4, plus territory bonus +2 for owning Alaska.
	if got != 6 {
		t.Fatalf("expected trade value 6, got %d", got)
	}
	if g.PendingReinforcements != 6 {
		t.Fatalf("expected pending reinforcements 6, got %d", g.PendingReinforcements)
	}
}

func TestEndTurnDrawsCardIfConquered(t *testing.T) {
	g := mustGame(t)
	g.Phase = PhaseFortify
	g.CurrentPlayer = 0
	g.ConqueredThisTurn = true
	deckBefore := len(g.Deck)
	if err := g.EndTurn("p1"); err != nil {
		t.Fatalf("end turn: %v", err)
	}
	if len(g.Players[0].Cards) != 1 {
		t.Fatalf("expected one drawn card, got %d", len(g.Players[0].Cards))
	}
	if len(g.Deck) != deckBefore-1 {
		t.Fatalf("expected deck to decrement by 1")
	}
}
