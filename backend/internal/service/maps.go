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
	// mapgen.Generate's validation, or CreateMapFromGraph/UpdateMap's graph
	// fails mapgen.FromGraph's (wraps mapgen.ErrInvalidSpec).
	ErrInvalidMapInput = errors.New("invalid map input")
	// ErrMapInUse is returned by DeleteMap and UpdateMap when one or more
	// games still reference the map (games.map_id is ON DELETE RESTRICT;
	// see also store.PostgresMapsStore.Update).
	ErrMapInUse = errors.New("map is in use by one or more games")
)

type mapsDB interface {
	Queryer() db.Querier
}

// MapsService is the business-logic layer for user-authored custom maps:
// generating a playable risk.Board from a small MapSpec (see
// internal/mapgen) and persisting/listing/deleting the result. Any
// signed-in user may create maps, and every map is readable (and playable)
// by everyone; only its owner or an admin may update or delete it. That
// ownership check lives in httpapi (via CanModifyMap) -- UpdateMap and
// DeleteMap themselves do not re-check the caller.
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
// board/layout — used for list views (the maps page, the create-game map
// picker).
type MapSummary struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	OwnerUserID    string    `json:"owner_user_id"`
	OwnerUserName  string    `json:"owner_user_name"`
	ContinentCount int       `json:"continent_count"`
	TerritoryCount int       `json:"territory_count"`
	CreatedAt      time.Time `json:"created_at"`
}

// MapDetail is the full projection of a stored map, including its playable
// board and visual layout.
type MapDetail struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	OwnerUserID string `json:"owner_user_id"`
	// OwnerUserName is set by GetMap only.
	OwnerUserName string               `json:"owner_user_name"`
	Definition    mapgen.MapDefinition `json:"definition"`
	// GameCount is how many games reference this map. A map with any games
	// is read-only (see UpdateMap); populated by GetMap only, always zero
	// on a freshly created or updated map.
	GameCount int       `json:"game_count"`
	CreatedAt time.Time `json:"created_at"`
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

// GenerateMap runs mapgen.Generate on spec without persisting anything, so
// the map editor can show a generated board as an editable draft (saved
// later through CreateMapFromGraph). The returned detail has no ID. name
// may be empty. It returns a wrapped ErrInvalidMapInput if spec fails
// validation.
func (s *MapsService) GenerateMap(name string, spec mapgen.MapSpec) (MapDetail, error) {
	spec.Name = name
	def, err := mapgen.Generate(spec, nil)
	if err != nil {
		return MapDetail{}, fmt.Errorf("%w: %v", ErrInvalidMapInput, err)
	}
	return MapDetail{Name: name, Definition: def}, nil
}

// CreateMapFromGraph builds a board from a hand-drawn graph (via
// mapgen.FromGraph) and persists it, owned by ownerUserID. It returns a
// wrapped ErrInvalidMapInput if graph fails validation.
func (s *MapsService) CreateMapFromGraph(ctx context.Context, ownerUserID, name string, graph mapgen.MapGraph) (MapDetail, error) {
	if ownerUserID == "" || name == "" {
		return MapDetail{}, ErrInvalidMapInput
	}
	defJSON, err := definitionFromGraph(name, graph)
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

// UpdateMap replaces an existing map's name and board with a hand-drawn
// graph. It returns a wrapped ErrInvalidMapInput if graph fails
// validation, ErrMapNotFound if mapID does not correspond to a stored map,
// or ErrMapInUse if any game references it (games read their layout from
// the map row, so an in-use map is frozen -- save a copy instead).
func (s *MapsService) UpdateMap(ctx context.Context, mapID, name string, graph mapgen.MapGraph) (MapDetail, error) {
	if mapID == "" || name == "" {
		return MapDetail{}, ErrInvalidMapInput
	}
	defJSON, err := definitionFromGraph(name, graph)
	if err != nil {
		return MapDetail{}, err
	}
	row, err := s.maps.Update(ctx, s.db.Queryer(), mapID, store.MapUpdate{Name: name, Definition: defJSON})
	if err == nil {
		return toMapDetail(row)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return MapDetail{}, err
	}
	// Update matches no row both for a missing map and for one in use.
	if _, getErr := s.maps.GetByID(ctx, s.db.Queryer(), mapID); getErr != nil {
		if errors.Is(getErr, pgx.ErrNoRows) {
			return MapDetail{}, ErrMapNotFound
		}
		return MapDetail{}, getErr
	}
	return MapDetail{}, ErrMapInUse
}

func definitionFromGraph(name string, graph mapgen.MapGraph) ([]byte, error) {
	graph.Name = name
	def, err := mapgen.FromGraph(graph)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidMapInput, err)
	}
	return json.Marshal(def)
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
			OwnerUserName:  row.OwnerUserName,
			ContinentCount: len(def.Board.Continents),
			TerritoryCount: len(def.Board.Order),
			CreatedAt:      row.CreatedAt,
		})
	}
	return out, nil
}

// GetMap fetches a single map's full detail (board + layout, plus how many
// games use it) by ID. It
// returns ErrMapNotFound if mapID does not correspond to a stored map.
func (s *MapsService) GetMap(ctx context.Context, mapID string) (MapDetail, error) {
	row, err := s.maps.GetByID(ctx, s.db.Queryer(), mapID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MapDetail{}, ErrMapNotFound
		}
		return MapDetail{}, err
	}
	detail, err := toMapDetail(row)
	if err != nil {
		return MapDetail{}, err
	}
	detail.GameCount, err = s.maps.CountGames(ctx, s.db.Queryer(), mapID)
	if err != nil {
		return MapDetail{}, err
	}
	return detail, nil
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
		ID:            row.ID,
		Name:          row.Name,
		OwnerUserID:   row.OwnerUserID,
		OwnerUserName: row.OwnerUserName,
		Definition:    def,
		CreatedAt:     row.CreatedAt,
	}, nil
}

// CanModifyMap reports whether a user may update or delete the map: its
// owner always can, and admins can modify anyone's.
func CanModifyMap(m MapDetail, userID string, isAdmin bool) bool {
	return isAdmin || (userID != "" && m.OwnerUserID == userID)
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23503"
	}
	return false
}
