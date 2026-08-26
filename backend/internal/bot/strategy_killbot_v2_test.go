package bot

import (
	"context"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// TestKillbotV2GuardPlacementRedirectsLockedInTerritory directly exercises
// guardPlacement against the locked-in-territory bug described in
// StrategyKillbotV2's doc comment: Argentina has zero enemy neighbors once
// Kamchatka is the board's only enemy-owned territory (its own neighbors,
// Peru and Brazil, are both pi-owned), so a pick of Argentina must be
// redirected to a territory that can actually attack something.
func TestKillbotV2GuardPlacementRedirectsLockedInTerritory(t *testing.T) {
	g, p0 := newTestGame(t)
	pi := 0
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: pi, Armies: 1}
	}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1}

	v2 := NewKillbotV2Strategy()
	actions := risk.LegalReinforcements(g, p0)
	locked := Command{Action: ActionPlaceReinforcement, Territory: "Argentina", Armies: 3, KillTarget: "someone"}
	got := v2.guardPlacement(g, p0, locked, actions)

	if got.Territory == "Argentina" {
		t.Fatalf("expected guardPlacement to redirect away from Argentina (zero enemy neighbors), got %+v", got)
	}
	if enemyNeighborCount(g, risk.Territory(got.Territory), pi) == 0 {
		t.Fatalf("expected the redirected territory to have at least one enemy neighbor, got %+v", got)
	}
	if got.KillTarget != "" {
		t.Fatalf("expected KillTarget cleared on redirect, got %q", got.KillTarget)
	}
}

// TestKillbotV2GuardPlacementLeavesAttackableTerritoryAlone confirms the
// guard is a no-op when the underlying pick already has a real enemy
// neighbor (Alaska borders Kamchatka), so it never second-guesses a
// legitimate decision.
func TestKillbotV2GuardPlacementLeavesAttackableTerritoryAlone(t *testing.T) {
	g, p0 := newTestGame(t)
	pi := 0
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 3
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: pi, Armies: 1}
	}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1}

	v2 := NewKillbotV2Strategy()
	actions := risk.LegalReinforcements(g, p0)
	cmd := Command{Action: ActionPlaceReinforcement, Territory: "Alaska", Armies: 3, KillTarget: "someone"}
	got := v2.guardPlacement(g, p0, cmd, actions)

	if got != cmd {
		t.Fatalf("expected guardPlacement to leave an already-attackable pick unchanged, got %+v want %+v", got, cmd)
	}
}

// TestKillbotV2ShouldGoHogWildScalesWithBoardSize confirms
// killbotV2ShouldGoHogWild's per-territory threshold, not shouldGoHogWild's
// flat 1500-army cap, decides the outcome: pi's own armies are concentrated
// on a single territory (so the "own > total-own" majority clause never
// fires either way) at values just above and just below
// territories*killbotV2HogWildArmiesPerTerritory.
func TestKillbotV2ShouldGoHogWildScalesWithBoardSize(t *testing.T) {
	g, _ := newTestGame(t)
	pi := 0
	territories := len(g.Board.Order)
	threshold := territories * killbotV2HogWildArmiesPerTerritory

	setBoard := func(ownArmies, restArmies int) {
		for i, terr := range g.Board.Order {
			if i == 0 {
				g.Territories[terr] = risk.TerritoryState{Owner: pi, Armies: ownArmies}
				continue
			}
			g.Territories[terr] = risk.TerritoryState{Owner: 1, Armies: restArmies}
		}
	}

	// restArmies must keep the other 41 territories' combined total above
	// own at both test values, so the "own > total-own" majority clause
	// never fires either way -- only the per-territory cap is on trial.
	const restArmies = 40

	setBoard(threshold+1, restArmies)
	if !killbotV2ShouldGoHogWild(g, pi, killbotV2HogWildArmiesPerTerritory) {
		t.Fatalf("expected hogwild once own armies exceed territories*%d=%d, got false", killbotV2HogWildArmiesPerTerritory, threshold)
	}

	setBoard(threshold-1, restArmies)
	if killbotV2ShouldGoHogWild(g, pi, killbotV2HogWildArmiesPerTerritory) {
		t.Fatalf("expected no hogwild just under territories*%d=%d, got true", killbotV2HogWildArmiesPerTerritory, threshold)
	}
}

// TestKillbotV2AttackFallsThroughToBackerWhenKillRouteStalls sets up a
// committed kill plan whose route can't fire this turn (the routed
// territory only has 1 army, so src.Armies > 1 fails) with hogwild's
// conditions unmet either. StrategyKillbotV1 unconditionally ends the
// attack phase in this state (see strategy_killbot.go's attack);
// StrategyKillbotV2 must instead defer to the backer's own attack()
// decision, whatever it is.
func TestKillbotV2AttackFallsThroughToBackerWhenKillRouteStalls(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	pi := 0
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: pi, Armies: 1}
	}
	g.Territories["Central America"] = risk.TerritoryState{Owner: pi, Armies: 1} // routed territory, too weak to attack
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 1, Armies: 5}
	g.KillPlans = map[int]risk.KillPlan{pi: {Target: 1, Committed: true}}

	v1 := NewKillbotStrategy()
	v1Cmd, _, err := v1.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("v1 NextCommand: %v", err)
	}
	if v1Cmd.Action != ActionEndAttack {
		t.Fatalf("expected StrategyKillbotV1 to end the attack phase outright when a committed kill route stalls, got %+v", v1Cmd)
	}

	v2 := NewKillbotV2Strategy()
	v2Cmd, _, err := v2.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("v2 NextCommand: %v", err)
	}
	backerCmd, err := v2.KillbotStrategy.backer.attack(g, p0)
	if err != nil {
		t.Fatalf("backer.attack: %v", err)
	}
	if v2Cmd != backerCmd {
		t.Fatalf("expected KillbotV2 to fall through to the backer's own attack() when the kill route stalls, got %+v want %+v", v2Cmd, backerCmd)
	}
}

// TestKillbotV2ReinforceDoesNotCommitToUnfavorableKill sets up a kill
// target (Venezuela, 25 armies, player 2's only territory) that
// killTarget still selects -- pi's decoy stack on Kamchatka (40 armies)
// clears the eligibility bar -- but whose real defense the actual attack
// route (Brazil, 15 armies + 5 pending = 20 total) is only ~33% favored
// to beat (risk.ForecastAttack(20, 25) ~= 0.326, computed separately).
// StrategyKillbotV1's cost check never looks at Venezuela's real army
// count at all (Brazil borders it directly, so cost is always 0) and
// commits regardless; StrategyKillbotV2 must decline.
func TestKillbotV2ReinforceDoesNotCommitToUnfavorableKill(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 5
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 1, Armies: 1}
	}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 0, Armies: 40}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: 0, Armies: 15}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 2, Armies: 25}

	v1 := NewKillbotStrategy()
	v1Cmd, _, err := v1.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("v1 NextCommand: %v", err)
	}
	if v1Cmd.KillTarget == "" || v1Cmd.Territory != "Brazil" {
		t.Fatalf("expected StrategyKillbotV1 to commit toward Brazil regardless of Venezuela's defense, got %+v", v1Cmd)
	}

	v2 := NewKillbotV2Strategy()
	v2Cmd, _, err := v2.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("v2 NextCommand: %v", err)
	}
	if v2Cmd.KillTarget != "" {
		t.Fatalf("expected StrategyKillbotV2 to decline an unfavorable kill commitment (~33%% win probability), got %+v", v2Cmd)
	}
}

// TestKillbotV2ReinforceCommitsToFavorableKill is
// TestKillbotV2ReinforceDoesNotCommitToUnfavorableKill's mirror: Venezuela
// is weak (2 armies) rather than well-defended, so the same route is
// overwhelmingly favored. Confirms the win-probability gate doesn't
// suppress a genuinely good kill commitment, only a bad one.
func TestKillbotV2ReinforceCommitsToFavorableKill(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 5
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 1, Armies: 1}
	}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 0, Armies: 40}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: 0, Armies: 15}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 2, Armies: 2}

	v2 := NewKillbotV2Strategy()
	cmd, _, err := v2.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Territory != "Brazil" || cmd.KillTarget != g.Players[2].ID {
		t.Fatalf("expected StrategyKillbotV2 to commit toward Brazil against a weakly-defended target, got %+v", cmd)
	}
}

// TestKillbotV2AttackDeclinesUnfavorableKillRoute is
// TestKillbotV2ReinforceDoesNotCommitToUnfavorableKill's attack-phase
// counterpart: a kill plan is already committed (bypassing reinforce
// entirely) against a target whose defended territory the routed
// attacker (20 armies vs. 25) is only ~33% favored to beat.
// StrategyKillbotV1's attack() fires anyway (same cost=0 blind spot);
// StrategyKillbotV2 must decline and fall through to the backer instead
// of attacking.
func TestKillbotV2AttackDeclinesUnfavorableKillRoute(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	pi := 0
	// Everything but Brazil/Venezuela stays with player 2 -- keeping pi a
	// small minority of the board's total armies so shouldGoHogWild's
	// majority clause (unchanged, still shared with StrategyKillbotV1)
	// can't fire and mask the direct win-probability decision under test.
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 2, Armies: 5}
	}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: pi, Armies: 20}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 1, Armies: 25}
	g.KillPlans = map[int]risk.KillPlan{pi: {Target: 1, Committed: true}}

	v1 := NewKillbotStrategy()
	v1Cmd, _, err := v1.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("v1 NextCommand: %v", err)
	}
	if v1Cmd.Action != ActionAttack || v1Cmd.From != "Brazil" {
		t.Fatalf("expected StrategyKillbotV1 to attack from Brazil regardless of Venezuela's defense, got %+v", v1Cmd)
	}

	v2 := NewKillbotV2Strategy()
	v2Cmd, _, err := v2.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("v2 NextCommand: %v", err)
	}
	if v2Cmd.Action == ActionAttack && v2Cmd.From == "Brazil" && v2Cmd.To == "Venezuela" {
		t.Fatalf("expected StrategyKillbotV2 to decline the unfavorable attack (~33%% win probability), got %+v", v2Cmd)
	}
}

// TestKillbotV2CommitsAtLowerAdvantageRatioThanV1 sets pi's total armies
// (Brazil, its only territory) to 31 against a target (Venezuela, 20
// armies, player 2's only territory) -- between killTargetDefaultAdvantageRatio
// (2.0: needs > 40) and killbotV2KillAdvantageRatio (1.5: needs > 30).
// StrategyKillbotV1 must decline this commitment; StrategyKillbotV2 must
// take it. The matchup itself (31 vs 20, further padded by 5 pending
// reinforcements) is comfortably win-probability-favored, isolating the
// ratio as the only thing distinguishing the two decisions.
func TestKillbotV2CommitsAtLowerAdvantageRatioThanV1(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 5
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 1, Armies: 1}
	}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: 0, Armies: 31}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 2, Armies: 20}

	v1 := NewKillbotStrategy()
	v1Cmd, _, err := v1.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("v1 NextCommand: %v", err)
	}
	if v1Cmd.KillTarget != "" {
		t.Fatalf("expected StrategyKillbotV1 (2.0x ratio) to decline this commitment (31 armies <= 2.0*20=40), got %+v", v1Cmd)
	}

	v2 := NewKillbotV2Strategy()
	v2Cmd, _, err := v2.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("v2 NextCommand: %v", err)
	}
	if v2Cmd.Territory != "Brazil" || v2Cmd.KillTarget != g.Players[2].ID {
		t.Fatalf("expected StrategyKillbotV2 (1.5x ratio) to commit toward Brazil (31 armies > 1.5*20=30), got %+v", v2Cmd)
	}
}

// TestKillbotV2UsesGameBotTuningSnapshotOverDefault sets Brazil to 25
// armies against Venezuela's 20 -- below DefaultBotTuning's 1.5x
// commitment bar (25 <= 1.5*20=30) but above a custom 1.1x snapshot
// (25 > 1.1*20=22). The only difference between the two NextCommand calls
// is g.BotTuning itself, proving KillbotV2Strategy actually reads the
// per-game snapshot (effectiveBotTuning) rather than always falling back
// to the compiled-in default.
func TestKillbotV2UsesGameBotTuningSnapshotOverDefault(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseReinforce
	g.PendingReinforcements = 5
	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: 1, Armies: 1}
	}
	g.Territories["Brazil"] = risk.TerritoryState{Owner: 0, Armies: 25}
	g.Territories["Venezuela"] = risk.TerritoryState{Owner: 2, Armies: 20}

	v2 := NewKillbotV2Strategy()

	defaultCmd, _, err := v2.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("default NextCommand: %v", err)
	}
	if defaultCmd.KillTarget != "" {
		t.Fatalf("expected the default 1.5x tuning to decline this commitment (25 armies <= 1.5*20=30), got %+v", defaultCmd)
	}

	tuning := DefaultBotTuning()
	tuning.KillAdvantageRatio = 1.1
	g.BotTuning = &tuning

	tunedCmd, _, err := v2.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("tuned NextCommand: %v", err)
	}
	if tunedCmd.Territory != "Brazil" || tunedCmd.KillTarget != g.Players[2].ID {
		t.Fatalf("expected a 1.1x snapshot to commit toward Brazil (25 armies > 1.1*20=22), got %+v", tunedCmd)
	}
}

// TestKillbotV2ShouldGoHogWildFiresInTwoPlayerStandoff confirms the
// endgame stalemate-breaker (point 8): pi's army share here is deliberately
// modest -- not a majority, and nowhere near the flat per-territory cap --
// so neither of the other two hogwild triggers can explain a true result;
// only the "2 players remain" check can.
func TestKillbotV2ShouldGoHogWildFiresInTwoPlayerStandoff(t *testing.T) {
	g, _ := newTestGame(t)
	pi := 0
	g.Players[2].Eliminated = true // only pi and player 1 remain
	g.Territories["Alaska"] = risk.TerritoryState{Owner: pi, Armies: 10}

	if !killbotV2ShouldGoHogWild(g, pi, killbotV2HogWildArmiesPerTerritory) {
		t.Fatalf("expected hogwild once only 2 players remain, regardless of army share")
	}
}

// TestKillbotV2ShouldGoHogWildDoesNotFireWithThreePlayersAlive is the
// contrast case: the identical army distribution, but all 3 players are
// still alive, confirming the new trigger doesn't fire prematurely in an
// ordinary mid-game state.
func TestKillbotV2ShouldGoHogWildDoesNotFireWithThreePlayersAlive(t *testing.T) {
	g, _ := newTestGame(t)
	pi := 0
	g.Territories["Alaska"] = risk.TerritoryState{Owner: pi, Armies: 10}

	if killbotV2ShouldGoHogWild(g, pi, killbotV2HogWildArmiesPerTerritory) {
		t.Fatalf("expected no hogwild with 3 players still alive and a modest, non-majority army share")
	}
}

// TestKillbotV2EndgameHogWildAttacksEvenWithoutWinProbabilityFavor is the
// decisive end-to-end check: not just that killbotV2ShouldGoHogWild
// returns true in a 2-player standoff, but that attack() actually reaches
// and fires the resulting attack -- with no kill committed (the common
// case in a genuinely close standoff, per the doc comment above) and a
// 2-vs-1 matchup that risk.ForecastAttack rates well under 50% (a single
// do-or-die die roll, favoring the defender on ties: ~41.7%). Before this
// fix, attack() only reached killbotV2ShouldGoHogWild inside the
// kill-committed branch, and even then passed the backer's
// win-probability comparator into attackAsMuchAsPossible -- either gap
// alone would have made this attack unreachable.
func TestKillbotV2EndgameHogWildAttacksEvenWithoutWinProbabilityFavor(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	pi := 0
	g.Players[2].Eliminated = true // only pi and player 1 remain

	g.Territories["Alaska"] = risk.TerritoryState{Owner: pi, Armies: 2}
	g.Territories["Northwest Territory"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Alberta"] = risk.TerritoryState{Owner: pi, Armies: 1}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: 1, Armies: 1}

	if !killbotV2ShouldGoHogWild(g, pi, killbotV2HogWildArmiesPerTerritory) {
		t.Fatalf("expected hogwild in this 2-player standoff")
	}
	forecast := risk.ForecastAttack(2, 1)
	if forecast.WinProbability >= killbotV2MinAttackWinProbability {
		t.Fatalf("test setup invalid: expected a 2v1 attack to be win-probability-unfavored, got %.3f", forecast.WinProbability)
	}

	v2 := NewKillbotV2Strategy()
	cmd, _, err := v2.NextCommand(context.Background(), g, p0)
	if err != nil {
		t.Fatalf("NextCommand: %v", err)
	}
	if cmd.Action != ActionAttack || cmd.From != "Alaska" || cmd.To != "Kamchatka" {
		t.Fatalf("expected the endgame hogwild attack Alaska->Kamchatka despite unfavorable win probability, got %+v", cmd)
	}
}
