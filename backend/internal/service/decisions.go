package service

// Attack-decision extraction: rebuilds the board before every attack-phase
// decision from game_replay_events (each row's payload is the full board
// after that action, so the previous row is the board the player was
// looking at), enumerates the legal attacks with risk.LegalAttacks, and
// stores the choice next to a summary of the alternatives. Runs in the
// background over completed games (see DecisionExtractor.Run).

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/db"
	"github.com/nmiano1111/global-conquest/backend/internal/risk"
	"github.com/nmiano1111/global-conquest/backend/internal/store"
)

// decisionExtractorVersion is stored per processed game; bump it when the
// extracted features change and every game is re-extracted (while its
// replay history still exists).
const decisionExtractorVersion = 2 // 2: per-target option arrays (V24)

// winProbBuckets are the upper bounds of the option win-probability
// buckets: <25%, 25-50%, 50-75%, 75-90%, >=90%.
var winProbBuckets = []float64{0.25, 0.5, 0.75, 0.9}

const numWinProbBuckets = 5

func winProbBucket(p float64) int {
	for i, hi := range winProbBuckets {
		if p < hi {
			return i
		}
	}
	return len(winProbBuckets)
}

type decisionStore interface {
	PendingGames(ctx context.Context, q db.Querier, version, limit int) ([]string, error)
	LoadSource(ctx context.Context, q db.Querier, gameID string) (store.DecisionSource, error)
	Replace(ctx context.Context, q db.Querier, gameID string, version int, available bool, decisions []store.AttackDecision) error
}

type replayLister interface {
	ListReplayEvents(ctx context.Context, q db.Querier, gameID string, afterSequence int64, limit int) ([]store.GameReplayEvent, error)
}

type txDB interface {
	Queryer() db.Querier
	WithTxQ(ctx context.Context, fn func(q db.Querier) error) error
}

// DecisionExtractor fills attack_decisions for completed games.
type DecisionExtractor struct {
	db      txDB
	store   decisionStore
	replays replayLister
}

// NewDecisionExtractor constructs a DecisionExtractor.
func NewDecisionExtractor(d txDB, s decisionStore, replays replayLister) *DecisionExtractor {
	return &DecisionExtractor{db: d, store: s, replays: replays}
}

// Run processes pending games now and then every interval until ctx ends.
// Games are picked up shortly after they finish, while their replay
// history is guaranteed to exist.
func (x *DecisionExtractor) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if n, err := x.ProcessPending(ctx); err != nil {
			log.Printf("attack decision extraction: %v", err)
		} else if n > 0 {
			log.Printf("attack decision extraction: processed %d games", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// ProcessPending extracts every pending game, returning how many it
// processed. A game that fails is logged and skipped (retried next run).
func (x *DecisionExtractor) ProcessPending(ctx context.Context) (int, error) {
	done := 0
	failed := map[string]bool{}
	for ctx.Err() == nil {
		ids, err := x.store.PendingGames(ctx, x.db.Queryer(), decisionExtractorVersion, 50+len(failed))
		if err != nil {
			return done, err
		}
		progressed := false
		for _, id := range ids {
			if failed[id] {
				continue
			}
			if err := x.ProcessGame(ctx, id); err != nil {
				log.Printf("attack decision extraction: game %s: %v", id, err)
				failed[id] = true
				continue
			}
			done++
			progressed = true
		}
		if !progressed {
			return done, nil
		}
	}
	return done, ctx.Err()
}

// ProcessGame (re)extracts one game's decisions.
func (x *DecisionExtractor) ProcessGame(ctx context.Context, gameID string) error {
	q := x.db.Queryer()
	src, err := x.store.LoadSource(ctx, q, gameID)
	if err != nil {
		return err
	}
	var events []store.GameReplayEvent
	var after int64
	for {
		page, err := x.replays.ListReplayEvents(ctx, q, gameID, after, 1000)
		if err != nil {
			return err
		}
		events = append(events, page...)
		if len(page) < 1000 {
			break
		}
		after = page[len(page)-1].GameSequence
	}
	available := src.GameExists && len(events) > 0
	var decisions []store.AttackDecision
	if available {
		board := risk.ClassicBoard()
		if len(src.Board) > 0 && string(src.Board) != "null" {
			var b risk.Board
			if err := json.Unmarshal(src.Board, &b); err != nil {
				return err
			}
			if len(b.Order) > 0 {
				board = b
			}
		}
		decisions = extractAttackDecisions(board, src.PlayerKeys, events)
	}
	return x.db.WithTxQ(ctx, func(q db.Querier) error {
		return x.store.Replace(ctx, q, gameID, decisionExtractorVersion, available, decisions)
	})
}

// replaySnapshot is the subset of a replay payload (service.GameActionUpdate)
// the extractor reads.
type replaySnapshot struct {
	Phase   string `json:"phase"`
	Players []struct {
		UserID     string `json:"user_id"`
		Eliminated bool   `json:"eliminated"`
	} `json:"players"`
	Territories map[risk.Territory]risk.TerritoryState `json:"territories"`
	ActionFrom  risk.Territory                         `json:"action_from"`
	ActionTo    risk.Territory                         `json:"action_to"`
	Result      json.RawMessage                        `json:"result"`
}

type replayAttackResult struct {
	Conquered     bool  `json:"conquered"`
	AttackerLoss  int   `json:"attacker_loss"`
	DefenderLoss  int   `json:"defender_loss"`
	AttackerRolls []int `json:"attacker_rolls"`
	DefenderRolls []int `json:"defender_rolls"`
}

// boardContext is per-decision-point derived state shared by every option.
type boardContext struct {
	owner       map[risk.Territory]int
	continentOf map[risk.Territory]risk.Continent
	board       risk.Board
	counts      map[int]int // territories owned per seat
	maxOpp      int
	minOpp      int
}

func newBoardContext(board risk.Board, continentOf map[risk.Territory]risk.Continent, terr map[risk.Territory]risk.TerritoryState, actor int) boardContext {
	c := boardContext{owner: map[risk.Territory]int{}, continentOf: continentOf, board: board, counts: map[int]int{}}
	for t, s := range terr {
		c.owner[t] = s.Owner
		if s.Owner >= 0 {
			c.counts[s.Owner]++
		}
	}
	c.maxOpp, c.minOpp = -1, math.MaxInt
	for seat, n := range c.counts {
		if seat == actor || n == 0 {
			continue
		}
		c.maxOpp = max(c.maxOpp, n)
		c.minOpp = min(c.minOpp, n)
	}
	return c
}

// ownsAllOf reports whether seat owns every territory of continent,
// optionally pretending it also owns extra.
func (c boardContext) ownsAllOf(seat int, cont risk.Continent, extra risk.Territory) bool {
	for _, t := range c.board.Continents[cont].Territories {
		if t != extra && c.owner[t] != seat {
			return false
		}
	}
	return true
}

type optionFeatures struct {
	winProb             float64
	completes, breaks   bool
	vsLeader, vsWeakest bool
}

func (c boardContext) features(actor int, a risk.AttackAction, odds *oddsCache) optionFeatures {
	def := c.owner[a.To]
	cont, hasCont := c.continentOf[a.To]
	n := c.counts[def]
	return optionFeatures{
		winProb:   odds.winProb(a.SourceArmies, a.TargetArmies),
		completes: hasCont && c.ownsAllOf(actor, cont, a.To),
		breaks:    hasCont && c.ownsAllOf(def, cont, ""),
		vsLeader:  n == c.maxOpp,
		vsWeakest: n == c.minOpp,
	}
}

// oddsCache memoizes risk.ForecastAttack win probabilities. Very large
// stacks are scaled down to at most maxForecastArmies (keeping the ratio):
// exact odds at hundreds of armies are both expensive and, for a decision
// summary, indistinguishable from the scaled estimate.
type oddsCache map[[2]int]float64

const maxForecastArmies = 120

func (o *oddsCache) winProb(attacker, defender int) float64 {
	if attacker > maxForecastArmies || defender > maxForecastArmies {
		f := float64(maxForecastArmies) / float64(max(attacker, defender))
		attacker = max(2, int(math.Round(float64(attacker)*f)))
		defender = max(1, int(math.Round(float64(defender)*f)))
	}
	k := [2]int{attacker, defender}
	if p, ok := (*o)[k]; ok {
		return p
	}
	p := risk.ForecastAttack(attacker, defender).WinProbability
	(*o)[k] = p
	return p
}

// expectedRollLosses[a][d] is the expected (attacker, defender) losses of
// one roll with a attacker dice and d defender dice.
var expectedRollLosses = func() (out [4][3][2]float64) {
	for a := 1; a <= 3; a++ {
		for d := 1; d <= 2; d++ {
			out[a][d] = enumerateRoll(a, d)
		}
	}
	return out
}()

func enumerateRoll(a, d int) [2]float64 {
	total, attLoss, defLoss := 0, 0, 0
	dice := make([]int, a+d)
	var rec func(i int)
	rec = func(i int) {
		if i == len(dice) {
			att := sortDesc(append([]int(nil), dice[:a]...))
			def := sortDesc(append([]int(nil), dice[a:]...))
			for j := 0; j < min(a, d); j++ {
				if att[j] > def[j] {
					defLoss++
				} else {
					attLoss++
				}
			}
			total++
			return
		}
		for v := 1; v <= 6; v++ {
			dice[i] = v
			rec(i + 1)
		}
	}
	rec(0)
	return [2]float64{float64(attLoss) / float64(total), float64(defLoss) / float64(total)}
}

func sortDesc(xs []int) []int {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] > xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
	return xs
}

// extractAttackDecisions turns one game's ordered replay events into
// decision rows. A decision is an "attack" or "end_attack" action whose
// previous event left the game in the attack phase; the first event (no
// prior board) and players with no known player key are skipped.
func extractAttackDecisions(board risk.Board, playerKeys map[int]string, events []store.GameReplayEvent) []store.AttackDecision {
	continentOf := map[risk.Territory]risk.Continent{}
	for c, info := range board.Continents {
		for _, t := range info.Territories {
			continentOf[t] = c
		}
	}
	odds := oddsCache{}
	var out []store.AttackDecision
	var prev *replaySnapshot
	turn := 0
	// The previous event, when it was an uncaptured attack roll.
	var lastRoll *struct {
		actor    string
		from, to risk.Territory
	}

	for _, ev := range events {
		var snap replaySnapshot
		if err := json.Unmarshal(ev.Payload, &snap); err != nil {
			prev, lastRoll = nil, nil
			continue
		}
		isDecision := ev.ActionType == "attack" || ev.ActionType == "end_attack"
		var roll *replayAttackResult
		if ev.ActionType == "attack" && len(snap.Result) > 0 {
			roll = &replayAttackResult{}
			if json.Unmarshal(snap.Result, roll) != nil {
				roll = nil
			}
		}

		if isDecision && prev != nil && prev.Phase == string(risk.PhaseAttack) {
			if row, ok := buildDecision(board, continentOf, playerKeys, prev, &snap, ev, roll, turn, &odds); ok {
				if row.Kind == "attack" {
					c := lastRoll != nil && lastRoll.actor == ev.ActorPlayerID && lastRoll.from == snap.ActionFrom && lastRoll.to == snap.ActionTo
					row.Continued = &c
				}
				out = append(out, row)
			}
		}

		lastRoll = nil
		if roll != nil && !roll.Conquered {
			lastRoll = &struct {
				actor    string
				from, to risk.Territory
			}{ev.ActorPlayerID, snap.ActionFrom, snap.ActionTo}
		}
		if ev.ActionType == "end_turn" {
			turn++
		}
		s := snap
		prev = &s
	}
	return out
}

func buildDecision(
	board risk.Board,
	continentOf map[risk.Territory]risk.Continent,
	playerKeys map[int]string,
	prev, snap *replaySnapshot,
	ev store.GameReplayEvent,
	roll *replayAttackResult,
	turn int,
	odds *oddsCache,
) (store.AttackDecision, bool) {
	actor := -1
	for i, p := range prev.Players {
		if p.UserID == ev.ActorPlayerID {
			actor = i
		}
	}
	key, ok := playerKeys[actor]
	if actor < 0 || !ok {
		return store.AttackDecision{}, false
	}
	g := &risk.Game{Board: board, Phase: risk.PhaseAttack, Territories: prev.Territories}
	for _, p := range prev.Players {
		g.Players = append(g.Players, risk.PlayerState{ID: p.UserID, Eliminated: p.Eliminated})
	}
	ctx := newBoardContext(board, continentOf, prev.Territories, actor)

	row := store.AttackDecision{
		GameSequence:    ev.GameSequence,
		Seat:            actor,
		PlayerKey:       key,
		TurnIndex:       turn,
		Kind:            "stop",
		OptionsByBucket: make([]int, numWinProbBuckets),
		// Non-nil so a decision with no legal attacks stores {} rather than NULL
		// (NULL marks rows extracted before these columns existed).
		OptionsTargets:          []string{},
		OptionsTargetContinents: []string{},
		OptionsWinProbs:         []float64{},
	}
	best := -1.0
	// Best odds per distinct target, in first-seen (board) order.
	targetOdds := map[risk.Territory]int{}
	for _, a := range risk.LegalAttacks(g, ev.ActorPlayerID) {
		f := ctx.features(actor, a, odds)
		if i, seen := targetOdds[a.To]; seen {
			row.OptionsWinProbs[i] = max(row.OptionsWinProbs[i], f.winProb)
		} else {
			targetOdds[a.To] = len(row.OptionsTargets)
			row.OptionsTargets = append(row.OptionsTargets, string(a.To))
			row.OptionsTargetContinents = append(row.OptionsTargetContinents, string(continentOf[a.To]))
			row.OptionsWinProbs = append(row.OptionsWinProbs, f.winProb)
		}
		row.Options++
		row.OptionsByBucket[winProbBucket(f.winProb)]++
		best = max(best, f.winProb)
		if f.completes {
			row.OptionsCompletesContinent++
		}
		if f.breaks {
			row.OptionsBreaksContinent++
		}
		if f.vsLeader {
			row.OptionsVsLeader++
		}
		if f.vsWeakest {
			row.OptionsVsWeakest++
		}
	}
	if best >= 0 {
		row.BestWinProb = &best
	}
	if ev.ActionType != "attack" {
		return row, true
	}

	from, to := snap.ActionFrom, snap.ActionTo
	src, okSrc := prev.Territories[from]
	dst, okDst := prev.Territories[to]
	if !okSrc || !okDst || roll == nil {
		return store.AttackDecision{}, false
	}
	f := ctx.features(actor, risk.AttackAction{From: from, To: to, SourceArmies: src.Armies, TargetArmies: dst.Armies}, odds)
	ad, dd := len(roll.AttackerRolls), len(roll.DefenderRolls)
	row.Kind = "attack"
	row.FromTerritory, row.ToTerritory = ptr(string(from)), ptr(string(to))
	row.DefenderSeat = ptr(dst.Owner)
	row.AttackerArmies, row.DefenderArmies = ptr(src.Armies), ptr(dst.Armies)
	row.WinProb = ptr(f.winProb)
	row.AttackerDice, row.DefenderDice = ptr(ad), ptr(dd)
	row.AttackerLoss, row.DefenderLoss = ptr(roll.AttackerLoss), ptr(roll.DefenderLoss)
	if ad >= 1 && ad <= 3 && dd >= 1 && dd <= 2 {
		e := expectedRollLosses[ad][dd]
		row.ExpectedAttackerLoss, row.ExpectedDefenderLoss = ptr(e[0]), ptr(e[1])
	}
	row.Captured = ptr(roll.Conquered)
	row.CompletesContinent, row.BreaksContinent = ptr(f.completes), ptr(f.breaks)
	row.ToContinent = ptr(string(continentOf[to]))
	row.TargetsLeader, row.TargetsWeakest = ptr(f.vsLeader), ptr(f.vsWeakest)
	return row, true
}

func ptr[T any](v T) *T { return &v }
