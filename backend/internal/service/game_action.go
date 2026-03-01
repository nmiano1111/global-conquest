package service

import (
	"backend/internal/game"
	"context"
)

type GameActionService struct {
	games *GamesService
}

func NewGameActionService(games *GamesService) *GameActionService {
	return &GameActionService{games: games}
}

func (s *GameActionService) ApplyGameAction(ctx context.Context, in game.GameActionInput) (game.GameActionUpdate, error) {
	out, err := s.games.ApplyGameAction(ctx, GameActionInput{
		GameID:       in.GameID,
		PlayerUserID: in.PlayerUserID,
		Action:       in.Action,
		Territory:    in.Territory,
		From:         in.From,
		To:           in.To,
		Armies:       in.Armies,
		AttackerDice: in.AttackerDice,
		DefenderDice: in.DefenderDice,
	})
	if err != nil {
		return game.GameActionUpdate{}, err
	}

	players := make([]game.GameActionPlayer, 0, len(out.Players))
	for _, p := range out.Players {
		players = append(players, game.GameActionPlayer{
			UserID:     p.UserID,
			CardCount:  p.CardCount,
			Eliminated: p.Eliminated,
		})
	}
	return game.GameActionUpdate{
		GameID:                out.GameID,
		Action:                out.Action,
		ActorUserID:           out.ActorUserID,
		Phase:                 out.Phase,
		CurrentPlayer:         out.CurrentPlayer,
		PendingReinforcements: out.PendingReinforcements,
		Players:               players,
		Territories:           out.Territories,
		Result:                out.Result,
	}, nil
}
