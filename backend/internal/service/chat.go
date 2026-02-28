package service

import (
	"backend/internal/db"
	"backend/internal/store"
	"context"
	"errors"
	"strings"
)

const lobbyChatRoom = "lobby"

var ErrInvalidChatInput = errors.New("invalid chat input")

type chatDB interface {
	Queryer() db.Querier
}

type ChatService struct {
	db   chatDB
	chat store.ChatStore
}

func NewChatService(db chatDB, chat store.ChatStore) *ChatService {
	return &ChatService{db: db, chat: chat}
}

func (s *ChatService) ListLobbyMessages(ctx context.Context, limit int) ([]store.ChatMessage, error) {
	if limit < 0 {
		return nil, ErrInvalidChatInput
	}
	if limit == 0 {
		limit = 50
	}
	return s.chat.ListMessages(ctx, s.db.Queryer(), lobbyChatRoom, limit)
}

func (s *ChatService) PostLobbyMessage(ctx context.Context, userID, body string) (store.ChatMessage, error) {
	userID = strings.TrimSpace(userID)
	body = strings.TrimSpace(body)
	if userID == "" || body == "" || len(body) > 1000 {
		return store.ChatMessage{}, ErrInvalidChatInput
	}

	return s.chat.CreateMessage(ctx, s.db.Queryer(), store.NewChatMessage{
		Room:   lobbyChatRoom,
		UserID: userID,
		Body:   body,
	})
}
