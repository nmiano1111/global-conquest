package game

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/proto/wsmsg"
)

// findLast returns the most recently sent envelope of type t, since a
// presence broadcast (unlike most other message types exercised by
// recordingConn.find) is expected to be sent more than once per
// connection as room membership changes.
func findLast(c *recordingConn, t wsmsg.Type) (wsmsg.Envelope, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.sent) - 1; i >= 0; i-- {
		if c.sent[i].Type == t {
			return c.sent[i], true
		}
	}
	return wsmsg.Envelope{}, false
}

func decodePresence(t *testing.T, env wsmsg.Envelope) wsmsg.GamePresencePayload {
	t.Helper()
	var got wsmsg.GamePresencePayload
	if err := json.Unmarshal(env.Payload, &got); err != nil {
		t.Fatalf("decode presence payload: %v", err)
	}
	return got
}

func sameUserIDs(t *testing.T, got []string, want ...string) bool {
	t.Helper()
	if len(got) != len(want) {
		return false
	}
	gotSorted := slices.Clone(got)
	wantSorted := slices.Clone(want)
	slices.Sort(gotSorted)
	slices.Sort(wantSorted)
	return slices.Equal(gotSorted, wantSorted)
}

func TestGamePresenceBroadcastsOnJoinLeaveAndDisconnect(t *testing.T) {
	s := NewServer()
	go s.Run()

	connA := &recordingConn{}
	clA := &Client{ID: "cA", UserID: "user-a", Conn: connA}
	s.Inbox() <- Register{C: clA}
	waitForTrue(t, func() bool { _, ok := connA.find(wsmsg.Type("hello")); return ok })
	s.Inbox() <- Incoming{ClientID: "cA", Env: wsmsg.Envelope{Type: wsmsg.TypeGameChatJoin, GameID: "game-1"}}

	waitForTrue(t, func() bool {
		env, ok := findLast(connA, wsmsg.TypeGamePresence)
		return ok && sameUserIDs(t, decodePresence(t, env).UserIDs, "user-a")
	})

	connB := &recordingConn{}
	clB := &Client{ID: "cB", UserID: "user-b", Conn: connB}
	s.Inbox() <- Register{C: clB}
	waitForTrue(t, func() bool { _, ok := connB.find(wsmsg.Type("hello")); return ok })
	s.Inbox() <- Incoming{ClientID: "cB", Env: wsmsg.Envelope{Type: wsmsg.TypeGameChatJoin, GameID: "game-1"}}

	// Both existing member A and new joiner B should now see both viewers.
	waitForTrue(t, func() bool {
		env, ok := findLast(connA, wsmsg.TypeGamePresence)
		return ok && sameUserIDs(t, decodePresence(t, env).UserIDs, "user-a", "user-b")
	})
	waitForTrue(t, func() bool {
		env, ok := findLast(connB, wsmsg.TypeGamePresence)
		return ok && sameUserIDs(t, decodePresence(t, env).UserIDs, "user-a", "user-b")
	})

	// B explicitly leaves: A should see the presence set shrink back down.
	s.Inbox() <- Incoming{ClientID: "cB", Env: wsmsg.Envelope{Type: wsmsg.TypeGameChatLeave, GameID: "game-1"}}
	waitForTrue(t, func() bool {
		env, ok := findLast(connA, wsmsg.TypeGamePresence)
		return ok && sameUserIDs(t, decodePresence(t, env).UserIDs, "user-a")
	})

	// B rejoins, then disconnects without an explicit leave: A should still
	// see the presence set shrink back down, since handleDisconnect also
	// leaves the chat room.
	s.Inbox() <- Incoming{ClientID: "cB", Env: wsmsg.Envelope{Type: wsmsg.TypeGameChatJoin, GameID: "game-1"}}
	waitForTrue(t, func() bool {
		env, ok := findLast(connA, wsmsg.TypeGamePresence)
		return ok && sameUserIDs(t, decodePresence(t, env).UserIDs, "user-a", "user-b")
	})
	s.Inbox() <- Unregister{ClientID: "cB"}
	waitForTrue(t, func() bool {
		env, ok := findLast(connA, wsmsg.TypeGamePresence)
		return ok && sameUserIDs(t, decodePresence(t, env).UserIDs, "user-a")
	})
}

func TestGamePresenceOmitsAnonymousClients(t *testing.T) {
	s := NewServer()
	go s.Run()

	connAnon := &recordingConn{}
	clAnon := &Client{ID: "cAnon", UserID: "", Conn: connAnon}
	s.Inbox() <- Register{C: clAnon}
	waitForTrue(t, func() bool { _, ok := connAnon.find(wsmsg.Type("hello")); return ok })
	s.Inbox() <- Incoming{ClientID: "cAnon", Env: wsmsg.Envelope{Type: wsmsg.TypeGameChatJoin, GameID: "game-2"}}

	waitForTrue(t, func() bool {
		_, ok := findLast(connAnon, wsmsg.TypeGamePresence)
		return ok
	})
	env, _ := findLast(connAnon, wsmsg.TypeGamePresence)
	if got := decodePresence(t, env).UserIDs; len(got) != 0 {
		t.Fatalf("expected an anonymous client to contribute no user ID, got %v", got)
	}
}
