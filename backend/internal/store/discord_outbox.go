package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"backend/internal/db"
)

const NotificationTypeTurnStarted = "turn_started"
const PayloadSchemaVersionTurnStarted = 1

const NotificationTypeCardsTrade = "cards_trade"
const PayloadSchemaVersionCardsTrade = 1

// TurnStartedPayload is the structured payload for a turn_started notification.
type TurnStartedPayload struct {
	SchemaVersion             int     `json:"schema_version"`
	PreviousPlayerDisplayName string  `json:"previous_player_display_name"`
	PreviousPlayerDiscordName *string `json:"previous_player_discord_name,omitempty"`
	PlayerID                  string  `json:"player_id"`
	PlayerDisplayName         string  `json:"player_display_name"`
	PlayerDiscordName         *string `json:"player_discord_name,omitempty"`
	TurnNumber                int     `json:"turn_number"`
}

// CardsTradePayload is the structured payload for a cards_trade notification.
type CardsTradePayload struct {
	SchemaVersion      int     `json:"schema_version"`
	PlayerID           string  `json:"player_id"`
	PlayerDisplayName  string  `json:"player_display_name"`
	PlayerDiscordName  *string `json:"player_discord_name,omitempty"`
	Armies             int     `json:"armies"`
}

// DiscordOutboxEntry is a row returned from discord_outbox.
type DiscordOutboxEntry struct {
	ID               string
	GameID           string
	GameSequence     int64
	NotificationType string
	Payload          json.RawMessage
	AttemptCount     int
	CreatedAt        time.Time
}

// DiscordOutboxTransactor wraps a DB that can run transactions.
// *db.DB satisfies this interface.
type DiscordOutboxTransactor interface {
	WithTxQ(ctx context.Context, fn func(db.Querier) error) error
	Queryer() db.Querier
}

type PostgresDiscordOutboxStore struct{}

func NewPostgresDiscordOutboxStore() *PostgresDiscordOutboxStore {
	return &PostgresDiscordOutboxStore{}
}

// EnqueueTurnStarted inserts a turn_started notification row inside the caller's
// transaction. The games row must already be locked (SELECT FOR UPDATE) so that
// the event_sequence increment is safe.
func (s *PostgresDiscordOutboxStore) EnqueueTurnStarted(
	ctx context.Context,
	q db.Querier,
	gameID, previousPlayerDisplayName, playerID, playerDisplayName string,
	previousPlayerDiscordName, playerDiscordName *string,
	turnNumber int,
) error {
	payload, err := json.Marshal(TurnStartedPayload{
		SchemaVersion:             PayloadSchemaVersionTurnStarted,
		PreviousPlayerDisplayName: previousPlayerDisplayName,
		PreviousPlayerDiscordName: previousPlayerDiscordName,
		PlayerID:                  playerID,
		PlayerDisplayName:         playerDisplayName,
		PlayerDiscordName:         playerDiscordName,
		TurnNumber:                turnNumber,
	})
	if err != nil {
		return fmt.Errorf("marshal turn_started payload: %w", err)
	}

	const stmt = `
		WITH seq AS (
			UPDATE games
			SET event_sequence = event_sequence + 1
			WHERE id = $1::uuid
			RETURNING event_sequence
		)
		INSERT INTO discord_outbox
			(game_id, game_sequence, notification_type, deduplication_key, payload)
		SELECT
			$1::uuid,
			seq.event_sequence,
			'turn_started',
			format('game:%s:sequence:%s:turn-started', $1::text, seq.event_sequence::text),
			$2::jsonb
		FROM seq
		RETURNING id::text, game_id::text, game_sequence
	`
	var id, gid string
	var seq int64
	return q.QueryRow(ctx, stmt, gameID, string(payload)).Scan(&id, &gid, &seq)
}

// EnqueueCardsTrade inserts a cards_trade notification row inside the caller's transaction.
func (s *PostgresDiscordOutboxStore) EnqueueCardsTrade(
	ctx context.Context,
	q db.Querier,
	gameID, playerID, playerDisplayName string,
	playerDiscordName *string,
	armies int,
) error {
	payload, err := json.Marshal(CardsTradePayload{
		SchemaVersion:     PayloadSchemaVersionCardsTrade,
		PlayerID:          playerID,
		PlayerDisplayName: playerDisplayName,
		PlayerDiscordName: playerDiscordName,
		Armies:            armies,
	})
	if err != nil {
		return fmt.Errorf("marshal cards_trade payload: %w", err)
	}

	const stmt = `
		WITH seq AS (
			UPDATE games
			SET event_sequence = event_sequence + 1
			WHERE id = $1::uuid
			RETURNING event_sequence
		)
		INSERT INTO discord_outbox
			(game_id, game_sequence, notification_type, deduplication_key, payload)
		SELECT
			$1::uuid,
			seq.event_sequence,
			'cards_trade',
			format('game:%s:sequence:%s:cards-trade', $1::text, seq.event_sequence::text),
			$2::jsonb
		FROM seq
		RETURNING id::text, game_id::text, game_sequence
	`
	var id, gid string
	var seq int64
	return q.QueryRow(ctx, stmt, gameID, string(payload)).Scan(&id, &gid, &seq)
}

// ClaimPending atomically claims up to limit pending rows using FOR UPDATE SKIP LOCKED.
func (s *PostgresDiscordOutboxStore) ClaimPending(
	ctx context.Context,
	d DiscordOutboxTransactor,
	limit int,
) ([]DiscordOutboxEntry, error) {
	var out []DiscordOutboxEntry
	err := d.WithTxQ(ctx, func(q db.Querier) error {
		var err error
		out, err = s.claimPendingQ(ctx, q, limit)
		return err
	})
	return out, err
}

func (s *PostgresDiscordOutboxStore) claimPendingQ(
	ctx context.Context,
	q db.Querier,
	limit int,
) ([]DiscordOutboxEntry, error) {
	const stmt = `
		WITH candidates AS (
			SELECT id
			FROM discord_outbox
			WHERE delivered_at IS NULL
			  AND available_at <= now()
			  AND (claimed_at IS NULL OR claimed_at < now() - interval '2 minutes')
			ORDER BY available_at, created_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE discord_outbox o
		SET claimed_at = now(),
		    attempt_count = attempt_count + 1
		FROM candidates
		WHERE o.id = candidates.id
		RETURNING o.id::text, o.game_id::text, o.game_sequence, o.notification_type,
		          o.payload, o.attempt_count, o.created_at
	`
	rows, err := q.Query(ctx, stmt, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DiscordOutboxEntry
	for rows.Next() {
		var e DiscordOutboxEntry
		if err := rows.Scan(
			&e.ID, &e.GameID, &e.GameSequence, &e.NotificationType,
			&e.Payload, &e.AttemptCount, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// MarkDelivered marks the row as successfully delivered.
func (s *PostgresDiscordOutboxStore) MarkDelivered(ctx context.Context, q db.Querier, id string) error {
	const stmt = `
		WITH r AS (
			UPDATE discord_outbox
			SET delivered_at = now(), last_error = NULL
			WHERE id = $1::uuid
			RETURNING 1
		)
		SELECT count(*)::bigint FROM r
	`
	var n int64
	return q.QueryRow(ctx, stmt, id).Scan(&n)
}

// MarkFailed clears the claim and schedules a retry based on attempt count.
func (s *PostgresDiscordOutboxStore) MarkFailed(
	ctx context.Context,
	q db.Querier,
	id string,
	attempt int,
	errMsg string,
) error {
	const maxErrLen = 500
	if len(errMsg) > maxErrLen {
		errMsg = errMsg[:maxErrLen]
	}
	delay := retryDelay(attempt)
	const stmt = `
		WITH r AS (
			UPDATE discord_outbox
			SET claimed_at = NULL,
			    last_error  = $2,
			    available_at = now() + ($3 * interval '1 second')
			WHERE id = $1::uuid
			RETURNING 1
		)
		SELECT count(*)::bigint FROM r
	`
	var n int64
	return q.QueryRow(ctx, stmt, id, errMsg, delay.Seconds()).Scan(&n)
}

// retryDelay returns the back-off duration for the given 1-based attempt count.
func retryDelay(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 5 * time.Second
	case 2:
		return 30 * time.Second
	case 3:
		return 2 * time.Minute
	case 4:
		return 10 * time.Minute
	default:
		return 6 * time.Hour
	}
}
