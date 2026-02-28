package store

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPostgresChatStoreCreateMessage(t *testing.T) {
	now := time.Now().UTC()
	q := &stubQuerier{
		row: &stubRow{values: []any{"m1", "lobby", "u1", "alice", "hello", now}},
	}
	s := NewPostgresChatStore()

	out, err := s.CreateMessage(context.Background(), q, NewChatMessage{
		Room:   "lobby",
		UserID: "u1",
		Body:   "hello",
	})
	if err != nil {
		t.Fatalf("create message: %v", err)
	}
	if !strings.Contains(q.lastSQL, "INSERT INTO chat_messages") {
		t.Fatalf("expected insert SQL, got %q", q.lastSQL)
	}
	if len(q.lastArgs) != 3 || q.lastArgs[0] != "lobby" || q.lastArgs[1] != "u1" || q.lastArgs[2] != "hello" {
		t.Fatalf("unexpected args: %#v", q.lastArgs)
	}
	if out.ID != "m1" || out.UserName != "alice" || out.Body != "hello" {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestPostgresChatStoreListMessages(t *testing.T) {
	now := time.Now().UTC()
	q := &stubQuerier{
		rows: &stubRows{values: [][]any{
			{"m2", "lobby", "u2", "bob", "second", now.Add(2 * time.Second)},
			{"m1", "lobby", "u1", "alice", "first", now},
		}},
	}
	s := NewPostgresChatStore()

	out, err := s.ListMessages(context.Background(), q, "lobby", 50)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if !strings.Contains(q.lastSQL, "FROM chat_messages") {
		t.Fatalf("expected chat select SQL, got %q", q.lastSQL)
	}
	if len(q.lastArgs) != 2 || q.lastArgs[0] != "lobby" || q.lastArgs[1] != 50 {
		t.Fatalf("unexpected args: %#v", q.lastArgs)
	}
	if len(out) != 2 || out[0].ID != "m1" || out[1].ID != "m2" {
		t.Fatalf("expected ascending order by created_at, got %#v", out)
	}
	if !reflect.DeepEqual([]string{out[0].Body, out[1].Body}, []string{"first", "second"}) {
		t.Fatalf("unexpected bodies: %#v", out)
	}
}
