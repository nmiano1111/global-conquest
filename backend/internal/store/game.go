package store

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/nmiano1111/global-conquest/backend/internal/db"
	"strings"
	"time"
)

// Game is a row from the games table, including its JSONB-serialized
// risk.Game state.
type Game struct {
	// ID is the game's UUID.
	ID string `json:"id"`
	// OwnerUserID is the UUID of the user who created the game.
	OwnerUserID string `json:"owner_user_id"`
	// Name is the game's display name.
	Name string `json:"name"`
	// Status is the game's lifecycle status (e.g. lobby, in_progress, finished).
	Status string `json:"status"`
	// State is the JSONB-serialized risk.Game engine state.
	State json.RawMessage `swaggertype:"object" json:"state,omitempty"`
	// IsSandboxed is a snapshot of the creator's is_sandboxed flag taken at
	// creation time: fixed for the game's lifetime regardless of later
	// changes to the creator's own flag. See GameListFilter for how this
	// drives visibility.
	IsSandboxed bool `json:"is_sandboxed"`
	// CreatedAt is when the game row was inserted.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is when the game row was last updated.
	UpdatedAt time.Time `json:"updated_at"`
}

// NewGame is the input for creating a new game row via Create.
type NewGame struct {
	// OwnerUserID is the UUID of the user creating the game.
	OwnerUserID string
	// Name is the game's display name.
	Name string
	// Status is the initial lifecycle status of the game.
	Status string
	// State is the initial JSONB-serialized risk.Game engine state.
	State json.RawMessage
	// IsSandboxed snapshots the creator's is_sandboxed flag at creation time.
	IsSandboxed bool
}

// UpdateGameState is the input for updating a game's status and state via UpdateState.
type UpdateGameState struct {
	// GameID is the UUID of the game to update.
	GameID string
	// Status is the new lifecycle status to set.
	Status string
	// State is the new JSONB-serialized risk.Game engine state to set.
	State json.RawMessage
}

// GameListFilter narrows and paginates the results of List.
type GameListFilter struct {
	// OwnerUserID, if non-empty, restricts results to games owned by this user.
	OwnerUserID string
	// Status, if non-empty, restricts results to games with this status.
	Status string
	// Limit caps the number of rows returned; see List for defaulting/clamping behavior.
	Limit int
	// Offset skips this many rows before returning results.
	Offset int

	// ViewerUserID, ViewerIsAdmin, and ViewerIsSandboxed together drive the
	// sandbox visibility rule applied to every List call: a game is included
	// only if the viewer is an admin, the viewer created it themselves, or
	// neither the viewer nor the game is sandboxed. A sandboxed player is
	// therefore isolated even from other sandboxed players' games -- the
	// only games visible to them are their own. ViewerUserID must be set for
	// this filter to apply (every List caller is expected to have an
	// authenticated viewer; callers that need every game regardless of
	// sandboxing, e.g. an internal maintenance scan, should pass
	// ViewerIsAdmin true instead of leaving ViewerUserID empty).
	ViewerUserID      string
	ViewerIsAdmin     bool
	ViewerIsSandboxed bool
}

// GamesStore defines the persistence operations for games.
type GamesStore interface {
	Create(ctx context.Context, q db.Querier, in NewGame) (Game, error)
	GetByID(ctx context.Context, q db.Querier, gameID string) (Game, error)
	GetByIDForUpdate(ctx context.Context, q db.Querier, gameID string) (Game, error)
	List(ctx context.Context, q db.Querier, filter GameListFilter) ([]Game, error)
	UpdateState(ctx context.Context, q db.Querier, in UpdateGameState) (Game, error)
	Delete(ctx context.Context, q db.Querier, gameID string) error
}

// PostgresGamesStore is the Postgres-backed implementation of GamesStore.
type PostgresGamesStore struct{}

// NewPostgresGamesStore constructs a PostgresGamesStore.
func NewPostgresGamesStore() *PostgresGamesStore { return &PostgresGamesStore{} }

// Create inserts a new game row and returns it as stored.
func (s *PostgresGamesStore) Create(ctx context.Context, exec db.Querier, in NewGame) (Game, error) {
	const stmt = `
		INSERT INTO games (owner_user_id, name, status, state, is_sandboxed)
		VALUES ($1::uuid, $2, $3, $4::jsonb, $5)
		RETURNING id::text, owner_user_id::text, name, status, state, is_sandboxed, created_at, updated_at
	`
	var g Game
	err := exec.QueryRow(ctx, stmt, in.OwnerUserID, in.Name, in.Status, in.State, in.IsSandboxed).Scan(
		&g.ID, &g.OwnerUserID, &g.Name, &g.Status, &g.State, &g.IsSandboxed, &g.CreatedAt, &g.UpdatedAt,
	)
	return g, err
}

// GetByID fetches a game by ID without locking the row. Callers that intend
// to mutate the game within a transaction must use GetByIDForUpdate instead.
func (s *PostgresGamesStore) GetByID(ctx context.Context, exec db.Querier, gameID string) (Game, error) {
	const stmt = `
		SELECT id::text, owner_user_id::text, name, status, state, is_sandboxed, created_at, updated_at
		FROM games
		WHERE id = $1::uuid
	`
	var g Game
	err := exec.QueryRow(ctx, stmt, gameID).Scan(
		&g.ID, &g.OwnerUserID, &g.Name, &g.Status, &g.State, &g.IsSandboxed, &g.CreatedAt, &g.UpdatedAt,
	)
	return g, err
}

// GetByIDForUpdate fetches a game by ID with a row-level SELECT ... FOR
// UPDATE lock, blocking concurrent updates until the enclosing transaction
// commits or rolls back. It must be called within a transaction (e.g. via
// WithTxQ) before any read-modify-write mutation of game state.
func (s *PostgresGamesStore) GetByIDForUpdate(ctx context.Context, exec db.Querier, gameID string) (Game, error) {
	const stmt = `
		SELECT id::text, owner_user_id::text, name, status, state, is_sandboxed, created_at, updated_at
		FROM games
		WHERE id = $1::uuid
		FOR UPDATE
	`
	var g Game
	err := exec.QueryRow(ctx, stmt, gameID).Scan(
		&g.ID, &g.OwnerUserID, &g.Name, &g.Status, &g.State, &g.IsSandboxed, &g.CreatedAt, &g.UpdatedAt,
	)
	return g, err
}

// List returns games matching filter, most recently created first.
// filter.Limit is clamped to a default of 50 when <= 0 and a maximum of 200;
// filter.Offset is clamped to 0 when negative. OwnerUserID and Status filter
// conditions are applied only when non-empty.
func (s *PostgresGamesStore) List(ctx context.Context, exec db.Querier, filter GameListFilter) ([]Game, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	stmt := `
		SELECT id::text, owner_user_id::text, name, status, state, is_sandboxed, created_at, updated_at
		FROM games
	`
	conds := make([]string, 0, 3)
	args := make([]any, 0, 5)
	if filter.OwnerUserID != "" {
		conds = append(conds, fmt.Sprintf("owner_user_id = $%d::uuid", len(args)+1))
		args = append(args, filter.OwnerUserID)
	}
	if filter.Status != "" {
		conds = append(conds, fmt.Sprintf("status = $%d", len(args)+1))
		args = append(args, filter.Status)
	}
	// See GameListFilter's doc comment for the visibility rule this
	// encodes. Applied whenever a viewer is given and isn't an admin --
	// admins (or an empty ViewerUserID, meaning "no visibility filtering
	// requested") see every game regardless of sandboxing.
	if filter.ViewerUserID != "" && !filter.ViewerIsAdmin {
		conds = append(conds, fmt.Sprintf(
			"(owner_user_id = $%d::uuid OR (NOT $%d AND NOT is_sandboxed))",
			len(args)+1, len(args)+2,
		))
		args = append(args, filter.ViewerUserID, filter.ViewerIsSandboxed)
	}
	if len(conds) > 0 {
		stmt += " WHERE " + strings.Join(conds, " AND ")
	}
	stmt += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	rows, err := exec.Query(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Game, 0, limit)
	for rows.Next() {
		var g Game
		if err := rows.Scan(&g.ID, &g.OwnerUserID, &g.Name, &g.Status, &g.State, &g.IsSandboxed, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateState updates a game's status, state, and updated_at timestamp, and
// returns the row as stored. Callers mutating state read via
// GetByIDForUpdate should call this within the same transaction to avoid
// lost updates.
func (s *PostgresGamesStore) UpdateState(ctx context.Context, exec db.Querier, in UpdateGameState) (Game, error) {
	const stmt = `
		UPDATE games
		SET status = $2,
		    state = $3::jsonb,
		    updated_at = now()
		WHERE id = $1::uuid
		RETURNING id::text, owner_user_id::text, name, status, state, is_sandboxed, created_at, updated_at
	`
	var g Game
	err := exec.QueryRow(ctx, stmt, in.GameID, in.Status, in.State).Scan(
		&g.ID, &g.OwnerUserID, &g.Name, &g.Status, &g.State, &g.IsSandboxed, &g.CreatedAt, &g.UpdatedAt,
	)
	return g, err
}

// Delete removes a game row. Every dependent table (game_events,
// game_domain_events, discord_outbox, game_players, game_chat_messages)
// has an ON DELETE CASCADE FK to games(id), so this cleans up everything.
// Returns ErrNoRows (via pgx) if the game doesn't exist.
func (s *PostgresGamesStore) Delete(ctx context.Context, exec db.Querier, gameID string) error {
	const stmt = `DELETE FROM games WHERE id = $1::uuid RETURNING id`
	var id string
	return exec.QueryRow(ctx, stmt, gameID).Scan(&id)
}
