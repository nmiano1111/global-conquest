package httpapi

import (
	"backend/internal/auth"
	"backend/internal/game"
	"backend/internal/service"
	"backend/internal/store"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

type fakeUsersService struct {
	createUserFn func(ctx context.Context, userName, password string) (store.User, error)
	getUserFn    func(ctx context.Context, userName string) (store.User, error)
	loginFn      func(ctx context.Context, userName, password string) (service.LoginResult, error)
}

type fakeGamesService struct {
	createClassicGameFn func(ctx context.Context, ownerUserID string, playerIDs []string) (store.Game, error)
	getGameFn           func(ctx context.Context, gameID string) (store.Game, error)
	listGamesFn         func(ctx context.Context, ownerUserID, status string, limit, offset int) ([]store.Game, error)
	updateGameStateFn   func(ctx context.Context, gameID, status string, state json.RawMessage) (store.Game, error)
}

func (f *fakeUsersService) CreateUser(ctx context.Context, userName, password string) (store.User, error) {
	return f.createUserFn(ctx, userName, password)
}

func (f *fakeUsersService) GetUser(ctx context.Context, userName string) (store.User, error) {
	return f.getUserFn(ctx, userName)
}

func (f *fakeUsersService) Login(ctx context.Context, userName, password string) (service.LoginResult, error) {
	return f.loginFn(ctx, userName, password)
}

func (f *fakeGamesService) CreateClassicGame(ctx context.Context, ownerUserID string, playerIDs []string) (store.Game, error) {
	return f.createClassicGameFn(ctx, ownerUserID, playerIDs)
}

func (f *fakeGamesService) GetGame(ctx context.Context, gameID string) (store.Game, error) {
	return f.getGameFn(ctx, gameID)
}

func (f *fakeGamesService) ListGames(ctx context.Context, ownerUserID, status string, limit, offset int) ([]store.Game, error) {
	return f.listGamesFn(ctx, ownerUserID, status, limit, offset)
}

func (f *fakeGamesService) UpdateGameState(ctx context.Context, gameID, status string, state json.RawMessage) (store.Game, error) {
	return f.updateGameStateFn(ctx, gameID, status, state)
}

func newTestRouterWithServices(userSvc *fakeUsersService, games *fakeGamesService) http.Handler {
	h := NewHandler(game.NewServer(), userSvc, games)
	return NewRouter(h)
}

func newTestRouter(svc *fakeUsersService) http.Handler {
	games := &fakeGamesService{
		createClassicGameFn: func(context.Context, string, []string) (store.Game, error) { return store.Game{}, nil },
		getGameFn:           func(context.Context, string) (store.Game, error) { return store.Game{}, nil },
		listGamesFn:         func(context.Context, string, string, int, int) ([]store.Game, error) { return nil, nil },
		updateGameStateFn:   func(context.Context, string, string, json.RawMessage) (store.Game, error) { return store.Game{}, nil },
	}
	return newTestRouterWithServices(svc, games)
}

type responseRecorder struct {
	code int
	h    http.Header
	body bytes.Buffer
}

func newRecorder() *responseRecorder {
	return &responseRecorder{
		h: make(http.Header),
	}
}

func (r *responseRecorder) Header() http.Header {
	return r.h
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.code == 0 {
		r.code = http.StatusOK
	}
	return r.body.Write(b)
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.code = statusCode
}

func doJSON(t *testing.T, router http.Handler, method, path string, body any) *responseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req, err := http.NewRequest(method, path, &buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	rr := newRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func TestPingEndpoint(t *testing.T) {
	router := newTestRouter(&fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		getUserFn:    func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:      func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	})

	rr := doJSON(t, router, http.MethodGet, "/api/ping", nil)
	if rr.code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.code)
	}
}

func TestCreateUserSuccess(t *testing.T) {
	now := time.Now().UTC()
	router := newTestRouter(&fakeUsersService{
		createUserFn: func(_ context.Context, userName, password string) (store.User, error) {
			if userName != "alice" || password != "strong-password" {
				t.Fatalf("unexpected credentials: %q %q", userName, password)
			}
			return store.User{ID: "u1", UserName: "alice", Role: "player", CreatedAt: now, UpdatedAt: now}, nil
		},
		getUserFn: func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:   func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	})

	rr := doJSON(t, router, http.MethodPost, "/api/users/", map[string]string{
		"username": "alice",
		"password": "strong-password",
	})
	if rr.code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.code, rr.body.String())
	}
}

func TestCreateUserValidationError(t *testing.T) {
	router := newTestRouter(&fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) {
			t.Fatalf("create should not be called on bad payload")
			return store.User{}, nil
		},
		getUserFn: func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:   func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	})

	rr := doJSON(t, router, http.MethodPost, "/api/users/", map[string]string{
		"username": "alice",
	})
	if rr.code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.code)
	}
}

func TestCreateUserConflict(t *testing.T) {
	router := newTestRouter(&fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) {
			return store.User{}, service.ErrUsernameTaken
		},
		getUserFn: func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:   func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	})

	rr := doJSON(t, router, http.MethodPost, "/api/users/", map[string]string{
		"username": "alice",
		"password": "strong-password",
	})
	if rr.code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rr.code)
	}
}

func TestLoginUnauthorized(t *testing.T) {
	router := newTestRouter(&fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		getUserFn:    func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn: func(context.Context, string, string) (service.LoginResult, error) {
			return service.LoginResult{}, auth.ErrInvalidUsernameOrPassword
		},
	})

	rr := doJSON(t, router, http.MethodPost, "/api/auth/login", map[string]string{
		"username": "alice",
		"password": "wrong-password",
	})
	if rr.code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.code)
	}
}

func TestLoginSuccess(t *testing.T) {
	now := time.Now().UTC().Add(30 * 24 * time.Hour)
	router := newTestRouter(&fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		getUserFn:    func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn: func(_ context.Context, userName, password string) (service.LoginResult, error) {
			if userName != "alice" || password != "strong-password" {
				t.Fatalf("unexpected credentials")
			}
			return service.LoginResult{
				User:      store.User{ID: "u1", UserName: "alice", Role: "player"},
				Token:     "session-token",
				ExpiresAt: now,
			}, nil
		},
	})

	rr := doJSON(t, router, http.MethodPost, "/api/auth/login", map[string]string{
		"username": "alice",
		"password": "strong-password",
	})
	if rr.code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.code, rr.body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rr.body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["token"] != "session-token" {
		t.Fatalf("unexpected token: %#v", body["token"])
	}
}

func TestGetUserNotFound(t *testing.T) {
	router := newTestRouter(&fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		getUserFn: func(context.Context, string) (store.User, error) {
			return store.User{}, errors.New("not found")
		},
		loginFn: func(context.Context, string, string) (service.LoginResult, error) {
			return service.LoginResult{}, nil
		},
	})

	rr := doJSON(t, router, http.MethodGet, "/api/users/missing", nil)
	if rr.code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.code)
	}
}

func TestCreateGameSuccess(t *testing.T) {
	userSvc := &fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		getUserFn:    func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:      func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	}
	gamesSvc := &fakeGamesService{
		createClassicGameFn: func(_ context.Context, ownerUserID string, playerIDs []string) (store.Game, error) {
			if ownerUserID != "u1" || len(playerIDs) != 3 {
				t.Fatalf("unexpected create game input")
			}
			return store.Game{ID: "g1", OwnerUserID: "u1", Status: "lobby", State: json.RawMessage(`{"phase":"setup_claim"}`)}, nil
		},
		getGameFn:         func(context.Context, string) (store.Game, error) { return store.Game{}, nil },
		listGamesFn:       func(context.Context, string, string, int, int) ([]store.Game, error) { return nil, nil },
		updateGameStateFn: func(context.Context, string, string, json.RawMessage) (store.Game, error) { return store.Game{}, nil },
	}
	router := newTestRouterWithServices(userSvc, gamesSvc)

	rr := doJSON(t, router, http.MethodPost, "/api/games/", map[string]any{
		"owner_user_id": "u1",
		"player_ids":    []string{"u1", "u2", "u3"},
	})
	if rr.code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.code, rr.body.String())
	}
}

func TestGetGameNotFound(t *testing.T) {
	userSvc := &fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		getUserFn:    func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:      func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	}
	gamesSvc := &fakeGamesService{
		createClassicGameFn: func(context.Context, string, []string) (store.Game, error) { return store.Game{}, nil },
		getGameFn:           func(context.Context, string) (store.Game, error) { return store.Game{}, service.ErrGameNotFound },
		listGamesFn:         func(context.Context, string, string, int, int) ([]store.Game, error) { return nil, nil },
		updateGameStateFn:   func(context.Context, string, string, json.RawMessage) (store.Game, error) { return store.Game{}, nil },
	}
	router := newTestRouterWithServices(userSvc, gamesSvc)

	rr := doJSON(t, router, http.MethodGet, "/api/games/g_missing", nil)
	if rr.code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.code)
	}
}

func TestUpdateGameStateSuccess(t *testing.T) {
	userSvc := &fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		getUserFn:    func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:      func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	}
	gamesSvc := &fakeGamesService{
		createClassicGameFn: func(context.Context, string, []string) (store.Game, error) { return store.Game{}, nil },
		getGameFn:           func(context.Context, string) (store.Game, error) { return store.Game{}, nil },
		listGamesFn:         func(context.Context, string, string, int, int) ([]store.Game, error) { return nil, nil },
		updateGameStateFn: func(_ context.Context, gameID, status string, state json.RawMessage) (store.Game, error) {
			if gameID != "g1" || status != "in_progress" || len(state) == 0 {
				t.Fatalf("unexpected update input")
			}
			return store.Game{ID: "g1", Status: "in_progress", State: state}, nil
		},
	}
	router := newTestRouterWithServices(userSvc, gamesSvc)

	rr := doJSON(t, router, http.MethodPut, "/api/games/g1/state", map[string]any{
		"status": "in_progress",
		"state":  map[string]any{"turn": 2},
	})
	if rr.code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.code, rr.body.String())
	}
}

func TestListGamesSuccess(t *testing.T) {
	userSvc := &fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		getUserFn:    func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:      func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	}
	gamesSvc := &fakeGamesService{
		createClassicGameFn: func(context.Context, string, []string) (store.Game, error) { return store.Game{}, nil },
		getGameFn:           func(context.Context, string) (store.Game, error) { return store.Game{}, nil },
		listGamesFn: func(_ context.Context, owner, status string, limit, offset int) ([]store.Game, error) {
			if owner != "u1" || status != "lobby" || limit != 10 || offset != 5 {
				t.Fatalf("unexpected list filters")
			}
			return []store.Game{{ID: "g1", OwnerUserID: "u1", Status: "lobby"}}, nil
		},
		updateGameStateFn: func(context.Context, string, string, json.RawMessage) (store.Game, error) { return store.Game{}, nil },
	}
	router := newTestRouterWithServices(userSvc, gamesSvc)

	rr := doJSON(t, router, http.MethodGet, "/api/games/?owner_user_id=u1&status=lobby&limit=10&offset=5", nil)
	if rr.code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.code, rr.body.String())
	}
}

func TestListGamesBadLimit(t *testing.T) {
	userSvc := &fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		getUserFn:    func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:      func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	}
	gamesSvc := &fakeGamesService{
		createClassicGameFn: func(context.Context, string, []string) (store.Game, error) { return store.Game{}, nil },
		getGameFn:           func(context.Context, string) (store.Game, error) { return store.Game{}, nil },
		listGamesFn: func(context.Context, string, string, int, int) ([]store.Game, error) {
			t.Fatalf("list should not be called")
			return nil, nil
		},
		updateGameStateFn: func(context.Context, string, string, json.RawMessage) (store.Game, error) { return store.Game{}, nil },
	}
	router := newTestRouterWithServices(userSvc, gamesSvc)

	rr := doJSON(t, router, http.MethodGet, "/api/games/?limit=bad", nil)
	if rr.code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.code)
	}
}
