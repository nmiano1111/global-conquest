package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/db"
)

// NotificationTypeTurnStarted identifies a turn_started notification row,
// enqueued whenever a player's turn ends and the next player's turn begins.
const NotificationTypeTurnStarted = "turn_started"

// PayloadSchemaVersionTurnStarted is the schema version stamped into
// TurnStartedPayload.SchemaVersion for turn_started notifications.
const PayloadSchemaVersionTurnStarted = 1

// NotificationTypeCardsTrade identifies a cards_trade notification row,
// enqueued whenever a player trades in a set of cards for reinforcements.
const NotificationTypeCardsTrade = "cards_trade"

// PayloadSchemaVersionCardsTrade is the schema version stamped into
// CardsTradePayload.SchemaVersion for cards_trade notifications.
const PayloadSchemaVersionCardsTrade = 1

// NotificationTypePlayerEliminated identifies a player_eliminated
// notification row, enqueued whenever an attack eliminates a player.
const NotificationTypePlayerEliminated = "player_eliminated"

// PayloadSchemaVersionPlayerEliminated is the schema version stamped into
// PlayerEliminatedPayload.SchemaVersion for player_eliminated notifications.
const PayloadSchemaVersionPlayerEliminated = 1

// NotificationTypeGameOver identifies a game_over notification row, enqueued
// when a game ends with a single remaining player.
const NotificationTypeGameOver = "game_over"

// PayloadSchemaVersionGameOver is the schema version stamped into
// GameOverPayload.SchemaVersion for game_over notifications.
const PayloadSchemaVersionGameOver = 1

// NotificationTypeGameStarted identifies a game_started notification row,
// enqueued once for the very first turn of a game.
const NotificationTypeGameStarted = "game_started"

// PayloadSchemaVersionGameStarted is the schema version stamped into
// GameStartedPayload.SchemaVersion for game_started notifications.
const PayloadSchemaVersionGameStarted = 1

// TurnStartedPayload is the structured payload for a turn_started notification.
type TurnStartedPayload struct {
	// SchemaVersion is the payload schema version, PayloadSchemaVersionTurnStarted.
	SchemaVersion int `json:"schema_version"`
	// PreviousPlayerDisplayName is the display name of the player whose turn just ended.
	PreviousPlayerDisplayName string `json:"previous_player_display_name"`
	// PreviousPlayerDiscordName is the Discord mention name of the player whose turn just ended, if linked.
	PreviousPlayerDiscordName *string `json:"previous_player_discord_name,omitempty"`
	// PlayerID is the ID of the player whose turn is starting.
	PlayerID string `json:"player_id"`
	// PlayerDisplayName is the display name of the player whose turn is starting.
	PlayerDisplayName string `json:"player_display_name"`
	// PlayerDiscordName is the Discord mention name of the player whose turn is starting, if linked.
	PlayerDiscordName *string `json:"player_discord_name,omitempty"`
	// TurnNumber is the 1-based number of the turn that is starting.
	TurnNumber int `json:"turn_number"`
}

// CardsTradePayload is the structured payload for a cards_trade notification.
type CardsTradePayload struct {
	// SchemaVersion is the payload schema version, PayloadSchemaVersionCardsTrade.
	SchemaVersion int `json:"schema_version"`
	// PlayerID is the ID of the player who traded in a set of cards.
	PlayerID string `json:"player_id"`
	// PlayerDisplayName is the display name of the player who traded in the cards.
	PlayerDisplayName string `json:"player_display_name"`
	// PlayerDiscordName is the Discord mention name of the trading player, if linked.
	PlayerDiscordName *string `json:"player_discord_name,omitempty"`
	// Armies is the number of reinforcement armies awarded for the trade.
	Armies int `json:"armies"`
}

// PlayerEliminatedPayload is the structured payload for a player_eliminated notification.
type PlayerEliminatedPayload struct {
	// SchemaVersion is the payload schema version, PayloadSchemaVersionPlayerEliminated.
	SchemaVersion int `json:"schema_version"`
	// AttackerID is the ID of the player whose attack eliminated another player.
	AttackerID string `json:"attacker_id"`
	// AttackerDisplayName is the display name of the eliminating player.
	AttackerDisplayName string `json:"attacker_display_name"`
	// AttackerDiscordName is the Discord mention name of the eliminating player, if linked.
	AttackerDiscordName *string `json:"attacker_discord_name,omitempty"`
	// EliminatedPlayerID is the ID of the player who was eliminated.
	EliminatedPlayerID string `json:"eliminated_player_id"`
	// EliminatedPlayerDisplayName is the display name of the eliminated player.
	EliminatedPlayerDisplayName string `json:"eliminated_player_display_name"`
	// EliminatedPlayerDiscordName is the Discord mention name of the eliminated player, if linked.
	EliminatedPlayerDiscordName *string `json:"eliminated_player_discord_name,omitempty"`
}

// GameOverPayload is the structured payload for a game_over notification.
type GameOverPayload struct {
	// SchemaVersion is the payload schema version, PayloadSchemaVersionGameOver.
	SchemaVersion int `json:"schema_version"`
	// WinnerID is the ID of the player who won the game.
	WinnerID string `json:"winner_id"`
	// WinnerDisplayName is the display name of the winning player.
	WinnerDisplayName string `json:"winner_display_name"`
	// WinnerDiscordName is the Discord mention name of the winning player, if linked.
	WinnerDiscordName *string `json:"winner_discord_name,omitempty"`
}

// GameStartedPayload is the structured payload for a game_started
// notification — the very first turn of a game, which turn_started never
// covers since it only fires when a player's turn ends (see EnqueueTurnStarted).
type GameStartedPayload struct {
	// SchemaVersion is the payload schema version, PayloadSchemaVersionGameStarted.
	SchemaVersion int `json:"schema_version"`
	// PlayerID is the ID of the player whose turn is first in the game.
	PlayerID string `json:"player_id"`
	// PlayerDisplayName is the display name of the first player.
	PlayerDisplayName string `json:"player_display_name"`
	// PlayerDiscordName is the Discord mention name of the first player, if linked.
	PlayerDiscordName *string `json:"player_discord_name,omitempty"`
}

// DiscordOutboxEntry is a row returned from discord_outbox.
type DiscordOutboxEntry struct {
	// ID is the outbox row's UUID.
	ID string
	// GameID is the ID of the game this notification belongs to.
	GameID string
	// GameName is the display name of the game at enqueue time.
	GameName string
	// GameSequence is the per-game monotonic event sequence number, used for
	// ordering and deduplication of notifications.
	GameSequence int64
	// NotificationType identifies the kind of notification, e.g. NotificationTypeTurnStarted.
	NotificationType string
	// Payload is the JSON-encoded notification payload, whose shape depends on NotificationType.
	Payload json.RawMessage
	// AttemptCount is the number of delivery attempts made so far.
	AttemptCount int
	// CreatedAt is when the outbox row was inserted.
	CreatedAt time.Time
}

// DiscordOutboxTransactor wraps a DB that can run transactions.
// *db.DB satisfies this interface.
type DiscordOutboxTransactor interface {
	WithTxQ(ctx context.Context, fn func(db.Querier) error) error
	Queryer() db.Querier
}

// PostgresDiscordOutboxStore is the Postgres-backed implementation of the
// discord_outbox enqueue and claim/delivery operations.
type PostgresDiscordOutboxStore struct{}

// NewPostgresDiscordOutboxStore constructs a PostgresDiscordOutboxStore.
func NewPostgresDiscordOutboxStore() *PostgresDiscordOutboxStore {
	return &PostgresDiscordOutboxStore{}
}

// EnqueueTurnStarted inserts a turn_started notification row inside the caller's
// transaction. The games row must already be locked (SELECT FOR UPDATE) so that
// the event_sequence increment is safe.
func (s *PostgresDiscordOutboxStore) EnqueueTurnStarted(
	ctx context.Context,
	q db.Querier,
	gameID, gameName, previousPlayerDisplayName, playerID, playerDisplayName string,
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
			(game_id, game_name, game_sequence, notification_type, deduplication_key, payload)
		SELECT
			$1::uuid,
			$2,
			seq.event_sequence,
			'turn_started',
			format('game:%s:sequence:%s:turn-started', $1::text, seq.event_sequence::text),
			$3::jsonb
		FROM seq
		RETURNING id::text, game_id::text, game_sequence
	`
	var id, gid string
	var seq int64
	return q.QueryRow(ctx, stmt, gameID, gameName, string(payload)).Scan(&id, &gid, &seq)
}

// EnqueueCardsTrade inserts a cards_trade notification row inside the caller's transaction.
func (s *PostgresDiscordOutboxStore) EnqueueCardsTrade(
	ctx context.Context,
	q db.Querier,
	gameID, gameName, playerID, playerDisplayName string,
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
			(game_id, game_name, game_sequence, notification_type, deduplication_key, payload)
		SELECT
			$1::uuid,
			$2,
			seq.event_sequence,
			'cards_trade',
			format('game:%s:sequence:%s:cards-trade', $1::text, seq.event_sequence::text),
			$3::jsonb
		FROM seq
		RETURNING id::text, game_id::text, game_sequence
	`
	var id, gid string
	var seq int64
	return q.QueryRow(ctx, stmt, gameID, gameName, string(payload)).Scan(&id, &gid, &seq)
}

// EnqueuePlayerEliminated inserts a player_eliminated notification row inside the caller's transaction.
func (s *PostgresDiscordOutboxStore) EnqueuePlayerEliminated(
	ctx context.Context,
	q db.Querier,
	gameID, gameName, attackerID, attackerDisplayName string,
	attackerDiscordName *string,
	eliminatedPlayerID, eliminatedPlayerDisplayName string,
	eliminatedPlayerDiscordName *string,
) error {
	payload, err := json.Marshal(PlayerEliminatedPayload{
		SchemaVersion:               PayloadSchemaVersionPlayerEliminated,
		AttackerID:                  attackerID,
		AttackerDisplayName:         attackerDisplayName,
		AttackerDiscordName:         attackerDiscordName,
		EliminatedPlayerID:          eliminatedPlayerID,
		EliminatedPlayerDisplayName: eliminatedPlayerDisplayName,
		EliminatedPlayerDiscordName: eliminatedPlayerDiscordName,
	})
	if err != nil {
		return fmt.Errorf("marshal player_eliminated payload: %w", err)
	}

	const stmt = `
		WITH seq AS (
			UPDATE games
			SET event_sequence = event_sequence + 1
			WHERE id = $1::uuid
			RETURNING event_sequence
		)
		INSERT INTO discord_outbox
			(game_id, game_name, game_sequence, notification_type, deduplication_key, payload)
		SELECT
			$1::uuid,
			$2,
			seq.event_sequence,
			'player_eliminated',
			format('game:%s:sequence:%s:player-eliminated', $1::text, seq.event_sequence::text),
			$3::jsonb
		FROM seq
		RETURNING id::text, game_id::text, game_sequence
	`
	var id, gid string
	var seq int64
	return q.QueryRow(ctx, stmt, gameID, gameName, string(payload)).Scan(&id, &gid, &seq)
}

// EnqueueGameOver inserts a game_over notification row inside the caller's transaction.
func (s *PostgresDiscordOutboxStore) EnqueueGameOver(
	ctx context.Context,
	q db.Querier,
	gameID, gameName, winnerID, winnerDisplayName string,
	winnerDiscordName *string,
) error {
	payload, err := json.Marshal(GameOverPayload{
		SchemaVersion:     PayloadSchemaVersionGameOver,
		WinnerID:          winnerID,
		WinnerDisplayName: winnerDisplayName,
		WinnerDiscordName: winnerDiscordName,
	})
	if err != nil {
		return fmt.Errorf("marshal game_over payload: %w", err)
	}

	const stmt = `
		WITH seq AS (
			UPDATE games
			SET event_sequence = event_sequence + 1
			WHERE id = $1::uuid
			RETURNING event_sequence
		)
		INSERT INTO discord_outbox
			(game_id, game_name, game_sequence, notification_type, deduplication_key, payload)
		SELECT
			$1::uuid,
			$2,
			seq.event_sequence,
			'game_over',
			format('game:%s:sequence:%s:game-over', $1::text, seq.event_sequence::text),
			$3::jsonb
		FROM seq
		RETURNING id::text, game_id::text, game_sequence
	`
	var id, gid string
	var seq int64
	return q.QueryRow(ctx, stmt, gameID, gameName, string(payload)).Scan(&id, &gid, &seq)
}

// EnqueueGameStarted inserts a game_started notification row inside the
// caller's transaction — the game's very first turn, distinct from every
// later turn_started notification.
func (s *PostgresDiscordOutboxStore) EnqueueGameStarted(
	ctx context.Context,
	q db.Querier,
	gameID, gameName, playerID, playerDisplayName string,
	playerDiscordName *string,
) error {
	payload, err := json.Marshal(GameStartedPayload{
		SchemaVersion:     PayloadSchemaVersionGameStarted,
		PlayerID:          playerID,
		PlayerDisplayName: playerDisplayName,
		PlayerDiscordName: playerDiscordName,
	})
	if err != nil {
		return fmt.Errorf("marshal game_started payload: %w", err)
	}

	const stmt = `
		WITH seq AS (
			UPDATE games
			SET event_sequence = event_sequence + 1
			WHERE id = $1::uuid
			RETURNING event_sequence
		)
		INSERT INTO discord_outbox
			(game_id, game_name, game_sequence, notification_type, deduplication_key, payload)
		SELECT
			$1::uuid,
			$2,
			seq.event_sequence,
			'game_started',
			format('game:%s:sequence:%s:game-started', $1::text, seq.event_sequence::text),
			$3::jsonb
		FROM seq
		RETURNING id::text, game_id::text, game_sequence
	`
	var id, gid string
	var seq int64
	return q.QueryRow(ctx, stmt, gameID, gameName, string(payload)).Scan(&id, &gid, &seq)
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
		RETURNING o.id::text, o.game_id::text, o.game_name, o.game_sequence, o.notification_type,
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
			&e.ID, &e.GameID, &e.GameName, &e.GameSequence, &e.NotificationType,
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
