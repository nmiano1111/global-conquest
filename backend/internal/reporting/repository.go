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

const queryLatestGame = `
SELECT id::text, name
FROM games
WHERE status != 'lobby'
ORDER BY updated_at DESC
LIMIT 1`

const queryGameByName = `
SELECT id::text, name
FROM games
WHERE lower(name) = lower($1)
  AND status != 'lobby'
ORDER BY updated_at DESC
LIMIT 1`

const queryPlayerByUsername = `
SELECT id::text
FROM users
WHERE lower(username) = lower($1)
LIMIT 1`

const queryCurrentPlayer = `
SELECT u.username, u.discord_name
FROM games g
JOIN game_players gp
  ON gp.game_id = g.id
 AND gp.player_index = (g.state->>'current_player')::int
JOIN users u ON u.id = gp.user_id
WHERE g.id = $1::uuid`

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

// LoadLatestGame returns the ID and name of the most recently updated non-lobby
// game. Returns ErrNoActiveGame if no such game exists.
func (r *Repository) LoadLatestGame(ctx context.Context) (string, string, error) {
	var id, name string
	err := r.db.QueryRow(ctx, queryLatestGame).Scan(&id, &name)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", "", ErrNoActiveGame
		}
		return "", "", fmt.Errorf("query latest game: %w", err)
	}
	return id, name, nil
}

// LoadGameByName returns the ID and canonical name of a non-lobby game whose
// name matches (case-insensitive). Returns ErrGameNotFound if no match.
func (r *Repository) LoadGameByName(ctx context.Context, name string) (string, string, error) {
	var id, canonicalName string
	err := r.db.QueryRow(ctx, queryGameByName, name).Scan(&id, &canonicalName)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", "", ErrGameNotFound
		}
		return "", "", fmt.Errorf("query game by name: %w", err)
	}
	return id, canonicalName, nil
}

// LoadPlayerByUsername returns the UUID of the player with the given username
// (case-insensitive). Returns ErrPlayerNotFound if no match.
func (r *Repository) LoadPlayerByUsername(ctx context.Context, username string) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, queryPlayerByUsername, username).Scan(&id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", ErrPlayerNotFound
		}
		return "", fmt.Errorf("query player by username: %w", err)
	}
	return id, nil
}

// LoadCurrentPlayer returns the username and optional Discord name of the player
// whose turn it currently is. Returns ErrNoCurrentPlayer if the join finds no row.
func (r *Repository) LoadCurrentPlayer(ctx context.Context, gameID string) (username string, discordName *string, err error) {
	e := r.db.QueryRow(ctx, queryCurrentPlayer, gameID).Scan(&username, &discordName)
	if e != nil {
		if e == pgx.ErrNoRows {
			return "", nil, ErrNoCurrentPlayer
		}
		return "", nil, fmt.Errorf("query current player: %w", e)
	}
	return username, discordName, nil
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
