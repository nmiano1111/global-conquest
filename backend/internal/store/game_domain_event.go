package store

import (
	"backend/internal/db"
	"backend/internal/risk"
	"context"
	"encoding/json"
	"time"
)

type GameDomainEvent struct {
	ID            string
	GameID        string
	GameSequence  int64
	EventType     string
	EventVersion  int16
	ActorPlayerID string
	OccurredAt    time.Time
	Payload       json.RawMessage
}

type PostgresGameDomainEventStore struct{}

func NewPostgresGameDomainEventStore() *PostgresGameDomainEventStore {
	return &PostgresGameDomainEventStore{}
}

// InsertDomainEvent atomically increments games.event_sequence and inserts the event row.
// The game row must already be locked (via SELECT FOR UPDATE) in the surrounding transaction.
func (s *PostgresGameDomainEventStore) InsertDomainEvent(
	ctx context.Context,
	q db.Querier,
	gameID string,
	ev risk.DomainEvent,
	payload []byte,
) (GameDomainEvent, error) {
	const stmt = `
		WITH seq AS (
			UPDATE games
			SET event_sequence = event_sequence + 1
			WHERE id = $1::uuid
			RETURNING event_sequence
		)
		INSERT INTO game_domain_events
			(game_id, game_sequence, event_type, event_version, actor_player_id, payload)
		SELECT $1::uuid, seq.event_sequence, $2, $3, NULLIF($4, '')::uuid, $5::jsonb
		FROM seq
		RETURNING
			id::text,
			game_id::text,
			game_sequence,
			event_type,
			event_version,
			COALESCE(actor_player_id::text, ''),
			occurred_at,
			payload
	`
	var out GameDomainEvent
	err := q.QueryRow(ctx, stmt, gameID, ev.Type, ev.Version, ev.ActorPlayerID, payload).Scan(
		&out.ID,
		&out.GameID,
		&out.GameSequence,
		&out.EventType,
		&out.EventVersion,
		&out.ActorPlayerID,
		&out.OccurredAt,
		&out.Payload,
	)
	return out, err
}
