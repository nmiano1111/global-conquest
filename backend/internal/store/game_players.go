package store

import (
	"github.com/nmiano1111/global-conquest/backend/internal/db"
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

// GamePlayersStore defines persistence operations for the game_players join table, which records
// which human users occupy which seats in a game and tracks per-game win/loss outcomes.
//
// GamePlayersStore no longer serves leaderboard reads -- see
// PlayerGameStatsStore.GetLeaderboard (player_game_stats.go), which sources
// from a table that survives game deletion instead of joining games/
// game_players directly.
type GamePlayersStore interface {
	InsertGamePlayers(ctx context.Context, q db.Querier, players []NewGamePlayer) error
	SetGameWinner(ctx context.Context, q db.Querier, gameID, winnerUserID string) error
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
