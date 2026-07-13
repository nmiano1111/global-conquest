package store

import (
	"backend/internal/db"
	"context"
)

// NewGamePlayer is the input for registering a human player's seat in a game's game_players rows.
// Bot players never get a NewGamePlayer / game_players row (see humanGamePlayers in service/game.go).
type NewGamePlayer struct {
	// GameID is the identifier of the game the player is seated in.
	GameID string
	// UserID is the identifier of the human user occupying the seat.
	UserID string
	// PlayerIndex is the player's position within risk.Game.Players.
	PlayerIndex int
}

// LeaderboardEntry is one user's aggregated win/loss record across completed, human-only games.
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
}

// GamePlayersStore defines persistence operations for the game_players join table, which records
// which human users occupy which seats in a game and tracks per-game win/loss outcomes.
type GamePlayersStore interface {
	InsertGamePlayers(ctx context.Context, q db.Querier, players []NewGamePlayer) error
	SetGameWinner(ctx context.Context, q db.Querier, gameID, winnerUserID string) error
	GetLeaderboard(ctx context.Context, q db.Querier, limit int) ([]LeaderboardEntry, error)
}

// PostgresGamePlayersStore is a Postgres-backed implementation of GamePlayersStore.
type PostgresGamePlayersStore struct{}

// NewPostgresGamePlayersStore constructs a PostgresGamePlayersStore.
func NewPostgresGamePlayersStore() *PostgresGamePlayersStore {
	return &PostgresGamePlayersStore{}
}

// InsertGamePlayers inserts one game_players row per entry in players, one INSERT statement per player. It returns the first error encountered, leaving any already-inserted rows in place (the caller is expected to run this inside a transaction that will be rolled back on error).
func (s *PostgresGamePlayersStore) InsertGamePlayers(ctx context.Context, exec db.Querier, players []NewGamePlayer) error {
	const stmt = `
		INSERT INTO game_players (game_id, user_id, player_index)
		VALUES ($1::uuid, $2::uuid, $3)
	`
	for _, p := range players {
		rows, err := exec.Query(ctx, stmt, p.GameID, p.UserID, p.PlayerIndex)
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

// SetGameWinner marks the game_players row for gameID and winnerUserID as won. It does not verify that gameID and winnerUserID correspond to an existing row; if no row matches, it succeeds without updating anything.
func (s *PostgresGamePlayersStore) SetGameWinner(ctx context.Context, exec db.Querier, gameID, winnerUserID string) error {
	const stmt = `
		UPDATE game_players
		SET won = true
		WHERE game_id = $1::uuid AND user_id = $2::uuid
	`
	rows, err := exec.Query(ctx, stmt, gameID, winnerUserID)
	if err != nil {
		return err
	}
	rows.Close()
	return rows.Err()
}

// GetLeaderboard returns up to limit users' win/loss records across completed games, ordered by wins descending then losses ascending. Games involving any bot player are excluded entirely (detected by inspecting the game's JSONB state for a player with controller = "bot", since bots never have a game_players row); only pure human-vs-human completed games count. Users with zero completed games are omitted.
func (s *PostgresGamePlayersStore) GetLeaderboard(ctx context.Context, exec db.Querier, limit int) ([]LeaderboardEntry, error) {
	// Games with any bot player don't count toward the leaderboard at all —
	// only pure human-vs-human games do. Bots never get a game_players row
	// (see humanGamePlayers in service/game.go), so this can't be filtered
	// by joining that table; it has to check the authoritative JSONB state
	// directly for any player with controller = "bot".
	const stmt = `
		SELECT
			u.id::text,
			u.username,
			COUNT(*) FILTER (WHERE gp.won = true)                          AS wins,
			COUNT(*) FILTER (WHERE gp.won = false AND g.status = 'completed') AS losses,
			COUNT(*) FILTER (WHERE g.status = 'completed')                 AS games_played
		FROM game_players gp
		JOIN users  u ON u.id = gp.user_id
		JOIN games  g ON g.id = gp.game_id
		WHERE NOT EXISTS (
			SELECT 1
			FROM jsonb_array_elements(g.state -> 'players') AS p
			WHERE p ->> 'controller' = 'bot'
		)
		GROUP BY u.id, u.username
		HAVING COUNT(*) FILTER (WHERE g.status = 'completed') > 0
		ORDER BY wins DESC, losses ASC
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
		if err := rows.Scan(&e.UserID, &e.UserName, &e.Wins, &e.Losses, &e.GamesPlayed); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
