package store

import (
	"backend/internal/db"
	"context"
	"database/sql"
	"time"
)

type GameEvent struct {
	ID          string
	GameID      string
	ActorUserID string
	EventType   string
	Body        string
	CreatedAt   time.Time
}

type PostgresGameEventStore struct{}

func NewPostgresGameEventStore() *PostgresGameEventStore { return &PostgresGameEventStore{} }

func (s *PostgresGameEventStore) SaveGameEvent(ctx context.Context, q db.Querier, gameID, actorUserID, eventType, body string) (GameEvent, error) {
	const stmt = `
		INSERT INTO game_events (game_id, actor_user_id, event_type, body)
		VALUES ($1::uuid, NULLIF($2, '')::uuid, $3, $4)
		RETURNING id::text, game_id::text, COALESCE(actor_user_id::text, ''), event_type, body, created_at
	`
	var out GameEvent
	err := q.QueryRow(ctx, stmt, gameID, actorUserID, eventType, body).Scan(
		&out.ID,
		&out.GameID,
		&out.ActorUserID,
		&out.EventType,
		&out.Body,
		&out.CreatedAt,
	)
	return out, err
}

func (s *PostgresGameEventStore) ListGameEvents(ctx context.Context, q db.Querier, gameID string, limit int) ([]GameEvent, error) {
	const stmt = `
		SELECT id::text, game_id::text, actor_user_id::text, event_type, body, created_at
		FROM game_events
		WHERE game_id = $1::uuid
		ORDER BY created_at DESC
		LIMIT $2
	`
	rows, err := q.Query(ctx, stmt, gameID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]GameEvent, 0, limit)
	for rows.Next() {
		var ev GameEvent
		var actor sql.NullString
		if err := rows.Scan(&ev.ID, &ev.GameID, &actor, &ev.EventType, &ev.Body, &ev.CreatedAt); err != nil {
			return nil, err
		}
		if actor.Valid {
			ev.ActorUserID = actor.String
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}
