package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/nmiano1111/global-conquest/backend/internal/db"
	"time"
)

// GameReplayEvent is one persisted, ordered snapshot of a committed game
// action — the same JSON shape already broadcast to clients as
// game_state_updated — used to power player-facing replay. Distinct from
// GameDomainEvent (attacks only, feeds analytics) and GameEvent (free-text
// log line only, no board state).
type GameReplayEvent struct {
	// ID is the event's unique identifier.
	ID string
	// GameID is the identifier of the game the event belongs to.
	GameID string
	// GameSequence is this table's own monotonically increasing per-game
	// sequence number, establishing the total order actions were committed
	// in. It shares the games.event_sequence column with GameDomainEvent
	// but ticks independently, since every action gets a replay row here,
	// unlike GameDomainEvent's attack-only rows — the two tables'
	// sequence numbers are not expected to correlate.
	GameSequence int64
	// OccurredAt is when the action was committed.
	OccurredAt time.Time
	// ActorPlayerID is the player who performed the action, or empty for
	// a system-generated one.
	ActorPlayerID string
	// ActionType names the action kind (e.g. "attack", "fortify",
	// "end_turn"), mirroring GameActionInput.Action.
	ActionType string
	// Payload is the same JSON shape sent to clients as the
	// game_state_updated broadcast for this action, stored verbatim.
	Payload json.RawMessage
}

// PostgresGameReplayEventStore is a Postgres-backed store for the
// append-only game replay event log.
type PostgresGameReplayEventStore struct{}

// NewPostgresGameReplayEventStore constructs a PostgresGameReplayEventStore.
func NewPostgresGameReplayEventStore() *PostgresGameReplayEventStore {
	return &PostgresGameReplayEventStore{}
}

// InsertReplayEvent atomically increments games.event_sequence and inserts
// the replay row. The game row must already be locked (via SELECT FOR
// UPDATE) in the surrounding transaction. actorPlayerID may be empty, in
// which case the row is stored with a NULL actor.
func (s *PostgresGameReplayEventStore) InsertReplayEvent(
	ctx context.Context,
	q db.Querier,
	gameID, actorPlayerID, actionType string,
	payload []byte,
) (GameReplayEvent, error) {
	const stmt = `
		WITH seq AS (
			UPDATE games
			SET event_sequence = event_sequence + 1
			WHERE id = $1::uuid
			RETURNING event_sequence
		)
		INSERT INTO game_replay_events
			(game_id, game_sequence, actor_player_id, action_type, payload)
		SELECT $1::uuid, seq.event_sequence, NULLIF($2, '')::uuid, $3, $4::jsonb
		FROM seq
		RETURNING
			id::text,
			game_id::text,
			game_sequence,
			occurred_at,
			COALESCE(actor_player_id::text, ''),
			action_type,
			payload
	`
	var out GameReplayEvent
	err := q.QueryRow(ctx, stmt, gameID, actorPlayerID, actionType, payload).Scan(
		&out.ID,
		&out.GameID,
		&out.GameSequence,
		&out.OccurredAt,
		&out.ActorPlayerID,
		&out.ActionType,
		&out.Payload,
	)
	return out, err
}

// ListReplayEvents returns up to limit replay events for gameID with a
// game_sequence greater than afterSequence, ordered oldest to newest —
// pass afterSequence 0 to fetch from the start of the game.
func (s *PostgresGameReplayEventStore) ListReplayEvents(
	ctx context.Context,
	q db.Querier,
	gameID string,
	afterSequence int64,
	limit int,
) ([]GameReplayEvent, error) {
	const stmt = `
		SELECT id::text, game_id::text, game_sequence, occurred_at,
		       COALESCE(actor_player_id::text, ''), action_type, payload
		FROM game_replay_events
		WHERE game_id = $1::uuid AND game_sequence > $2
		ORDER BY game_sequence ASC
		LIMIT $3
	`
	rows, err := q.Query(ctx, stmt, gameID, afterSequence, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]GameReplayEvent, 0, limit)
	for rows.Next() {
		var ev GameReplayEvent
		var actor sql.NullString
		if err := rows.Scan(&ev.ID, &ev.GameID, &ev.GameSequence, &ev.OccurredAt, &actor, &ev.ActionType, &ev.Payload); err != nil {
			return nil, err
		}
		if actor.Valid {
			ev.ActorPlayerID = actor.String
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ReplayEventsExist reports whether gameID has any persisted replay
// events, letting callers cheaply gate a "replay available" flag without
// fetching rows.
func (s *PostgresGameReplayEventStore) ReplayEventsExist(ctx context.Context, q db.Querier, gameID string) (bool, error) {
	const stmt = `SELECT EXISTS(SELECT 1 FROM game_replay_events WHERE game_id = $1::uuid)`
	var exists bool
	err := q.QueryRow(ctx, stmt, gameID).Scan(&exists)
	return exists, err
}
