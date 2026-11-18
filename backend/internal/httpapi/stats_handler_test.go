package httpapi

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/game"
	"github.com/nmiano1111/global-conquest/backend/internal/service"
	"github.com/nmiano1111/global-conquest/backend/internal/store"
)

type fakeStatsService struct {
	lastFilter store.GameResultsFilter
	lastKey    string
	detailErr  error
}

func (f *fakeStatsService) Overview(_ context.Context, fl store.GameResultsFilter) (service.StatsOverview, error) {
	f.lastFilter = fl
	return service.StatsOverview{}, nil
}

func (f *fakeStatsService) Players(_ context.Context, fl store.GameResultsFilter) ([]service.PlayerSummary, error) {
	f.lastFilter = fl
	return []service.PlayerSummary{}, nil
}

func (f *fakeStatsService) PlayerDetail(_ context.Context, key string, fl store.GameResultsFilter) (service.PlayerDetail, error) {
	f.lastKey, f.lastFilter = key, fl
	return service.PlayerDetail{}, f.detailErr
}

func newStatsTestRouter(role string, stats *fakeStatsService) http.Handler {
	userSvc := &fakeUsersService{
		authenticateSessionFn: func(context.Context, string) (store.User, error) {
			return store.User{ID: "u1", UserName: "root", Role: role}, nil
		},
	}
	h := NewHandler(game.NewServer(), userSvc, &fakeGamesService{}, newFakeChatService())
	h.SetStatsService(stats)
	return NewRouter(h)
}

func TestStatsForbiddenForNonAdmin(t *testing.T) {
	router := newStatsTestRouter("player", &fakeStatsService{})
	for _, path := range []string{"/api/admin/stats/overview", "/api/admin/stats/players", "/api/admin/stats/players/u1"} {
		if rr := doJSONWithAuth(t, router, http.MethodGet, path, "valid-token", nil); rr.code != http.StatusForbidden {
			t.Fatalf("%s: expected 403, got %d", path, rr.code)
		}
	}
}

func TestStatsFilterParsing(t *testing.T) {
	stats := &fakeStatsService{}
	router := newStatsTestRouter("admin", stats)
	rr := doJSONWithAuth(t, router, http.MethodGet,
		"/api/admin/stats/players?composition=mixed&map=classic&player_count=4&include_sandbox=true&from=2026-01-01&to=2026-01-31",
		"valid-token", nil)
	if rr.code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.code, rr.body.String())
	}
	f := stats.lastFilter
	if f.Composition != store.CompositionMixed || f.Map != store.MapFilterClassic || f.PlayerCount != 4 || !f.IncludeSandboxed {
		t.Fatalf("filter = %+v", f)
	}
	if !f.From.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) || !f.To.Equal(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("date range = %v..%v (bare 'to' date should be inclusive)", f.From, f.To)
	}
}

func TestStatsFilterRejectsBadValues(t *testing.T) {
	router := newStatsTestRouter("admin", &fakeStatsService{})
	for _, q := range []string{"composition=nope", "map=moon", "player_count=x", "include_sandbox=maybe", "from=yesterday"} {
		if rr := doJSONWithAuth(t, router, http.MethodGet, "/api/admin/stats/overview?"+q, "valid-token", nil); rr.code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d", q, rr.code)
		}
	}
}

func TestStatsPlayerDetailBotKeyAndNotFound(t *testing.T) {
	stats := &fakeStatsService{}
	router := newStatsTestRouter("admin", stats)
	if rr := doJSONWithAuth(t, router, http.MethodGet, "/api/admin/stats/players/bot:killbot-v2", "valid-token", nil); rr.code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.code)
	}
	if stats.lastKey != "bot:killbot-v2" {
		t.Fatalf("key = %q", stats.lastKey)
	}
	stats.detailErr = service.ErrStatsPlayerNotFound
	if rr := doJSONWithAuth(t, router, http.MethodGet, "/api/admin/stats/players/ghost", "valid-token", nil); rr.code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.code)
	}
}
