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

func (f *fakeUsersService) CreateUser(ctx context.Context, userName, password string) (store.User, error) {
	return f.createUserFn(ctx, userName, password)
}

func (f *fakeUsersService) GetUser(ctx context.Context, userName string) (store.User, error) {
	return f.getUserFn(ctx, userName)
}

func (f *fakeUsersService) Login(ctx context.Context, userName, password string) (service.LoginResult, error) {
	return f.loginFn(ctx, userName, password)
}

func newTestRouter(svc *fakeUsersService) http.Handler {
	h := NewHandler(game.NewServer(), svc)
	return NewRouter(h)
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
