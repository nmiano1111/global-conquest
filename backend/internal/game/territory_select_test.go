package game

import (
	"encoding/json"
	"testing"

	"backend/internal/proto/wsmsg"
)

func TestTerritorySelectRelaysToOtherClientsInRoom(t *testing.T) {
	s := NewServer()
	go s.Run()

	connA := &recordingConn{}
	clA := &Client{ID: "cA", UserID: "user-a", Conn: connA}
	s.Inbox() <- Register{C: clA}
	waitForTrue(t, func() bool {
		_, ok := connA.find(wsmsg.Type("hello"))
		return ok
	})
	s.Inbox() <- Incoming{ClientID: "cA", Env: wsmsg.Envelope{Type: wsmsg.TypeGameChatJoin, GameID: "game-1"}}

	connB := &recordingConn{}
	clB := &Client{ID: "cB", UserID: "user-b", Conn: connB}
	s.Inbox() <- Register{C: clB}
	waitForTrue(t, func() bool {
		_, ok := connB.find(wsmsg.Type("hello"))
		return ok
	})
	s.Inbox() <- Incoming{ClientID: "cB", Env: wsmsg.Envelope{Type: wsmsg.TypeGameChatJoin, GameID: "game-1"}}

	payload, _ := json.Marshal(wsmsg.TerritorySelectPayload{From: "Alaska", To: "Kamchatka"})
	s.Inbox() <- Incoming{ClientID: "cA", Env: wsmsg.Envelope{Type: wsmsg.TypeTerritorySelect, GameID: "game-1", Payload: payload}}

	waitForTrue(t, func() bool {
		_, ok := connB.find(wsmsg.TypeTerritorySelected)
		return ok
	})
	env, _ := connB.find(wsmsg.TypeTerritorySelected)
	var got wsmsg.TerritorySelectedPayload
	if err := json.Unmarshal(env.Payload, &got); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if got.UserID != "user-a" || got.From != "Alaska" || got.To != "Kamchatka" {
		t.Fatalf("unexpected relayed payload: %+v", got)
	}

	// The sender is never required to have gotten its own broadcast back —
	// it doesn't need it — but the important, testable invariant is that
	// it went out to the *room*, which we've already verified via connB.
}

func TestTerritorySelectRequiresChatRoomMembership(t *testing.T) {
	s := NewServer()
	go s.Run()

	conn := &recordingConn{}
	cl := &Client{ID: "c1", UserID: "user-a", Conn: conn}
	s.Inbox() <- Register{C: cl}
	waitForTrue(t, func() bool {
		_, ok := conn.find(wsmsg.Type("hello"))
		return ok
	})

	// Never joined the game's chat room.
	payload, _ := json.Marshal(wsmsg.TerritorySelectPayload{Territory: "Alaska"})
	s.Inbox() <- Incoming{ClientID: "c1", Env: wsmsg.Envelope{Type: wsmsg.TypeTerritorySelect, GameID: "game-1", Payload: payload}}

	waitForTrue(t, func() bool {
		_, ok := conn.find(wsmsg.TypeError)
		return ok
	})
	env, _ := conn.find(wsmsg.TypeError)
	var errPayload map[string]string
	if err := json.Unmarshal(env.Payload, &errPayload); err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	if errPayload["code"] != "not_in_room" {
		t.Fatalf("expected not_in_room error, got %+v", errPayload)
	}
}
