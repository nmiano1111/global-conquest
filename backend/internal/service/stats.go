package service

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/db"
	"github.com/nmiano1111/global-conquest/backend/internal/store"
)

// ErrStatsPlayerNotFound is returned by PlayerDetail when the player key has
// no completed games matching the filter.
var ErrStatsPlayerNotFound = errors.New("no completed games for player")

// rollingWindow is the number of most recent games RollingWinPct and
// PlayerSummary.Recent cover.
const rollingWindow = 10

type statsDB interface {
	Queryer() db.Querier
}

type gameResultsReader interface {
	ListPlayerResults(ctx context.Context, q db.Querier, filter store.GameResultsFilter) ([]store.PlayerResultRow, error)
}

// StatsService computes the admin player statistics (overview, per-player
// summaries, per-player time series) from the durable game_results tables.
// All aggregation happens in Go over the filtered rows; it is only reachable
// through admin-gated routes and does not itself re-check the caller's role.
type StatsService struct {
	db      statsDB
	results gameResultsReader
}

// NewStatsService constructs a StatsService.
func NewStatsService(db statsDB, results gameResultsReader) *StatsService {
	return &StatsService{db: db, results: results}
}

// PlayerSummary is one player's (a human, or every bot of one strategy)
// aggregate record over the filtered games. Ratio fields are nil when their
// denominator is zero; combat/action totals only cover games with the
// needed event history (HistoryGames).
type PlayerSummary struct {
	Key          string   `json:"key"`
	Name         string   `json:"name"`
	IsBot        bool     `json:"is_bot"`
	Games        int      `json:"games"`
	Wins         int      `json:"wins"`
	Losses       int      `json:"losses"`
	WinPct       float64  `json:"win_pct"`
	WLRatio      *float64 `json:"wl_ratio"`
	AvgPlacement *float64 `json:"avg_placement"`
	// ExpectedWinPct is the win rate a player of average skill would have
	// had in the same games (mean of 1/player_count) — a baseline for WinPct.
	ExpectedWinPct   float64   `json:"expected_win_pct"`
	HistoryGames     int       `json:"history_games"`
	Attacks          int       `json:"attacks"`
	Conquests        int       `json:"conquests"`
	Eliminations     int       `json:"eliminations"`
	CardTrades       int       `json:"card_trades"`
	ArmiesDestroyed  int       `json:"armies_destroyed"`
	ArmiesLost       int       `json:"armies_lost"`
	KDRatio          *float64  `json:"kd_ratio"`
	ConquestsPerGame *float64  `json:"conquests_per_game"`
	CurrentStreak    int       `json:"current_streak"` // >0 wins in a row, <0 losses in a row
	LongestWinStreak int       `json:"longest_win_streak"`
	FirstPlayed      time.Time `json:"first_played"`
	LastPlayed       time.Time `json:"last_played"`
	// Recent is the last rollingWindow results, oldest first.
	Recent []bool `json:"recent"`
}

// PlayerSeriesPoint is a player's running record immediately after one game.
type PlayerSeriesPoint struct {
	GameID        string    `json:"game_id"`
	CompletedAt   time.Time `json:"completed_at"`
	Won           bool      `json:"won"`
	Placement     *int      `json:"placement"`
	PlayerCount   int       `json:"player_count"`
	CumWins       int       `json:"cum_wins"`
	CumLosses     int       `json:"cum_losses"`
	WinPct        float64   `json:"win_pct"`
	WLRatio       *float64  `json:"wl_ratio"`
	RollingWinPct float64   `json:"rolling_win_pct"`
	// GameKD is this game's armies destroyed / lost (nil without history or
	// with zero losses).
	GameKD *float64 `json:"game_kd"`
}

// RecordBreakdown is a wins/losses split for one bucket (e.g. a player count).
type RecordBreakdown struct {
	PlayerCount    int     `json:"player_count"`
	Games          int     `json:"games"`
	Wins           int     `json:"wins"`
	Losses         int     `json:"losses"`
	WinPct         float64 `json:"win_pct"`
	ExpectedWinPct float64 `json:"expected_win_pct"`
}

// PlacementCount is how many games a player finished in a given place.
type PlacementCount struct {
	Placement int `json:"placement"`
	Count     int `json:"count"`
}

// HeadToHead summarizes the games a player shared with one opponent.
type HeadToHead struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	IsBot bool   `json:"is_bot"`
	Games int    `json:"games"`
	// Wins counts shared games the subject player won; OpponentWins those
	// the opponent won (the rest went to a third player).
	Wins         int `json:"wins"`
	OpponentWins int `json:"opponent_wins"`
	// FinishedAhead counts shared games where both placements are known and
	// the subject placed better.
	FinishedAhead  int `json:"finished_ahead"`
	PlacementGames int `json:"placement_games"`
}

// PlayerGameEntry is one game in a player's history, newest first.
type PlayerGameEntry struct {
	GameID          string    `json:"game_id"`
	CompletedAt     time.Time `json:"completed_at"`
	DurationSeconds int64     `json:"duration_seconds"`
	MapName         string    `json:"map_name"`
	PlayerCount     int       `json:"player_count"`
	TurnCount       int       `json:"turn_count"`
	Won             bool      `json:"won"`
	Placement       *int      `json:"placement"`
	WinnerName      string    `json:"winner_name"`
	Opponents       []string  `json:"opponents"`
	Conquests       *int      `json:"conquests"`
	Eliminations    *int      `json:"eliminations"`
	ArmiesDestroyed *int      `json:"armies_destroyed"`
	ArmiesLost      *int      `json:"armies_lost"`
}

// PlayerDetail is everything the admin per-player stats page shows.
type PlayerDetail struct {
	Summary       PlayerSummary       `json:"summary"`
	Series        []PlayerSeriesPoint `json:"series"`
	ByPlayerCount []RecordBreakdown   `json:"by_player_count"`
	Placements    []PlacementCount    `json:"placements"`
	Opponents     []HeadToHead        `json:"opponents"`
	Games         []PlayerGameEntry   `json:"games"`
}

// OverviewBucket aggregates the games completed in one time bucket.
type OverviewBucket struct {
	Start          time.Time `json:"start"`
	Games          int       `json:"games"`
	ActiveHumans   int       `json:"active_humans"`
	AvgTurns       *float64  `json:"avg_turns"`
	AvgDurationSec *float64  `json:"avg_duration_sec"`
	// MixedGames counts games with both humans and bots; HumanWinPctMixed is
	// the share of those a human won (nil when MixedGames is 0).
	MixedGames       int      `json:"mixed_games"`
	HumanWinPctMixed *float64 `json:"human_win_pct_mixed"`
}

// StrategyRecord is one bot strategy's record in mixed human/bot games.
type StrategyRecord struct {
	Strategy       string  `json:"strategy"`
	Games          int     `json:"games"`
	Wins           int     `json:"wins"`
	WinPct         float64 `json:"win_pct"`
	ExpectedWinPct float64 `json:"expected_win_pct"`
}

// PlayerCountGames is how many games were played at one seat count.
type PlayerCountGames struct {
	PlayerCount int `json:"player_count"`
	Games       int `json:"games"`
}

// StatsOverview is the admin stats landing page's site-wide summary.
type StatsOverview struct {
	Games          int                `json:"games"`
	HumanPlayers   int                `json:"human_players"`
	HumanOnlyGames int                `json:"human_only_games"`
	MixedGames     int                `json:"mixed_games"`
	BotOnlyGames   int                `json:"bot_only_games"`
	AvgTurns       *float64           `json:"avg_turns"`
	AvgDurationSec *float64           `json:"avg_duration_sec"`
	Bucket         string             `json:"bucket"` // "day" or "week"
	Series         []OverviewBucket   `json:"series"`
	ByPlayerCount  []PlayerCountGames `json:"by_player_count"`
	BotStrategies  []StrategyRecord   `json:"bot_strategies"`
}

// gameGroup is one completed game: its metadata (from the first seat row)
// plus every seat.
type gameGroup struct {
	meta  store.PlayerResultRow
	seats []store.PlayerResultRow
}

func groupGames(rows []store.PlayerResultRow) []gameGroup {
	var out []gameGroup
	for _, r := range rows {
		if n := len(out); n > 0 && out[n-1].meta.GameID == r.GameID {
			out[n-1].seats = append(out[n-1].seats, r)
			continue
		}
		out = append(out, gameGroup{meta: r, seats: []store.PlayerResultRow{r}})
	}
	return out
}

func (g gameGroup) winner() *store.PlayerResultRow {
	for i := range g.seats {
		if g.seats[i].Won {
			return &g.seats[i]
		}
	}
	return nil
}

// subject is one stats subject's participation in a game: a human's seat,
// or every seat of one bot strategy merged (won if any seat won, best known
// placement, summed activity), so a strategy fielding two bots in one game
// counts as one game, not a win plus a loss.
type subject struct {
	row   store.PlayerResultRow
	seats int
}

// subjects merges g's seats by PlayerKey, in first-seat order.
func (g gameGroup) subjects() []subject {
	var out []subject
	idx := map[string]int{}
	for _, seat := range g.seats {
		i, ok := idx[seat.PlayerKey]
		if !ok {
			idx[seat.PlayerKey] = len(out)
			out = append(out, subject{row: seat, seats: 1})
			continue
		}
		m := &out[i]
		m.seats++
		m.row.Won = m.row.Won || seat.Won
		if seat.Placement != nil && (m.row.Placement == nil || *seat.Placement < *m.row.Placement) {
			m.row.Placement = seat.Placement
		}
		for _, f := range []struct {
			dst **int
			src *int
		}{
			{&m.row.TurnsTaken, seat.TurnsTaken}, {&m.row.Attacks, seat.Attacks},
			{&m.row.Conquests, seat.Conquests}, {&m.row.Eliminations, seat.Eliminations},
			{&m.row.CardTrades, seat.CardTrades}, {&m.row.ArmiesDestroyed, seat.ArmiesDestroyed},
			{&m.row.ArmiesLost, seat.ArmiesLost},
		} {
			if *f.dst != nil && f.src != nil {
				v := **f.dst + *f.src
				*f.dst = &v
			}
		}
	}
	return out
}

// expectedShare is a subject's even-odds chance of winning g: seats/players.
func (sub subject) expectedShare(g gameGroup) float64 {
	return float64(sub.seats) / float64(max(g.meta.PlayerCount, 1))
}

func ratio(num, den int) *float64 {
	if den == 0 {
		return nil
	}
	v := float64(num) / float64(den)
	return &v
}

func pct(num, den int) float64 {
	if den == 0 {
		return 0
	}
	return 100 * float64(num) / float64(den)
}

func (s *StatsService) load(ctx context.Context, f store.GameResultsFilter) ([]gameGroup, error) {
	rows, err := s.results.ListPlayerResults(ctx, s.db.Queryer(), f)
	if err != nil {
		return nil, err
	}
	return groupGames(rows), nil
}

// Players returns a summary per player (humans and bot strategies) over the
// filtered games, ordered by games played descending.
func (s *StatsService) Players(ctx context.Context, f store.GameResultsFilter) ([]PlayerSummary, error) {
	games, err := s.load(ctx, f)
	if err != nil {
		return nil, err
	}
	return summarizePlayers(games), nil
}

// summaryAcc accumulates one player's PlayerSummary across games in
// chronological order.
type summaryAcc struct {
	sum                   PlayerSummary
	placementSum, placedN int
	expectedSum           float64
	streak                int
	results               []bool
}

func (a *summaryAcc) add(g gameGroup, sub subject) {
	seat := sub.row
	sm := &a.sum
	if sm.Games == 0 {
		sm.FirstPlayed = g.meta.CompletedAt
	}
	sm.Name = seat.DisplayName
	if seat.IsBot {
		sm.Name = seat.BotStrategy
	}
	sm.LastPlayed = g.meta.CompletedAt
	sm.Games++
	if seat.Won {
		sm.Wins++
		if a.streak < 0 {
			a.streak = 0
		}
		a.streak++
		if a.streak > sm.LongestWinStreak {
			sm.LongestWinStreak = a.streak
		}
	} else {
		sm.Losses++
		if a.streak > 0 {
			a.streak = 0
		}
		a.streak--
	}
	a.results = append(a.results, seat.Won)
	a.expectedSum += sub.expectedShare(g)
	if seat.Placement != nil {
		a.placementSum += *seat.Placement
		a.placedN++
	}
	if seat.Conquests != nil && seat.ArmiesDestroyed != nil {
		sm.HistoryGames++
		sm.Attacks += deref(seat.Attacks)
		sm.Conquests += deref(seat.Conquests)
		sm.Eliminations += deref(seat.Eliminations)
		sm.CardTrades += deref(seat.CardTrades)
		sm.ArmiesDestroyed += deref(seat.ArmiesDestroyed)
		sm.ArmiesLost += deref(seat.ArmiesLost)
	}
}

func (a *summaryAcc) finish() PlayerSummary {
	sm := a.sum
	sm.WinPct = pct(sm.Wins, sm.Games)
	sm.WLRatio = ratio(sm.Wins, sm.Losses)
	sm.ExpectedWinPct = 100 * a.expectedSum / float64(max(sm.Games, 1))
	if a.placedN > 0 {
		v := float64(a.placementSum) / float64(a.placedN)
		sm.AvgPlacement = &v
	}
	sm.KDRatio = ratio(sm.ArmiesDestroyed, sm.ArmiesLost)
	sm.ConquestsPerGame = ratio(sm.Conquests, sm.HistoryGames)
	sm.CurrentStreak = a.streak
	start := max(len(a.results)-rollingWindow, 0)
	sm.Recent = append([]bool{}, a.results[start:]...)
	return sm
}

func deref(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func summarizePlayers(games []gameGroup) []PlayerSummary {
	accs := map[string]*summaryAcc{}
	for _, g := range games {
		for _, sub := range g.subjects() {
			a, ok := accs[sub.row.PlayerKey]
			if !ok {
				a = &summaryAcc{sum: PlayerSummary{Key: sub.row.PlayerKey, IsBot: sub.row.IsBot}}
				accs[sub.row.PlayerKey] = a
			}
			a.add(g, sub)
		}
	}
	out := make([]PlayerSummary, 0, len(accs))
	for _, a := range accs {
		out = append(out, a.finish())
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Games != out[j].Games {
			return out[i].Games > out[j].Games
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// PlayerDetail returns one player's summary, per-game running series,
// breakdowns, head-to-head records, and game history over the filtered
// games. It returns ErrStatsPlayerNotFound if the player has none.
func (s *StatsService) PlayerDetail(ctx context.Context, key string, f store.GameResultsFilter) (PlayerDetail, error) {
	games, err := s.load(ctx, f)
	if err != nil {
		return PlayerDetail{}, err
	}
	return buildPlayerDetail(key, games)
}

func buildPlayerDetail(key string, games []gameGroup) (PlayerDetail, error) {
	var acc summaryAcc
	acc.sum.Key = key
	d := PlayerDetail{}
	byCount := map[int]*RecordBreakdown{}
	byCountExpected := map[int]float64{}
	placements := map[int]int{}
	opps := map[string]*HeadToHead{}
	var window []bool

	for _, g := range games {
		subs := g.subjects()
		var mine *subject
		for i := range subs {
			if subs[i].row.PlayerKey == key {
				mine = &subs[i]
				break
			}
		}
		if mine == nil {
			continue
		}
		me := &mine.row
		acc.sum.IsBot = me.IsBot
		acc.add(g, *mine)

		window = append(window, me.Won)
		if len(window) > rollingWindow {
			window = window[1:]
		}
		rw := 0
		for _, w := range window {
			if w {
				rw++
			}
		}
		pt := PlayerSeriesPoint{
			GameID:        g.meta.GameID,
			CompletedAt:   g.meta.CompletedAt,
			Won:           me.Won,
			Placement:     me.Placement,
			PlayerCount:   g.meta.PlayerCount,
			CumWins:       acc.sum.Wins,
			CumLosses:     acc.sum.Losses,
			WinPct:        pct(acc.sum.Wins, acc.sum.Games),
			WLRatio:       ratio(acc.sum.Wins, acc.sum.Losses),
			RollingWinPct: pct(rw, len(window)),
		}
		if me.ArmiesDestroyed != nil && me.ArmiesLost != nil {
			pt.GameKD = ratio(*me.ArmiesDestroyed, *me.ArmiesLost)
		}
		d.Series = append(d.Series, pt)

		bc, ok := byCount[g.meta.PlayerCount]
		if !ok {
			bc = &RecordBreakdown{PlayerCount: g.meta.PlayerCount}
			byCount[g.meta.PlayerCount] = bc
		}
		bc.Games++
		byCountExpected[g.meta.PlayerCount] += mine.expectedShare(g)
		if me.Won {
			bc.Wins++
		} else {
			bc.Losses++
		}
		if me.Placement != nil {
			placements[*me.Placement]++
		}

		winner := g.winner()
		entry := PlayerGameEntry{
			GameID:          g.meta.GameID,
			CompletedAt:     g.meta.CompletedAt,
			DurationSeconds: int64(g.meta.CompletedAt.Sub(g.meta.StartedAt).Seconds()),
			MapName:         g.meta.MapName,
			PlayerCount:     g.meta.PlayerCount,
			TurnCount:       g.meta.TurnCount,
			Won:             me.Won,
			Placement:       me.Placement,
			Conquests:       me.Conquests,
			Eliminations:    me.Eliminations,
			ArmiesDestroyed: me.ArmiesDestroyed,
			ArmiesLost:      me.ArmiesLost,
			Opponents:       []string{},
		}
		if winner != nil {
			entry.WinnerName = winner.DisplayName
		}
		for _, o := range g.seats {
			if o.PlayerKey != key {
				entry.Opponents = append(entry.Opponents, o.DisplayName)
			}
		}
		for _, sub := range subs {
			o := sub.row
			if o.PlayerKey == key {
				continue
			}
			h, ok := opps[o.PlayerKey]
			if !ok {
				h = &HeadToHead{Key: o.PlayerKey, IsBot: o.IsBot}
				opps[o.PlayerKey] = h
			}
			h.Name = o.DisplayName
			if o.IsBot {
				h.Name = o.BotStrategy
			}
			h.Games++
			if me.Won {
				h.Wins++
			}
			if winner != nil && winner.PlayerKey == o.PlayerKey {
				h.OpponentWins++
			}
			if me.Placement != nil && o.Placement != nil {
				h.PlacementGames++
				if *me.Placement < *o.Placement {
					h.FinishedAhead++
				}
			}
		}
		d.Games = append(d.Games, entry)
	}

	if acc.sum.Games == 0 {
		return PlayerDetail{}, ErrStatsPlayerNotFound
	}
	d.Summary = acc.finish()

	for _, bc := range byCount {
		bc.WinPct = pct(bc.Wins, bc.Games)
		bc.ExpectedWinPct = 100 * byCountExpected[bc.PlayerCount] / float64(bc.Games)
		d.ByPlayerCount = append(d.ByPlayerCount, *bc)
	}
	sort.Slice(d.ByPlayerCount, func(i, j int) bool { return d.ByPlayerCount[i].PlayerCount < d.ByPlayerCount[j].PlayerCount })

	for p, n := range placements {
		d.Placements = append(d.Placements, PlacementCount{Placement: p, Count: n})
	}
	sort.Slice(d.Placements, func(i, j int) bool { return d.Placements[i].Placement < d.Placements[j].Placement })

	for _, h := range opps {
		d.Opponents = append(d.Opponents, *h)
	}
	sort.Slice(d.Opponents, func(i, j int) bool {
		if d.Opponents[i].Games != d.Opponents[j].Games {
			return d.Opponents[i].Games > d.Opponents[j].Games
		}
		return d.Opponents[i].Key < d.Opponents[j].Key
	})

	// Newest game first for the history table.
	for i, j := 0, len(d.Games)-1; i < j; i, j = i+1, j-1 {
		d.Games[i], d.Games[j] = d.Games[j], d.Games[i]
	}
	if d.Placements == nil {
		d.Placements = []PlacementCount{}
	}
	if d.Opponents == nil {
		d.Opponents = []HeadToHead{}
	}
	return d, nil
}

// Overview returns site-wide totals and a per-day or per-week series over
// the filtered games.
func (s *StatsService) Overview(ctx context.Context, f store.GameResultsFilter) (StatsOverview, error) {
	games, err := s.load(ctx, f)
	if err != nil {
		return StatsOverview{}, err
	}
	return buildOverview(games), nil
}

// bucketStart truncates t (UTC) to the start of its day, or of its
// ISO week (Monday) when bucket is "week".
func bucketStart(t time.Time, bucket string) time.Time {
	t = t.UTC()
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	if bucket == "day" {
		return day
	}
	offset := (int(day.Weekday()) + 6) % 7 // Monday = 0
	return day.AddDate(0, 0, -offset)
}

func buildOverview(games []gameGroup) StatsOverview {
	ov := StatsOverview{Bucket: "week", Series: []OverviewBucket{}, ByPlayerCount: []PlayerCountGames{}, BotStrategies: []StrategyRecord{}}
	if len(games) == 0 {
		return ov
	}
	// Daily buckets when the data spans under ~6 weeks, otherwise weekly.
	first, last := games[0].meta.CompletedAt, games[len(games)-1].meta.CompletedAt
	if last.Sub(first) < 42*24*time.Hour {
		ov.Bucket = "day"
	}

	type bucketAcc struct {
		b              OverviewBucket
		turns, dur     float64
		humans         map[string]bool
		mixedHumanWins int
	}
	buckets := map[time.Time]*bucketAcc{}
	humans := map[string]bool{}
	byCount := map[int]int{}
	type stratAcc struct {
		games, wins int
		expected    float64
	}
	strats := map[string]*stratAcc{}
	var turnSum, durSum float64

	for _, g := range games {
		m := g.meta
		ov.Games++
		dur := m.CompletedAt.Sub(m.StartedAt).Seconds()
		turnSum += float64(m.TurnCount)
		durSum += dur
		byCount[m.PlayerCount]++
		mixed := m.HumanCount > 0 && m.BotCount > 0
		switch {
		case m.BotCount == 0:
			ov.HumanOnlyGames++
		case m.HumanCount == 0:
			ov.BotOnlyGames++
		default:
			ov.MixedGames++
		}

		start := bucketStart(m.CompletedAt, ov.Bucket)
		ba, ok := buckets[start]
		if !ok {
			ba = &bucketAcc{b: OverviewBucket{Start: start}, humans: map[string]bool{}}
			buckets[start] = ba
		}
		ba.b.Games++
		ba.turns += float64(m.TurnCount)
		ba.dur += dur
		winner := g.winner()
		if mixed {
			ba.b.MixedGames++
			if winner != nil && !winner.IsBot {
				ba.mixedHumanWins++
			}
		}

		seenStrat := map[string]bool{}
		for _, seat := range g.seats {
			if !seat.IsBot {
				humans[seat.PlayerKey] = true
				ba.humans[seat.PlayerKey] = true
				continue
			}
			if !mixed || seenStrat[seat.BotStrategy] {
				continue
			}
			seenStrat[seat.BotStrategy] = true
			sa, ok := strats[seat.BotStrategy]
			if !ok {
				sa = &stratAcc{}
				strats[seat.BotStrategy] = sa
			}
			sa.games++
			// A strategy with k seats in an n-player game has a k/n baseline.
			seatsOfStrat := 0
			for _, o := range g.seats {
				if o.IsBot && o.BotStrategy == seat.BotStrategy {
					seatsOfStrat++
				}
			}
			sa.expected += float64(seatsOfStrat) / float64(max(m.PlayerCount, 1))
			if winner != nil && winner.IsBot && winner.BotStrategy == seat.BotStrategy {
				sa.wins++
			}
		}
	}

	ov.HumanPlayers = len(humans)
	ov.AvgTurns = ratioF(turnSum, ov.Games)
	ov.AvgDurationSec = ratioF(durSum, ov.Games)

	// Emit every bucket from first to last, including empty ones, so the
	// series has a continuous time axis.
	end := bucketStart(last, ov.Bucket)
	for t := bucketStart(first, ov.Bucket); !t.After(end); {
		b := OverviewBucket{Start: t}
		if ba, ok := buckets[t]; ok {
			b = ba.b
			b.ActiveHumans = len(ba.humans)
			b.AvgTurns = ratioF(ba.turns, b.Games)
			b.AvgDurationSec = ratioF(ba.dur, b.Games)
			if b.MixedGames > 0 {
				v := pct(ba.mixedHumanWins, b.MixedGames)
				b.HumanWinPctMixed = &v
			}
		}
		ov.Series = append(ov.Series, b)
		if ov.Bucket == "day" {
			t = t.AddDate(0, 0, 1)
		} else {
			t = t.AddDate(0, 0, 7)
		}
	}

	for pc, n := range byCount {
		ov.ByPlayerCount = append(ov.ByPlayerCount, PlayerCountGames{PlayerCount: pc, Games: n})
	}
	sort.Slice(ov.ByPlayerCount, func(i, j int) bool { return ov.ByPlayerCount[i].PlayerCount < ov.ByPlayerCount[j].PlayerCount })

	for name, sa := range strats {
		ov.BotStrategies = append(ov.BotStrategies, StrategyRecord{
			Strategy:       name,
			Games:          sa.games,
			Wins:           sa.wins,
			WinPct:         pct(sa.wins, sa.games),
			ExpectedWinPct: 100 * sa.expected / float64(sa.games),
		})
	}
	sort.Slice(ov.BotStrategies, func(i, j int) bool { return ov.BotStrategies[i].Games > ov.BotStrategies[j].Games })
	return ov
}

func ratioF(num float64, den int) *float64 {
	if den == 0 {
		return nil
	}
	v := num / float64(den)
	return &v
}
