package store

import (
	"backend/internal/db"
	"context"
)

type NewGamePlayer struct {
	GameID      string
	UserID      string
	PlayerIndex int
}

type LeaderboardEntry struct {
	UserID      string `json:"user_id"`
	UserName    string `json:"username"`
	Wins        int    `json:"wins"`
	Losses      int    `json:"losses"`
	GamesPlayed int    `json:"games_played"`
}

type GamePlayersStore interface {
	InsertGamePlayers(ctx context.Context, q db.Querier, players []NewGamePlayer) error
	SetGameWinner(ctx context.Context, q db.Querier, gameID, winnerUserID string) error
	GetLeaderboard(ctx context.Context, q db.Querier, limit int) ([]LeaderboardEntry, error)
}

type PostgresGamePlayersStore struct{}

func NewPostgresGamePlayersStore() *PostgresGamePlayersStore {
	return &PostgresGamePlayersStore{}
}

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
