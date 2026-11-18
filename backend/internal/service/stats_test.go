package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/db"
	"github.com/nmiano1111/global-conquest/backend/internal/store"
)

type stubStatsDB struct{}

func (stubStatsDB) Queryer() db.Querier { return nil }

type stubResults struct {
	rows []store.PlayerResultRow
	got  store.GameResultsFilter
}

func (s *stubResults) ListPlayerResults(_ context.Context, _ db.Querier, f store.GameResultsFilter) ([]store.PlayerResultRow, error) {
	s.got = f
	return s.rows, nil
}

func iptr(v int) *int { return &v }

var t0 = time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC) // a Monday

// seat builds one seat row; combat stats are only set when hist is true.
func seat(game string, day int, players int, key string, isBot, won bool, place int, hist bool) store.PlayerResultRow {
	r := store.PlayerResultRow{
		GameID:      game,
		StartedAt:   t0.AddDate(0, 0, day).Add(-time.Hour),
		CompletedAt: t0.AddDate(0, 0, day),
		PlayerCount: players,
		TurnCount:   10 * players,
		PlayerKey:   key,
		DisplayName: key,
		IsBot:       isBot,
		Won:         won,
	}
	if isBot {
		r.BotStrategy = key[len("bot:"):]
	}
	if place > 0 {
		r.Placement = iptr(place)
	}
	if hist {
		r.Attacks, r.Conquests, r.Eliminations, r.CardTrades = iptr(5), iptr(2), iptr(0), iptr(1)
		r.ArmiesDestroyed, r.ArmiesLost = iptr(6), iptr(3)
		if won {
			r.Eliminations = iptr(1)
		}
	}
	return r
}

// fixture: alice plays 4 games: W, L, W, W.
func fixture() []store.PlayerResultRow {
	var rows []store.PlayerResultRow
	add := func(seats ...store.PlayerResultRow) { rows = append(rows, seats...) }
	add(
		seat("g1", 0, 2, "alice", false, true, 1, true),
		seat("g1", 0, 2, "bob", false, false, 2, true))
	add(
		seat("g2", 1, 3, "alice", false, false, 3, true),
		seat("g2", 1, 3, "bob", false, false, 2, true),
		seat("g2", 1, 3, "bot:killbot-v2", true, true, 1, true))
	add(
		seat("g3", 3, 2, "alice", false, true, 1, false),
		seat("g3", 3, 2, "bot:killbot-v2", true, false, 0, false))
	add(
		seat("g4", 9, 3, "alice", false, true, 1, true),
		seat("g4", 9, 3, "bot:killbot-v2", true, false, 2, true),
		seat("g4", 9, 3, "bot:killbot-v2", true, false, 3, true))
	return rows
}

func TestPlayerDetailSeriesAndSummary(t *testing.T) {
	d, err := buildPlayerDetail("alice", groupGames(fixture()))
	if err != nil {
		t.Fatal(err)
	}
	s := d.Summary
	if s.Games != 4 || s.Wins != 3 || s.Losses != 1 || s.WinPct != 75 {
		t.Fatalf("summary record = %+v", s)
	}
	if s.WLRatio == nil || *s.WLRatio != 3 {
		t.Fatalf("wl ratio = %v", s.WLRatio)
	}
	if s.CurrentStreak != 2 || s.LongestWinStreak != 2 {
		t.Fatalf("streaks = %d/%d", s.CurrentStreak, s.LongestWinStreak)
	}
	// g3 has no history: combat totals cover 3 games.
	if s.HistoryGames != 3 || s.ArmiesDestroyed != 18 || s.KDRatio == nil || *s.KDRatio != 2 {
		t.Fatalf("combat = %+v kd=%v", s, s.KDRatio)
	}
	// Expected win rate = mean(1/2, 1/3, 1/2, 1/3) = 41.67%.
	if s.ExpectedWinPct < 41.6 || s.ExpectedWinPct > 41.7 {
		t.Fatalf("expected win pct = %v", s.ExpectedWinPct)
	}
	if want := []bool{true, false, true, true}; len(s.Recent) != 4 || s.Recent[1] != want[1] {
		t.Fatalf("recent = %v", s.Recent)
	}

	if len(d.Series) != 4 {
		t.Fatalf("series len = %d", len(d.Series))
	}
	p := d.Series[1] // after the loss
	if p.CumWins != 1 || p.CumLosses != 1 || p.WinPct != 50 || *p.WLRatio != 1 {
		t.Fatalf("series[1] = %+v", p)
	}
	if d.Series[0].WLRatio != nil {
		t.Fatalf("wl ratio with zero losses should be nil")
	}

	// Head-to-head: two killbot seats in g4 count as one shared game.
	var kb *HeadToHead
	for i := range d.Opponents {
		if d.Opponents[i].Key == "bot:killbot-v2" {
			kb = &d.Opponents[i]
		}
	}
	if kb == nil || kb.Games != 3 || kb.Wins != 2 || kb.OpponentWins != 1 || kb.Name != "killbot-v2" {
		t.Fatalf("killbot h2h = %+v", kb)
	}
	// Comparable placements: g2 (bot ahead) and g4 (alice ahead); g3's bot has none.
	if kb.PlacementGames != 2 || kb.FinishedAhead != 1 {
		t.Fatalf("killbot placement h2h = %+v", kb)
	}

	if len(d.ByPlayerCount) != 2 || d.ByPlayerCount[0].PlayerCount != 2 || d.ByPlayerCount[0].Wins != 2 {
		t.Fatalf("by player count = %+v", d.ByPlayerCount)
	}
	if d.Games[0].GameID != "g4" || d.Games[0].DurationSeconds != 3600 {
		t.Fatalf("history should be newest first with duration: %+v", d.Games[0])
	}
}

func TestPlayerDetailUnknownPlayer(t *testing.T) {
	if _, err := buildPlayerDetail("nobody", groupGames(fixture())); !errors.Is(err, ErrStatsPlayerNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestSummarizePlayersOrdersByGamesAndNamesBotsByStrategy(t *testing.T) {
	out := summarizePlayers(groupGames(fixture()))
	if len(out) != 3 || out[0].Key != "alice" {
		t.Fatalf("order = %+v", out)
	}
	for _, p := range out {
		// g4's two killbot seats merge into one game: 3 games, 1 win, best
		// placement 2 in g4, combat summed across both seats. Even-odds
		// baseline = mean(1/3, 1/2, 2/3) = 50%.
		if p.Key == "bot:killbot-v2" {
			if p.Name != "killbot-v2" || p.Games != 3 || p.Wins != 1 || p.ExpectedWinPct != 50 {
				t.Fatalf("bot summary = %+v", p)
			}
			if *p.AvgPlacement != 1.5 || p.ArmiesDestroyed != 18 {
				t.Fatalf("merged seats: placement %v destroyed %d", *p.AvgPlacement, p.ArmiesDestroyed)
			}
		}
		if p.Key == "bob" && (p.CurrentStreak != -2 || p.LongestWinStreak != 0) {
			t.Fatalf("bob streak = %+v", p)
		}
	}
}

func TestBuildOverviewDailyBucketsAndMixedWinRate(t *testing.T) {
	rows := fixture()
	for i := range rows {
		switch rows[i].GameID {
		case "g1":
			rows[i].HumanCount, rows[i].BotCount = 2, 0
		case "g2":
			rows[i].HumanCount, rows[i].BotCount = 2, 1
		case "g3":
			rows[i].HumanCount, rows[i].BotCount = 1, 1
		case "g4":
			rows[i].HumanCount, rows[i].BotCount = 1, 2
		}
	}
	ov := buildOverview(groupGames(rows))
	if ov.Games != 4 || ov.HumanOnlyGames != 1 || ov.MixedGames != 3 || ov.HumanPlayers != 2 {
		t.Fatalf("totals = %+v", ov)
	}
	if ov.Bucket != "day" || len(ov.Series) != 10 {
		t.Fatalf("expected 10 contiguous daily buckets, got %s x %d", ov.Bucket, len(ov.Series))
	}
	if ov.Series[2].Games != 0 || ov.Series[2].AvgTurns != nil {
		t.Fatalf("empty bucket should be zero/nil: %+v", ov.Series[2])
	}
	if b := ov.Series[1]; b.MixedGames != 1 || b.HumanWinPctMixed == nil || *b.HumanWinPctMixed != 0 {
		t.Fatalf("day 1 mixed = %+v", b)
	}
	if len(ov.BotStrategies) != 1 {
		t.Fatalf("strategies = %+v", ov.BotStrategies)
	}
	kb := ov.BotStrategies[0]
	// 3 mixed games, 1 win; baseline = (1/3 + 1/2 + 2/3) / 3 = 50%.
	if kb.Games != 3 || kb.Wins != 1 || kb.ExpectedWinPct < 49.9 || kb.ExpectedWinPct > 50.1 {
		t.Fatalf("killbot record = %+v", kb)
	}
}

func TestBuildOverviewWeeklyBucketsStartMonday(t *testing.T) {
	rows := []store.PlayerResultRow{
		seat("a", 2, 2, "alice", false, true, 1, false),  // Wed Jan 7
		seat("b", 60, 2, "alice", false, true, 1, false), // > 6 weeks later
	}
	ov := buildOverview(groupGames(rows))
	if ov.Bucket != "week" {
		t.Fatalf("bucket = %s", ov.Bucket)
	}
	if !ov.Series[0].Start.Equal(time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("first week start = %v", ov.Series[0].Start)
	}
}

func TestStatsServicePassesFilterThrough(t *testing.T) {
	rs := &stubResults{rows: fixture()}
	svc := NewStatsService(stubStatsDB{}, rs)
	f := store.GameResultsFilter{Composition: store.CompositionMixed, PlayerCount: 3}
	if _, err := svc.Players(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if rs.got != f {
		t.Fatalf("filter = %+v", rs.got)
	}
}
