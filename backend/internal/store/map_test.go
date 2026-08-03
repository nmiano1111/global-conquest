package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestPostgresMapsStoreCreate(t *testing.T) {
	now := time.Now().UTC()
	def := json.RawMessage(`{"board":{}}`)
	q := &stubQuerier{
		row: &stubRow{values: []any{"m1", "u1", "Custom Map", def, now, now}},
	}
	s := NewPostgresMapsStore()

	out, err := s.Create(context.Background(), q, NewMap{
		OwnerUserID: "u1",
		Name:        "Custom Map",
		Definition:  def,
	})
	if err != nil {
		t.Fatalf("create map: %v", err)
	}
	if !strings.Contains(q.lastSQL, "INSERT INTO maps") {
		t.Fatalf("expected maps insert SQL, got %q", q.lastSQL)
	}
	if out.ID != "m1" || out.OwnerUserID != "u1" || out.Name != "Custom Map" {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestPostgresMapsStoreGetByID(t *testing.T) {
	now := time.Now().UTC()
	def := json.RawMessage(`{"board":{}}`)
	q := &stubQuerier{
		row: &stubRow{values: []any{"m1", "u1", "Custom Map", def, now, now}},
	}
	s := NewPostgresMapsStore()

	out, err := s.GetByID(context.Background(), q, "m1")
	if err != nil {
		t.Fatalf("get map: %v", err)
	}
	if !strings.Contains(q.lastSQL, "FROM maps") {
		t.Fatalf("expected maps select SQL, got %q", q.lastSQL)
	}
	if out.Name != "Custom Map" {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestPostgresMapsStoreGetByIDNotFound(t *testing.T) {
	q := &stubQuerier{row: &stubRow{err: pgx.ErrNoRows}}
	s := NewPostgresMapsStore()

	_, err := s.GetByID(context.Background(), q, "missing")
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected pgx.ErrNoRows, got %v", err)
	}
}

func TestPostgresMapsStoreList(t *testing.T) {
	now := time.Now().UTC()
	def := json.RawMessage(`{"board":{}}`)
	q := &stubQuerier{
		rows: &stubRows{values: [][]any{
			{"m1", "u1", "Map One", def, now, now},
			{"m2", "u1", "Map Two", def, now, now},
		}},
	}
	s := NewPostgresMapsStore()

	out, err := s.List(context.Background(), q, "")
	if err != nil {
		t.Fatalf("list maps: %v", err)
	}
	if !strings.Contains(q.lastSQL, "FROM maps") {
		t.Fatalf("expected maps select SQL, got %q", q.lastSQL)
	}
	if len(out) != 2 || out[0].Name != "Map One" || out[1].Name != "Map Two" {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestPostgresMapsStoreListByOwner(t *testing.T) {
	q := &stubQuerier{rows: &stubRows{values: [][]any{}}}
	s := NewPostgresMapsStore()

	if _, err := s.List(context.Background(), q, "u1"); err != nil {
		t.Fatalf("list maps by owner: %v", err)
	}
	if !strings.Contains(q.lastSQL, "WHERE owner_user_id") {
		t.Fatalf("expected owner filter in SQL, got %q", q.lastSQL)
	}
}

func TestPostgresMapsStoreDelete(t *testing.T) {
	q := &stubQuerier{row: &stubRow{values: []any{"m1"}}}
	s := NewPostgresMapsStore()

	if err := s.Delete(context.Background(), q, "m1"); err != nil {
		t.Fatalf("delete map: %v", err)
	}
	if !strings.Contains(q.lastSQL, "DELETE FROM maps") {
		t.Fatalf("expected maps delete SQL, got %q", q.lastSQL)
	}
}

func TestPostgresMapsStoreDeleteNotFound(t *testing.T) {
	q := &stubQuerier{row: &stubRow{err: pgx.ErrNoRows}}
	s := NewPostgresMapsStore()

	err := s.Delete(context.Background(), q, "missing")
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected pgx.ErrNoRows, got %v", err)
	}
}
