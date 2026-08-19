package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPostgresGameReplayEventStoreInsert(t *testing.T) {
	now := time.Now().UTC()
	payload := json.RawMessage(`{"action":"attack"}`)
	q := &stubQuerier{
		row: &stubRow{
			values: []any{
				"evt-id-1",
				"game-id-1",
				int64(1),
				now,
				"actor-id-1",
				"attack",
				payload,
			},
		},
	}
	s := NewPostgresGameReplayEventStore()

	out, err := s.InsertReplayEvent(context.Background(), q, "game-id-1", "actor-id-1", "attack", []byte(`{"action":"attack"}`))
	if err != nil {
		t.Fatalf("InsertReplayEvent: %v", err)
	}
	if !strings.Contains(q.lastSQL, "INSERT INTO game_replay_events") {
		t.Fatalf("expected INSERT SQL, got: %q", q.lastSQL)
	}
	if !strings.Contains(q.lastSQL, "UPDATE games") {
		t.Fatalf("expected games sequence UPDATE in CTE, got: %q", q.lastSQL)
	}
	if out.ID != "evt-id-1" {
		t.Fatalf("unexpected ID: %q", out.ID)
	}
	if out.GameID != "game-id-1" {
		t.Fatalf("unexpected GameID: %q", out.GameID)
	}
	if out.GameSequence != 1 {
		t.Fatalf("unexpected GameSequence: %d", out.GameSequence)
	}
	if out.ActionType != "attack" {
		t.Fatalf("unexpected ActionType: %q", out.ActionType)
	}
	if out.ActorPlayerID != "actor-id-1" {
		t.Fatalf("unexpected ActorPlayerID: %q", out.ActorPlayerID)
	}
}

func TestPostgresGameReplayEventStoreInsertSQLArgs(t *testing.T) {
	now := time.Now().UTC()
	payload := json.RawMessage(`{}`)
	q := &stubQuerier{
		row: &stubRow{
			values: []any{
				"eid", "gid", int64(2), now, "", "end_turn", payload,
			},
		},
	}
	s := NewPostgresGameReplayEventStore()

	_, err := s.InsertReplayEvent(context.Background(), q, "gid", "", "end_turn", []byte(`{}`))
	if err != nil {
		t.Fatalf("InsertReplayEvent: %v", err)
	}
	if len(q.lastArgs) < 1 || q.lastArgs[0] != "gid" {
		t.Fatalf("expected game ID as first arg, got: %v", q.lastArgs)
	}
}

func TestPostgresGameReplayEventStoreList(t *testing.T) {
	now := time.Now().UTC()
	q := &stubQuerier{
		rows: &stubRows{
			values: [][]any{
				{"id-1", "gid", int64(1), now, sql.NullString{String: "actor-1", Valid: true}, "place_reinforcement", json.RawMessage(`{}`)},
				{"id-2", "gid", int64(2), now, sql.NullString{}, "end_turn", json.RawMessage(`{}`)},
			},
		},
	}
	s := NewPostgresGameReplayEventStore()

	out, err := s.ListReplayEvents(context.Background(), q, "gid", 0, 100)
	if err != nil {
		t.Fatalf("ListReplayEvents: %v", err)
	}
	if !strings.Contains(q.lastSQL, "FROM game_replay_events") {
		t.Fatalf("expected SELECT from game_replay_events, got: %q", q.lastSQL)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 events, got %d", len(out))
	}
	if out[0].GameSequence != 1 || out[1].GameSequence != 2 {
		t.Fatalf("unexpected sequence order: %+v", out)
	}
	if out[0].ActorPlayerID != "actor-1" {
		t.Fatalf("unexpected actor: %q", out[0].ActorPlayerID)
	}
	if out[1].ActorPlayerID != "" {
		t.Fatalf("expected empty actor for system action, got %q", out[1].ActorPlayerID)
	}
}

func TestPostgresGameReplayEventStoreReplayEventsExist(t *testing.T) {
	q := &stubQuerier{row: &stubRow{values: []any{true}}}
	s := NewPostgresGameReplayEventStore()

	exists, err := s.ReplayEventsExist(context.Background(), q, "gid")
	if err != nil {
		t.Fatalf("ReplayEventsExist: %v", err)
	}
	if !exists {
		t.Fatalf("expected exists=true")
	}
	if !strings.Contains(q.lastSQL, "EXISTS") {
		t.Fatalf("expected EXISTS SQL, got: %q", q.lastSQL)
	}
}
