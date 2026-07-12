package game

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"backend/internal/proto/wsmsg"
)

var ErrForTest = errors.New("test error")

// recordingConn captures every envelope sent to it.
type recordingConn struct {
	mu   sync.Mutex
	sent []wsmsg.Envelope
}

func (c *recordingConn) Send(env wsmsg.Envelope) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, env)
	return true
}

func (c *recordingConn) find(t wsmsg.Type) (wsmsg.Envelope, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.sent {
		if e.Type == t {
			return e, true
		}
	}
	return wsmsg.Envelope{}, false
}

// fakeActionService is a minimal GameActionService double.
type fakeActionService struct {
	mu    sync.Mutex
	calls []GameActionInput
	resp  GameActionUpdate
	err   error
}

func (f *fakeActionService) ApplyGameAction(_ context.Context, in GameActionInput) (GameActionUpdate, error) {
	f.mu.Lock()
	f.calls = append(f.calls, in)
	f.mu.Unlock()
	if f.err != nil {
		return GameActionUpdate{}, f.err
	}
	resp := f.resp
	resp.GameID = in.GameID
	resp.Action = in.Action
	resp.ActorUserID = in.PlayerUserID
	return resp, nil
}

func waitForTrue(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("condition not met within timeout")
}

func TestSubmitGameActionUsesNormalPathAndBroadcasts(t *testing.T) {
	s := NewServer()
	go s.Run()

	actions := &fakeActionService{resp: GameActionUpdate{Phase: "reinforce"}}
	s.SetGameActionService(actions)

	var triggered []string
	var triggerMu sync.Mutex
	s.SetBotTrigger(func(gameID string) {
		triggerMu.Lock()
		triggered = append(triggered, gameID)
		triggerMu.Unlock()
	})

	// Join a client into the game's chat room so broadcastGameStateUpdate
	// has somewhere to deliver the update.
	conn := &recordingConn{}
	cl := &Client{ID: "c1", UserID: "human-1", Conn: conn}
	s.Inbox() <- Register{C: cl}
	waitForTrue(t, func() bool {
		_, ok := conn.find(wsmsg.Type("hello"))
		return ok
	})
	s.Inbox() <- Incoming{ClientID: "c1", Env: wsmsg.Envelope{Type: wsmsg.TypeGameChatJoin, GameID: "game-1"}}

	update, err := s.SubmitGameAction(context.Background(), GameActionInput{
		GameID:       "game-1",
		PlayerUserID: "bot-1",
		Action:       "end_turn",
	})
	if err != nil {
		t.Fatalf("SubmitGameAction: %v", err)
	}
	if update.GameID != "game-1" || update.ActorUserID != "bot-1" {
		t.Fatalf("unexpected update: %+v", update)
	}

	actions.mu.Lock()
	calls := actions.calls
	actions.mu.Unlock()
	if len(calls) != 1 || calls[0].GameID != "game-1" || calls[0].PlayerUserID != "bot-1" || calls[0].Action != "end_turn" {
		t.Fatalf("expected ApplyGameAction to be called once with the bot's input, got %+v", calls)
	}

	env, ok := conn.find(wsmsg.TypeGameStateUpdated)
	if !ok {
		t.Fatalf("expected the joined client to receive a game_state_updated broadcast")
	}
	var payload wsmsg.GameStateUpdatedPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		t.Fatalf("decode broadcast payload: %v", err)
	}
	if payload.ActorUserID != "bot-1" {
		t.Fatalf("expected broadcast actor_user_id=bot-1, got %q", payload.ActorUserID)
	}

	triggerMu.Lock()
	got := append([]string(nil), triggered...)
	triggerMu.Unlock()
	if len(got) != 1 || got[0] != "game-1" {
		t.Fatalf("expected bot trigger to fire once for game-1, got %v", got)
	}
}

func TestSubmitGameActionPropagatesApplicationError(t *testing.T) {
	s := NewServer()
	go s.Run()

	actions := &fakeActionService{err: ErrForTest}
	s.SetGameActionService(actions)
	var triggered bool
	s.SetBotTrigger(func(string) { triggered = true })

	_, err := s.SubmitGameAction(context.Background(), GameActionInput{
		GameID:       "game-1",
		PlayerUserID: "bot-1",
		Action:       "end_turn",
	})
	if err != ErrForTest {
		t.Fatalf("expected ErrForTest, got %v", err)
	}
	if triggered {
		t.Fatalf("bot trigger must not fire when the application rejects the action")
	}
}

func TestSubmitGameActionRespectsContextCancellation(t *testing.T) {
	s := NewServer()
	// Deliberately do not run s.Run(): the inbox send must fail fast on a
	// canceled context rather than hang forever.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.SubmitGameAction(ctx, GameActionInput{GameID: "game-1", PlayerUserID: "bot-1", Action: "end_turn"})
	if err == nil {
		t.Fatalf("expected an error from a canceled context")
	}
}

func TestHumanGameActionAlsoTriggersBotTrigger(t *testing.T) {
	s := NewServer()
	go s.Run()

	actions := &fakeActionService{resp: GameActionUpdate{Phase: "reinforce"}}
	s.SetGameActionService(actions)
	var triggered []string
	var mu sync.Mutex
	s.SetBotTrigger(func(gameID string) {
		mu.Lock()
		triggered = append(triggered, gameID)
		mu.Unlock()
	})

	conn := &recordingConn{}
	cl := &Client{ID: "c1", UserID: "human-1", Conn: conn}
	s.Inbox() <- Register{C: cl}
	waitForTrue(t, func() bool {
		_, ok := conn.find(wsmsg.Type("hello"))
		return ok
	})
	s.Inbox() <- Incoming{ClientID: "c1", Env: wsmsg.Envelope{Type: wsmsg.TypeGameChatJoin, GameID: "game-1"}}

	payload, _ := json.Marshal(wsmsg.GameActionPayload{Action: "end_turn"})
	s.Inbox() <- Incoming{ClientID: "c1", Env: wsmsg.Envelope{Type: wsmsg.TypeGameAction, GameID: "game-1", Payload: payload}}

	waitForTrue(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(triggered) == 1
	})
}
