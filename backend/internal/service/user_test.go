package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"backend/internal/auth"
	"backend/internal/db"
	"backend/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type fakeDB struct {
	q            db.Querier
	txQ          db.Querier
	withTxQCalls int
}

func (f *fakeDB) Queryer() db.Querier { return f.q }

func (f *fakeDB) WithTxQ(ctx context.Context, fn func(q db.Querier) error) error {
	f.withTxQCalls++
	return fn(f.txQ)
}

type fakeStore struct {
	createFn                func(context.Context, db.Querier, store.NewUser) (store.User, error)
	listUsersFn             func(context.Context, db.Querier) ([]store.User, error)
	getUserFn               func(context.Context, db.Querier, string) (store.User, error)
	getUserBySessionTokenFn func(context.Context, db.Querier, []byte) (store.User, error)
	getUserAuthFn           func(context.Context, db.Querier, string) (store.UserAuth, error)
	createSessionFn         func(context.Context, db.Querier, store.NewSession) (store.Session, error)
}

func (f *fakeStore) Create(ctx context.Context, q db.Querier, in store.NewUser) (store.User, error) {
	return f.createFn(ctx, q, in)
}

func (f *fakeStore) ListUsers(ctx context.Context, q db.Querier) ([]store.User, error) {
	return f.listUsersFn(ctx, q)
}

func (f *fakeStore) GetUser(ctx context.Context, q db.Querier, userName string) (store.User, error) {
	return f.getUserFn(ctx, q, userName)
}

func (f *fakeStore) GetUserBySessionToken(ctx context.Context, q db.Querier, tokenHash []byte) (store.User, error) {
	if f.getUserBySessionTokenFn == nil {
		return store.User{}, pgx.ErrNoRows
	}
	return f.getUserBySessionTokenFn(ctx, q, tokenHash)
}

func (f *fakeStore) GetUserAuth(ctx context.Context, q db.Querier, userName string) (store.UserAuth, error) {
	return f.getUserAuthFn(ctx, q, userName)
}

func (f *fakeStore) CreateSession(ctx context.Context, q db.Querier, in store.NewSession) (store.Session, error) {
	return f.createSessionFn(ctx, q, in)
}

type noopQuerier struct{}

func (noopQuerier) QueryRow(context.Context, string, ...any) pgx.Row { return nil }
func (noopQuerier) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func TestCreateUserValidatesAndHashes(t *testing.T) {
	q := noopQuerier{}
	svc := NewUsersService(&fakeDB{q: q}, &fakeStore{
		createFn: func(_ context.Context, gotQ db.Querier, in store.NewUser) (store.User, error) {
			if gotQ != q {
				t.Fatalf("expected queryer to be passed through")
			}
			if in.UserName != "alice" {
				t.Fatalf("expected trimmed username, got %q", in.UserName)
			}
			if in.PasswordHash == "" {
				t.Fatalf("expected password hash")
			}
			ok, err := auth.VerifyPassword("very-secure-password", in.PasswordHash)
			if err != nil || !ok {
				t.Fatalf("stored hash did not verify: ok=%v err=%v", ok, err)
			}
			return store.User{ID: "u1", UserName: in.UserName}, nil
		},
		getUserFn: func(context.Context, db.Querier, string) (store.User, error) { return store.User{}, nil },
		getUserAuthFn: func(context.Context, db.Querier, string) (store.UserAuth, error) {
			return store.UserAuth{}, nil
		},
		createSessionFn: func(context.Context, db.Querier, store.NewSession) (store.Session, error) {
			return store.Session{}, nil
		},
	})

	out, err := svc.CreateUser(context.Background(), "  alice  ", "very-secure-password")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if out.UserName != "alice" {
		t.Fatalf("unexpected user: %#v", out)
	}
}

func TestCreateUserMapsUniqueViolation(t *testing.T) {
	svc := NewUsersService(&fakeDB{q: noopQuerier{}}, &fakeStore{
		createFn: func(context.Context, db.Querier, store.NewUser) (store.User, error) {
			return store.User{}, &pgconn.PgError{Code: "23505"}
		},
		getUserFn: func(context.Context, db.Querier, string) (store.User, error) { return store.User{}, nil },
		getUserAuthFn: func(context.Context, db.Querier, string) (store.UserAuth, error) {
			return store.UserAuth{}, nil
		},
		createSessionFn: func(context.Context, db.Querier, store.NewSession) (store.Session, error) {
			return store.Session{}, nil
		},
	})

	_, err := svc.CreateUser(context.Background(), "alice", "very-secure-password")
	if !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("expected ErrUsernameTaken, got %v", err)
	}
}

func TestCreateUserValidationErrors(t *testing.T) {
	svc := NewUsersService(&fakeDB{q: noopQuerier{}}, &fakeStore{
		createFn: func(context.Context, db.Querier, store.NewUser) (store.User, error) {
			t.Fatalf("create should not be called")
			return store.User{}, nil
		},
		getUserFn: func(context.Context, db.Querier, string) (store.User, error) { return store.User{}, nil },
		getUserAuthFn: func(context.Context, db.Querier, string) (store.UserAuth, error) {
			return store.UserAuth{}, nil
		},
		createSessionFn: func(context.Context, db.Querier, store.NewSession) (store.Session, error) {
			return store.Session{}, nil
		},
	})

	_, err := svc.CreateUser(context.Background(), "ab", "very-secure-password")
	if !errors.Is(err, auth.ErrUsernameInvalid) {
		t.Fatalf("expected ErrUsernameInvalid, got %v", err)
	}

	_, err = svc.CreateUser(context.Background(), "alice", "short")
	if !errors.Is(err, auth.ErrPasswordTooShort) {
		t.Fatalf("expected ErrPasswordTooShort, got %v", err)
	}
}

func TestGetUserDelegatesToStore(t *testing.T) {
	q := noopQuerier{}
	svc := NewUsersService(&fakeDB{q: q}, &fakeStore{
		createFn: func(context.Context, db.Querier, store.NewUser) (store.User, error) { return store.User{}, nil },
		getUserFn: func(_ context.Context, gotQ db.Querier, userName string) (store.User, error) {
			if gotQ != q || userName != "alice" {
				t.Fatalf("unexpected delegation values")
			}
			return store.User{ID: "u1", UserName: "alice"}, nil
		},
		getUserAuthFn: func(context.Context, db.Querier, string) (store.UserAuth, error) {
			return store.UserAuth{}, nil
		},
		createSessionFn: func(context.Context, db.Querier, store.NewSession) (store.Session, error) {
			return store.Session{}, nil
		},
	})

	out, err := svc.GetUser(context.Background(), "alice")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if out.UserName != "alice" {
		t.Fatalf("unexpected user: %#v", out)
	}
}

func TestListUsersDelegatesToStore(t *testing.T) {
	q := noopQuerier{}
	svc := NewUsersService(&fakeDB{q: q}, &fakeStore{
		createFn: func(context.Context, db.Querier, store.NewUser) (store.User, error) { return store.User{}, nil },
		listUsersFn: func(_ context.Context, gotQ db.Querier) ([]store.User, error) {
			if gotQ != q {
				t.Fatalf("unexpected querier")
			}
			return []store.User{{ID: "u1", UserName: "alice"}}, nil
		},
		getUserFn: func(context.Context, db.Querier, string) (store.User, error) { return store.User{}, nil },
		getUserAuthFn: func(context.Context, db.Querier, string) (store.UserAuth, error) {
			return store.UserAuth{}, nil
		},
		createSessionFn: func(context.Context, db.Querier, store.NewSession) (store.Session, error) {
			return store.Session{}, nil
		},
	})

	out, err := svc.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(out) != 1 || out[0].UserName != "alice" {
		t.Fatalf("unexpected users: %#v", out)
	}
}

func TestLoginSuccessCreatesSession(t *testing.T) {
	txQ := noopQuerier{}
	pwHash, err := auth.HashPassword("correct horse battery staple", auth.DefaultPasswordParams())
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	var createdSession store.NewSession
	svc := NewUsersService(&fakeDB{q: noopQuerier{}, txQ: txQ}, &fakeStore{
		createFn:  func(context.Context, db.Querier, store.NewUser) (store.User, error) { return store.User{}, nil },
		getUserFn: func(context.Context, db.Querier, string) (store.User, error) { return store.User{}, nil },
		getUserAuthFn: func(_ context.Context, gotQ db.Querier, userName string) (store.UserAuth, error) {
			if gotQ != txQ || userName != "alice" {
				t.Fatalf("expected tx querier and username")
			}
			now := time.Now().UTC()
			return store.UserAuth{
				User: store.User{
					ID:        "u1",
					UserName:  "alice",
					Role:      "player",
					CreatedAt: now,
					UpdatedAt: now,
				},
				PasswordHash: pwHash,
			}, nil
		},
		createSessionFn: func(_ context.Context, gotQ db.Querier, in store.NewSession) (store.Session, error) {
			if gotQ != txQ {
				t.Fatalf("expected tx querier")
			}
			createdSession = in
			return store.Session{ID: "s1", UserID: in.UserID}, nil
		},
	})

	out, err := svc.Login(context.Background(), "alice", "correct horse battery staple")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if out.Token == "" {
		t.Fatalf("expected token")
	}
	if createdSession.UserID != "u1" || len(createdSession.TokenHash) == 0 {
		t.Fatalf("unexpected session payload: %#v", createdSession)
	}
	hashedOut, err := auth.HashSessionToken(out.Token)
	if err != nil {
		t.Fatalf("hash token: %v", err)
	}
	if string(hashedOut) != string(createdSession.TokenHash) {
		t.Fatalf("stored token hash does not match returned token")
	}
}

func TestAuthenticateSessionSuccess(t *testing.T) {
	q := noopQuerier{}
	svc := NewUsersService(&fakeDB{q: q}, &fakeStore{
		createFn:    func(context.Context, db.Querier, store.NewUser) (store.User, error) { return store.User{}, nil },
		listUsersFn: func(context.Context, db.Querier) ([]store.User, error) { return nil, nil },
		getUserFn:   func(context.Context, db.Querier, string) (store.User, error) { return store.User{}, nil },
		getUserBySessionTokenFn: func(_ context.Context, gotQ db.Querier, tokenHash []byte) (store.User, error) {
			if gotQ != q || len(tokenHash) == 0 {
				t.Fatalf("expected queryer and non-empty token hash")
			}
			return store.User{ID: "u1", UserName: "alice"}, nil
		},
		getUserAuthFn: func(context.Context, db.Querier, string) (store.UserAuth, error) { return store.UserAuth{}, nil },
		createSessionFn: func(context.Context, db.Querier, store.NewSession) (store.Session, error) {
			return store.Session{}, nil
		},
	})

	out, err := svc.AuthenticateSession(context.Background(), "session-token")
	if err != nil {
		t.Fatalf("authenticate session: %v", err)
	}
	if out.UserName != "alice" {
		t.Fatalf("unexpected user: %#v", out)
	}
}

func TestAuthenticateSessionInvalid(t *testing.T) {
	svc := NewUsersService(&fakeDB{q: noopQuerier{}}, &fakeStore{
		createFn:                func(context.Context, db.Querier, store.NewUser) (store.User, error) { return store.User{}, nil },
		listUsersFn:             func(context.Context, db.Querier) ([]store.User, error) { return nil, nil },
		getUserFn:               func(context.Context, db.Querier, string) (store.User, error) { return store.User{}, nil },
		getUserBySessionTokenFn: func(context.Context, db.Querier, []byte) (store.User, error) { return store.User{}, pgx.ErrNoRows },
		getUserAuthFn:           func(context.Context, db.Querier, string) (store.UserAuth, error) { return store.UserAuth{}, nil },
		createSessionFn: func(context.Context, db.Querier, store.NewSession) (store.Session, error) {
			return store.Session{}, nil
		},
	})

	_, err := svc.AuthenticateSession(context.Background(), "session-token")
	if !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatalf("expected invalid session, got %v", err)
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	pwHash, err := auth.HashPassword("real-password", auth.DefaultPasswordParams())
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	sessionCalls := 0
	svc := NewUsersService(&fakeDB{q: noopQuerier{}, txQ: noopQuerier{}}, &fakeStore{
		createFn:  func(context.Context, db.Querier, store.NewUser) (store.User, error) { return store.User{}, nil },
		getUserFn: func(context.Context, db.Querier, string) (store.User, error) { return store.User{}, nil },
		getUserAuthFn: func(context.Context, db.Querier, string) (store.UserAuth, error) {
			return store.UserAuth{
				User:         store.User{ID: "u1", UserName: "alice"},
				PasswordHash: pwHash,
			}, nil
		},
		createSessionFn: func(context.Context, db.Querier, store.NewSession) (store.Session, error) {
			sessionCalls++
			return store.Session{}, nil
		},
	})

	_, err = svc.Login(context.Background(), "alice", "wrong-password")
	if !errors.Is(err, auth.ErrInvalidUsernameOrPassword) {
		t.Fatalf("expected invalid credentials, got %v", err)
	}
	if sessionCalls != 0 {
		t.Fatalf("session should not be created on invalid password")
	}
}

func TestLoginUserNotFound(t *testing.T) {
	svc := NewUsersService(&fakeDB{q: noopQuerier{}, txQ: noopQuerier{}}, &fakeStore{
		createFn:  func(context.Context, db.Querier, store.NewUser) (store.User, error) { return store.User{}, nil },
		getUserFn: func(context.Context, db.Querier, string) (store.User, error) { return store.User{}, nil },
		getUserAuthFn: func(context.Context, db.Querier, string) (store.UserAuth, error) {
			return store.UserAuth{}, pgx.ErrNoRows
		},
		createSessionFn: func(context.Context, db.Querier, store.NewSession) (store.Session, error) {
			t.Fatalf("session should not be created")
			return store.Session{}, nil
		},
	})

	_, err := svc.Login(context.Background(), "missing", "any-password")
	if !errors.Is(err, auth.ErrInvalidUsernameOrPassword) {
		t.Fatalf("expected invalid credentials, got %v", err)
	}
}
