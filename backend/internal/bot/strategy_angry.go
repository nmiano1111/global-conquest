package bot

import (
	"context"
	"fmt"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// StrategyAngryV1 is the registry identifier for AngryStrategy, the first
// of several strategies ported from Lux Delux's built-in AIs (see
// project-docs/bot_player/proposals/Lux_Delux_AI_Research_Notes.md and
// Lux_Port_Notes.md). It has no continent or cluster awareness at all --
// every decision is a purely local, greedy comparison over a territory's
// immediate neighbors.
const StrategyAngryV1 = "angry-v1"

// AngryStrategy implements StrategyAngryV1.
type AngryStrategy struct{}

// NewAngryStrategy creates an AngryStrategy.
func NewAngryStrategy() *AngryStrategy { return &AngryStrategy{} }

// NextCommand dispatches on g.Phase, mirroring BasicStrategy/ValueStrategy's
// shape. It always returns a zero-value Explanation, since Angry has no
// scoring model to report.
func (a *AngryStrategy) NextCommand(_ context.Context, g *risk.Game, playerID string) (Command, Explanation, error) {
	cmd, err := a.nextCommand(g, playerID)
	return cmd, Explanation{}, err
}

func (a *AngryStrategy) nextCommand(g *risk.Game, playerID string) (Command, error) {
	switch g.Phase {
	case risk.PhaseSetupReinforce:
		return a.setupReinforce(g, playerID)
	case risk.PhaseReinforce:
		return a.reinforce(g, playerID)
	case risk.PhaseAttack:
		return a.attack(g, playerID)
	case risk.PhaseOccupy:
		return a.occupy(g, playerID)
	case risk.PhaseFortify:
		return a.fortify(g, playerID)
	default:
		return Command{}, fmt.Errorf("bot: %s has no move for phase %q", StrategyAngryV1, g.Phase)
	}
}

// setupReinforce places the one initial army on the owned territory with
// the most enemy-owned neighbor territories, tie-broken by canonical board
// order -- Lux's Angry.placeInitialArmies delegates straight to
// placeArmies with the same criterion.
func (a *AngryStrategy) setupReinforce(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalSetupReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal setup reinforcement for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	best := bestByEnemyNeighborCount(g, pi, actions, func(a risk.SetupReinforcementAction) risk.Territory { return a.Territory })
	return Command{Action: ActionPlaceInitialArmy, Territory: string(best)}, nil
}

// reinforce only trades cards when a set is mandatory (Angry's Lux
// cardsPhase is empty -- it never voluntarily cashes, unlike
// BasicStrategy/ScoredStrategy/ValueStrategy, which trade whenever legal),
// then dumps every pending reinforcement onto the single owned territory
// with the most enemy-owned neighbor territories -- Lux's Angry.placeArmies
// places everything in one board.placeArmies call, never batching.
func (a *AngryStrategy) reinforce(g *risk.Game, playerID string) (Command, error) {
	if risk.CardTurnInRequired(g, playerID) {
		// risk.LegalCardTurnIns already enumerates in ascending index order,
		// so sets[0] is the deterministic lowest-index candidate.
		if sets := risk.LegalCardTurnIns(g, playerID); len(sets) > 0 {
			return Command{Action: ActionTradeCards, CardIndices: sets[0].Indices}, nil
		}
	}

	actions := risk.LegalReinforcements(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal reinforcement for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	best := bestByEnemyNeighborCount(g, pi, actions, func(a risk.ReinforcementAction) risk.Territory { return a.Territory })
	return Command{Action: ActionPlaceReinforcement, Territory: string(best), Armies: g.PendingReinforcements}, nil
}

// attack scans owned territories in canonical board order for the first
// one that outnumbers its weakest enemy neighbor at all (a bare one-army
// edge, no odds-awareness), attacking there with maximum dice -- Lux's
// Angry.attackPhase() attacks every such matchup it finds each pass; since
// this engine resolves one round per Command and the bot runner calls back
// in with freshly reloaded state, returning the first qualifying pair each
// call reproduces the same "keep attacking until nothing qualifies"
// behavior without needing an explicit loop.
func (a *AngryStrategy) attack(g *risk.Game, playerID string) (Command, error) {
	pi := playerIndex(g, playerID)
	for _, from := range g.Board.Order {
		src := g.Territories[from]
		if src.Owner != pi || src.Armies <= 1 {
			continue
		}
		weakest, ok := weakestEnemyNeighbor(g, from, pi)
		if !ok {
			continue
		}
		if src.Armies > g.Territories[weakest].Armies {
			return Command{
				Action:       ActionAttack,
				From:         string(from),
				To:           string(weakest),
				AttackerDice: min(3, src.Armies-1),
			}, nil
		}
	}
	return Command{Action: ActionEndAttack}, nil
}

// occupy leaves the conquering territory well-defended when it still faces
// more enemy neighbors than the newly conquered one (moving only the legal
// minimum), otherwise sends the maximum forward -- Lux's
// Angry.moveArmiesIn compares getNumberEnemyNeighbors() the same way.
func (a *AngryStrategy) occupy(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalOccupations(g, playerID)
	if len(actions) == 0 {
		return Command{}, fmt.Errorf("bot: no legal occupation for player %s", playerID)
	}
	pi := playerIndex(g, playerID)
	fromEnemies := enemyNeighborCount(g, g.Occupy.From, pi)
	toEnemies := enemyNeighborCount(g, g.Occupy.To, pi)

	armies := actions[len(actions)-1].Armies // maximum
	if fromEnemies > toEnemies {
		armies = actions[0].Armies // minimum
	}
	return Command{Action: ActionOccupy, Armies: armies}, nil
}

// fortify moves the maximum legal amount toward whichever legal
// destination has the most enemy-owned neighbor territories, tie-broken by
// canonical board order; if the best candidate faces no enemy at all, it
// ends the turn instead of a pointless move -- Lux's Angry.fortifyPhase
// only ever redeploys toward the front (falling back to a random neighbor
// when nothing does, which this port replaces with the same deterministic
// tie-break every other strategy in this codebase uses, to keep
// --seed-start reproducible).
func (a *AngryStrategy) fortify(g *risk.Game, playerID string) (Command, error) {
	actions := risk.LegalFortifications(g, playerID)
	if len(actions) == 0 {
		return Command{Action: ActionEndTurn}, nil
	}
	pi := playerIndex(g, playerID)
	order := orderIndex(g)

	best := actions[0]
	bestScore := enemyNeighborCount(g, best.To, pi)
	for _, cand := range actions[1:] {
		score := enemyNeighborCount(g, cand.To, pi)
		if score > bestScore ||
			(score == bestScore && (order[cand.From] < order[best.From] ||
				(order[cand.From] == order[best.From] && order[cand.To] < order[best.To]))) {
			best, bestScore = cand, score
		}
	}

	if bestScore == 0 {
		return Command{Action: ActionEndTurn}, nil
	}
	return Command{Action: ActionFortify, From: string(best.From), To: string(best.To), Armies: best.MaxArmies}, nil
}

// bestByEnemyNeighborCount picks the action from actions whose territory
// (via get) has the highest enemyNeighborCount, tie-broken by canonical
// board order.
func bestByEnemyNeighborCount[T any](g *risk.Game, pi int, actions []T, get func(T) risk.Territory) risk.Territory {
	order := orderIndex(g)
	best := get(actions[0])
	bestScore := enemyNeighborCount(g, best, pi)
	for _, a := range actions[1:] {
		cand := get(a)
		score := enemyNeighborCount(g, cand, pi)
		if score > bestScore || (score == bestScore && order[cand] < order[best]) {
			best, bestScore = cand, score
		}
	}
	return best
}
