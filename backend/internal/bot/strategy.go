package bot

import (
	"context"

	"backend/internal/risk"
)

// Strategy chooses the next command for a bot-controlled player. A Strategy
// must not mutate game, write to Postgres, send WebSocket messages, or call
// Discord — it only inspects the authoritative state it is given and
// returns the same kind of command a human would submit. The engine
// remains solely responsible for legality. The returned Explanation records
// why that command won over its legal alternatives; a strategy with no
// scoring model (basic-v1) may return a zero-value Explanation.
type Strategy interface {
	NextCommand(ctx context.Context, game *risk.Game, playerID string) (Command, Explanation, error)
}

// StrategyRegistry looks strategies up by their PlayerState.Strategy
// identifier. An empty identifier resolves to StrategyBasicV1, the only
// strategy this milestone ships.
type StrategyRegistry map[string]Strategy

// Get resolves a strategy identifier, defaulting empty to basic-v1.
func (r StrategyRegistry) Get(name string) (Strategy, bool) {
	if name == "" {
		name = StrategyBasicV1
	}
	s, ok := r[name]
	return s, ok
}
