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

func TestPostgresGamesStoreCreate(t *testing.T) {
	now := time.Now().UTC()
	state := json.RawMessage(`{"phase":"setup_claim"}`)
	q := &stubQuerier{
		row: &stubRow{values: []any{"g1", "u1", "Fierce Badger", "lobby", state, now, now}},
	}
	s := NewPostgresGamesStore()

	out, err := s.Create(context.Background(), q, NewGame{
		OwnerUserID: "u1",
		Name:        "Fierce Badger",
		Status:      "lobby",
		State:       state,
	})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	if !strings.Contains(q.lastSQL, "INSERT INTO games") {
		t.Fatalf("expected games insert SQL, got %q", q.lastSQL)
	}
	if out.ID != "g1" || out.OwnerUserID != "u1" || out.Name != "Fierce Badger" || out.Status != "lobby" {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestPostgresGamesStoreGetByID(t *testing.T) {
	now := time.Now().UTC()
	state := json.RawMessage(`{"phase":"attack"}`)
	q := &stubQuerier{
		row: &stubRow{values: []any{"g1", "u1", "Iron Legion", "in_progress", state, now, now}},
	}
	s := NewPostgresGamesStore()

	out, err := s.GetByID(context.Background(), q, "g1")
	if err != nil {
		t.Fatalf("get game: %v", err)
	}
	if !strings.Contains(q.lastSQL, "FROM games") {
		t.Fatalf("expected games select SQL, got %q", q.lastSQL)
	}
	if out.Status != "in_progress" || out.Name != "Iron Legion" {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestPostgresGamesStoreGetByIDForUpdate(t *testing.T) {
	now := time.Now().UTC()
	state := json.RawMessage(`{"phase":"attack"}`)
	q := &stubQuerier{
		row: &stubRow{values: []any{"g1", "u1", "Iron Legion", "in_progress", state, now, now}},
	}
	s := NewPostgresGamesStore()

	out, err := s.GetByIDForUpdate(context.Background(), q, "g1")
	if err != nil {
		t.Fatalf("get game for update: %v", err)
	}
	if !strings.Contains(q.lastSQL, "FOR UPDATE") {
		t.Fatalf("expected FOR UPDATE SQL, got %q", q.lastSQL)
	}
	if out.Status != "in_progress" {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestPostgresGamesStoreDelete(t *testing.T) {
	q := &stubQuerier{row: &stubRow{values: []any{"g1"}}}
	s := NewPostgresGamesStore()

	if err := s.Delete(context.Background(), q, "g1"); err != nil {
		t.Fatalf("delete game: %v", err)
	}
	if !strings.Contains(q.lastSQL, "DELETE FROM games") {
		t.Fatalf("expected games delete SQL, got %q", q.lastSQL)
	}
}

func TestPostgresGamesStoreDeleteNotFound(t *testing.T) {
	q := &stubQuerier{row: &stubRow{err: pgx.ErrNoRows}}
	s := NewPostgresGamesStore()

	err := s.Delete(context.Background(), q, "missing")
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected pgx.ErrNoRows, got %v", err)
	}
}

func TestPostgresGamesStoreUpdateState(t *testing.T) {
	now := time.Now().UTC()
	state := json.RawMessage(`{"phase":"fortify"}`)
	q := &stubQuerier{
		row: &stubRow{values: []any{"g1", "u1", "Iron Legion", "in_progress", state, now, now}},
	}
	s := NewPostgresGamesStore()

	out, err := s.UpdateState(context.Background(), q, UpdateGameState{
		GameID: "g1",
		Status: "in_progress",
		State:  state,
	})
	if err != nil {
		t.Fatalf("update game state: %v", err)
	}
	if !strings.Contains(q.lastSQL, "UPDATE games") {
		t.Fatalf("expected games update SQL, got %q", q.lastSQL)
	}
	if out.Status != "in_progress" {
		t.Fatalf("unexpected output: %#v", out)
	}
}
