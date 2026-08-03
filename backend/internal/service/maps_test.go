package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/nmiano1111/global-conquest/backend/internal/db"
	"github.com/nmiano1111/global-conquest/backend/internal/mapgen"
	"github.com/nmiano1111/global-conquest/backend/internal/store"
)

type fakeMapsStore struct {
	createFn  func(context.Context, db.Querier, store.NewMap) (store.Map, error)
	getByIDFn func(context.Context, db.Querier, string) (store.Map, error)
	listFn    func(context.Context, db.Querier, string) ([]store.Map, error)
	deleteFn  func(context.Context, db.Querier, string) error
}

func (f *fakeMapsStore) Create(ctx context.Context, q db.Querier, in store.NewMap) (store.Map, error) {
	return f.createFn(ctx, q, in)
}

func (f *fakeMapsStore) GetByID(ctx context.Context, q db.Querier, mapID string) (store.Map, error) {
	return f.getByIDFn(ctx, q, mapID)
}

func (f *fakeMapsStore) List(ctx context.Context, q db.Querier, ownerUserID string) ([]store.Map, error) {
	return f.listFn(ctx, q, ownerUserID)
}

func (f *fakeMapsStore) Delete(ctx context.Context, q db.Querier, mapID string) error {
	return f.deleteFn(ctx, q, mapID)
}

func sampleMapSpec() mapgen.MapSpec {
	return mapgen.MapSpec{
		Continents: []mapgen.ContinentSpec{
			{Name: "Redland", Bonus: 2, TerritoryCount: 3},
			{Name: "Blueland", Bonus: 1, TerritoryCount: 3},
		},
		Borders: []mapgen.ContinentBorder{
			{A: "Redland", B: "Blueland", Crossings: 1},
		},
	}
}

func TestCreateMap(t *testing.T) {
	var created store.NewMap
	fake := &fakeMapsStore{
		createFn: func(_ context.Context, _ db.Querier, in store.NewMap) (store.Map, error) {
			created = in
			return store.Map{ID: "m1", OwnerUserID: in.OwnerUserID, Name: in.Name, Definition: in.Definition}, nil
		},
	}
	svc := NewMapsService(&fakeDB{q: countQuerier{}}, fake)

	out, err := svc.CreateMap(context.Background(), "admin1", "My Map", sampleMapSpec())
	if err != nil {
		t.Fatalf("CreateMap: %v", err)
	}
	if out.ID != "m1" || out.Name != "My Map" || out.OwnerUserID != "admin1" {
		t.Fatalf("unexpected output: %#v", out)
	}
	if len(out.Definition.Board.Order) != 6 {
		t.Fatalf("expected 6 territories, got %d", len(out.Definition.Board.Order))
	}
	if created.OwnerUserID != "admin1" || created.Name != "My Map" {
		t.Fatalf("unexpected persisted input: %#v", created)
	}
	var roundTrip mapgen.MapDefinition
	if err := json.Unmarshal(created.Definition, &roundTrip); err != nil {
		t.Fatalf("stored definition did not round-trip: %v", err)
	}
}

func TestCreateMap_InvalidSpecRejected(t *testing.T) {
	fake := &fakeMapsStore{
		createFn: func(context.Context, db.Querier, store.NewMap) (store.Map, error) {
			t.Fatalf("create should not be called for an invalid spec")
			return store.Map{}, nil
		},
	}
	svc := NewMapsService(&fakeDB{q: countQuerier{}}, fake)

	_, err := svc.CreateMap(context.Background(), "admin1", "Bad Map", mapgen.MapSpec{})
	if !errors.Is(err, ErrInvalidMapInput) {
		t.Fatalf("expected ErrInvalidMapInput, got %v", err)
	}
}

func TestCreateMap_RequiresOwnerAndName(t *testing.T) {
	svc := NewMapsService(&fakeDB{q: countQuerier{}}, &fakeMapsStore{})

	if _, err := svc.CreateMap(context.Background(), "", "name", sampleMapSpec()); !errors.Is(err, ErrInvalidMapInput) {
		t.Fatalf("expected ErrInvalidMapInput for empty owner, got %v", err)
	}
	if _, err := svc.CreateMap(context.Background(), "admin1", "", sampleMapSpec()); !errors.Is(err, ErrInvalidMapInput) {
		t.Fatalf("expected ErrInvalidMapInput for empty name, got %v", err)
	}
}

func TestGetMap_NotFound(t *testing.T) {
	fake := &fakeMapsStore{
		getByIDFn: func(context.Context, db.Querier, string) (store.Map, error) {
			return store.Map{}, pgx.ErrNoRows
		},
	}
	svc := NewMapsService(&fakeDB{q: countQuerier{}}, fake)

	_, err := svc.GetMap(context.Background(), "missing")
	if !errors.Is(err, ErrMapNotFound) {
		t.Fatalf("expected ErrMapNotFound, got %v", err)
	}
}

func TestListMaps(t *testing.T) {
	def, _ := mapgen.Generate(sampleMapSpec(), nil)
	defJSON, _ := json.Marshal(def)
	fake := &fakeMapsStore{
		listFn: func(context.Context, db.Querier, string) ([]store.Map, error) {
			return []store.Map{
				{ID: "m1", Name: "Map One", OwnerUserID: "admin1", Definition: defJSON},
			}, nil
		},
	}
	svc := NewMapsService(&fakeDB{q: countQuerier{}}, fake)

	out, err := svc.ListMaps(context.Background())
	if err != nil {
		t.Fatalf("ListMaps: %v", err)
	}
	if len(out) != 1 || out[0].Name != "Map One" || out[0].ContinentCount != 2 || out[0].TerritoryCount != 6 {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestDeleteMap_InUse(t *testing.T) {
	fake := &fakeMapsStore{
		deleteFn: func(context.Context, db.Querier, string) error {
			return &pgconn.PgError{Code: "23503"}
		},
	}
	svc := NewMapsService(&fakeDB{q: countQuerier{}}, fake)

	err := svc.DeleteMap(context.Background(), "m1")
	if !errors.Is(err, ErrMapInUse) {
		t.Fatalf("expected ErrMapInUse, got %v", err)
	}
}

func TestDeleteMap_NotFound(t *testing.T) {
	fake := &fakeMapsStore{
		deleteFn: func(context.Context, db.Querier, string) error {
			return pgx.ErrNoRows
		},
	}
	svc := NewMapsService(&fakeDB{q: countQuerier{}}, fake)

	err := svc.DeleteMap(context.Background(), "missing")
	if !errors.Is(err, ErrMapNotFound) {
		t.Fatalf("expected ErrMapNotFound, got %v", err)
	}
}

func TestDeleteMap_Success(t *testing.T) {
	fake := &fakeMapsStore{
		deleteFn: func(context.Context, db.Querier, string) error {
			return nil
		},
	}
	svc := NewMapsService(&fakeDB{q: countQuerier{}}, fake)

	if err := svc.DeleteMap(context.Background(), "m1"); err != nil {
		t.Fatalf("DeleteMap: %v", err)
	}
}
