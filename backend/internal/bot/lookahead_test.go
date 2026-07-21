package bot

import (
	"math"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// sumValueFunction scores a feature vector as its plain sum -- a linear
// function, chosen so that blendFeatures' per-feature blend and a
// post-hoc blend of two already-computed Score results are mathematically
// identical, letting these tests assert exact relationships instead of
// approximate ones.
type sumValueFunction struct{}

func (sumValueFunction) Score(features []float64) float64 {
	var sum float64
	for _, f := range features {
		sum += f
	}
	return sum
}
func (sumValueFunction) AttackMargin() float64  { return 0 }
func (sumValueFunction) FortifyMargin() float64 { return 0 }

func TestBestOpponentReplyFalseWhenDefenderHasNoLegalAttack(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	pi := playerIndex(g, p0)
	defenderIdx := 1

	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: pi, Armies: 5}
	}
	// The defender's only territory has exactly 1 army -- LegalAttacks
	// requires more than 1 to attack with.
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: defenderIdx, Armies: 1}

	if _, ok := bestOpponentReply(g, defenderIdx, sumValueFunction{}); ok {
		t.Fatalf("expected no legal reply for a defender owning only a 1-army territory")
	}
}

func TestBestOpponentReplyFindsLegalCounterAttack(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	pi := playerIndex(g, p0)
	defenderIdx := 1

	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: pi, Armies: 5}
	}
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: defenderIdx, Armies: 1}
	// Yakutsk is adjacent to Kamchatka and owned by pi -- the defender's
	// only legal attack.
	g.Territories["Yakutsk"] = risk.TerritoryState{Owner: defenderIdx, Armies: 5}

	reply, ok := bestOpponentReply(g, defenderIdx, sumValueFunction{})
	if !ok {
		t.Fatalf("expected the defender to have a legal counter-attack from Yakutsk")
	}
	if reply.From != "Yakutsk" {
		t.Errorf("expected the reply to originate from Yakutsk, got %s", reply.From)
	}
}

func TestLookaheadAttackScoreMatchesPlainBlendWhenNoReplyExists(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	pi := playerIndex(g, p0)
	defenderIdx := 1

	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: pi, Armies: 5}
	}
	// The defender owns nothing but the 1-army target -- no legal reply
	// exists from either the conquered or the held branch, so
	// lookaheadAttackScore should reduce to exactly the plain 0-ply blend.
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: defenderIdx, Armies: 1}
	a := risk.AttackAction{From: "Alaska", To: "Kamchatka", SourceArmies: 5, TargetArmies: 1, MaxAttackerDice: 3}
	g.Territories["Alaska"] = risk.TerritoryState{Owner: pi, Armies: 5}

	value := sumValueFunction{}
	plain := value.Score(attackAfterstateBlend(g, pi, a))
	looked := lookaheadAttackScore(g, pi, a, value)

	if math.Abs(plain-looked) > 1e-9 {
		t.Errorf("expected lookaheadAttackScore to match the plain blend when no reply exists, plain=%v lookahead=%v", plain, looked)
	}
}

func TestLookaheadAttackScoreDivergesWhenReplyExists(t *testing.T) {
	g, p0 := newTestGame(t)
	g.Phase = risk.PhaseAttack
	pi := playerIndex(g, p0)
	defenderIdx := 1

	for _, terr := range g.Board.Order {
		g.Territories[terr] = risk.TerritoryState{Owner: pi, Armies: 5}
	}
	// A hugely favorable attack, so the conquered branch dominates the
	// blend. The defender keeps Yakutsk (adjacent to Kamchatka), giving
	// them a legal counter-attack back into the territory just conquered.
	g.Territories["Kamchatka"] = risk.TerritoryState{Owner: defenderIdx, Armies: 1}
	g.Territories["Yakutsk"] = risk.TerritoryState{Owner: defenderIdx, Armies: 20}
	a := risk.AttackAction{From: "Alaska", To: "Kamchatka", SourceArmies: 30, TargetArmies: 1, MaxAttackerDice: 3}
	g.Territories["Alaska"] = risk.TerritoryState{Owner: pi, Armies: 30}

	forecast := ForecastAttack(a.SourceArmies, a.TargetArmies)
	if forecast.WinProbability < 0.95 {
		t.Fatalf("expected this matchup to be hugely favorable, got WinProbability=%v", forecast.WinProbability)
	}

	value := sumValueFunction{}
	plain := value.Score(attackAfterstateBlend(g, pi, a))
	looked := lookaheadAttackScore(g, pi, a, value)

	if math.Abs(plain-looked) < 1e-6 {
		t.Errorf("expected lookaheadAttackScore to diverge from the plain blend once the defender's counter-attack is rolled forward, plain=%v lookahead=%v", plain, looked)
	}
}
