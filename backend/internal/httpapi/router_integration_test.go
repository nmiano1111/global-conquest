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
	createUserFn          func(ctx context.Context, userName, password string) (store.User, error)
	listUsersFn           func(ctx context.Context) ([]store.User, error)
	listAdminUsersFn      func(ctx context.Context) ([]store.AdminUser, error)
	getUserFn             func(ctx context.Context, userName string) (store.User, error)
	updateUserAccessFn    func(ctx context.Context, userID, accessStatus string) (store.User, error)
	revokeUserSessionsFn  func(ctx context.Context, userID string) (int64, error)
	authenticateSessionFn func(ctx context.Context, token string) (store.User, error)
	loginFn               func(ctx context.Context, userName, password string) (service.LoginResult, error)
}

type fakeGamesService struct {
	createClassicGameFn func(ctx context.Context, ownerUserID string, playerCount int) (store.Game, error)
	joinClassicGameFn   func(ctx context.Context, gameID, playerID string) (store.Game, error)
	getGameFn           func(ctx context.Context, gameID string) (store.Game, error)
	getGameBootstrapFn  func(ctx context.Context, gameID, requesterUserID string) (service.GameBootstrap, error)
	listGamesFn         func(ctx context.Context, ownerUserID, status string, limit, offset int) ([]store.Game, error)
	updateGameStateFn   func(ctx context.Context, gameID, status string, state json.RawMessage) (store.Game, error)
}

type fakeChatService struct {
	listLobbyMessagesFn func(ctx context.Context, limit int) ([]store.ChatMessage, error)
	postLobbyMessageFn  func(ctx context.Context, userID, body string) (store.ChatMessage, error)
}

func (f *fakeUsersService) CreateUser(ctx context.Context, userName, password string) (store.User, error) {
	return f.createUserFn(ctx, userName, password)
}

func (f *fakeUsersService) ListUsers(ctx context.Context) ([]store.User, error) {
	return f.listUsersFn(ctx)
}

func (f *fakeUsersService) ListAdminUsers(ctx context.Context) ([]store.AdminUser, error) {
	if f.listAdminUsersFn == nil {
		return nil, nil
	}
	return f.listAdminUsersFn(ctx)
}

func (f *fakeUsersService) GetUser(ctx context.Context, userName string) (store.User, error) {
	return f.getUserFn(ctx, userName)
}

func (f *fakeUsersService) UpdateUserAccess(ctx context.Context, userID, accessStatus string) (store.User, error) {
	if f.updateUserAccessFn == nil {
		return store.User{}, nil
	}
	return f.updateUserAccessFn(ctx, userID, accessStatus)
}

func (f *fakeUsersService) RevokeUserSessions(ctx context.Context, userID string) (int64, error) {
	if f.revokeUserSessionsFn == nil {
		return 0, nil
	}
	return f.revokeUserSessionsFn(ctx, userID)
}

func (f *fakeUsersService) AuthenticateSession(ctx context.Context, token string) (store.User, error) {
	if f.authenticateSessionFn == nil {
		return store.User{}, auth.ErrInvalidSession
	}
	return f.authenticateSessionFn(ctx, token)
}

func (f *fakeUsersService) Login(ctx context.Context, userName, password string) (service.LoginResult, error) {
	return f.loginFn(ctx, userName, password)
}

func (f *fakeGamesService) CreateClassicGame(ctx context.Context, ownerUserID string, playerCount int) (store.Game, error) {
	return f.createClassicGameFn(ctx, ownerUserID, playerCount)
}

func (f *fakeGamesService) JoinClassicGame(ctx context.Context, gameID, playerID string) (store.Game, error) {
	return f.joinClassicGameFn(ctx, gameID, playerID)
}

func (f *fakeGamesService) GetGame(ctx context.Context, gameID string) (store.Game, error) {
	return f.getGameFn(ctx, gameID)
}

func (f *fakeGamesService) GetGameBootstrap(ctx context.Context, gameID, requesterUserID string) (service.GameBootstrap, error) {
	return f.getGameBootstrapFn(ctx, gameID, requesterUserID)
}

func (f *fakeGamesService) ListGames(ctx context.Context, ownerUserID, status string, limit, offset int) ([]store.Game, error) {
	return f.listGamesFn(ctx, ownerUserID, status, limit, offset)
}

func (f *fakeGamesService) UpdateGameState(ctx context.Context, gameID, status string, state json.RawMessage) (store.Game, error) {
	return f.updateGameStateFn(ctx, gameID, status, state)
}

func (f *fakeChatService) ListLobbyMessages(ctx context.Context, limit int) ([]store.ChatMessage, error) {
	return f.listLobbyMessagesFn(ctx, limit)
}

func (f *fakeChatService) PostLobbyMessage(ctx context.Context, userID, body string) (store.ChatMessage, error) {
	return f.postLobbyMessageFn(ctx, userID, body)
}

func newTestRouterWithServices(userSvc *fakeUsersService, games *fakeGamesService, chats *fakeChatService) http.Handler {
	h := NewHandler(game.NewServer(), userSvc, games, chats)
	return NewRouter(h)
}

func newTestRouter(svc *fakeUsersService) http.Handler {
	games := &fakeGamesService{
		createClassicGameFn: func(context.Context, string, int) (store.Game, error) { return store.Game{}, nil },
		joinClassicGameFn:   func(context.Context, string, string) (store.Game, error) { return store.Game{}, nil },
		getGameFn:           func(context.Context, string) (store.Game, error) { return store.Game{}, nil },
		getGameBootstrapFn: func(context.Context, string, string) (service.GameBootstrap, error) {
			return service.GameBootstrap{}, nil
		},
		listGamesFn:       func(context.Context, string, string, int, int) ([]store.Game, error) { return nil, nil },
		updateGameStateFn: func(context.Context, string, string, json.RawMessage) (store.Game, error) { return store.Game{}, nil },
	}
	chats := &fakeChatService{
		listLobbyMessagesFn: func(context.Context, int) ([]store.ChatMessage, error) { return nil, nil },
		postLobbyMessageFn:  func(context.Context, string, string) (store.ChatMessage, error) { return store.ChatMessage{}, nil },
	}
	return newTestRouterWithServices(svc, games, chats)
}

func newFakeChatService() *fakeChatService {
	return &fakeChatService{
		listLobbyMessagesFn: func(context.Context, int) ([]store.ChatMessage, error) { return nil, nil },
		postLobbyMessageFn:  func(context.Context, string, string) (store.ChatMessage, error) { return store.ChatMessage{}, nil },
	}
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

func doJSONWithAuth(t *testing.T, router http.Handler, method, path, bearerToken string, body any) *responseRecorder {
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
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	rr := newRecorder()
	router.ServeHTTP(rr, req)
	return rr
}

func TestPingEndpoint(t *testing.T) {
	router := newTestRouter(&fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		listUsersFn:  func(context.Context) ([]store.User, error) { return nil, nil },
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
		listUsersFn: func(context.Context) ([]store.User, error) { return nil, nil },
		getUserFn:   func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:     func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
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
		listUsersFn: func(context.Context) ([]store.User, error) { return nil, nil },
		getUserFn:   func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:     func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
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
		listUsersFn: func(context.Context) ([]store.User, error) { return nil, nil },
		getUserFn:   func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:     func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
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
		listUsersFn:  func(context.Context) ([]store.User, error) { return nil, nil },
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
		listUsersFn:  func(context.Context) ([]store.User, error) { return nil, nil },
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

func TestListUsersSuccess(t *testing.T) {
	now := time.Now().UTC()
	router := newTestRouter(&fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		authenticateSessionFn: func(context.Context, string) (store.User, error) {
			return store.User{ID: "u1", UserName: "alice"}, nil
		},
		listUsersFn: func(context.Context) ([]store.User, error) {
			return []store.User{
				{ID: "u1", UserName: "alice", Role: "player", CreatedAt: now, UpdatedAt: now},
				{ID: "u2", UserName: "bob", Role: "player", CreatedAt: now, UpdatedAt: now},
			}, nil
		},
		getUserFn: func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:   func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	})

	rr := doJSONWithAuth(t, router, http.MethodGet, "/api/users/", "valid-token", nil)
	if rr.code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.code, rr.body.String())
	}
}

func TestListUsersFailure(t *testing.T) {
	router := newTestRouter(&fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		authenticateSessionFn: func(context.Context, string) (store.User, error) {
			return store.User{ID: "u1", UserName: "alice"}, nil
		},
		listUsersFn: func(context.Context) ([]store.User, error) {
			return nil, errors.New("db down")
		},
		getUserFn: func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:   func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	})

	rr := doJSONWithAuth(t, router, http.MethodGet, "/api/users/", "valid-token", nil)
	if rr.code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rr.code, rr.body.String())
	}
}

func TestGetUserNotFound(t *testing.T) {
	router := newTestRouter(&fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		authenticateSessionFn: func(context.Context, string) (store.User, error) {
			return store.User{ID: "u1", UserName: "alice"}, nil
		},
		listUsersFn: func(context.Context) ([]store.User, error) { return nil, nil },
		getUserFn: func(context.Context, string) (store.User, error) {
			return store.User{}, errors.New("not found")
		},
		loginFn: func(context.Context, string, string) (service.LoginResult, error) {
			return service.LoginResult{}, nil
		},
	})

	rr := doJSONWithAuth(t, router, http.MethodGet, "/api/users/missing", "valid-token", nil)
	if rr.code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.code)
	}
}

func TestGetGameBootstrapSuccess(t *testing.T) {
	now := time.Now().UTC()
	userSvc := &fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		authenticateSessionFn: func(context.Context, string) (store.User, error) {
			return store.User{ID: "u1", UserName: "alice"}, nil
		},
		listUsersFn: func(context.Context) ([]store.User, error) { return nil, nil },
		getUserFn:   func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:     func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	}
	gamesSvc := &fakeGamesService{
		createClassicGameFn: func(context.Context, string, int) (store.Game, error) { return store.Game{}, nil },
		joinClassicGameFn:   func(context.Context, string, string) (store.Game, error) { return store.Game{}, nil },
		getGameFn:           func(context.Context, string) (store.Game, error) { return store.Game{}, nil },
		getGameBootstrapFn: func(_ context.Context, gameID, requesterUserID string) (service.GameBootstrap, error) {
			if gameID != "g1" || requesterUserID != "u1" {
				t.Fatalf("unexpected args: gameID=%s requester=%s", gameID, requesterUserID)
			}
			return service.GameBootstrap{
				ID:            "g1",
				OwnerUserID:   "u1",
				Status:        "in_progress",
				Phase:         "attack",
				CurrentPlayer: 0,
				Players: []service.GameBootstrapPlayer{
					{UserID: "u1", UserName: "alice", CardCount: 2, Eliminated: false},
				},
				Territories: json.RawMessage(`{"Alaska":{"owner":0,"armies":3}}`),
				CreatedAt:   now,
				UpdatedAt:   now,
			}, nil
		},
		listGamesFn:       func(context.Context, string, string, int, int) ([]store.Game, error) { return nil, nil },
		updateGameStateFn: func(context.Context, string, string, json.RawMessage) (store.Game, error) { return store.Game{}, nil },
	}
	router := newTestRouterWithServices(userSvc, gamesSvc, newFakeChatService())

	rr := doJSONWithAuth(t, router, http.MethodGet, "/api/games/g1/bootstrap", "valid-token", nil)
	if rr.code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.code, rr.body.String())
	}
}

func TestGetGameBootstrapForbidden(t *testing.T) {
	userSvc := &fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		authenticateSessionFn: func(context.Context, string) (store.User, error) {
			return store.User{ID: "u1", UserName: "alice"}, nil
		},
		listUsersFn: func(context.Context) ([]store.User, error) { return nil, nil },
		getUserFn:   func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:     func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	}
	gamesSvc := &fakeGamesService{
		createClassicGameFn: func(context.Context, string, int) (store.Game, error) { return store.Game{}, nil },
		joinClassicGameFn:   func(context.Context, string, string) (store.Game, error) { return store.Game{}, nil },
		getGameFn:           func(context.Context, string) (store.Game, error) { return store.Game{}, nil },
		getGameBootstrapFn: func(context.Context, string, string) (service.GameBootstrap, error) {
			return service.GameBootstrap{}, service.ErrGameForbidden
		},
		listGamesFn:       func(context.Context, string, string, int, int) ([]store.Game, error) { return nil, nil },
		updateGameStateFn: func(context.Context, string, string, json.RawMessage) (store.Game, error) { return store.Game{}, nil },
	}
	router := newTestRouterWithServices(userSvc, gamesSvc, newFakeChatService())

	rr := doJSONWithAuth(t, router, http.MethodGet, "/api/games/g1/bootstrap", "valid-token", nil)
	if rr.code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rr.code, rr.body.String())
	}
}

func TestProtectedRouteUnauthorized(t *testing.T) {
	router := newTestRouter(&fakeUsersService{
		createUserFn:          func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		listUsersFn:           func(context.Context) ([]store.User, error) { return nil, nil },
		getUserFn:             func(context.Context, string) (store.User, error) { return store.User{}, nil },
		authenticateSessionFn: func(context.Context, string) (store.User, error) { return store.User{}, auth.ErrInvalidSession },
		loginFn:               func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	})

	rr := doJSON(t, router, http.MethodGet, "/api/users/", nil)
	if rr.code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rr.code, rr.body.String())
	}
}

func TestAdminUsersForbiddenForNonAdmin(t *testing.T) {
	router := newTestRouter(&fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		authenticateSessionFn: func(context.Context, string) (store.User, error) {
			return store.User{ID: "u1", UserName: "alice", Role: "player"}, nil
		},
		listUsersFn: func(context.Context) ([]store.User, error) { return nil, nil },
		getUserFn:   func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:     func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	})

	rr := doJSONWithAuth(t, router, http.MethodGet, "/api/admin/users", "valid-token", nil)
	if rr.code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rr.code, rr.body.String())
	}
}

func TestAdminUsersSuccess(t *testing.T) {
	now := time.Now().UTC()
	router := newTestRouter(&fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		authenticateSessionFn: func(context.Context, string) (store.User, error) {
			return store.User{ID: "u1", UserName: "root", Role: "admin"}, nil
		},
		listUsersFn: func(context.Context) ([]store.User, error) { return nil, nil },
		listAdminUsersFn: func(context.Context) ([]store.AdminUser, error) {
			return []store.AdminUser{
				{
					User: store.User{
						ID:           "u2",
						UserName:     "alice",
						Role:         "player",
						AccessStatus: "active",
						CreatedAt:    now,
						UpdatedAt:    now,
					},
					ActiveSessions: 1,
				},
			}, nil
		},
		getUserFn: func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:   func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	})

	rr := doJSONWithAuth(t, router, http.MethodGet, "/api/admin/users", "valid-token", nil)
	if rr.code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.code, rr.body.String())
	}
}

func TestAdminRevokeSessionsSuccess(t *testing.T) {
	router := newTestRouter(&fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		authenticateSessionFn: func(context.Context, string) (store.User, error) {
			return store.User{ID: "u1", UserName: "root", Role: "admin"}, nil
		},
		listUsersFn: func(context.Context) ([]store.User, error) { return nil, nil },
		revokeUserSessionsFn: func(_ context.Context, userID string) (int64, error) {
			if userID != "u2" {
				t.Fatalf("unexpected user id: %s", userID)
			}
			return 2, nil
		},
		getUserFn: func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:   func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	})

	rr := doJSONWithAuth(t, router, http.MethodPost, "/api/admin/users/u2/revoke-sessions", "valid-token", nil)
	if rr.code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.code, rr.body.String())
	}
}

func TestAdminUpdateAccessSuccess(t *testing.T) {
	now := time.Now().UTC()
	router := newTestRouter(&fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		authenticateSessionFn: func(context.Context, string) (store.User, error) {
			return store.User{ID: "u1", UserName: "root", Role: "admin"}, nil
		},
		listUsersFn: func(context.Context) ([]store.User, error) { return nil, nil },
		updateUserAccessFn: func(_ context.Context, userID, accessStatus string) (store.User, error) {
			if userID != "u2" || accessStatus != "blocked" {
				t.Fatalf("unexpected update payload")
			}
			return store.User{
				ID:           "u2",
				UserName:     "alice",
				Role:         "player",
				AccessStatus: "blocked",
				CreatedAt:    now,
				UpdatedAt:    now,
			}, nil
		},
		getUserFn: func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:   func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	})

	rr := doJSONWithAuth(t, router, http.MethodPut, "/api/admin/users/u2/access", "valid-token", map[string]string{
		"access_status": "blocked",
	})
	if rr.code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.code, rr.body.String())
	}
}

func TestListLobbyMessagesSuccess(t *testing.T) {
	now := time.Now().UTC()
	svc := &fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		listUsersFn:  func(context.Context) ([]store.User, error) { return nil, nil },
		getUserFn:    func(context.Context, string) (store.User, error) { return store.User{}, nil },
		authenticateSessionFn: func(context.Context, string) (store.User, error) {
			return store.User{ID: "u1", UserName: "alice"}, nil
		},
		loginFn: func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	}
	router := newTestRouterWithServices(svc, &fakeGamesService{
		createClassicGameFn: func(context.Context, string, int) (store.Game, error) { return store.Game{}, nil },
		joinClassicGameFn:   func(context.Context, string, string) (store.Game, error) { return store.Game{}, nil },
		getGameFn:           func(context.Context, string) (store.Game, error) { return store.Game{}, nil },
		listGamesFn:         func(context.Context, string, string, int, int) ([]store.Game, error) { return nil, nil },
		updateGameStateFn:   func(context.Context, string, string, json.RawMessage) (store.Game, error) { return store.Game{}, nil },
	}, &fakeChatService{
		listLobbyMessagesFn: func(context.Context, int) ([]store.ChatMessage, error) {
			return []store.ChatMessage{{ID: "m1", Room: "lobby", UserID: "u1", UserName: "alice", Body: "hello", CreatedAt: now}}, nil
		},
		postLobbyMessageFn: func(context.Context, string, string) (store.ChatMessage, error) { return store.ChatMessage{}, nil },
	})

	rr := doJSONWithAuth(t, router, http.MethodGet, "/api/chat/lobby/messages?limit=25", "valid-token", nil)
	if rr.code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.code, rr.body.String())
	}
}

func TestPostLobbyMessageSuccess(t *testing.T) {
	now := time.Now().UTC()
	svc := &fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		listUsersFn:  func(context.Context) ([]store.User, error) { return nil, nil },
		getUserFn:    func(context.Context, string) (store.User, error) { return store.User{}, nil },
		authenticateSessionFn: func(context.Context, string) (store.User, error) {
			return store.User{ID: "u1", UserName: "alice"}, nil
		},
		loginFn: func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	}
	router := newTestRouterWithServices(svc, &fakeGamesService{
		createClassicGameFn: func(context.Context, string, int) (store.Game, error) { return store.Game{}, nil },
		joinClassicGameFn:   func(context.Context, string, string) (store.Game, error) { return store.Game{}, nil },
		getGameFn:           func(context.Context, string) (store.Game, error) { return store.Game{}, nil },
		listGamesFn:         func(context.Context, string, string, int, int) ([]store.Game, error) { return nil, nil },
		updateGameStateFn:   func(context.Context, string, string, json.RawMessage) (store.Game, error) { return store.Game{}, nil },
	}, &fakeChatService{
		listLobbyMessagesFn: func(context.Context, int) ([]store.ChatMessage, error) { return nil, nil },
		postLobbyMessageFn: func(_ context.Context, userID, body string) (store.ChatMessage, error) {
			if userID != "u1" || body != "hey all" {
				t.Fatalf("unexpected post args")
			}
			return store.ChatMessage{ID: "m1", Room: "lobby", UserID: userID, UserName: "alice", Body: body, CreatedAt: now}, nil
		},
	})

	rr := doJSONWithAuth(t, router, http.MethodPost, "/api/chat/lobby/messages", "valid-token", map[string]string{"body": "hey all"})
	if rr.code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.code, rr.body.String())
	}
}

func TestCreateGameSuccess(t *testing.T) {
	userSvc := &fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		authenticateSessionFn: func(context.Context, string) (store.User, error) {
			return store.User{ID: "u1"}, nil
		},
		listUsersFn: func(context.Context) ([]store.User, error) { return nil, nil },
		getUserFn:   func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:     func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	}
	gamesSvc := &fakeGamesService{
		createClassicGameFn: func(_ context.Context, ownerUserID string, playerCount int) (store.Game, error) {
			if ownerUserID != "u1" || playerCount != 3 {
				t.Fatalf("unexpected create game input")
			}
			return store.Game{ID: "g1", OwnerUserID: "u1", Status: "lobby", State: json.RawMessage(`{"phase":"setup_claim"}`)}, nil
		},
		joinClassicGameFn: func(context.Context, string, string) (store.Game, error) { return store.Game{}, nil },
		getGameFn:         func(context.Context, string) (store.Game, error) { return store.Game{}, nil },
		listGamesFn:       func(context.Context, string, string, int, int) ([]store.Game, error) { return nil, nil },
		updateGameStateFn: func(context.Context, string, string, json.RawMessage) (store.Game, error) { return store.Game{}, nil },
	}
	router := newTestRouterWithServices(userSvc, gamesSvc, newFakeChatService())

	rr := doJSONWithAuth(t, router, http.MethodPost, "/api/games/", "valid-token", map[string]any{
		"player_count": 3,
	})
	if rr.code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rr.code, rr.body.String())
	}
}

func TestJoinGameSuccess(t *testing.T) {
	userSvc := &fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		authenticateSessionFn: func(context.Context, string) (store.User, error) {
			return store.User{ID: "u2"}, nil
		},
		listUsersFn: func(context.Context) ([]store.User, error) { return nil, nil },
		getUserFn:   func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:     func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	}
	gamesSvc := &fakeGamesService{
		createClassicGameFn: func(context.Context, string, int) (store.Game, error) { return store.Game{}, nil },
		joinClassicGameFn: func(_ context.Context, gameID, playerID string) (store.Game, error) {
			if gameID != "g1" || playerID != "u2" {
				t.Fatalf("unexpected join input")
			}
			return store.Game{ID: "g1", Status: "lobby"}, nil
		},
		getGameFn:         func(context.Context, string) (store.Game, error) { return store.Game{}, nil },
		listGamesFn:       func(context.Context, string, string, int, int) ([]store.Game, error) { return nil, nil },
		updateGameStateFn: func(context.Context, string, string, json.RawMessage) (store.Game, error) { return store.Game{}, nil },
	}
	router := newTestRouterWithServices(userSvc, gamesSvc, newFakeChatService())

	rr := doJSONWithAuth(t, router, http.MethodPost, "/api/games/g1/join", "valid-token", nil)
	if rr.code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.code, rr.body.String())
	}
}

func TestGetGameNotFound(t *testing.T) {
	userSvc := &fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		authenticateSessionFn: func(context.Context, string) (store.User, error) {
			return store.User{ID: "u1"}, nil
		},
		listUsersFn: func(context.Context) ([]store.User, error) { return nil, nil },
		getUserFn:   func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:     func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	}
	gamesSvc := &fakeGamesService{
		createClassicGameFn: func(context.Context, string, int) (store.Game, error) { return store.Game{}, nil },
		joinClassicGameFn:   func(context.Context, string, string) (store.Game, error) { return store.Game{}, nil },
		getGameFn:           func(context.Context, string) (store.Game, error) { return store.Game{}, service.ErrGameNotFound },
		listGamesFn:         func(context.Context, string, string, int, int) ([]store.Game, error) { return nil, nil },
		updateGameStateFn:   func(context.Context, string, string, json.RawMessage) (store.Game, error) { return store.Game{}, nil },
	}
	router := newTestRouterWithServices(userSvc, gamesSvc, newFakeChatService())

	rr := doJSONWithAuth(t, router, http.MethodGet, "/api/games/g_missing", "valid-token", nil)
	if rr.code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.code)
	}
}

func TestUpdateGameStateSuccess(t *testing.T) {
	userSvc := &fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		authenticateSessionFn: func(context.Context, string) (store.User, error) {
			return store.User{ID: "u1"}, nil
		},
		listUsersFn: func(context.Context) ([]store.User, error) { return nil, nil },
		getUserFn:   func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:     func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	}
	gamesSvc := &fakeGamesService{
		createClassicGameFn: func(context.Context, string, int) (store.Game, error) { return store.Game{}, nil },
		joinClassicGameFn:   func(context.Context, string, string) (store.Game, error) { return store.Game{}, nil },
		getGameFn:           func(context.Context, string) (store.Game, error) { return store.Game{}, nil },
		listGamesFn:         func(context.Context, string, string, int, int) ([]store.Game, error) { return nil, nil },
		updateGameStateFn: func(_ context.Context, gameID, status string, state json.RawMessage) (store.Game, error) {
			if gameID != "g1" || status != "in_progress" || len(state) == 0 {
				t.Fatalf("unexpected update input")
			}
			return store.Game{ID: "g1", Status: "in_progress", State: state}, nil
		},
	}
	router := newTestRouterWithServices(userSvc, gamesSvc, newFakeChatService())

	rr := doJSONWithAuth(t, router, http.MethodPut, "/api/games/g1/state", "valid-token", map[string]any{
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
		authenticateSessionFn: func(context.Context, string) (store.User, error) {
			return store.User{ID: "u1"}, nil
		},
		listUsersFn: func(context.Context) ([]store.User, error) { return nil, nil },
		getUserFn:   func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:     func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	}
	gamesSvc := &fakeGamesService{
		createClassicGameFn: func(context.Context, string, int) (store.Game, error) { return store.Game{}, nil },
		joinClassicGameFn:   func(context.Context, string, string) (store.Game, error) { return store.Game{}, nil },
		getGameFn:           func(context.Context, string) (store.Game, error) { return store.Game{}, nil },
		listGamesFn: func(_ context.Context, owner, status string, limit, offset int) ([]store.Game, error) {
			if owner != "u1" || status != "lobby" || limit != 10 || offset != 5 {
				t.Fatalf("unexpected list filters")
			}
			return []store.Game{{ID: "g1", OwnerUserID: "u1", Status: "lobby"}}, nil
		},
		updateGameStateFn: func(context.Context, string, string, json.RawMessage) (store.Game, error) { return store.Game{}, nil },
	}
	router := newTestRouterWithServices(userSvc, gamesSvc, newFakeChatService())

	rr := doJSONWithAuth(t, router, http.MethodGet, "/api/games/?owner_user_id=u1&status=lobby&limit=10&offset=5", "valid-token", nil)
	if rr.code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.code, rr.body.String())
	}
}

func TestListGamesBadLimit(t *testing.T) {
	userSvc := &fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		authenticateSessionFn: func(context.Context, string) (store.User, error) {
			return store.User{ID: "u1"}, nil
		},
		listUsersFn: func(context.Context) ([]store.User, error) { return nil, nil },
		getUserFn:   func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:     func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	}
	gamesSvc := &fakeGamesService{
		createClassicGameFn: func(context.Context, string, int) (store.Game, error) { return store.Game{}, nil },
		joinClassicGameFn:   func(context.Context, string, string) (store.Game, error) { return store.Game{}, nil },
		getGameFn:           func(context.Context, string) (store.Game, error) { return store.Game{}, nil },
		listGamesFn: func(context.Context, string, string, int, int) ([]store.Game, error) {
			t.Fatalf("list should not be called")
			return nil, nil
		},
		updateGameStateFn: func(context.Context, string, string, json.RawMessage) (store.Game, error) { return store.Game{}, nil },
	}
	router := newTestRouterWithServices(userSvc, gamesSvc, newFakeChatService())

	rr := doJSONWithAuth(t, router, http.MethodGet, "/api/games/?limit=bad", "valid-token", nil)
	if rr.code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.code)
	}
}
