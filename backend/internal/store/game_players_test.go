package store

import (
	"context"
	"testing"
)

func TestSetGameWinnerUpdatesWonFlag(t *testing.T) {
	q := &stubQuerier{rows: &stubRows{}}
	s := NewPostgresGamePlayersStore()

	if err := s.SetGameWinner(context.Background(), q, "g1", "u1"); err != nil {
		t.Fatalf("set game winner: %v", err)
	}
	if q.lastArgs[0] != "g1" || q.lastArgs[1] != "u1" {
		t.Fatalf("unexpected args: %#v", q.lastArgs)
	}
}
