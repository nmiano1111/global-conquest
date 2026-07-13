package store

import (
	"github.com/nmiano1111/global-conquest/backend/internal/risk"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPostgresGameDomainEventStoreInsert(t *testing.T) {
	now := time.Now().UTC()
	payload := json.RawMessage(`{"schema_version":1}`)
	q := &stubQuerier{
		row: &stubRow{
			values: []any{
				"evt-id-1",
				"game-id-1",
				int64(1),
				risk.EventTypeCombatRollResolved,
				risk.EventVersionCombatRollResolved,
				"actor-id-1",
				now,
				payload,
			},
		},
	}
	s := NewPostgresGameDomainEventStore()

	ev := risk.DomainEvent{
		Type:          risk.EventTypeCombatRollResolved,
		Version:       risk.EventVersionCombatRollResolved,
		ActorPlayerID: "actor-id-1",
		Payload:       nil,
	}
	out, err := s.InsertDomainEvent(context.Background(), q, "game-id-1", ev, []byte(`{}`))
	if err != nil {
		t.Fatalf("InsertDomainEvent: %v", err)
	}
	if !strings.Contains(q.lastSQL, "INSERT INTO game_domain_events") {
		t.Fatalf("expected INSERT SQL, got: %q", q.lastSQL)
	}
	if !strings.Contains(q.lastSQL, "UPDATE games") {
		t.Fatalf("expected games sequence UPDATE in CTE, got: %q", q.lastSQL)
	}
	if !strings.Contains(q.lastSQL, "event_sequence") {
		t.Fatalf("expected event_sequence in SQL, got: %q", q.lastSQL)
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
	if out.EventType != risk.EventTypeCombatRollResolved {
		t.Fatalf("unexpected EventType: %q", out.EventType)
	}
	if out.EventVersion != risk.EventVersionCombatRollResolved {
		t.Fatalf("unexpected EventVersion: %d", out.EventVersion)
	}
	if out.ActorPlayerID != "actor-id-1" {
		t.Fatalf("unexpected ActorPlayerID: %q", out.ActorPlayerID)
	}
}

func TestPostgresGameDomainEventStoreInsertSQLArgs(t *testing.T) {
	now := time.Now().UTC()
	payload := json.RawMessage(`{}`)
	q := &stubQuerier{
		row: &stubRow{
			values: []any{
				"eid", "gid", int64(2),
				"combat_roll_resolved", int16(1), "", now, payload,
			},
		},
	}
	s := NewPostgresGameDomainEventStore()

	ev := risk.DomainEvent{
		Type:          risk.EventTypeCombatRollResolved,
		Version:       risk.EventVersionCombatRollResolved,
		ActorPlayerID: "",
	}
	_, err := s.InsertDomainEvent(context.Background(), q, "gid", ev, []byte(`{}`))
	if err != nil {
		t.Fatalf("InsertDomainEvent: %v", err)
	}
	// First arg must be the game ID
	if len(q.lastArgs) < 1 || q.lastArgs[0] != "gid" {
		t.Fatalf("expected game ID as first arg, got: %v", q.lastArgs)
	}
}
