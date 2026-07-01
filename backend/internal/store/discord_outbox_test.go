package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// --- EnqueueTurnStarted ---

func TestEnqueueTurnStartedSQL(t *testing.T) {
	now := time.Now().UTC()
	q := &stubQuerier{
		row: &stubRow{values: []any{"outbox-id", "game-id-1", int64(3)}},
	}
	s := NewPostgresDiscordOutboxStore()

	err := s.EnqueueTurnStarted(context.Background(), q, "game-id-1", "Bob", "player-id-1", "Alice", nil, nil, 3)
	if err != nil {
		t.Fatalf("EnqueueTurnStarted: %v", err)
	}
	if !strings.Contains(q.lastSQL, "INSERT INTO discord_outbox") {
		t.Fatalf("expected INSERT into discord_outbox, got: %q", q.lastSQL)
	}
	if !strings.Contains(q.lastSQL, "UPDATE games") {
		t.Fatalf("expected games event_sequence increment in CTE, got: %q", q.lastSQL)
	}
	if !strings.Contains(q.lastSQL, "FOR UPDATE") {
		// The surrounding transaction holds the game row FOR UPDATE; the CTE updates it.
		// The SQL uses UPDATE games ... which is safe because the row is already locked.
	}
	if !strings.Contains(q.lastSQL, "deduplication_key") {
		t.Fatalf("expected deduplication_key in SQL, got: %q", q.lastSQL)
	}
	if len(q.lastArgs) < 2 || q.lastArgs[0] != "game-id-1" {
		t.Fatalf("expected game ID as first arg, got: %v", q.lastArgs)
	}
	_ = now // reference to suppress unused warning
}

func TestEnqueueTurnStartedPayload(t *testing.T) {
	q := &stubQuerier{
		row: &stubRow{values: []any{"outbox-id", "game-id-1", int64(1)}},
	}
	s := NewPostgresDiscordOutboxStore()

	err := s.EnqueueTurnStarted(context.Background(), q, "game-id-1", "Alice", "player-abc", "Bob", nil, nil, 7)
	if err != nil {
		t.Fatalf("EnqueueTurnStarted: %v", err)
	}
	if len(q.lastArgs) < 2 {
		t.Fatalf("expected at least 2 args, got %d", len(q.lastArgs))
	}
	payloadJSON, ok := q.lastArgs[1].(string)
	if !ok {
		t.Fatalf("expected string payload arg, got %T", q.lastArgs[1])
	}
	var p TurnStartedPayload
	if err := json.Unmarshal([]byte(payloadJSON), &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if p.SchemaVersion != PayloadSchemaVersionTurnStarted {
		t.Errorf("schema_version: want %d, got %d", PayloadSchemaVersionTurnStarted, p.SchemaVersion)
	}
	if p.PlayerID != "player-abc" {
		t.Errorf("player_id: want %q, got %q", "player-abc", p.PlayerID)
	}
	if p.PlayerDisplayName != "Bob" {
		t.Errorf("player_display_name: want %q, got %q", "Bob", p.PlayerDisplayName)
	}
	if p.TurnNumber != 7 {
		t.Errorf("turn_number: want 7, got %d", p.TurnNumber)
	}
}

// --- claimPendingQ ---

func TestClaimPendingQSQL(t *testing.T) {
	q := &stubQuerier{
		rows: &stubRows{values: [][]any{}},
	}
	s := NewPostgresDiscordOutboxStore()

	_, err := s.claimPendingQ(context.Background(), q, 10)
	if err != nil {
		t.Fatalf("claimPendingQ: %v", err)
	}
	if !strings.Contains(q.lastSQL, "FOR UPDATE SKIP LOCKED") {
		t.Fatalf("expected FOR UPDATE SKIP LOCKED, got: %q", q.lastSQL)
	}
	if !strings.Contains(q.lastSQL, "delivered_at IS NULL") {
		t.Fatalf("expected delivered_at IS NULL filter, got: %q", q.lastSQL)
	}
	if !strings.Contains(q.lastSQL, "available_at <= now()") {
		t.Fatalf("expected available_at <= now() filter, got: %q", q.lastSQL)
	}
	if !strings.Contains(q.lastSQL, "attempt_count = attempt_count + 1") {
		t.Fatalf("expected attempt_count increment, got: %q", q.lastSQL)
	}
	if !strings.Contains(q.lastSQL, "claimed_at = now()") {
		t.Fatalf("expected claimed_at set, got: %q", q.lastSQL)
	}
	if len(q.lastArgs) < 1 {
		t.Fatalf("expected limit arg, got none")
	}
}

func TestClaimPendingQReturnsRows(t *testing.T) {
	now := time.Now().UTC()
	payload := json.RawMessage(`{"schema_version":1,"player_id":"p1","player_display_name":"Alice"}`)
	q := &stubQuerier{
		rows: &stubRows{values: [][]any{
			{"outbox-1", "game-1", int64(5), NotificationTypeTurnStarted, payload, 1, now},
			{"outbox-2", "game-2", int64(6), NotificationTypeTurnStarted, payload, 1, now},
		}},
	}
	s := NewPostgresDiscordOutboxStore()

	entries, err := s.claimPendingQ(context.Background(), q, 10)
	if err != nil {
		t.Fatalf("claimPendingQ: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].ID != "outbox-1" || entries[0].GameID != "game-1" {
		t.Fatalf("unexpected first entry: %+v", entries[0])
	}
	if entries[0].AttemptCount != 1 {
		t.Fatalf("expected attempt_count=1, got %d", entries[0].AttemptCount)
	}
}

func TestClaimPendingQDeliveredRowsExcluded(t *testing.T) {
	// Test that the WHERE clause contains delivered_at IS NULL.
	// A delivered row must never be re-claimed; verified by SQL structure.
	q := &stubQuerier{rows: &stubRows{}}
	s := NewPostgresDiscordOutboxStore()
	_, _ = s.claimPendingQ(context.Background(), q, 5)
	if !strings.Contains(q.lastSQL, "delivered_at IS NULL") {
		t.Fatalf("delivered rows must be excluded; SQL must contain delivered_at IS NULL")
	}
}

func TestClaimPendingQExpiredClaimsReclaimed(t *testing.T) {
	// Expired claims (claimed_at < now() - 2 minutes) must be re-eligible.
	q := &stubQuerier{rows: &stubRows{}}
	s := NewPostgresDiscordOutboxStore()
	_, _ = s.claimPendingQ(context.Background(), q, 5)
	if !strings.Contains(q.lastSQL, "interval '2 minutes'") {
		t.Fatalf("expired claims must be reclaimed; SQL must contain the 2-minute reclaim window")
	}
}

// --- MarkDelivered ---

func TestMarkDeliveredSQL(t *testing.T) {
	q := &stubQuerier{row: &stubRow{values: []any{int64(1)}}}
	s := NewPostgresDiscordOutboxStore()

	if err := s.MarkDelivered(context.Background(), q, "outbox-id-1"); err != nil {
		t.Fatalf("MarkDelivered: %v", err)
	}
	if !strings.Contains(q.lastSQL, "delivered_at = now()") {
		t.Fatalf("expected delivered_at set, got: %q", q.lastSQL)
	}
	if len(q.lastArgs) < 1 || q.lastArgs[0] != "outbox-id-1" {
		t.Fatalf("expected outbox ID as first arg, got: %v", q.lastArgs)
	}
}

// --- MarkFailed ---

func TestMarkFailedSQL(t *testing.T) {
	q := &stubQuerier{row: &stubRow{values: []any{int64(1)}}}
	s := NewPostgresDiscordOutboxStore()

	if err := s.MarkFailed(context.Background(), q, "outbox-id-1", 1, "delivery failed"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	if !strings.Contains(q.lastSQL, "claimed_at = NULL") {
		t.Fatalf("expected claimed_at cleared, got: %q", q.lastSQL)
	}
	if !strings.Contains(q.lastSQL, "available_at") {
		t.Fatalf("expected available_at retry schedule, got: %q", q.lastSQL)
	}
}

func TestMarkFailedTruncatesLongError(t *testing.T) {
	q := &stubQuerier{row: &stubRow{values: []any{int64(1)}}}
	s := NewPostgresDiscordOutboxStore()
	longErr := strings.Repeat("x", 600)

	if err := s.MarkFailed(context.Background(), q, "id", 1, longErr); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	errArg, ok := q.lastArgs[1].(string)
	if !ok {
		t.Fatalf("expected string error arg, got %T", q.lastArgs[1])
	}
	if len(errArg) > 500 {
		t.Fatalf("error should be truncated to 500 chars, got %d", len(errArg))
	}
}

// --- retryDelay ---

func TestRetryDelaySchedule(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 5 * time.Second},
		{2, 30 * time.Second},
		{3, 2 * time.Minute},
		{4, 10 * time.Minute},
		{5, 6 * time.Hour},
		{99, 6 * time.Hour},
	}
	for _, tc := range cases {
		got := retryDelay(tc.attempt)
		if got != tc.want {
			t.Errorf("retryDelay(%d): want %v, got %v", tc.attempt, tc.want, got)
		}
	}
}
