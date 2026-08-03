package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/db"
)

// Map is a row from the maps table: an admin-authored custom board
// definition, JSONB-serialized as a mapgen.MapDefinition.
type Map struct {
	// ID is the map's UUID.
	ID string `json:"id"`
	// OwnerUserID is the UUID of the admin who created the map.
	OwnerUserID string `json:"owner_user_id"`
	// Name is the map's display name.
	Name string `json:"name"`
	// Definition is the JSONB-serialized mapgen.MapDefinition (board,
	// layout, and the spec it was generated from).
	Definition json.RawMessage `swaggertype:"object" json:"definition"`
	// CreatedAt is when the map row was inserted.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is when the map row was last updated.
	UpdatedAt time.Time `json:"updated_at"`
}

// NewMap is the input for creating a new map row via Create.
type NewMap struct {
	// OwnerUserID is the UUID of the admin creating the map.
	OwnerUserID string
	// Name is the map's display name.
	Name string
	// Definition is the JSONB-serialized mapgen.MapDefinition to store.
	Definition json.RawMessage
}

// MapsStore defines the persistence operations for custom maps.
type MapsStore interface {
	Create(ctx context.Context, q db.Querier, in NewMap) (Map, error)
	GetByID(ctx context.Context, q db.Querier, mapID string) (Map, error)
	List(ctx context.Context, q db.Querier, ownerUserID string) ([]Map, error)
	Delete(ctx context.Context, q db.Querier, mapID string) error
}

// PostgresMapsStore is the Postgres-backed implementation of MapsStore.
type PostgresMapsStore struct{}

// NewPostgresMapsStore constructs a PostgresMapsStore.
func NewPostgresMapsStore() *PostgresMapsStore { return &PostgresMapsStore{} }

// Create inserts a new map row and returns it as stored.
func (s *PostgresMapsStore) Create(ctx context.Context, exec db.Querier, in NewMap) (Map, error) {
	const stmt = `
		INSERT INTO maps (owner_user_id, name, definition)
		VALUES ($1::uuid, $2, $3::jsonb)
		RETURNING id::text, owner_user_id::text, name, definition, created_at, updated_at
	`
	var m Map
	err := exec.QueryRow(ctx, stmt, in.OwnerUserID, in.Name, in.Definition).Scan(
		&m.ID, &m.OwnerUserID, &m.Name, &m.Definition, &m.CreatedAt, &m.UpdatedAt,
	)
	return m, err
}

// GetByID fetches a map by ID.
func (s *PostgresMapsStore) GetByID(ctx context.Context, exec db.Querier, mapID string) (Map, error) {
	const stmt = `
		SELECT id::text, owner_user_id::text, name, definition, created_at, updated_at
		FROM maps
		WHERE id = $1::uuid
	`
	var m Map
	err := exec.QueryRow(ctx, stmt, mapID).Scan(
		&m.ID, &m.OwnerUserID, &m.Name, &m.Definition, &m.CreatedAt, &m.UpdatedAt,
	)
	return m, err
}

// List returns every map, most recently created first, optionally
// restricted to maps owned by ownerUserID (all maps if empty). There are no
// non-admin readers of this store today, so no further filtering/pagination
// is applied.
func (s *PostgresMapsStore) List(ctx context.Context, exec db.Querier, ownerUserID string) ([]Map, error) {
	stmt := `
		SELECT id::text, owner_user_id::text, name, definition, created_at, updated_at
		FROM maps
	`
	args := make([]any, 0, 1)
	if ownerUserID != "" {
		stmt += " WHERE owner_user_id = $1::uuid"
		args = append(args, ownerUserID)
	}
	stmt += " ORDER BY created_at DESC"

	rows, err := exec.Query(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Map, 0)
	for rows.Next() {
		var m Map
		if err := rows.Scan(&m.ID, &m.OwnerUserID, &m.Name, &m.Definition, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Delete removes a map row. Fails with a foreign_key_violation (pg code
// 23503) if any game still references this map via games.map_id, since
// that column is ON DELETE RESTRICT; callers should translate that into a
// user-facing "map is in use" error (see service.isForeignKeyViolation).
func (s *PostgresMapsStore) Delete(ctx context.Context, exec db.Querier, mapID string) error {
	const stmt = `DELETE FROM maps WHERE id = $1::uuid RETURNING id`
	var id string
	return exec.QueryRow(ctx, stmt, mapID).Scan(&id)
}
