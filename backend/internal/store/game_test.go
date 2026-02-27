package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPostgresGamesStoreCreate(t *testing.T) {
	now := time.Now().UTC()
	state := json.RawMessage(`{"phase":"setup_claim"}`)
	q := &stubQuerier{
		row: &stubRow{values: []any{"g1", "u1", "lobby", state, now, now}},
	}
	s := NewPostgresGamesStore()

	out, err := s.Create(context.Background(), q, NewGame{
		OwnerUserID: "u1",
		Status:      "lobby",
		State:       state,
	})
	if err != nil {
		t.Fatalf("create game: %v", err)
	}
	if !strings.Contains(q.lastSQL, "INSERT INTO games") {
		t.Fatalf("expected games insert SQL, got %q", q.lastSQL)
	}
	if out.ID != "g1" || out.OwnerUserID != "u1" || out.Status != "lobby" {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestPostgresGamesStoreGetByID(t *testing.T) {
	now := time.Now().UTC()
	state := json.RawMessage(`{"phase":"attack"}`)
	q := &stubQuerier{
		row: &stubRow{values: []any{"g1", "u1", "in_progress", state, now, now}},
	}
	s := NewPostgresGamesStore()

	out, err := s.GetByID(context.Background(), q, "g1")
	if err != nil {
		t.Fatalf("get game: %v", err)
	}
	if !strings.Contains(q.lastSQL, "FROM games") {
		t.Fatalf("expected games select SQL, got %q", q.lastSQL)
	}
	if out.Status != "in_progress" {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestPostgresGamesStoreUpdateState(t *testing.T) {
	now := time.Now().UTC()
	state := json.RawMessage(`{"phase":"fortify"}`)
	q := &stubQuerier{
		row: &stubRow{values: []any{"g1", "u1", "in_progress", state, now, now}},
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
