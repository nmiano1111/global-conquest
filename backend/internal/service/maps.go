package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/nmiano1111/global-conquest/backend/internal/db"
	"github.com/nmiano1111/global-conquest/backend/internal/mapgen"
	"github.com/nmiano1111/global-conquest/backend/internal/store"
)

var (
	// ErrMapNotFound is returned when a lookup targets a map ID that does
	// not exist.
	ErrMapNotFound = errors.New("map not found")
	// ErrInvalidMapInput is returned when CreateMap's spec fails
	// mapgen.Generate's validation (wraps mapgen.ErrInvalidSpec).
	ErrInvalidMapInput = errors.New("invalid map input")
	// ErrMapInUse is returned by DeleteMap when one or more games still
	// reference the map (games.map_id is ON DELETE RESTRICT).
	ErrMapInUse = errors.New("map is in use by one or more games")
)

type mapsDB interface {
	Queryer() db.Querier
}

// MapsService is the business-logic layer for admin-authored custom maps:
// generating a playable risk.Board from a small MapSpec (see
// internal/mapgen) and persisting/listing/deleting the result. Every
// exported method here is only ever reachable through admin-gated HTTP
// routes (see httpapi's admin.Group("/maps")); this service itself does
// not re-check the caller's role.
type MapsService struct {
	db   mapsDB
	maps store.MapsStore
}

// NewMapsService constructs a MapsService backed by the given database and
// maps store.
func NewMapsService(db mapsDB, maps store.MapsStore) *MapsService {
	return &MapsService{db: db, maps: maps}
}

// MapSummary is a lightweight projection of a stored map, omitting the full
// board/layout — used for list views (the admin maps table, the
// create-game map picker).
type MapSummary struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	OwnerUserID    string    `json:"owner_user_id"`
	ContinentCount int       `json:"continent_count"`
	TerritoryCount int       `json:"territory_count"`
	CreatedAt      time.Time `json:"created_at"`
}

// MapDetail is the full projection of a stored map, including its playable
// board and visual layout.
type MapDetail struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	OwnerUserID string               `json:"owner_user_id"`
	Definition  mapgen.MapDefinition `json:"definition"`
	CreatedAt   time.Time            `json:"created_at"`
}

// CreateMap generates a new board from spec (via mapgen.Generate) and
// persists it, owned by ownerUserID. It returns a wrapped
// ErrInvalidMapInput if spec fails validation.
func (s *MapsService) CreateMap(ctx context.Context, ownerUserID, name string, spec mapgen.MapSpec) (MapDetail, error) {
	if ownerUserID == "" || name == "" {
		return MapDetail{}, ErrInvalidMapInput
	}
	spec.Name = name

	def, err := mapgen.Generate(spec, nil)
	if err != nil {
		return MapDetail{}, fmt.Errorf("%w: %v", ErrInvalidMapInput, err)
	}

	defJSON, err := json.Marshal(def)
	if err != nil {
		return MapDetail{}, err
	}

	row, err := s.maps.Create(ctx, s.db.Queryer(), store.NewMap{
		OwnerUserID: ownerUserID,
		Name:        name,
		Definition:  defJSON,
	})
	if err != nil {
		return MapDetail{}, err
	}
	return toMapDetail(row)
}

// ListMaps returns every stored map as a summary, most recently created
// first.
func (s *MapsService) ListMaps(ctx context.Context) ([]MapSummary, error) {
	rows, err := s.maps.List(ctx, s.db.Queryer(), "")
	if err != nil {
		return nil, err
	}
	out := make([]MapSummary, 0, len(rows))
	for _, row := range rows {
		var def mapgen.MapDefinition
		if err := json.Unmarshal(row.Definition, &def); err != nil {
			return nil, err
		}
		out = append(out, MapSummary{
			ID:             row.ID,
			Name:           row.Name,
			OwnerUserID:    row.OwnerUserID,
			ContinentCount: len(def.Board.Continents),
			TerritoryCount: len(def.Board.Order),
			CreatedAt:      row.CreatedAt,
		})
	}
	return out, nil
}

// GetMap fetches a single map's full detail (board + layout) by ID. It
// returns ErrMapNotFound if mapID does not correspond to a stored map.
func (s *MapsService) GetMap(ctx context.Context, mapID string) (MapDetail, error) {
	row, err := s.maps.GetByID(ctx, s.db.Queryer(), mapID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MapDetail{}, ErrMapNotFound
		}
		return MapDetail{}, err
	}
	return toMapDetail(row)
}

// DeleteMap permanently deletes a map. It returns ErrMapNotFound if mapID
// does not correspond to a stored map, or ErrMapInUse if one or more games
// still reference it.
func (s *MapsService) DeleteMap(ctx context.Context, mapID string) error {
	err := s.maps.Delete(ctx, s.db.Queryer(), mapID)
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMapNotFound
	}
	if isForeignKeyViolation(err) {
		return ErrMapInUse
	}
	return err
}

func toMapDetail(row store.Map) (MapDetail, error) {
	var def mapgen.MapDefinition
	if err := json.Unmarshal(row.Definition, &def); err != nil {
		return MapDetail{}, err
	}
	return MapDetail{
		ID:          row.ID,
		Name:        row.Name,
		OwnerUserID: row.OwnerUserID,
		Definition:  def,
		CreatedAt:   row.CreatedAt,
	}, nil
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23503"
	}
	return false
}
