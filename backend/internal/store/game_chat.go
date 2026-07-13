package store

import (
	"backend/internal/db"
	"context"
	"time"
)

// GameChatMessage is a single in-game chat message persisted for a game.
type GameChatMessage struct {
	// ID is the message's unique identifier.
	ID string
	// GameID is the identifier of the game the message was posted in.
	GameID string
	// SenderClientID is the WebSocket client identifier of the sender.
	SenderClientID string
	// SenderName is the display name of the sender at the time the message was sent.
	SenderName string
	// Body is the message text.
	Body string
	// CreatedAt is when the message was persisted.
	CreatedAt time.Time
}

// PostgresGameChatStore is a Postgres-backed store for in-game chat messages.
type PostgresGameChatStore struct{}

// NewPostgresGameChatStore constructs a PostgresGameChatStore.
func NewPostgresGameChatStore() *PostgresGameChatStore { return &PostgresGameChatStore{} }

// SaveGameMessage inserts a new game chat message and returns the persisted row, including its generated ID and creation timestamp.
func (s *PostgresGameChatStore) SaveGameMessage(ctx context.Context, q db.Querier, gameID, senderClientID, senderName, body string) (GameChatMessage, error) {
	const stmt = `
		INSERT INTO game_chat_messages (game_id, sender_client_id, sender_name, body)
		VALUES ($1::uuid, $2, $3, $4)
		RETURNING id::text, game_id::text, sender_client_id, sender_name, body, created_at
	`
	var out GameChatMessage
	err := q.QueryRow(ctx, stmt, gameID, senderClientID, senderName, body).Scan(
		&out.ID,
		&out.GameID,
		&out.SenderClientID,
		&out.SenderName,
		&out.Body,
		&out.CreatedAt,
	)
	return out, err
}

// ListGameMessages returns up to limit chat messages for the given game in chronological order (oldest first). The underlying query fetches the most recent messages first and this method reverses them before returning.
func (s *PostgresGameChatStore) ListGameMessages(ctx context.Context, q db.Querier, gameID string, limit int) ([]GameChatMessage, error) {
	const stmt = `
		SELECT id::text, game_id::text, sender_client_id, sender_name, body, created_at
		FROM game_chat_messages
		WHERE game_id = $1::uuid
		ORDER BY created_at DESC
		LIMIT $2
	`
	rows, err := q.Query(ctx, stmt, gameID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]GameChatMessage, 0, limit)
	for rows.Next() {
		var msg GameChatMessage
		if err := rows.Scan(&msg.ID, &msg.GameID, &msg.SenderClientID, &msg.SenderName, &msg.Body, &msg.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Query returns newest first; UI expects chronological order.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}
