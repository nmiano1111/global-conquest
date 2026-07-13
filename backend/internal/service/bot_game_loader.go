package service

import (
	"context"
	"encoding/json"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// BotGameLoader adapts GamesService to bot.GameLoader, so the bot runner
// can read authoritative state without depending on the service or store
// packages directly.
type BotGameLoader struct {
	games *GamesService
}

// NewBotGameLoader constructs a BotGameLoader backed by the given
// GamesService, from which it loads authoritative game state.
func NewBotGameLoader(games *GamesService) *BotGameLoader {
	return &BotGameLoader{games: games}
}

// LoadGame loads the game identified by gameID via GamesService.GetGame,
// unmarshals its persisted JSONB state into a risk.Game, and returns it
// alongside the game's store status ("lobby", "in_progress", or
// "completed"). It returns an error if the game cannot be found or its
// state fails to unmarshal.
func (l *BotGameLoader) LoadGame(ctx context.Context, gameID string) (*risk.Game, string, error) {
	g, err := l.games.GetGame(ctx, gameID)
	if err != nil {
		return nil, "", err
	}
	var engine risk.Game
	if err := json.Unmarshal(g.State, &engine); err != nil {
		return nil, "", err
	}
	return &engine, g.Status, nil
}
