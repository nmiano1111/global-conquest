package service

import (
	"github.com/nmiano1111/global-conquest/backend/internal/db"
	"github.com/nmiano1111/global-conquest/backend/internal/store"
	"context"
	"errors"
	"strings"
)

const lobbyChatRoom = "lobby"

// ErrInvalidChatInput is returned when chat input fails validation: a
// negative message limit, an empty user ID or body after trimming, or a
// body exceeding the maximum allowed length.
var ErrInvalidChatInput = errors.New("invalid chat input")

type chatDB interface {
	Queryer() db.Querier
}

// ChatService manages the single global lobby chat room, backed by a
// store.ChatStore.
type ChatService struct {
	db   chatDB
	chat store.ChatStore
}

// NewChatService constructs a ChatService backed by the given database and
// chat store.
func NewChatService(db chatDB, chat store.ChatStore) *ChatService {
	return &ChatService{db: db, chat: chat}
}

// ListLobbyMessages returns up to limit of the most recent messages posted
// to the lobby chat room. A limit of 0 defaults to 50 messages; a negative
// limit returns ErrInvalidChatInput.
func (s *ChatService) ListLobbyMessages(ctx context.Context, limit int) ([]store.ChatMessage, error) {
	if limit < 0 {
		return nil, ErrInvalidChatInput
	}
	if limit == 0 {
		limit = 50
	}
	return s.chat.ListMessages(ctx, s.db.Queryer(), lobbyChatRoom, limit)
}

// PostLobbyMessage saves a chat message from userID to the lobby chat room,
// after trimming leading/trailing whitespace from both userID and body. It
// returns ErrInvalidChatInput if either is empty after trimming or if body
// exceeds 1000 characters.
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
