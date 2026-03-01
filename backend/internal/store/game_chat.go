package store

import (
	"backend/internal/db"
	"context"
	"time"
)

type GameChatMessage struct {
	ID             string
	GameID         string
	SenderClientID string
	SenderName     string
	Body           string
	CreatedAt      time.Time
}

type PostgresGameChatStore struct{}

func NewPostgresGameChatStore() *PostgresGameChatStore { return &PostgresGameChatStore{} }

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
