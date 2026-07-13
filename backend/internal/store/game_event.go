package store

import (
	"github.com/nmiano1111/global-conquest/backend/internal/db"
	"context"
	"database/sql"
	"time"
)

// GameEvent is a free-text, human-readable event logged against a game (e.g. join/leave/chat-adjacent
// notices), distinct from the structured, sequenced GameDomainEvent audit log.
type GameEvent struct {
	// ID is the event's unique identifier.
	ID string
	// GameID is the identifier of the game the event belongs to.
	GameID string
	// ActorUserID is the user who caused the event, or empty if the event has no attributable actor.
	ActorUserID string
	// EventType names the kind of event.
	EventType string
	// Body is the event's human-readable description.
	Body string
	// CreatedAt is when the event was persisted.
	CreatedAt time.Time
}

// PostgresGameEventStore is a Postgres-backed store for the free-text game event log.
type PostgresGameEventStore struct{}

// NewPostgresGameEventStore constructs a PostgresGameEventStore.
func NewPostgresGameEventStore() *PostgresGameEventStore { return &PostgresGameEventStore{} }

// SaveGameEvent inserts a new game event and returns the persisted row, including its generated ID and creation timestamp. ActorUserID may be empty, in which case the row is stored with a NULL actor.
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

// ListGameEvents returns up to limit events for the given game in chronological order (oldest first). The underlying query fetches the most recent events first and this method reverses them before returning.
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
