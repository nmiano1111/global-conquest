package store

import (
	"context"
	"encoding/json"

	"github.com/nmiano1111/global-conquest/backend/internal/db"
)

// PlayerGameResult is one human player's outcome in a single completed
// game, used as input to RecordGameResult.
type PlayerGameResult struct {
	// UserID is the identifier of the human user this result belongs to.
	UserID string
	// Won reports whether this user won the game.
	Won bool
}

// PlayerCountRecord is one user's win/loss record restricted to games of a
// single player count (e.g. all of a user's 4-player games).
type PlayerCountRecord struct {
	// PlayerCount is the total number of (human) players in the games this
	// record summarizes.
	PlayerCount int `json:"player_count"`
	// Wins is the number of games of this player count the user won.
	Wins int `json:"wins"`
	// Losses is the number of games of this player count the user did not win.
	Losses int `json:"losses"`
}

// LeaderboardEntry is one user's aggregated win/loss record across
// completed, human-only games, plus the same breakdown split by player
// count.
type LeaderboardEntry struct {
	// UserID is the identifier of the user this entry summarizes.
	UserID string `json:"user_id"`
	// UserName is the display name of the user this entry summarizes.
	UserName string `json:"username"`
	// Wins is the number of completed games the user won.
	Wins int `json:"wins"`
	// Losses is the number of completed games the user did not win.
	Losses int `json:"losses"`
	// GamesPlayed is the total number of completed games the user participated in.
	GamesPlayed int `json:"games_played"`
	// ByPlayerCount is Wins/Losses broken down per player count, ordered by
	// player count ascending.
	ByPlayerCount []PlayerCountRecord `json:"by_player_count"`
}

// PlayerGameStatsStore defines persistence operations for player_game_stats,
// the durable win/loss record that survives a game row being deleted (see
// migrations/V19__player_game_stats.sql for why it carries no FK to games).
type PlayerGameStatsStore interface {
	RecordGameResult(ctx context.Context, q db.Querier, gameID string, playerCount int, results []PlayerGameResult) error
	GetLeaderboard(ctx context.Context, q db.Querier, limit int) ([]LeaderboardEntry, error)
}

// PostgresPlayerGameStatsStore is a Postgres-backed implementation of
// PlayerGameStatsStore.
type PostgresPlayerGameStatsStore struct{}

// NewPostgresPlayerGameStatsStore constructs a PostgresPlayerGameStatsStore.
func NewPostgresPlayerGameStatsStore() *PostgresPlayerGameStatsStore {
	return &PostgresPlayerGameStatsStore{}
}

// RecordGameResult writes one player_game_stats row per entry in results,
// all sharing gameID and playerCount. It upserts on (game_id, user_id) so
// it is safe to call more than once for the same game (e.g. a retried
// transaction) without producing duplicate or conflicting rows.
func (s *PostgresPlayerGameStatsStore) RecordGameResult(ctx context.Context, exec db.Querier, gameID string, playerCount int, results []PlayerGameResult) error {
	const stmt = `
		INSERT INTO player_game_stats (game_id, user_id, won, player_count)
		VALUES ($1::uuid, $2::uuid, $3, $4)
		ON CONFLICT (game_id, user_id) DO UPDATE SET won = EXCLUDED.won
	`
	for _, r := range results {
		rows, err := exec.Query(ctx, stmt, gameID, r.UserID, r.Won, playerCount)
		if err != nil {
			return err
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}
	return nil
}

// GetLeaderboard returns up to limit users' win/loss records, ordered by
// wins descending then losses ascending, each with a by-player-count
// breakdown. Every row in player_game_stats already represents a
// completed, human-only game (see allHuman/humanGameResults in
// service/game.go and migrations/V20__backfill_player_game_stats.sql), so
// unlike the old game_players-backed query, no join or bot-exclusion
// filter is needed here -- and because this table has no FK to games,
// results are unaffected by a game being deleted afterward.
func (s *PostgresPlayerGameStatsStore) GetLeaderboard(ctx context.Context, exec db.Querier, limit int) ([]LeaderboardEntry, error) {
	const stmt = `
		WITH totals AS (
			SELECT
				user_id,
				COUNT(*) FILTER (WHERE won)     AS wins,
				COUNT(*) FILTER (WHERE NOT won) AS losses,
				COUNT(*)                        AS games_played
			FROM player_game_stats
			GROUP BY user_id
		),
		by_count AS (
			SELECT
				user_id,
				player_count,
				COUNT(*) FILTER (WHERE won)     AS wins,
				COUNT(*) FILTER (WHERE NOT won) AS losses
			FROM player_game_stats
			GROUP BY user_id, player_count
		)
		SELECT
			u.id::text,
			u.username,
			t.wins,
			t.losses,
			t.games_played,
			(
				SELECT jsonb_agg(
					jsonb_build_object('player_count', bc.player_count, 'wins', bc.wins, 'losses', bc.losses)
					ORDER BY bc.player_count
				)
				FROM by_count bc
				WHERE bc.user_id = t.user_id
			) AS by_player_count
		FROM totals t
		JOIN users u ON u.id = t.user_id
		ORDER BY t.wins DESC, t.losses ASC
		LIMIT $1
	`
	rows, err := exec.Query(ctx, stmt, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]LeaderboardEntry, 0, limit)
	for rows.Next() {
		var e LeaderboardEntry
		var byPlayerCountJSON []byte
		if err := rows.Scan(&e.UserID, &e.UserName, &e.Wins, &e.Losses, &e.GamesPlayed, &byPlayerCountJSON); err != nil {
			return nil, err
		}
		if len(byPlayerCountJSON) > 0 {
			if err := json.Unmarshal(byPlayerCountJSON, &e.ByPlayerCount); err != nil {
				return nil, err
			}
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
