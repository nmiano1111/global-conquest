package bot

import (
	"context"
	"fmt"
	"sort"

	"backend/internal/risk"
)

// StrategyBasicV1 is the only strategy identifier this milestone supports.
// It plays legal, complete games without pursuing strong play: no continent
// scoring, no elimination targeting, no card-timing optimization. Those are
// explicitly deferred to later strategy versions.
const StrategyBasicV1 = "basic-v1"

// BasicStrategy implements StrategyBasicV1.
type BasicStrategy struct{}

func NewBasicStrategy() *BasicStrategy { return &BasicStrategy{} }

func (b *BasicStrategy) NextCommand(_ context.Context, g *risk.Game, playerID string) (Command, Explanation, error) {
	cmd, err := b.nextCommand(g, playerID)
	return cmd, Explanation{}, err
}

// nextCommand is basic-v1's original rule-based dispatch, unchanged from
// before Explanation existed: it has no scoring model to report, so
// NextCommand wraps this in a zero-value Explanation rather than retrofit
// one.
func (b *BasicStrategy) nextCommand(g *risk.Game, playerID string) (Command, error) {
	switch g.Phase {
	case risk.PhaseSetupReinforce:
		return b.setupReinforce(g, playerID)
	case risk.PhaseReinforce:
		return b.reinforce(g, playerID)
	case risk.PhaseAttack:
		return b.attack(g, playerID)
	case risk.PhaseOccupy:
		return b.occupy(g, playerID)
	case risk.PhaseFortify:
		return b.fortify(g, playerID)
	default:
		return Command{}, fmt.Errorf("bot: %s has no move for phase %q", StrategyBasicV1, g.Phase)
	}
}

// setupReinforce prefers the owned territory adjacent to the largest total
// of enemy armies, breaking ties by canonical board order.
func (b *BasicStrategy) setupReinforce(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalSetupReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal setup reinforcement for player %s", playerID)
	}
	order := orderIndex(g)
	pi := playerIndex(g, playerID)
	best := actions[0].Territory
	bestScore := adjacentEnemyArmies(g, best, pi)
	for _, a := range actions[1:] {
		score := adjacentEnemyArmies(g, a.Territory, pi)
		if score > bestScore || (score == bestScore && order[a.Territory] < order[best]) {
			best, bestScore = a.Territory, score
		}
	}
	return Command{Action: ActionPlaceInitialArmy, Territory: string(best)}, nil
}

// reinforce handles card turn-in (per the basic policy) before placing
// reinforcements, and prefers the border territory facing the largest
// enemy threat relative to its own defense.
func (b *BasicStrategy) reinforce(g *risk.Game, playerID string) (Command, error) {
	if cmd, ok := b.cardTurnIn(g, playerID); ok {
		return cmd, nil
	}

	actions := risk.LegalReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal reinforcement for player %s", playerID)
	}
	order := orderIndex(g)
	pi := playerIndex(g, playerID)

	type scored struct {
		t        risk.Territory
		enemy    int
		weakness int
	}
	cands := make([]scored, 0, len(actions))
	for _, a := range actions {
		enemy := adjacentEnemyArmies(g, a.Territory, pi)
		own := g.Territories[a.Territory].Armies
		cands = append(cands, scored{t: a.Territory, enemy: enemy, weakness: enemy - own})
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].enemy != cands[j].enemy {
			return cands[i].enemy > cands[j].enemy
		}
		if cands[i].weakness != cands[j].weakness {
			return cands[i].weakness > cands[j].weakness
		}
		return order[cands[i].t] < order[cands[j].t]
	})

	return Command{
		Action:    ActionPlaceReinforcement,
		Territory: string(cands[0].t),
		Armies:    g.PendingReinforcements,
	}, nil
}

// cardTurnIn applies the Phase 1 policy: trade in a legal set whenever one
// exists (mandatory or not), choosing deterministically among ties by
// ascending card index.
func (b *BasicStrategy) cardTurnIn(g *risk.Game, playerID string) (Command, bool) {
	sets := risk.LegalCardTurnIns(g, playerID)
	if len(sets) == 0 {
		return Command{}, false
	}
	best := sets[0]
	for _, s := range sets[1:] {
		if s.Indices[0] < best.Indices[0] ||
			(s.Indices[0] == best.Indices[0] && s.Indices[1] < best.Indices[1]) ||
			(s.Indices[0] == best.Indices[0] && s.Indices[1] == best.Indices[1] && s.Indices[2] < best.Indices[2]) {
			best = s
		}
	}
	return Command{Action: ActionTradeCards, CardIndices: best.Indices}, true
}

// attack only engages when source armies exceed the target by at least 2,
// preferring the weakest target with the largest advantage, using the
// maximum legal number of attacker dice.
func (b *BasicStrategy) attack(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalAttacks(g, playerID)
	order := orderIndex(g)

	var eligible []risk.AttackAction
	for _, a := range actions {
		if a.SourceArmies >= a.TargetArmies+2 {
			eligible = append(eligible, a)
		}
	}
	if len(eligible) == 0 {
		return Command{Action: ActionEndAttack}, nil
	}
	sort.Slice(eligible, func(i, j int) bool {
		ei, ej := eligible[i], eligible[j]
		if ei.TargetArmies != ej.TargetArmies {
			return ei.TargetArmies < ej.TargetArmies
		}
		adi := ei.SourceArmies - ei.TargetArmies
		adj := ej.SourceArmies - ej.TargetArmies
		if adi != adj {
			return adi > adj
		}
		if order[ei.From] != order[ej.From] {
			return order[ei.From] < order[ej.From]
		}
		return order[ei.To] < order[ej.To]
	})
	best := eligible[0]
	return Command{
		Action:       ActionAttack,
		From:         string(best.From),
		To:           string(best.To),
		AttackerDice: best.MaxAttackerDice,
	}, nil
}

// occupy always moves the minimum legal amount, keeping momentum on the
// attacking front rather than the newly conquered territory.
func (b *BasicStrategy) occupy(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalOccupations(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal occupation for player %s", playerID)
	}
	return Command{Action: ActionOccupy, Armies: actions[0].Armies}, nil
}

// fortify prefers moving the maximum legal amount from an interior
// territory to whichever legal destination faces the largest enemy threat.
// If no candidate destination actually faces a threat, it ends the turn
// instead of making a pointless move.
func (b *BasicStrategy) fortify(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalFortifications(g, playerID)
	if len(actions) == 0 {
		return Command{Action: ActionEndTurn}, nil
	}
	order := orderIndex(g)
	pi := playerIndex(g, playerID)

	sort.Slice(actions, func(i, j int) bool {
		ai, aj := actions[i], actions[j]
		si := adjacentEnemyArmies(g, ai.To, pi)
		sj := adjacentEnemyArmies(g, aj.To, pi)
		if si != sj {
			return si > sj
		}
		interiorI := adjacentEnemyArmies(g, ai.From, pi) == 0
		interiorJ := adjacentEnemyArmies(g, aj.From, pi) == 0
		if interiorI != interiorJ {
			return interiorI
		}
		if order[ai.From] != order[aj.From] {
			return order[ai.From] < order[aj.From]
		}
		return order[ai.To] < order[aj.To]
	})

	best := actions[0]
	if adjacentEnemyArmies(g, best.To, pi) == 0 {
		return Command{Action: ActionEndTurn}, nil
	}
	return Command{
		Action: ActionFortify,
		From:   string(best.From),
		To:     string(best.To),
		Armies: best.MaxArmies,
	}, nil
}

// orderIndex maps each territory to its position in the board's canonical
// order, used as a stable, deterministic tie-break throughout this file.
func orderIndex(g *risk.Game) map[risk.Territory]int {
	idx := make(map[risk.Territory]int, len(g.Board.Order))
	for i, t := range g.Board.Order {
		idx[t] = i
	}
	return idx
}

func playerIndex(g *risk.Game, playerID string) int {
	for i, p := range g.Players {
		if p.ID == playerID {
			return i
		}
	}
	return -1
}

// adjacentEnemyArmies sums the army counts of every territory adjacent to t
// that is owned by someone other than pi.
func adjacentEnemyArmies(g *risk.Game, t risk.Territory, pi int) int {
	total := 0
	for _, other := range g.Board.Order {
		if other == t || !g.Board.IsAdjacent(t, other) {
			continue
		}
		ts := g.Territories[other]
		if ts.Owner != pi && ts.Owner >= 0 {
			total += ts.Armies
		}
	}
	return total
}
