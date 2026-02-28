package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"backend/internal/db"
	"backend/internal/store"
)

type fakeChatStore struct {
	createMessageFn func(context.Context, db.Querier, store.NewChatMessage) (store.ChatMessage, error)
	listMessagesFn  func(context.Context, db.Querier, string, int) ([]store.ChatMessage, error)
}

func (f *fakeChatStore) CreateMessage(ctx context.Context, q db.Querier, in store.NewChatMessage) (store.ChatMessage, error) {
	return f.createMessageFn(ctx, q, in)
}

func (f *fakeChatStore) ListMessages(ctx context.Context, q db.Querier, room string, limit int) ([]store.ChatMessage, error) {
	return f.listMessagesFn(ctx, q, room, limit)
}

func TestListLobbyMessages(t *testing.T) {
	svc := NewChatService(&fakeDB{q: noopQuerier{}}, &fakeChatStore{
		createMessageFn: func(context.Context, db.Querier, store.NewChatMessage) (store.ChatMessage, error) {
			return store.ChatMessage{}, nil
		},
		listMessagesFn: func(_ context.Context, _ db.Querier, room string, limit int) ([]store.ChatMessage, error) {
			if room != "lobby" || limit != 50 {
				t.Fatalf("unexpected list args room=%q limit=%d", room, limit)
			}
			return []store.ChatMessage{{ID: "m1"}}, nil
		},
	})

	out, err := svc.ListLobbyMessages(context.Background(), 0)
	if err != nil {
		t.Fatalf("list lobby messages: %v", err)
	}
	if len(out) != 1 || out[0].ID != "m1" {
		t.Fatalf("unexpected out: %#v", out)
	}
}

func TestPostLobbyMessage(t *testing.T) {
	now := time.Now().UTC()
	svc := NewChatService(&fakeDB{q: noopQuerier{}}, &fakeChatStore{
		createMessageFn: func(_ context.Context, _ db.Querier, in store.NewChatMessage) (store.ChatMessage, error) {
			if in.Room != "lobby" || in.UserID != "u1" || in.Body != "hello" {
				t.Fatalf("unexpected create args: %#v", in)
			}
			return store.ChatMessage{ID: "m1", Room: in.Room, UserID: in.UserID, Body: in.Body, CreatedAt: now}, nil
		},
		listMessagesFn: func(context.Context, db.Querier, string, int) ([]store.ChatMessage, error) { return nil, nil },
	})

	out, err := svc.PostLobbyMessage(context.Background(), "u1", "  hello ")
	if err != nil {
		t.Fatalf("post lobby message: %v", err)
	}
	if out.ID != "m1" || out.Body != "hello" {
		t.Fatalf("unexpected out: %#v", out)
	}
}

func TestPostLobbyMessageValidation(t *testing.T) {
	svc := NewChatService(&fakeDB{q: noopQuerier{}}, &fakeChatStore{
		createMessageFn: func(context.Context, db.Querier, store.NewChatMessage) (store.ChatMessage, error) {
			t.Fatalf("create should not be called on invalid input")
			return store.ChatMessage{}, nil
		},
		listMessagesFn: func(context.Context, db.Querier, string, int) ([]store.ChatMessage, error) { return nil, nil },
	})

	_, err := svc.PostLobbyMessage(context.Background(), "", "hello")
	if !errors.Is(err, ErrInvalidChatInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
	_, err = svc.PostLobbyMessage(context.Background(), "u1", "   ")
	if !errors.Is(err, ErrInvalidChatInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}
