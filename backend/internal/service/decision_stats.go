package service

import (
	"context"
	"sort"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/db"
	"github.com/nmiano1111/global-conquest/backend/internal/store"
)

type decisionReader interface {
	ListForPlayer(ctx context.Context, q db.Querier, playerKey string, f store.GameResultsFilter) ([]store.AttackDecisionRow, error)
}

// SetDecisionReader wires the attack-decision store used by PlayerDecisions.
func (s *StatsService) SetDecisionReader(r decisionReader) { s.decisions = r }

// DecisionOddsBucket compares, for one win-probability band, the attacks a
// player started against the attacks that were available when they did.
type DecisionOddsBucket struct {
	Label string `json:"label"`
	// Attacks is how many new attacks (not repeat rolls) started in this band.
	Attacks int `json:"attacks"`
	// ChosenPct is Attacks as a share of all new attacks.
	ChosenPct float64 `json:"chosen_pct"`
	// AvailablePct is this band's share of the legal attacks available at
	// those same decision points.
	AvailablePct float64 `json:"available_pct"`
	// Preference is ChosenPct ÷ AvailablePct: >1 means picked more often
	// than its availability alone would predict.
	Preference *float64 `json:"preference"`
	// CapturePct is how often those attacks ended in a capture (stopping
	// early counts as not capturing); ExpectedCapturePct is the average
	// odds of capturing when fought to a conclusion.
	CapturePct         *float64 `json:"capture_pct"`
	ExpectedCapturePct *float64 `json:"expected_capture_pct"`
}

// DecisionFeature compares how often chosen attacks had a target property
// against how often available attacks had it.
type DecisionFeature struct {
	Key          string   `json:"key"`
	Label        string   `json:"label"`
	Chosen       int      `json:"chosen"`
	ChosenPct    float64  `json:"chosen_pct"`
	AvailablePct float64  `json:"available_pct"`
	Preference   *float64 `json:"preference"`
}

// DecisionStopBucket counts attack-phase stops by the best odds left on the table.
type DecisionStopBucket struct {
	Label string  `json:"label"`
	Stops int     `json:"stops"`
	Pct   float64 `json:"pct"`
}

// DecisionGamePoint is one game's attack decisions, for time series.
type DecisionGamePoint struct {
	GameID      string    `json:"game_id"`
	CompletedAt time.Time `json:"completed_at"`
	Attacks     int       `json:"attacks"`
	// AvgWinProb is the mean odds (%) of the attacks started in this game.
	AvgWinProb *float64 `json:"avg_win_prob"`
	// CumKillLuckPct is defender armies destroyed vs. expected from the
	// dice rolled, cumulative through this game (+5 = 5% more than expected).
	CumKillLuckPct *float64 `json:"cum_kill_luck_pct"`
}

// DecisionPlaceStat is how a player treats one territory (or continent)
// as an attack target, compared with how often it was attackable.
type DecisionPlaceStat struct {
	Name string `json:"name"`
	// Continent is set for territories ("" if the territory has none).
	Continent string `json:"continent,omitempty"`
	// Attacks is how many attacks they started against it.
	Attacks int `json:"attacks"`
	// Available is how many of their attack decisions it could be attacked at.
	Available int `json:"available"`
	// TakePct is Attacks ÷ Available: how often they went for it when they could.
	TakePct      float64  `json:"take_pct"`
	ChosenPct    float64  `json:"chosen_pct"`
	AvailablePct float64  `json:"available_pct"`
	Preference   *float64 `json:"preference"`
	// AvgWinProbChosen / AvgWinProbAvailable are mean odds (%) when they
	// attacked it vs. whenever it was attackable: a big gap means they wait
	// for good odds here (or accept bad ones).
	AvgWinProbChosen    *float64 `json:"avg_win_prob_chosen"`
	AvgWinProbAvailable *float64 `json:"avg_win_prob_available"`
	CapturePct          *float64 `json:"capture_pct"`
}

// PlayerDecisionStats summarizes a player's attack-phase decisions.
type PlayerDecisionStats struct {
	// Games is how many of the player's games have decision data.
	Games   int `json:"games"`
	Rolls   int `json:"rolls"`
	Attacks int `json:"attacks"`
	Stops   int `json:"stops"`
	// AvgWinProb is the mean odds (%) of attacks started.
	AvgWinProb *float64 `json:"avg_win_prob"`
	// AvgRollsPerAttack is rolls per attack started.
	AvgRollsPerAttack *float64 `json:"avg_rolls_per_attack"`
	// AbandonedPct is the share of attacks stopped before capturing.
	AbandonedPct *float64 `json:"abandoned_pct"`
	// MaxDicePct is the share of rolls using the most dice allowed.
	MaxDicePct *float64 `json:"max_dice_pct"`
	// StopsWithGoodOddsPct is the share of stops that left an attack with
	// at least 75% odds available.
	StopsWithGoodOddsPct *float64 `json:"stops_with_good_odds_pct"`
	// KillLuckPct / LossLuckPct compare armies destroyed / lost with what
	// the dice counts rolled would produce on average (+ = more than expected).
	KillLuckPct *float64             `json:"kill_luck_pct"`
	LossLuckPct *float64             `json:"loss_luck_pct"`
	OddsBuckets []DecisionOddsBucket `json:"odds_buckets"`
	Features    []DecisionFeature    `json:"features"`
	StopBuckets []DecisionStopBucket `json:"stop_buckets"`
	Series      []DecisionGamePoint  `json:"series"`
	// PlaceAttacks is how many attacks have per-territory data (games
	// extracted before it existed and since deleted lack it).
	PlaceAttacks int `json:"place_attacks"`
	// Maps holds per-territory stats separately for each map the player
	// has attacked on, most attacks first. Shares are within that map.
	Maps []DecisionMapStats `json:"maps"`
}

// DecisionMapStats is a player's attack targets on one map.
type DecisionMapStats struct {
	// MapID is "" for the classic map.
	MapID string `json:"map_id"`
	// MapName is the map's name as of its most recent game ("Classic" for the classic map).
	MapName     string              `json:"map_name"`
	Attacks     int                 `json:"attacks"`
	Continents  []DecisionPlaceStat `json:"continents"`
	Territories []DecisionPlaceStat `json:"territories"`
}

// mapTally accumulates one map's per-territory and per-continent stats.
type mapTally struct {
	id, name                   string
	attacks                    int
	territories, continents    placeTally
	placeSlots, continentSlots int
}

type mapTallies struct {
	byID  map[string]*mapTally
	order []string
}

func (m *mapTallies) get(r store.AttackDecisionRow) *mapTally {
	if m.byID == nil {
		m.byID = map[string]*mapTally{}
	}
	t, ok := m.byID[r.MapID]
	if !ok {
		t = &mapTally{id: r.MapID}
		m.byID[r.MapID] = t
		m.order = append(m.order, r.MapID)
	}
	// Rows arrive oldest game first, so this ends on the latest name.
	t.name = r.MapName
	if r.MapID == "" {
		t.name = "Classic"
	}
	return t
}

func (m *mapTallies) finish() []DecisionMapStats {
	out := make([]DecisionMapStats, 0, len(m.order))
	for _, id := range m.order {
		t := m.byID[id]
		out = append(out, DecisionMapStats{
			MapID:       t.id,
			MapName:     t.name,
			Attacks:     t.attacks,
			Territories: t.territories.finish(t.attacks, t.placeSlots),
			Continents:  t.continents.finish(t.attacks, t.continentSlots),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Attacks > out[j].Attacks })
	return out
}

type placeAcc struct {
	stat                        DecisionPlaceStat
	chosenProbSum, availProbSum float64
	captured                    int
}

// placeTally accumulates territories (or continents) within one map.
type placeTally struct {
	byName map[string]*placeAcc
	order  []string
}

func (t *placeTally) get(name, continent string) *placeAcc {
	if t.byName == nil {
		t.byName = map[string]*placeAcc{}
	}
	a, ok := t.byName[name]
	if !ok {
		a = &placeAcc{stat: DecisionPlaceStat{Name: name, Continent: continent}}
		t.byName[name] = a
		t.order = append(t.order, name)
	}
	return a
}

// finish computes shares against the totals and returns places sorted by
// attacks started, then availability.
func (t *placeTally) finish(attacks, slots int) []DecisionPlaceStat {
	out := make([]DecisionPlaceStat, 0, len(t.order))
	for _, k := range t.order {
		a := t.byName[k]
		st := a.stat
		if st.Available > 0 {
			st.TakePct = 100 * float64(st.Attacks) / float64(st.Available)
			st.AvgWinProbAvailable = ptr(100 * a.availProbSum / float64(st.Available))
		}
		if attacks > 0 {
			st.ChosenPct = 100 * float64(st.Attacks) / float64(attacks)
		}
		if slots > 0 {
			st.AvailablePct = 100 * float64(st.Available) / float64(slots)
		}
		if st.AvailablePct > 0 {
			st.Preference = ptr(st.ChosenPct / st.AvailablePct)
		}
		if st.Attacks > 0 {
			st.AvgWinProbChosen = ptr(100 * a.chosenProbSum / float64(st.Attacks))
			st.CapturePct = ptr(100 * float64(a.captured) / float64(st.Attacks))
		}
		out = append(out, st)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Attacks != out[j].Attacks {
			return out[i].Attacks > out[j].Attacks
		}
		return out[i].Available > out[j].Available
	})
	return out
}

var oddsBucketLabels = []string{"<25%", "25–50%", "50–75%", "75–90%", "≥90%"}

// PlayerDecisions returns a player's attack-decision stats for games
// matching f. A player with no decision data gets zero-valued stats.
func (s *StatsService) PlayerDecisions(ctx context.Context, key string, f store.GameResultsFilter) (*PlayerDecisionStats, error) {
	if s.decisions == nil {
		return buildDecisionStats(nil), nil
	}
	rows, err := s.decisions.ListForPlayer(ctx, s.db.Queryer(), key, f)
	if err != nil {
		return nil, err
	}
	return buildDecisionStats(rows), nil
}

func buildDecisionStats(rows []store.AttackDecisionRow) *PlayerDecisionStats {
	out := &PlayerDecisionStats{
		OddsBuckets: make([]DecisionOddsBucket, numWinProbBuckets),
		StopBuckets: make([]DecisionStopBucket, numWinProbBuckets+1),
		Series:      []DecisionGamePoint{},
	}
	for i := range out.OddsBuckets {
		out.OddsBuckets[i].Label = oddsBucketLabels[i]
	}
	out.StopBuckets[0].Label = "No attack possible"
	for i := range numWinProbBuckets {
		out.StopBuckets[i+1].Label = "Best " + oddsBucketLabels[i]
	}

	var (
		availByBucket                 [numWinProbBuckets]int
		availOptions                  int
		capturedByBucket              [numWinProbBuckets]int
		winProbSumByBucket            [numWinProbBuckets]float64
		winProbSum                    float64
		abandoned, maxDice, goodStops int
		killActual, killExpected      float64
		lossActual, lossExpected      float64
	)
	type feat struct {
		key, label   string
		chosen       func(store.AttackDecisionRow) bool
		available    func(store.AttackDecisionRow) int
		nChosen, nAv int
	}
	feats := []*feat{
		{key: "completes_continent", label: "Completes a continent",
			chosen: func(r store.AttackDecisionRow) bool { return deref(r.CompletesContinent) }, available: func(r store.AttackDecisionRow) int { return r.OptionsCompletesContinent }},
		{key: "breaks_continent", label: "Breaks an enemy continent",
			chosen: func(r store.AttackDecisionRow) bool { return deref(r.BreaksContinent) }, available: func(r store.AttackDecisionRow) int { return r.OptionsBreaksContinent }},
		{key: "vs_leader", label: "Targets the strongest opponent",
			chosen: func(r store.AttackDecisionRow) bool { return deref(r.TargetsLeader) }, available: func(r store.AttackDecisionRow) int { return r.OptionsVsLeader }},
		{key: "vs_weakest", label: "Targets the weakest opponent",
			chosen: func(r store.AttackDecisionRow) bool { return deref(r.TargetsWeakest) }, available: func(r store.AttackDecisionRow) int { return r.OptionsVsWeakest }},
	}

	var maps mapTallies

	// The open attack (engagement), its odds bucket, and its target's
	// tallies (nil without per-territory data); closed when the next
	// non-continued decision arrives or a roll captures.
	engaged, engagedBucket := false, 0
	var engagedPlaces []*placeAcc
	closeEngagement := func(captured bool) {
		if !engaged {
			return
		}
		if captured {
			capturedByBucket[engagedBucket]++
			for _, p := range engagedPlaces {
				p.captured++
			}
		} else {
			abandoned++
		}
		engaged = false
	}

	var game *DecisionGamePoint
	var gameWinProbSum float64
	flushGame := func() {
		if game == nil {
			return
		}
		if game.Attacks > 0 {
			game.AvgWinProb = ptr(100 * gameWinProbSum / float64(game.Attacks))
		}
		if killExpected > 0 {
			game.CumKillLuckPct = ptr(100 * (killActual/killExpected - 1))
		}
		out.Series = append(out.Series, *game)
		game = nil
	}

	for _, r := range rows {
		if game == nil || game.GameID != r.GameID {
			closeEngagement(false)
			flushGame()
			game = &DecisionGamePoint{GameID: r.GameID, CompletedAt: r.CompletedAt}
			gameWinProbSum = 0
		}
		if r.Kind == "stop" {
			closeEngagement(false)
			out.Stops++
			if r.BestWinProb == nil {
				out.StopBuckets[0].Stops++
			} else {
				out.StopBuckets[winProbBucket(*r.BestWinProb)+1].Stops++
				if *r.BestWinProb >= 0.75 {
					goodStops++
				}
			}
			continue
		}

		out.Rolls++
		if r.AttackerArmies != nil && r.AttackerDice != nil && *r.AttackerDice >= min(3, *r.AttackerArmies-1) {
			maxDice++
		}
		if r.ExpectedDefenderLoss != nil && r.DefenderLoss != nil && r.AttackerLoss != nil {
			killActual += float64(*r.DefenderLoss)
			killExpected += *r.ExpectedDefenderLoss
			lossActual += float64(*r.AttackerLoss)
			lossExpected += deref(r.ExpectedAttackerLoss)
		}

		if !deref(r.Continued) {
			closeEngagement(false)
			wp := deref(r.WinProb)
			b := winProbBucket(wp)
			out.Attacks++
			game.Attacks++
			gameWinProbSum += wp
			winProbSum += wp
			out.OddsBuckets[b].Attacks++
			winProbSumByBucket[b] += wp
			for i, n := range r.OptionsByBucket {
				if i < numWinProbBuckets {
					availByBucket[i] += n
				}
			}
			availOptions += r.Options
			for _, f := range feats {
				if f.chosen(r) {
					f.nChosen++
				}
				f.nAv += f.available(r)
			}
			engaged, engagedBucket = true, b
			engagedPlaces = tallyPlaces(r, &maps)
			if engagedPlaces != nil {
				out.PlaceAttacks++
			}
		}
		if deref(r.Captured) {
			closeEngagement(true)
		}
	}
	closeEngagement(false)
	flushGame()

	out.Games = len(out.Series)
	if out.Attacks > 0 {
		out.AvgWinProb = ptr(100 * winProbSum / float64(out.Attacks))
		out.AvgRollsPerAttack = ptr(float64(out.Rolls) / float64(out.Attacks))
		out.AbandonedPct = ptr(100 * float64(abandoned) / float64(out.Attacks))
	}
	if out.Rolls > 0 {
		out.MaxDicePct = ptr(100 * float64(maxDice) / float64(out.Rolls))
	}
	if out.Stops > 0 {
		out.StopsWithGoodOddsPct = ptr(100 * float64(goodStops) / float64(out.Stops))
		for i := range out.StopBuckets {
			out.StopBuckets[i].Pct = 100 * float64(out.StopBuckets[i].Stops) / float64(out.Stops)
		}
	}
	if killExpected > 0 {
		out.KillLuckPct = ptr(100 * (killActual/killExpected - 1))
	}
	if lossExpected > 0 {
		out.LossLuckPct = ptr(100 * (lossActual/lossExpected - 1))
	}
	for i := range out.OddsBuckets {
		b := &out.OddsBuckets[i]
		if out.Attacks > 0 {
			b.ChosenPct = 100 * float64(b.Attacks) / float64(out.Attacks)
		}
		if availOptions > 0 {
			b.AvailablePct = 100 * float64(availByBucket[i]) / float64(availOptions)
		}
		if b.AvailablePct > 0 {
			b.Preference = ptr(b.ChosenPct / b.AvailablePct)
		}
		if b.Attacks > 0 {
			b.CapturePct = ptr(100 * float64(capturedByBucket[i]) / float64(b.Attacks))
			b.ExpectedCapturePct = ptr(100 * winProbSumByBucket[i] / float64(b.Attacks))
		}
	}
	out.Maps = maps.finish()
	for _, f := range feats {
		df := DecisionFeature{Key: f.key, Label: f.label, Chosen: f.nChosen}
		if out.Attacks > 0 {
			df.ChosenPct = 100 * float64(f.nChosen) / float64(out.Attacks)
		}
		if availOptions > 0 {
			df.AvailablePct = 100 * float64(f.nAv) / float64(availOptions)
		}
		if df.AvailablePct > 0 {
			df.Preference = ptr(df.ChosenPct / df.AvailablePct)
		}
		out.Features = append(out.Features, df)
	}
	return out
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

// tallyPlaces records, under the row's map, one new attack's chosen target and every attackable
// territory (and their continents, counted once per decision) and returns
// the chosen target's territory and continent tallies, or nil when the row
// predates per-territory data.
func tallyPlaces(r store.AttackDecisionRow, maps *mapTallies) []*placeAcc {
	if r.OptionsTargets == nil || r.ToTerritory == nil || len(r.OptionsWinProbs) != len(r.OptionsTargets) || len(r.OptionsTargetContinents) != len(r.OptionsTargets) {
		return nil
	}
	m := maps.get(r)
	m.attacks++
	territories, continents := &m.territories, &m.continents
	contName := func(c string) string {
		if c == "" {
			return "(no continent)"
		}
		return c
	}
	// Continents are available at most once per decision, at their best odds.
	contBest := map[string]float64{}
	var contOrder []string
	for i, t := range r.OptionsTargets {
		c := contName(r.OptionsTargetContinents[i])
		p := territories.get(t, r.OptionsTargetContinents[i])
		p.stat.Available++
		p.availProbSum += r.OptionsWinProbs[i]
		m.placeSlots++
		if best, ok := contBest[c]; !ok || r.OptionsWinProbs[i] > best {
			if !ok {
				contOrder = append(contOrder, c)
			}
			contBest[c] = r.OptionsWinProbs[i]
		}
	}
	for _, c := range contOrder {
		p := continents.get(c, "")
		p.stat.Available++
		p.availProbSum += contBest[c]
		m.continentSlots++
	}
	wp := deref(r.WinProb)
	t := territories.get(*r.ToTerritory, deref(r.ToContinent))
	t.stat.Attacks++
	t.chosenProbSum += wp
	c := continents.get(contName(deref(r.ToContinent)), "")
	c.stat.Attacks++
	c.chosenProbSum += wp
	return []*placeAcc{t, c}
}
