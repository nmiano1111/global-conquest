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
	"github.com/jackc/pgx/v5/pgconn"
)

type stubQuerier struct {
	lastSQL  string
	lastArgs []any
	row      *stubRow
	rows     *stubRows
}

func (s *stubQuerier) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	s.lastSQL = sql
	s.lastArgs = args
	return s.row
}

func (s *stubQuerier) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	s.lastSQL = sql
	s.lastArgs = args
	if s.rows != nil {
		return s.rows, nil
	}
	return nil, errors.New("query not configured in stubQuerier")
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

type stubRows struct {
	idx    int
	values [][]any
	err    error
}

func (r *stubRows) Close()                                       {}
func (r *stubRows) Err() error                                   { return r.err }
func (r *stubRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *stubRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *stubRows) RawValues() [][]byte                          { return nil }
func (r *stubRows) Conn() *pgx.Conn                              { return nil }
func (r *stubRows) Values() ([]any, error) {
	if r.idx == 0 || r.idx > len(r.values) {
		return nil, errors.New("rows cursor out of bounds")
	}
	return r.values[r.idx-1], nil
}
func (r *stubRows) Next() bool {
	if r.idx >= len(r.values) {
		return false
	}
	r.idx++
	return true
}
func (r *stubRows) Scan(dest ...any) error {
	if r.idx == 0 || r.idx > len(r.values) {
		return errors.New("rows cursor out of bounds")
	}
	row := r.values[r.idx-1]
	if len(dest) != len(row) {
		return errors.New("scan arity mismatch")
	}
	for i := range dest {
		dv := reflect.ValueOf(dest[i])
		if dv.Kind() != reflect.Ptr || dv.IsNil() {
			return errors.New("dest must be pointer")
		}
		dv.Elem().Set(reflect.ValueOf(row[i]))
	}
	return nil
}
func (r *stubRows) NextResultSet() bool { return false }

var _ db.Querier = (*stubQuerier)(nil)
var _ pgx.Rows = (*stubRows)(nil)

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

func TestPostgresUsersStoreListUsers(t *testing.T) {
	now := time.Now().UTC()
	q := &stubQuerier{
		rows: &stubRows{values: [][]any{
			{"u1", "alice", "player", now, now},
			{"u2", "bob", "admin", now, now},
		}},
	}
	s := NewPostgresUsersStore()

	out, err := s.ListUsers(context.Background(), q)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if !strings.Contains(q.lastSQL, "FROM users") || !strings.Contains(q.lastSQL, "ORDER BY created_at DESC") {
		t.Fatalf("expected users list SQL, got %q", q.lastSQL)
	}
	if len(out) != 2 || out[0].UserName != "alice" || out[1].UserName != "bob" {
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
