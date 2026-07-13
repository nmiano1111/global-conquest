package store

import (
	"backend/internal/db"
	"context"
	"time"
)

// ChatMessage is a single chat message row, joined with the sender's username.
type ChatMessage struct {
	// ID is the message's UUID.
	ID string
	// Room is the chat room the message belongs to (e.g. a lobby or game room key).
	Room string
	// UserID is the UUID of the user who sent the message.
	UserID string
	// UserName is the username of the sender, joined from the users table.
	UserName string
	// Body is the message text.
	Body string
	// CreatedAt is when the message was inserted.
	CreatedAt time.Time
}

// NewChatMessage is the input for creating a chat message via CreateMessage.
type NewChatMessage struct {
	// Room is the chat room the message should be posted to.
	Room string
	// UserID is the UUID of the sending user.
	UserID string
	// Body is the message text.
	Body string
}

// ChatStore defines the persistence operations for chat messages.
type ChatStore interface {
	CreateMessage(ctx context.Context, q db.Querier, in NewChatMessage) (ChatMessage, error)
	ListMessages(ctx context.Context, q db.Querier, room string, limit int) ([]ChatMessage, error)
}

// PostgresChatStore is the Postgres-backed implementation of ChatStore.
type PostgresChatStore struct{}

// NewPostgresChatStore constructs a PostgresChatStore.
func NewPostgresChatStore() *PostgresChatStore { return &PostgresChatStore{} }

// CreateMessage inserts a new chat message row and returns it joined with the
// sender's username. Returns an error if the insert or the users join fails
// (e.g. the user does not exist).
func (s *PostgresChatStore) CreateMessage(ctx context.Context, exec db.Querier, in NewChatMessage) (ChatMessage, error) {
	const stmt = `
		WITH inserted AS (
			INSERT INTO chat_messages (room, user_id, body)
			VALUES ($1, $2::uuid, $3)
			RETURNING id, room, user_id, body, created_at
		)
		SELECT inserted.id::text, inserted.room, inserted.user_id::text, u.username, inserted.body, inserted.created_at
		FROM inserted
		JOIN users u ON u.id = inserted.user_id
	`
	var out ChatMessage
	err := exec.QueryRow(ctx, stmt, in.Room, in.UserID, in.Body).Scan(
		&out.ID, &out.Room, &out.UserID, &out.UserName, &out.Body, &out.CreatedAt,
	)
	return out, err
}

// ListMessages returns up to limit messages for room, oldest-first. limit is
// clamped to a default of 50 when <= 0 and a maximum of 200. Internally the
// query fetches newest-first for efficient index usage, then reverses the
// slice before returning it for UI display.
func (s *PostgresChatStore) ListMessages(ctx context.Context, exec db.Querier, room string, limit int) ([]ChatMessage, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	const stmt = `
		SELECT m.id::text, m.room, m.user_id::text, u.username, m.body, m.created_at
		FROM chat_messages m
		JOIN users u ON u.id = m.user_id
		WHERE m.room = $1
		ORDER BY m.created_at DESC
		LIMIT $2
	`
	rows, err := exec.Query(ctx, stmt, room, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ChatMessage, 0, limit)
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(&m.ID, &m.Room, &m.UserID, &m.UserName, &m.Body, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// We fetched newest-first for efficient index usage; return oldest-first for UI display.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}
