package store

import (
	"github.com/nmiano1111/global-conquest/backend/internal/db"
	"github.com/nmiano1111/global-conquest/backend/internal/risk"
	"context"
	"encoding/json"
	"time"
)

// GameDomainEvent is a persisted, ordered domain event recorded against a game — the
// append-only audit/event-sourcing log distinct from the free-text GameEvent feed.
type GameDomainEvent struct {
	// ID is the event's unique identifier.
	ID string
	// GameID is the identifier of the game the event belongs to.
	GameID string
	// GameSequence is the monotonically increasing per-game sequence number assigned via
	// games.event_sequence, establishing a total order of domain events within the game.
	GameSequence int64
	// EventType names the kind of domain event (e.g. an engine action or phase transition).
	EventType string
	// EventVersion is the schema version of Payload, letting consumers deserialize older
	// event payload shapes correctly as the schema evolves.
	EventVersion int16
	// ActorPlayerID is the player who caused the event, or empty if the event has no
	// attributable actor (e.g. a system-generated event).
	ActorPlayerID string
	// OccurredAt is when the event was recorded.
	OccurredAt time.Time
	// Payload is the event's type-specific data, stored and returned as raw JSON.
	Payload json.RawMessage
}

// PostgresGameDomainEventStore is a Postgres-backed store for the append-only game domain event log.
type PostgresGameDomainEventStore struct{}

// NewPostgresGameDomainEventStore constructs a PostgresGameDomainEventStore.
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
