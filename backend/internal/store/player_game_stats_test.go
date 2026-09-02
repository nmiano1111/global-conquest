package store

import (
	"context"
	"strings"
	"testing"
)

func TestRecordGameResultInsertsOneRowPerResult(t *testing.T) {
	q := &stubQuerier{rows: &stubRows{}}
	s := NewPostgresPlayerGameStatsStore()

	results := []PlayerGameResult{
		{UserID: "u1", Won: true},
		{UserID: "u2", Won: false},
	}
	if err := s.RecordGameResult(context.Background(), q, "g1", 4, results); err != nil {
		t.Fatalf("record game result: %v", err)
	}
	if !strings.Contains(q.lastSQL, "ON CONFLICT (game_id, user_id)") {
		t.Fatalf("expected upsert on (game_id, user_id), got %q", q.lastSQL)
	}
	// stubQuerier only remembers the last call; confirm it reflects the
	// second (loser) result rather than the first.
	if len(q.lastArgs) != 4 || q.lastArgs[1] != "u2" || q.lastArgs[2] != false || q.lastArgs[3] != 4 {
		t.Fatalf("unexpected args for last insert: %#v", q.lastArgs)
	}
}

func TestRecordGameResultPropagatesQueryError(t *testing.T) {
	q := &stubQuerier{} // no rows configured, Query returns an error
	s := NewPostgresPlayerGameStatsStore()

	err := s.RecordGameResult(context.Background(), q, "g1", 2, []PlayerGameResult{{UserID: "u1", Won: true}})
	if err == nil {
		t.Fatal("expected an error when the underlying query fails")
	}
}

func TestGetLeaderboardParsesByPlayerCountBreakdown(t *testing.T) {
	q := &stubQuerier{
		rows: &stubRows{values: [][]any{
			{"u1", "alice", 3, 1, 4, []byte(`[{"player_count":3,"wins":1,"losses":0},{"player_count":5,"wins":2,"losses":1}]`)},
			{"u2", "bob", 1, 2, 3, []byte(`[{"player_count":5,"wins":1,"losses":2}]`)},
		}},
	}
	s := NewPostgresPlayerGameStatsStore()

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
	if len(out[0].ByPlayerCount) != 2 {
		t.Fatalf("expected 2 by-player-count records, got %#v", out[0].ByPlayerCount)
	}
	if out[0].ByPlayerCount[0] != (PlayerCountRecord{PlayerCount: 3, Wins: 1, Losses: 0}) {
		t.Fatalf("unexpected first by-player-count record: %#v", out[0].ByPlayerCount[0])
	}
	if out[0].ByPlayerCount[1] != (PlayerCountRecord{PlayerCount: 5, Wins: 2, Losses: 1}) {
		t.Fatalf("unexpected second by-player-count record: %#v", out[0].ByPlayerCount[1])
	}
}

func TestGetLeaderboardHandlesNilByPlayerCount(t *testing.T) {
	q := &stubQuerier{
		rows: &stubRows{values: [][]any{
			{"u1", "alice", 0, 0, 0, []byte(nil)},
		}},
	}
	s := NewPostgresPlayerGameStatsStore()

	out, err := s.GetLeaderboard(context.Background(), q, 20)
	if err != nil {
		t.Fatalf("get leaderboard: %v", err)
	}
	if len(out) != 1 || out[0].ByPlayerCount != nil {
		t.Fatalf("expected a nil breakdown for a NULL jsonb_agg, got %#v", out)
	}
}
