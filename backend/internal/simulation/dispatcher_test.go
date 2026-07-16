package simulation

import (
	"errors"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// baseGame builds a real 4-player classic game (real board, real deck) via
// the same constructor a simulation would use, so every dispatcher test
// exercises the actual engine, not a hand-rolled fake. It returns the
// current player's actual ID, not the "p0" string passed into the
// constructor -- risk.NewClassicAutoStartGame shuffles player order
// internally, so g.Players[0].ID (the seat CurrentPlayer=0 refers to)
// isn't necessarily the first ID in the input slice.
func baseGame(t *testing.T) (*risk.Game, string) {
	t.Helper()
	g, err := risk.NewClassicAutoStartGame([]string{"p0", "p1", "p2", "p3"}, NewDeterministicRNG(1))
	if err != nil {
		t.Fatalf("build base game: %v", err)
	}
	return g, g.Players[g.CurrentPlayer].ID
}

func TestDispatchPlaceInitialArmy(t *testing.T) {
	g, p0 := baseGame(t)
	g.Phase = risk.PhaseSetupReinforce
	g.SetupReserves[0] = 3
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}

	_, err := Dispatch(g, p0, bot.Command{Action: bot.ActionPlaceInitialArmy, Territory: "Alaska"})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if g.Territories["Alaska"].Armies != 2 {
		t.Fatalf("expected Alaska to gain 1 army, got %d", g.Territories["Alaska"].Armies)
	}
	if g.SetupReserves[0] != 2 {
		t.Fatalf("expected reserves to drop to 2, got %d", g.SetupReserves[0])
	}
}

func TestDispatchTradeCards(t *testing.T) {
	g, p0 := baseGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 0
	g.Players[0].Cards = []risk.Card{
		{Territory: "Alaska", Symbol: risk.Infantry},
		{Territory: "Peru", Symbol: risk.Cavalry},
		{Territory: "Egypt", Symbol: risk.Artillery},
	}

	result, err := Dispatch(g, p0, bot.Command{Action: bot.ActionTradeCards, CardIndices: [3]int{0, 1, 2}})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if result.ReinforcementsGranted <= 0 {
		t.Fatalf("expected a positive ReinforcementsGranted, got %d", result.ReinforcementsGranted)
	}
	if g.PendingReinforcements != result.ReinforcementsGranted {
		t.Fatalf("expected PendingReinforcements (%d) to match the reported grant (%d)", g.PendingReinforcements, result.ReinforcementsGranted)
	}
}

func TestDispatchPlaceReinforcement(t *testing.T) {
	g, p0 := baseGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}

	_, err := Dispatch(g, p0, bot.Command{Action: bot.ActionPlaceReinforcement, Territory: "Alaska", Armies: 3})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if g.Territories["Alaska"].Armies != 4 {
		t.Fatalf("expected Alaska to have 4 armies, got %d", g.Territories["Alaska"].Armies)
	}
	if g.Phase != risk.PhaseAttack {
		t.Fatalf("expected phase Attack once pending reinforcements hit 0, got %s", g.Phase)
	}
}

// TestDispatchAttackComputesDefenderDiceIndependentlyOfCommand is the key
// correctness test for this file: bot.Command.DefenderDice is deliberately
// set to a WRONG value here, and the dispatcher must ignore it, computing
// min(2, target's current armies) itself instead. AttackResult.DefenderRolls'
// length is the observable proof of how many defender dice were actually
// used.
func TestDispatchAttackComputesDefenderDiceIndependentlyOfCommand(t *testing.T) {
	cases := []struct {
		name         string
		targetArmies int
		wantDefDice  int
		commandLies  int // the wrong DefenderDice value the command carries
	}{
		{"target has 5 armies -> capped at 2", 5, 2, 1},
		{"target has 1 army -> only 1 available", 1, 1, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, p0 := baseGame(t)
			g.Phase = risk.PhaseAttack
			g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 5}
			g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: tc.targetArmies}

			result, err := Dispatch(g, p0, bot.Command{
				Action:       bot.ActionAttack,
				From:         "Alaska",
				To:           "Kamchatka",
				AttackerDice: 3,
				DefenderDice: tc.commandLies, // must be ignored
			})
			if err != nil {
				t.Fatalf("Dispatch: %v", err)
			}
			if got := len(result.AttackResult.DefenderRolls); got != tc.wantDefDice {
				t.Fatalf("expected %d defender dice actually rolled (min(2, %d armies)), got %d -- command's (wrong) DefenderDice=%d must never be used",
					tc.wantDefDice, tc.targetArmies, got, tc.commandLies)
			}
		})
	}
}

// TestDispatchAttackConquestTransitionsToOccupy: 20 attacking armies vs 1
// defending army is overwhelmingly favorable but still genuinely random
// per exchange, so this repeats the attack (the phase stays Attack until
// a conquest happens) until the target falls, bounded generously -- with
// this army disparity the probability of never conquering within the
// bound is astronomically small.
func TestDispatchAttackConquestTransitionsToOccupy(t *testing.T) {
	g, p0 := baseGame(t)
	g.Phase = risk.PhaseAttack
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 20}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1}

	conquered := false
	for i := 0; i < 20 && g.Phase == risk.PhaseAttack; i++ {
		result, err := Dispatch(g, p0, bot.Command{
			Action:       bot.ActionAttack,
			From:         "Alaska",
			To:           "Kamchatka",
			AttackerDice: min(3, g.Territories["Alaska"].Armies-1),
		})
		if err != nil {
			t.Fatalf("Dispatch: %v", err)
		}
		if result.AttackResult.Conquered {
			conquered = true
			break
		}
	}
	if !conquered {
		t.Fatalf("expected Kamchatka to fall to a 20-vs-1 assault within 20 exchanges")
	}
	if g.Phase != risk.PhaseOccupy {
		t.Fatalf("expected phase Occupy after conquest, got %s", g.Phase)
	}
	if g.Occupy == nil || g.Occupy.From != "Alaska" || g.Occupy.To != "Kamchatka" {
		t.Fatalf("expected Occupy state for Alaska -> Kamchatka, got %+v", g.Occupy)
	}
}

func TestDispatchEndAttack(t *testing.T) {
	g, p0 := baseGame(t)
	g.Phase = risk.PhaseAttack

	_, err := Dispatch(g, p0, bot.Command{Action: bot.ActionEndAttack})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if g.Phase != risk.PhaseFortify {
		t.Fatalf("expected phase Fortify, got %s", g.Phase)
	}
}

func TestDispatchOccupy(t *testing.T) {
	g, p0 := baseGame(t)
	g.Phase = risk.PhaseOccupy
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 0, Armies: 0}
	g.Occupy = &risk.OccupyState{From: "Alaska", To: "Kamchatka", MinMove: 1, MaxMove: 4}

	_, err := Dispatch(g, p0, bot.Command{Action: bot.ActionOccupy, Armies: 2})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if g.Territories["Alaska"].Armies != 3 || g.Territories["Kamchatka"].Armies != 2 {
		t.Fatalf("expected Alaska=3 Kamchatka=2, got Alaska=%d Kamchatka=%d",
			g.Territories["Alaska"].Armies, g.Territories["Kamchatka"].Armies)
	}
	if g.Phase != risk.PhaseAttack {
		t.Fatalf("expected phase Attack after occupying, got %s", g.Phase)
	}
}

// TestDispatchOccupyTriggersGameOver reconstructs the exact intermediate
// state risk.Attack leaves behind on a winning, board-clearing conquest:
// ownership already transferred to the attacker (ar's dst.Owner flip
// happens inside Attack, before Phase becomes Occupy), armies still at 0
// until OccupyTerritory adds some. Every other territory on the board is
// already owned by the same player, so completing this occupy should
// trigger checkWinner() to end the game.
func TestDispatchOccupyTriggersGameOver(t *testing.T) {
	g, p0 := baseGame(t)
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 3}
	}
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 0, Armies: 0} // just-conquered, not yet occupied
	g.Phase = risk.PhaseOccupy
	g.Occupy = &risk.OccupyState{From: "Alaska", To: "Kamchatka", MinMove: 1, MaxMove: 4}

	_, err := Dispatch(g, p0, bot.Command{Action: bot.ActionOccupy, Armies: 1})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if g.Phase != risk.PhaseGameOver {
		t.Fatalf("expected phase GameOver once the attacker owns every territory, got %s", g.Phase)
	}
}

func TestDispatchFortify(t *testing.T) {
	g, p0 := baseGame(t)
	g.Phase = risk.PhaseFortify
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 0, Armies: 1}

	_, err := Dispatch(g, p0, bot.Command{Action: bot.ActionFortify, From: "Alaska", To: "Northwest Territory", Armies: 3})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if g.Territories["Alaska"].Armies != 2 || g.Territories["Northwest Territory"].Armies != 4 {
		t.Fatalf("expected Alaska=2 Northwest Territory=4, got Alaska=%d NWT=%d",
			g.Territories["Alaska"].Armies, g.Territories["Northwest Territory"].Armies)
	}
	if !g.HasFortified {
		t.Fatalf("expected HasFortified to be set")
	}
}

func TestDispatchEndTurn(t *testing.T) {
	g, p0 := baseGame(t)
	g.Phase = risk.PhaseAttack

	_, err := Dispatch(g, p0, bot.Command{Action: bot.ActionEndTurn})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if g.Phase != risk.PhaseReinforce {
		t.Fatalf("expected phase Reinforce for the next player, got %s", g.Phase)
	}
	if g.Players[g.CurrentPlayer].ID == p0 {
		t.Fatalf("expected turn to advance to a different player")
	}
}

// TestDispatchEndTurnTriggersGameOver: unlike the occupy case, this
// reaches game-over through EndTurn's own checkWinner() call -- both
// paths must be exercised, since the simulator has to check for
// PhaseGameOver after either dispatch.
func TestDispatchEndTurnTriggersGameOver(t *testing.T) {
	g, p0 := baseGame(t)
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 0, Armies: 3}
	}
	g.Phase = risk.PhaseAttack

	_, err := Dispatch(g, p0, bot.Command{Action: bot.ActionEndTurn})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if g.Phase != risk.PhaseGameOver {
		t.Fatalf("expected phase GameOver once the current player owns every territory, got %s", g.Phase)
	}
}

func TestDispatchUnknownActionErrors(t *testing.T) {
	g, p0 := baseGame(t)
	g.Phase = risk.PhaseAttack

	_, err := Dispatch(g, p0, bot.Command{Action: "not_a_real_action"})
	if !errors.Is(err, ErrUnknownCommand) {
		t.Fatalf("expected ErrUnknownCommand, got %v", err)
	}
}
