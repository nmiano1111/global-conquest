package reporting

import (
	"context"
	"fmt"
	"time"

	"backend/internal/db"

	"github.com/jackc/pgx/v5"
)

// rawCombatRow holds columns scanned from game_domain_events before JSON decoding.
type rawCombatRow struct {
	id           string
	gameID       string
	gameSequence int64
	eventVersion int16
	occurredAt   time.Time
	payload      []byte
}

// Repository loads combat events from the game_domain_events table.
// All queries are read-only and parameterised; no game state is modified.
type Repository struct {
	db db.Querier
}

// NewRepository creates a Repository backed by the provided querier.
// Pass db.DB.Queryer() from main.
func NewRepository(q db.Querier) *Repository {
	return &Repository{db: q}
}

const queryAllCombatEvents = `
SELECT
    id::text,
    game_id::text,
    game_sequence,
    event_version,
    occurred_at,
    payload
FROM game_domain_events
WHERE game_id = $1::uuid
  AND event_type = 'combat_roll_resolved'
ORDER BY game_sequence ASC`

const queryRecentCombatEvents = `
SELECT
    id::text,
    game_id::text,
    game_sequence,
    event_version,
    occurred_at,
    payload
FROM game_domain_events
WHERE game_id = $1::uuid
  AND event_type = 'combat_roll_resolved'
ORDER BY game_sequence DESC
LIMIT $2`

const queryPlayerNames = `
SELECT id::text, username
FROM users
WHERE id::text = ANY($1::text[])`

const queryLatestGameID = `
SELECT id::text
FROM games
WHERE status != 'lobby'
ORDER BY updated_at DESC
LIMIT 1`

// LoadRawCombatEvents returns raw rows for all combat_roll_resolved events for a
// game in ascending game_sequence order.
func (r *Repository) LoadRawCombatEvents(ctx context.Context, gameID string) ([]rawCombatRow, error) {
	rows, err := r.db.Query(ctx, queryAllCombatEvents, gameID)
	if err != nil {
		return nil, fmt.Errorf("query combat events: %w", err)
	}
	return scanCombatRows(rows)
}

// LoadRawRecentCombatEvents returns at most limit raw rows in descending
// game_sequence order (most-recent first). The Service reverses this for display.
func (r *Repository) LoadRawRecentCombatEvents(ctx context.Context, gameID string, limit int) ([]rawCombatRow, error) {
	rows, err := r.db.Query(ctx, queryRecentCombatEvents, gameID, limit)
	if err != nil {
		return nil, fmt.Errorf("query recent combat events: %w", err)
	}
	return scanCombatRows(rows)
}

// LoadPlayerNames returns a map of player UUID string → username for the given IDs.
func (r *Repository) LoadPlayerNames(ctx context.Context, playerIDs []string) (map[string]string, error) {
	if len(playerIDs) == 0 {
		return map[string]string{}, nil
	}
	rows, err := r.db.Query(ctx, queryPlayerNames, playerIDs)
	if err != nil {
		return nil, fmt.Errorf("query player names: %w", err)
	}
	defer rows.Close()
	out := make(map[string]string, len(playerIDs))
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scan player name: %w", err)
		}
		out[id] = name
	}
	return out, rows.Err()
}

// LoadLatestGameID returns the ID of the most recently updated game that is not
// in lobby status. Returns ErrNoActiveGame if no such game exists.
func (r *Repository) LoadLatestGameID(ctx context.Context) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, queryLatestGameID).Scan(&id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", ErrNoActiveGame
		}
		return "", fmt.Errorf("query latest game: %w", err)
	}
	return id, nil
}

func scanCombatRows(rows pgx.Rows) ([]rawCombatRow, error) {
	defer rows.Close()
	var out []rawCombatRow
	for rows.Next() {
		var row rawCombatRow
		if err := rows.Scan(
			&row.id, &row.gameID, &row.gameSequence,
			&row.eventVersion, &row.occurredAt, &row.payload,
		); err != nil {
			return nil, fmt.Errorf("scan combat event: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
