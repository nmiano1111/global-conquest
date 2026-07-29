package bot

import (
	"context"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

func TestKillbotStrategyTradesCardsVoluntarily(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 3}
	g.Players[0].Cards = []risk.Card{
		{Territory: "Alaska", Symbol: risk.Infantry},
		{Territory: "Peru", Symbol: risk.Cavalry},
		{Territory: "Egypt", Symbol: risk.Artillery},
	}

	strat := NewKillbotStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionTradeCards {
		t.Fatalf("expected trade_cards (Killbot trades any legal set, via BetterPixie's own voluntary cash policy), got %s", cmd.Action)
	}
}

func TestKillbotStrategyReinforcePlacesTowardKillTarget(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 5
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 1, Armies: 1000}
	}
	g.Territories["Central America"] = risk.TerritoryState{Owner: 1, Armies: 2}
	g.Territories["Western United States"] = risk.TerritoryState{Owner: 0, Armies: 100}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 2, Armies: 1}

	strat := NewKillbotStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionPlaceReinforcement {
		t.Fatalf("expected place_reinforcement, got %s", cmd.Action)
	}
	if cmd.Territory != "Western United States" {
		t.Fatalf("expected Western United States (routing toward the kill target via Central America), got %s", cmd.Territory)
	}
	if cmd.Armies != 5 {
		t.Fatalf("expected every pending reinforcement dumped in one command, got %d", cmd.Armies)
	}
}

func TestKillbotStrategyAttackFiresKillBranch(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 1, Armies: 1000}
	}
	g.Territories["Central America"] = risk.TerritoryState{Owner: 1, Armies: 2}
	g.Territories["Western United States"] = risk.TerritoryState{Owner: 0, Armies: 100}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 2, Armies: 1}

	strat := NewKillbotStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack {
		t.Fatalf("expected attack, got %s", cmd.Action)
	}
	if cmd.From != "Western United States" || cmd.To != "Central America" {
		t.Fatalf("expected Western United States -> Central America (kill-target branch), got %s -> %s", cmd.From, cmd.To)
	}
}

// TestKillbotStrategyAttackFallsBackToBetterPixieWhenNoKillTarget confirms
// attack() delegates entirely to the BetterPixie backer (not plain
// Pixie -- see Lux_Port_Notes.md's Killbot addendum) once no rival is
// weak enough to trigger the kill branch. South America has no
// positive-bonus-continent target here worth BetterPixie's own
// continent-scoped attack sequence (pi owns 3 of 4 South America
// territories, but that's evaluated by betterPixieWantedContinents
// rather than assumed), so this exercises attackForCard's board-wide
// best-ratio scan, the same fallback stage both the old and new backer
// share.
func TestKillbotStrategyAttackFallsBackToBetterPixieWhenNoKillTarget(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Peru"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Argentina"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: 1, Armies: 3}

	strat := NewKillbotStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack {
		t.Fatalf("expected attack, got %s", cmd.Action)
	}
	if cmd.From != "Venezuela" || cmd.To != "Brazil" {
		t.Fatalf("expected Venezuela -> Brazil (BetterPixie fallback: no rival is weak enough to trigger the kill branch), got %s -> %s", cmd.From, cmd.To)
	}
}

func TestKillbotStrategyAttackEndsWhenNothingQualifies(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 1}

	strat := NewKillbotStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionEndAttack {
		t.Fatalf("expected end_attack, got %s", cmd.Action)
	}
}

// TestKillbotStrategyOccupyDelegatesToBetterPixieLogic confirms occupy
// delegates to the BetterPixie backer (not plain Pixie -- see
// Lux_Port_Notes.md's Killbot addendum): BetterPixie's moveArmiesIn only
// checks for a *zero* raw enemy-neighbor count on either side (unlike
// plain Pixie's own magnitude comparison at any nonzero tie), so this
// exercises exactly that branch -- Alaska has zero enemy neighbors (every
// neighbor, including the just-conquered Kamchatka, is pi-owned) while
// Kamchatka still faces four (Yakutsk/Irkutsk/Mongolia/Japan, all left at
// the default owner), giving a clean aEnemies==0 && dEnemies!=0 case that
// doesn't depend on BetterPixie's more elaborate connectivity fallback.
func TestKillbotStrategyOccupyDelegatesToBetterPixieLogic(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseOccupy
	g.Territories["Alaska"] = risk.TerritoryState{Owner: 0, Armies: 10}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 0, Armies: 1}
	g.Occupy = &risk.OccupyState{From: "Alaska", To: "Kamchatka", MinMove: 2, MaxMove: 4}

	strat := NewKillbotStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionOccupy {
		t.Fatalf("expected occupy, got %s", cmd.Action)
	}
	if cmd.Armies != 4 {
		t.Fatalf("expected the legal maximum (4): Alaska has zero enemy neighbors, Kamchatka has four, got %d", cmd.Armies)
	}
}

// TestKillbotStrategyFortifyDelegatesToBetterPixie confirms fortify()
// delegates entirely to the BetterPixie backer (not the generic
// bestFortifyDestination heuristic previously shared with every other
// Lux-inspired persona -- see Lux_Port_Notes.md's Killbot addendum). pi
// owns only 3 of Africa's 6 territories, so this exercises
// betterPixieFortifyDestination's "scraps" branch (weakest-nearby-enemy
// criterion), not its continent-border-draining one.
func TestKillbotStrategyFortifyDelegatesToBetterPixie(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseFortify
	g.Territories["Madagascar"] = risk.TerritoryState{Owner: 0, Armies: 5}
	g.Territories["South Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}
	g.Territories["East Africa"] = risk.TerritoryState{Owner: 0, Armies: 2}

	strat := NewKillbotStrategy()
	cmd, _, err := strat.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionFortify {
		t.Fatalf("expected fortify, got %s", cmd.Action)
	}
	if cmd.From != "South Africa" || cmd.To != "East Africa" {
		t.Fatalf("expected South Africa -> East Africa, got %s -> %s", cmd.From, cmd.To)
	}
}
