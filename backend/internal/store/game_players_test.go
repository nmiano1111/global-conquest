package store

import (
	"context"
	"strings"
	"testing"
)

func TestGetLeaderboardSQLExcludesBotGames(t *testing.T) {
	q := &stubQuerier{rows: &stubRows{}}
	s := NewPostgresGamePlayersStore()

	if _, err := s.GetLeaderboard(context.Background(), q, 20); err != nil {
		t.Fatalf("get leaderboard: %v", err)
	}
	if !strings.Contains(q.lastSQL, "NOT EXISTS") || !strings.Contains(q.lastSQL, "jsonb_array_elements") {
		t.Fatalf("expected the leaderboard query to exclude games with any bot player, got %q", q.lastSQL)
	}
	if !strings.Contains(q.lastSQL, `p ->> 'controller' = 'bot'`) {
		t.Fatalf("expected the query to check controller = 'bot', got %q", q.lastSQL)
	}
}

func TestGetLeaderboardReturnsRows(t *testing.T) {
	q := &stubQuerier{
		rows: &stubRows{values: [][]any{
			{"u1", "alice", 3, 1, 4},
			{"u2", "bob", 1, 2, 3},
		}},
	}
	s := NewPostgresGamePlayersStore()

	out, err := s.GetLeaderboard(context.Background(), q, 20)
	if err != nil {
		t.Fatalf("get leaderboard: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(out))
	}
	if out[0].UserName != "alice" || out[0].Wins != 3 || out[0].Losses != 1 || out[0].GamesPlayed != 4 {
		t.Fatalf("unexpected first entry: %#v", out[0])
	}
}
