package service

import (
	"context"
	"encoding/json"

	"backend/internal/risk"
)

// BotGameLoader adapts GamesService to bot.GameLoader, so the bot runner
// can read authoritative state without depending on the service or store
// packages directly.
type BotGameLoader struct {
	games *GamesService
}

func NewBotGameLoader(games *GamesService) *BotGameLoader {
	return &BotGameLoader{games: games}
}

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
