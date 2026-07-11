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
		placed := false
		for pi, p := range g.Players {
			if g.SetupReserves[pi] <= 0 {
				continue
			}
			var terr Territory
			for tt, ts := range g.Territories {
				if ts.Owner == pi {
					terr = tt
					break
				}
			}
			if terr == "" {
				t.Fatalf("player %s has no territory during setup reinforce", p.ID)
			}
			if err := g.PlaceInitialArmy(p.ID, terr); err != nil {
				t.Fatalf("place setup army: %v", err)
			}
			placed = true
			break
		}
		if !placed {
			t.Fatalf("setup stuck: no player has reserves but phase is still setup_reinforce")
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

func TestNewClassicGamePlayerCountBounds(t *testing.T) {
	if _, err := NewClassicGame([]string{"p1"}, &seqRNG{v: []int{0}}); err != ErrInvalidPlayerCount {
		t.Fatalf("expected ErrInvalidPlayerCount for 1 player, got %v", err)
	}
	if _, err := NewClassicGame([]string{"p1", "p2"}, &seqRNG{v: []int{0}}); err != ErrInvalidPlayerCount {
		t.Fatalf("expected ErrInvalidPlayerCount for 2 players, got %v", err)
	}
	if _, err := NewClassicGame([]string{"p1", "p2", "p3", "p4", "p5", "p6", "p7"}, &seqRNG{v: []int{0}}); err != ErrInvalidPlayerCount {
		t.Fatalf("expected ErrInvalidPlayerCount for 7 players, got %v", err)
	}
	g, err := NewClassicGame([]string{"p1", "p2", "p3"}, &seqRNG{v: []int{0}})
	if err != nil {
		t.Fatalf("new 3-player game: %v", err)
	}
	for i := range g.Players {
		if g.SetupReserves[i] != 35 {
			t.Fatalf("expected 35 starting armies for 3 players, got %d", g.SetupReserves[i])
		}
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
	p0 := g.Players[0].ID
	g.Territories["Alaska"] = TerritoryState{Owner: 0, Armies: 4}
	g.Territories["Kamchatka"] = TerritoryState{Owner: 1, Armies: 1}
	g.Territories["Peru"] = TerritoryState{Owner: 1, Armies: 1}

	// attacker roll: 6 (5%6+1), defender roll: 1 (0%6+1)
	g.rng = &seqRNG{v: []int{5, 0}}
	res, _, err := g.Attack(p0, "Alaska", "Kamchatka", 1, 1)
	if err != nil {
		t.Fatalf("attack: %v", err)
	}
	if !res.Conquered {
		t.Fatalf("expected conquest")
	}
	if g.Phase != PhaseOccupy || g.Occupy == nil {
		t.Fatalf("expected occupy phase")
	}
	if err := g.OccupyTerritory(p0, 1); err != nil {
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
	p0 := g.Players[0].ID
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

	if _, _, err := restored.Attack(p0, "Alaska", "Kamchatka", 1, 1); err != nil {
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
	p0 := g.Players[0].ID
	g.PendingReinforcements = 0
	g.Players[0].Cards = []Card{
		{Territory: "Alaska", Symbol: Infantry},
		{Territory: "Peru", Symbol: Cavalry},
		{Territory: "China", Symbol: Artillery},
	}
	g.Territories["Alaska"] = TerritoryState{Owner: 0, Armies: 3}

	got, err := g.TradeCards(p0, [3]int{0, 1, 2})
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

func TestAttackEventPayload(t *testing.T) {
	g := mustGame(t)
	g.Phase = PhaseAttack
	g.CurrentPlayer = 0
	g.TurnNumber = 3
	p0 := g.Players[0].ID
	p1 := g.Players[1].ID
	g.Territories["Alaska"] = TerritoryState{Owner: 0, Armies: 4}
	g.Territories["Kamchatka"] = TerritoryState{Owner: 1, Armies: 2}

	// attacker rolls: 6,4 (seqRNG values 5,3); defender rolls: 5,2 (values 4,1)
	g.rng = &seqRNG{v: []int{5, 3, 4, 1}}
	_, ev, err := g.Attack(p0, "Alaska", "Kamchatka", 2, 2)
	if err != nil {
		t.Fatalf("attack: %v", err)
	}
	if ev == nil {
		t.Fatal("expected non-nil domain event")
	}
	if ev.Type != EventTypeCombatRollResolved {
		t.Fatalf("unexpected event type: %q", ev.Type)
	}
	if ev.Version != EventVersionCombatRollResolved {
		t.Fatalf("unexpected event version: %d", ev.Version)
	}
	if ev.ActorPlayerID != p0 {
		t.Fatalf("expected actor %q, got %q", p0, ev.ActorPlayerID)
	}

	pl, ok := ev.Payload.(CombatRollResolvedPayload)
	if !ok {
		t.Fatalf("unexpected payload type: %T", ev.Payload)
	}
	if pl.SchemaVersion != SchemaVersionCombatRollResolved {
		t.Fatalf("unexpected schema version: %d", pl.SchemaVersion)
	}
	if pl.TurnNumber != 3 {
		t.Fatalf("expected turn_number 3, got %d", pl.TurnNumber)
	}
	if pl.Phase != string(PhaseAttack) {
		t.Fatalf("expected phase attack, got %q", pl.Phase)
	}
	if pl.AttackerPlayerID != p0 {
		t.Fatalf("unexpected attacker player id: %q", pl.AttackerPlayerID)
	}
	if pl.DefenderPlayerID != p1 {
		t.Fatalf("unexpected defender player id: %q", pl.DefenderPlayerID)
	}
	if pl.SourceTerritoryID != "Alaska" {
		t.Fatalf("unexpected source territory: %q", pl.SourceTerritoryID)
	}
	if pl.TargetTerritoryID != "Kamchatka" {
		t.Fatalf("unexpected target territory: %q", pl.TargetTerritoryID)
	}
	if pl.SourceArmiesBefore != 4 {
		t.Fatalf("expected source before=4, got %d", pl.SourceArmiesBefore)
	}
	if pl.TargetArmiesBefore != 2 {
		t.Fatalf("expected target before=2, got %d", pl.TargetArmiesBefore)
	}
	if len(pl.AttackerDice) != 2 || len(pl.DefenderDice) != 2 {
		t.Fatalf("unexpected dice lengths: att=%d def=%d", len(pl.AttackerDice), len(pl.DefenderDice))
	}
	// Dice must be in descending order
	if pl.AttackerDice[0] < pl.AttackerDice[1] {
		t.Fatalf("attacker dice not sorted descending: %v", pl.AttackerDice)
	}
	if pl.DefenderDice[0] < pl.DefenderDice[1] {
		t.Fatalf("defender dice not sorted descending: %v", pl.DefenderDice)
	}
	if len(pl.Comparisons) != 2 {
		t.Fatalf("expected 2 comparisons, got %d", len(pl.Comparisons))
	}
	// Army counts after must match game state
	if pl.SourceArmiesAfter != g.Territories["Alaska"].Armies {
		t.Fatalf("source after mismatch: payload=%d game=%d", pl.SourceArmiesAfter, g.Territories["Alaska"].Armies)
	}
	if pl.TargetArmiesAfter != g.Territories["Kamchatka"].Armies {
		t.Fatalf("target after mismatch: payload=%d game=%d", pl.TargetArmiesAfter, g.Territories["Kamchatka"].Armies)
	}
}

func TestAttackEventTieLoserIsAttacker(t *testing.T) {
	g := mustGame(t)
	g.Phase = PhaseAttack
	g.CurrentPlayer = 0
	g.Territories["Alaska"] = TerritoryState{Owner: 0, Armies: 3}
	g.Territories["Kamchatka"] = TerritoryState{Owner: 1, Armies: 2}

	// Both roll 4 (seqRNG value 3 → 3%6+1=4)
	g.rng = &seqRNG{v: []int{3}}
	_, ev, err := g.Attack(g.Players[0].ID, "Alaska", "Kamchatka", 1, 1)
	if err != nil {
		t.Fatalf("attack: %v", err)
	}
	pl := ev.Payload.(CombatRollResolvedPayload)
	if len(pl.Comparisons) != 1 {
		t.Fatalf("expected 1 comparison, got %d", len(pl.Comparisons))
	}
	if pl.Comparisons[0].Loser != "attacker" {
		t.Fatalf("tie must record attacker as loser, got %q", pl.Comparisons[0].Loser)
	}
	if pl.AttackerLosses != 1 || pl.DefenderLosses != 0 {
		t.Fatalf("expected attacker_losses=1 defender_losses=0, got att=%d def=%d",
			pl.AttackerLosses, pl.DefenderLosses)
	}
}

func TestAttackEventTerritoryCaptured(t *testing.T) {
	g := mustGame(t)
	g.Phase = PhaseAttack
	g.CurrentPlayer = 0
	g.Territories["Alaska"] = TerritoryState{Owner: 0, Armies: 4}
	g.Territories["Kamchatka"] = TerritoryState{Owner: 1, Armies: 1}

	// attacker rolls 6, defender rolls 1
	g.rng = &seqRNG{v: []int{5, 0}}
	_, ev, err := g.Attack(g.Players[0].ID, "Alaska", "Kamchatka", 1, 1)
	if err != nil {
		t.Fatalf("attack: %v", err)
	}
	pl := ev.Payload.(CombatRollResolvedPayload)
	if !pl.TerritoryCaptured {
		t.Fatal("expected territory_captured=true")
	}
	if pl.TargetArmiesAfter != 0 {
		t.Fatalf("expected target_armies_after=0, got %d", pl.TargetArmiesAfter)
	}
}

func TestInvalidAttackProducesNoEvent(t *testing.T) {
	g := mustGame(t)
	g.Phase = PhaseAttack
	g.CurrentPlayer = 0
	// Not enough armies to attack
	g.Territories["Alaska"] = TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Kamchatka"] = TerritoryState{Owner: 1, Armies: 2}

	_, ev, err := g.Attack(g.Players[0].ID, "Alaska", "Kamchatka", 1, 1)
	if err == nil {
		t.Fatal("expected error for invalid attack")
	}
	if ev != nil {
		t.Fatal("expected nil event for invalid attack")
	}
}

func TestAttackEventOutOfTurnProducesNoEvent(t *testing.T) {
	g := mustGame(t)
	g.Phase = PhaseAttack
	g.CurrentPlayer = 0
	g.Territories["Alaska"] = TerritoryState{Owner: 1, Armies: 4}
	g.Territories["Kamchatka"] = TerritoryState{Owner: 0, Armies: 2}

	// player 1 tries to attack but it's player 0's turn
	_, ev, err := g.Attack(g.Players[1].ID, "Alaska", "Kamchatka", 1, 1)
	if err == nil {
		t.Fatal("expected out-of-turn error")
	}
	if ev != nil {
		t.Fatal("expected nil event for out-of-turn attack")
	}
}

func TestEndTurnDrawsCardIfConquered(t *testing.T) {
	g := mustGame(t)
	g.Phase = PhaseFortify
	g.CurrentPlayer = 0
	g.ConqueredThisTurn = true
	deckBefore := len(g.Deck)
	if err := g.EndTurn(g.Players[0].ID); err != nil {
		t.Fatalf("end turn: %v", err)
	}
	if len(g.Players[0].Cards) != 1 {
		t.Fatalf("expected one drawn card, got %d", len(g.Players[0].Cards))
	}
	if len(g.Deck) != deckBefore-1 {
		t.Fatalf("expected deck to decrement by 1")
	}
}
