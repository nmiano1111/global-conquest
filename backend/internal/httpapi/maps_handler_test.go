package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/game"
	"github.com/nmiano1111/global-conquest/backend/internal/mapgen"
	"github.com/nmiano1111/global-conquest/backend/internal/service"
	"github.com/nmiano1111/global-conquest/backend/internal/store"
)

type fakeMapService struct {
	generateMapFn        func(name string, spec mapgen.MapSpec) (service.MapDetail, error)
	createMapFromGraphFn func(ctx context.Context, ownerUserID, name string, graph mapgen.MapGraph) (service.MapDetail, error)
	updateMapFn          func(ctx context.Context, mapID, name string, graph mapgen.MapGraph) (service.MapDetail, error)
	getMapFn             func(ctx context.Context, mapID string) (service.MapDetail, error)
	deleteMapFn          func(ctx context.Context, mapID string) error
}

func (f *fakeMapService) CreateMap(context.Context, string, string, mapgen.MapSpec) (service.MapDetail, error) {
	return service.MapDetail{}, nil
}

func (f *fakeMapService) GenerateMap(name string, spec mapgen.MapSpec) (service.MapDetail, error) {
	return f.generateMapFn(name, spec)
}

func (f *fakeMapService) CreateMapFromGraph(ctx context.Context, ownerUserID, name string, graph mapgen.MapGraph) (service.MapDetail, error) {
	return f.createMapFromGraphFn(ctx, ownerUserID, name, graph)
}

func (f *fakeMapService) UpdateMap(ctx context.Context, mapID, name string, graph mapgen.MapGraph) (service.MapDetail, error) {
	return f.updateMapFn(ctx, mapID, name, graph)
}

func (f *fakeMapService) ListMaps(context.Context) ([]service.MapSummary, error) { return nil, nil }

func (f *fakeMapService) GetMap(ctx context.Context, mapID string) (service.MapDetail, error) {
	if f.getMapFn != nil {
		return f.getMapFn(ctx, mapID)
	}
	return service.MapDetail{ID: mapID, OwnerUserID: "u1"}, nil
}

func (f *fakeMapService) DeleteMap(ctx context.Context, mapID string) error {
	if f.deleteMapFn != nil {
		return f.deleteMapFn(ctx, mapID)
	}
	return nil
}

// ownedBy makes GetMap report every map as owned by ownerUserID.
func ownedBy(ownerUserID string) func(context.Context, string) (service.MapDetail, error) {
	return func(_ context.Context, mapID string) (service.MapDetail, error) {
		return service.MapDetail{ID: mapID, OwnerUserID: ownerUserID}, nil
	}
}

func newMapsTestRouter(role string, maps *fakeMapService) http.Handler {
	userSvc := &fakeUsersService{
		authenticateSessionFn: func(context.Context, string) (store.User, error) {
			return store.User{ID: "u1", UserName: "root", Role: role}, nil
		},
	}
	h := NewHandler(game.NewServer(), userSvc, &fakeGamesService{}, newFakeChatService())
	h.SetMapsService(maps)
	return NewRouter(h)
}

func sampleMapGraphBody() map[string]any {
	return map[string]any{
		"name":       "Drawn",
		"continents": []map[string]any{{"name": "Redland", "bonus": 2}, {"name": "Blueland", "bonus": 1}},
		"territories": []map[string]any{
			{"name": "R1", "continent": "Redland", "x": 10, "y": 20},
			{"name": "B1", "continent": "Blueland", "x": 30, "y": 40},
		},
		"edges": [][2]string{{"R1", "B1"}},
	}
}

func TestCreateMapFromGraphSuccess(t *testing.T) {
	var gotOwner, gotName string
	var gotGraph mapgen.MapGraph
	// Map building is open to every signed-in user, not just admins.
	router := newMapsTestRouter("player", &fakeMapService{
		createMapFromGraphFn: func(_ context.Context, ownerUserID, name string, graph mapgen.MapGraph) (service.MapDetail, error) {
			gotOwner, gotName, gotGraph = ownerUserID, name, graph
			return service.MapDetail{ID: "m1", Name: name}, nil
		},
	})

	rr := doJSONWithAuth(t, router, http.MethodPost, "/api/maps/graph", "valid-token", sampleMapGraphBody())
	if rr.code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.code, rr.body.String())
	}
	if gotOwner != "u1" || gotName != "Drawn" {
		t.Fatalf("unexpected owner/name: %q %q", gotOwner, gotName)
	}
	if len(gotGraph.Territories) != 2 || gotGraph.Territories[1] != (mapgen.GraphTerritory{Name: "B1", Continent: "Blueland", X: 30, Y: 40}) {
		t.Fatalf("unexpected territories: %+v", gotGraph.Territories)
	}
	if len(gotGraph.Edges) != 1 || gotGraph.Edges[0] != [2]string{"R1", "B1"} {
		t.Fatalf("unexpected edges: %+v", gotGraph.Edges)
	}
}

func TestCreateMapFromGraphRejectsTooFewContinents(t *testing.T) {
	router := newMapsTestRouter("admin", &fakeMapService{})
	body := sampleMapGraphBody()
	body["continents"] = []map[string]any{{"name": "Redland", "bonus": 2}}
	if rr := doJSONWithAuth(t, router, http.MethodPost, "/api/maps/graph", "valid-token", body); rr.code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.code, rr.body.String())
	}
}

func TestUpdateMapErrors(t *testing.T) {
	cases := map[string]struct {
		err  error
		want int
	}{
		"invalid graph": {err: service.ErrInvalidMapInput, want: http.StatusBadRequest},
		"not found":     {err: service.ErrMapNotFound, want: http.StatusNotFound},
		"in use":        {err: service.ErrMapInUse, want: http.StatusConflict},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			router := newMapsTestRouter("admin", &fakeMapService{
				updateMapFn: func(context.Context, string, string, mapgen.MapGraph) (service.MapDetail, error) {
					return service.MapDetail{}, tc.err
				},
			})
			if rr := doJSONWithAuth(t, router, http.MethodPut, "/api/maps/m1", "valid-token", sampleMapGraphBody()); rr.code != tc.want {
				t.Fatalf("expected %d, got %d body=%s", tc.want, rr.code, rr.body.String())
			}
		})
	}
}

func TestUpdateMapSuccess(t *testing.T) {
	var gotID string
	router := newMapsTestRouter("admin", &fakeMapService{
		updateMapFn: func(_ context.Context, mapID, name string, _ mapgen.MapGraph) (service.MapDetail, error) {
			gotID = mapID
			return service.MapDetail{ID: mapID, Name: name}, nil
		},
	})
	if rr := doJSONWithAuth(t, router, http.MethodPut, "/api/maps/m1", "valid-token", sampleMapGraphBody()); rr.code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.code, rr.body.String())
	}
	if gotID != "m1" {
		t.Fatalf("expected map id m1, got %q", gotID)
	}
}

func sampleMapSpecBody() map[string]any {
	return map[string]any{
		"continents": []map[string]any{
			{"name": "Redland", "bonus": 2, "territory_count": 3},
			{"name": "Blueland", "bonus": 1, "territory_count": 2},
		},
		"borders": []map[string]any{{"a": "Redland", "b": "Blueland", "crossings": 1}},
	}
}

func TestGenerateMapSuccessWithoutName(t *testing.T) {
	var gotSpec mapgen.MapSpec
	router := newMapsTestRouter("player", &fakeMapService{
		generateMapFn: func(name string, spec mapgen.MapSpec) (service.MapDetail, error) {
			gotSpec = spec
			return service.MapDetail{Name: name}, nil
		},
	})
	rr := doJSONWithAuth(t, router, http.MethodPost, "/api/maps/generate", "valid-token", sampleMapSpecBody())
	if rr.code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.code, rr.body.String())
	}
	if len(gotSpec.Continents) != 2 || gotSpec.Continents[0] != (mapgen.ContinentSpec{Name: "Redland", Bonus: 2, TerritoryCount: 3}) {
		t.Fatalf("unexpected continents: %+v", gotSpec.Continents)
	}
	if len(gotSpec.Borders) != 1 || gotSpec.Borders[0].A != "Redland" || gotSpec.Borders[0].Crossings != 1 {
		t.Fatalf("unexpected borders: %+v", gotSpec.Borders)
	}
}

func TestGenerateMapInvalidSpec(t *testing.T) {
	router := newMapsTestRouter("admin", &fakeMapService{
		generateMapFn: func(string, mapgen.MapSpec) (service.MapDetail, error) {
			return service.MapDetail{}, service.ErrInvalidMapInput
		},
	})
	if rr := doJSONWithAuth(t, router, http.MethodPost, "/api/maps/generate", "valid-token", sampleMapSpecBody()); rr.code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.code, rr.body.String())
	}
}

func TestMapRoutesRequireAuth(t *testing.T) {
	router := newMapsTestRouter("player", &fakeMapService{})
	if rr := doJSON(t, router, http.MethodGet, "/api/maps", nil); rr.code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rr.code, rr.body.String())
	}
}

func TestUpdateMapOwnership(t *testing.T) {
	cases := map[string]struct {
		role, owner string
		want        int
	}{
		"owner":           {role: "player", owner: "u1", want: http.StatusOK},
		"other player":    {role: "player", owner: "u2", want: http.StatusForbidden},
		"admin, not mine": {role: "admin", owner: "u2", want: http.StatusOK},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			updated := false
			router := newMapsTestRouter(tc.role, &fakeMapService{
				getMapFn: ownedBy(tc.owner),
				updateMapFn: func(_ context.Context, mapID, name string, _ mapgen.MapGraph) (service.MapDetail, error) {
					updated = true
					return service.MapDetail{ID: mapID, Name: name}, nil
				},
			})
			if rr := doJSONWithAuth(t, router, http.MethodPut, "/api/maps/m1", "valid-token", sampleMapGraphBody()); rr.code != tc.want {
				t.Fatalf("expected %d, got %d body=%s", tc.want, rr.code, rr.body.String())
			}
			if updated != (tc.want == http.StatusOK) {
				t.Fatalf("UpdateMap called=%v for status %d", updated, tc.want)
			}
		})
	}
}

func TestDeleteMapOwnership(t *testing.T) {
	cases := map[string]struct {
		role, owner string
		want        int
	}{
		"owner":           {role: "player", owner: "u1", want: http.StatusNoContent},
		"other player":    {role: "player", owner: "u2", want: http.StatusForbidden},
		"admin, not mine": {role: "admin", owner: "u2", want: http.StatusNoContent},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			deleted := false
			router := newMapsTestRouter(tc.role, &fakeMapService{
				getMapFn:    ownedBy(tc.owner),
				deleteMapFn: func(context.Context, string) error { deleted = true; return nil },
			})
			if rr := doJSONWithAuth(t, router, http.MethodDelete, "/api/maps/m1", "valid-token", nil); rr.code != tc.want {
				t.Fatalf("expected %d, got %d body=%s", tc.want, rr.code, rr.body.String())
			}
			if deleted != (tc.want == http.StatusNoContent) {
				t.Fatalf("DeleteMap called=%v for status %d", deleted, tc.want)
			}
		})
	}
}

func TestDeleteMapNotFound(t *testing.T) {
	router := newMapsTestRouter("player", &fakeMapService{
		getMapFn: func(context.Context, string) (service.MapDetail, error) {
			return service.MapDetail{}, service.ErrMapNotFound
		},
	})
	if rr := doJSONWithAuth(t, router, http.MethodDelete, "/api/maps/missing", "valid-token", nil); rr.code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rr.code, rr.body.String())
	}
}
