package service

import (
	"backend/internal/db"
	"backend/internal/game"
	"backend/internal/store"
	"context"
)

type gameChatDB interface {
	Queryer() db.Querier
}

type gameChatStore interface {
	SaveGameMessage(ctx context.Context, q db.Querier, gameID, senderClientID, senderName, body string) (store.GameChatMessage, error)
	ListGameMessages(ctx context.Context, q db.Querier, gameID string, limit int) ([]store.GameChatMessage, error)
}

// GameChatService persists and retrieves per-game chat messages, backed by
// a gameChatStore.
type GameChatService struct {
	db    gameChatDB
	store gameChatStore
}

// NewGameChatService constructs a GameChatService backed by the given
// database and game chat store.
func NewGameChatService(db gameChatDB, store gameChatStore) *GameChatService {
	return &GameChatService{db: db, store: store}
}

// SaveGameMessage persists a chat message for the given gameID from a
// client identified by senderClientID and displayed as senderName, and
// returns it projected into a game.GameChatLogMessage.
func (s *GameChatService) SaveGameMessage(ctx context.Context, gameID, senderClientID, senderName, body string) (game.GameChatLogMessage, error) {
	out, err := s.store.SaveGameMessage(ctx, s.db.Queryer(), gameID, senderClientID, senderName, body)
	if err != nil {
		return game.GameChatLogMessage{}, err
	}
	return game.GameChatLogMessage{
		GameID:    out.GameID,
		UserName:  out.SenderName,
		Body:      out.Body,
		CreatedAt: out.CreatedAt,
	}, nil
}

// ListGameMessages returns up to limit of the given game's chat messages,
// projected into game.GameChatLogMessage, in the order returned by the
// underlying store.
func (s *GameChatService) ListGameMessages(ctx context.Context, gameID string, limit int) ([]game.GameChatLogMessage, error) {
	out, err := s.store.ListGameMessages(ctx, s.db.Queryer(), gameID, limit)
	if err != nil {
		return nil, err
	}
	messages := make([]game.GameChatLogMessage, 0, len(out))
	for _, m := range out {
		messages = append(messages, game.GameChatLogMessage{
			GameID:    m.GameID,
			UserName:  m.SenderName,
			Body:      m.Body,
			CreatedAt: m.CreatedAt,
		})
	}
	return messages, nil
}
