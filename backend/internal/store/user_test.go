package store

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"backend/internal/db"
	"github.com/jackc/pgx/v5"
)

type stubQuerier struct {
	lastSQL  string
	lastArgs []any
	row      *stubRow
}

func (s *stubQuerier) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	s.lastSQL = sql
	s.lastArgs = args
	return s.row
}

type stubRow struct {
	values []any
	err    error
}

func (r *stubRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return errors.New("scan arity mismatch")
	}
	for i := range dest {
		dv := reflect.ValueOf(dest[i])
		if dv.Kind() != reflect.Ptr || dv.IsNil() {
			return errors.New("dest must be pointer")
		}
		dv.Elem().Set(reflect.ValueOf(r.values[i]))
	}
	return nil
}

var _ db.Querier = (*stubQuerier)(nil)

func TestPostgresUsersStoreCreate(t *testing.T) {
	now := time.Now().UTC()
	q := &stubQuerier{
		row: &stubRow{values: []any{"u1", "alice", "player", now, now}},
	}
	s := NewPostgresUsersStore()

	out, err := s.Create(context.Background(), q, NewUser{
		UserName:     "alice",
		PasswordHash: "hash123",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !strings.Contains(q.lastSQL, "INSERT INTO users") {
		t.Fatalf("expected users insert SQL, got %q", q.lastSQL)
	}
	if len(q.lastArgs) != 2 || q.lastArgs[0] != "alice" || q.lastArgs[1] != "hash123" {
		t.Fatalf("unexpected args: %#v", q.lastArgs)
	}
	if out.UserName != "alice" || out.Role != "player" {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestPostgresUsersStoreGetUser(t *testing.T) {
	now := time.Now().UTC()
	q := &stubQuerier{
		row: &stubRow{values: []any{"u1", "alice", "player", now, now}},
	}
	s := NewPostgresUsersStore()

	out, err := s.GetUser(context.Background(), q, "alice")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if !strings.Contains(q.lastSQL, "FROM users") {
		t.Fatalf("expected users select SQL, got %q", q.lastSQL)
	}
	if len(q.lastArgs) != 1 || q.lastArgs[0] != "alice" {
		t.Fatalf("unexpected args: %#v", q.lastArgs)
	}
	if out.ID != "u1" || out.UserName != "alice" {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestPostgresUsersStoreGetUserAuth(t *testing.T) {
	now := time.Now().UTC()
	q := &stubQuerier{
		row: &stubRow{values: []any{"u1", "alice", "player", "pw_hash", now, now}},
	}
	s := NewPostgresUsersStore()

	out, err := s.GetUserAuth(context.Background(), q, "alice")
	if err != nil {
		t.Fatalf("get user auth: %v", err)
	}
	if !strings.Contains(q.lastSQL, "password_hash") {
		t.Fatalf("expected auth select SQL, got %q", q.lastSQL)
	}
	if out.PasswordHash != "pw_hash" {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestPostgresUsersStoreCreateSession(t *testing.T) {
	now := time.Now().UTC()
	tokenHash := []byte{1, 2, 3}
	q := &stubQuerier{
		row: &stubRow{values: []any{"s1", "u1", tokenHash, now, now, now.Add(24 * time.Hour)}},
	}
	s := NewPostgresUsersStore()

	out, err := s.CreateSession(context.Background(), q, NewSession{
		UserID:    "u1",
		TokenHash: tokenHash,
		ExpiresAt: now.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if !strings.Contains(q.lastSQL, "INSERT INTO sessions") {
		t.Fatalf("expected sessions insert SQL, got %q", q.lastSQL)
	}
	if len(q.lastArgs) != 3 || q.lastArgs[0] != "u1" {
		t.Fatalf("unexpected args: %#v", q.lastArgs)
	}
	if out.ID != "s1" || out.UserID != "u1" {
		t.Fatalf("unexpected output: %#v", out)
	}
}
