// Package tournament runs many internal/simulation games in parallel for a
// fixed strategy matchup across a sweep of seeds, and aggregates their
// Results into per-strategy summary statistics. It builds directly on
// simulation.Simulator.RunOne's existing guarantees -- stateless per call,
// safe to share across goroutines -- rather than introducing any new
// concurrency primitive into that package.
package tournament

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
	"github.com/nmiano1111/global-conquest/backend/internal/simulation"
)

// Config fully specifies one reproducible tournament: the same Config
// against the same registry always runs the same set of games (same seeds,
// same strategies, same limits), though the *order* results arrive in (and
// therefore any callback sequencing) is not guaranteed -- see Run.
type Config struct {
	// Strategies holds one strategy ID per seat, fixed across every game in
	// the tournament -- same shape and meaning as simulation.Config.Strategies.
	Strategies []string

	GameMode simulation.GameMode
	Limits   simulation.Limits

	// SeedStart is the first seed used; games run with seeds
	// SeedStart, SeedStart+1, ..., SeedStart+Games-1.
	SeedStart int64
	Games     int

	// Parallelism bounds how many games run concurrently.
	Parallelism int
}

// Validate checks a Config against the given strategy registry, delegating
// strategy/game-mode/limits validation to simulation.Config.Validate (built
// from a throwaway per-game Config with Seed 0 and Trace forced to
// TraceNone, matching what Run actually uses) rather than duplicating those
// checks here.
func (c Config) Validate(registry bot.StrategyRegistry) error {
	gameCfg := simulation.Config{
		Seed:       0,
		Strategies: c.Strategies,
		GameMode:   c.GameMode,
		Trace:      simulation.TraceNone,
		Limits:     c.Limits,
	}
	if err := gameCfg.Validate(registry); err != nil {
		return err
	}
	if c.Games <= 0 {
		return fmt.Errorf("%w: Games must be positive, got %d", simulation.ErrInvalidConfig, c.Games)
	}
	if c.Parallelism <= 0 {
		return fmt.Errorf("%w: Parallelism must be positive, got %d", simulation.ErrInvalidConfig, c.Parallelism)
	}
	return nil
}

// StrategyStats summarizes one strategy ID's performance across every seat
// (in every game) that used it. A matchup repeating the same strategy ID
// across multiple seats (a mirror match) naturally accumulates more samples
// for that ID -- each seat-instance is an independent sample of that
// strategy playing from that seat.
type StrategyStats struct {
	StrategyID string

	// Appearances counts every seat-instance of this strategy across ALL
	// games run, whether or not the game completed. CompletedAppearances
	// is the subset where the game actually reached PhaseGameOver -- every
	// other field below is computed over CompletedAppearances only, since
	// a stalemate/limit-hit game has no winner and no meaningful
	// FinishOrder for anyone, and folding it into these averages would
	// incorrectly read as a strategy weakness rather than a systemic
	// property of that matchup (see Aggregate.Failures for that instead).
	Appearances          int
	CompletedAppearances int

	// Wins counts completed games where the seat holding this StrategyID
	// was the winner. Since exactly one seat wins per game, this is
	// simultaneously "how many seat-appearances of this strategy won" and
	// "how many games this strategy identity won" -- the two ratios below
	// divide it by two different, easily-confused denominators.
	Wins int

	// SeatWinRate is Wins / CompletedAppearances: given a seat is playing
	// this strategy, how often does that specific seat win. When a
	// strategy occupies more than one seat in the matchup (a mirror
	// match, e.g. "basic-v1,scored-v1,scored-v1"), CompletedAppearances
	// counts every one of those seats every game, but only one can ever
	// win -- so a strategy occupying k of n seats has a maximum possible
	// SeatWinRate of 1/k even if it wins every single game. This is easy
	// to misread as "the strategy is weak" when it's really just seat
	// dilution; see GameWinRate for the metric that isn't affected by it.
	SeatWinRate float64

	// GameWinRate is Wins / (the tournament's completed game count): what
	// fraction of games did *a* seat playing this strategy win, regardless
	// of how many seats it occupied. This is the metric that answers "is
	// this strategy identity actually better than the other one" -- e.g.
	// two scored-v1 seats beating basic-v1 in every single game yields
	// GameWinRate 1.0 for scored-v1 even though each individual scored-v1
	// seat only won half of those games from SeatWinRate's point of view.
	GameWinRate float64

	// GameWinRateCI is a 95% confidence interval around GameWinRate (a
	// Wilson score interval over the tournament's completed game count --
	// see wilsonScoreInterval95 for why Wilson rather than a simpler
	// normal approximation). Answers "how much of this win rate could
	// plausibly be sampling noise," not just the point estimate -- e.g. a
	// candidate at 55% GameWinRate over 50 games might have a CI of
	// [41%, 68%], which comfortably includes 50/50, versus the same 55%
	// over 2000 games with a CI of [53%, 57%], which doesn't. Zero value
	// when CompletedGames is 0.
	GameWinRateCI ConfidenceInterval

	AvgFinishOrder       float64
	AvgCaptures          float64
	AvgEliminationsMade  float64
	AvgCombatLossesTaken float64
	AvgFinalTerritories  float64
}

// ConfidenceInterval is a two-sided interval around a point estimate.
type ConfidenceInterval struct {
	Low  float64
	High float64
}

// wilson95Z is the z-score for a two-sided 95% confidence interval
// (the 97.5th percentile of the standard normal distribution).
const wilson95Z = 1.959963984540054

// wilsonScoreInterval95 computes a 95% Wilson score confidence interval
// for a binomial proportion wins/n. Preferred over the simpler normal
// (Wald) approximation (p ± z*sqrt(p(1-p)/n)) because Wald intervals can
// extend outside [0,1] and have poor coverage at small n or p near 0 or
// 1 -- exactly the regime a "did this candidate actually beat baseline"
// check often lands in with a modest game count. Returns the zero
// ConfidenceInterval when n is 0.
func wilsonScoreInterval95(wins, n int) ConfidenceInterval {
	if n == 0 {
		return ConfidenceInterval{}
	}
	p := float64(wins) / float64(n)
	nf := float64(n)
	z := wilson95Z
	denom := 1 + z*z/nf
	center := (p + z*z/(2*nf)) / denom
	margin := (z / denom) * math.Sqrt(p*(1-p)/nf+z*z/(4*nf*nf))
	// Clamp: the true interval is always within [0,1], but floating-point
	// rounding can push a boundary case (e.g. wins=0 or wins=n) a hair
	// outside it.
	return ConfidenceInterval{
		Low:  math.Max(0, center-margin),
		High: math.Min(1, center+margin),
	}
}

// Aggregate is the summarized outcome of an entire tournament.
type Aggregate struct {
	TotalGames     int
	CompletedGames int
	FailedGames    int

	// Failures breaks down FailedGames by simulation.FailureType.
	Failures map[simulation.FailureType]int

	// AvgTurns, AvgCommands, and AvgGameDuration are computed over
	// completed games only -- a failed game's counts are truncated at the
	// point of failure and not comparable to a finished game's.
	AvgTurns        float64
	AvgCommands     float64
	AvgGameDuration time.Duration

	// Strategies holds one entry per distinct strategy ID present in
	// Config.Strategies, sorted by StrategyID for stable output.
	Strategies []StrategyStats
}

// aggregator accumulates running totals as Results arrive, then converts
// them to an Aggregate (averages, sorted slice) once the tournament is
// done. Kept separate from Aggregate itself so the zero-division-guarded
// "divide to get an average" step happens exactly once, in finalize.
type aggregator struct {
	totalGames     int
	completedGames int
	failedGames    int
	failures       map[simulation.FailureType]int

	turnsSum    int
	commandsSum int
	durationSum time.Duration

	byStrategy map[string]*strategyAccum
}

type strategyAccum struct {
	appearances          int
	completedAppearances int
	wins                 int
	finishOrderSum       int
	capturesSum          int
	eliminationsSum      int
	combatLossesSum      int
	territoriesSum       int
}

func newAggregator() *aggregator {
	return &aggregator{
		failures:   make(map[simulation.FailureType]int),
		byStrategy: make(map[string]*strategyAccum),
	}
}

func (a *aggregator) absorb(result simulation.Result) {
	a.totalGames++
	if result.Completed {
		a.completedGames++
		a.turnsSum += result.Turns
		a.commandsSum += result.Commands
		a.durationSum += result.Duration
	} else {
		a.failedGames++
		if result.Failure != nil {
			a.failures[result.Failure.Type]++
		}
	}

	for _, seat := range result.Seats {
		acc, ok := a.byStrategy[seat.StrategyID]
		if !ok {
			acc = &strategyAccum{}
			a.byStrategy[seat.StrategyID] = acc
		}
		acc.appearances++
		if !result.Completed {
			continue
		}
		acc.completedAppearances++
		if seat.Seat == result.WinnerSeat {
			acc.wins++
		}
		acc.finishOrderSum += seat.FinishOrder
		acc.capturesSum += seat.Captures
		acc.eliminationsSum += seat.EliminationsMade
		acc.combatLossesSum += seat.CombatLossesTaken
		acc.territoriesSum += seat.FinalTerritories
	}
}

func (a *aggregator) finalize() Aggregate {
	agg := Aggregate{
		TotalGames:     a.totalGames,
		CompletedGames: a.completedGames,
		FailedGames:    a.failedGames,
		Failures:       a.failures,
	}
	if a.completedGames > 0 {
		agg.AvgTurns = float64(a.turnsSum) / float64(a.completedGames)
		agg.AvgCommands = float64(a.commandsSum) / float64(a.completedGames)
		agg.AvgGameDuration = a.durationSum / time.Duration(a.completedGames)
	}

	ids := make([]string, 0, len(a.byStrategy))
	for id := range a.byStrategy {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	agg.Strategies = make([]StrategyStats, 0, len(ids))
	for _, id := range ids {
		acc := a.byStrategy[id]
		stats := StrategyStats{
			StrategyID:           id,
			Appearances:          acc.appearances,
			CompletedAppearances: acc.completedAppearances,
			Wins:                 acc.wins,
		}
		if acc.completedAppearances > 0 {
			n := float64(acc.completedAppearances)
			stats.SeatWinRate = float64(acc.wins) / n
			stats.AvgFinishOrder = float64(acc.finishOrderSum) / n
			stats.AvgCaptures = float64(acc.capturesSum) / n
			stats.AvgEliminationsMade = float64(acc.eliminationsSum) / n
			stats.AvgCombatLossesTaken = float64(acc.combatLossesSum) / n
			stats.AvgFinalTerritories = float64(acc.territoriesSum) / n
		}
		// GameWinRate's denominator is the tournament's completed game
		// count, not this strategy's own CompletedAppearances -- since
		// Config.Strategies is fixed for every game in a tournament, every
		// strategy ID present appears in literally every game, so the two
		// only coincide when that strategy occupies exactly one seat.
		if a.completedGames > 0 {
			stats.GameWinRate = float64(acc.wins) / float64(a.completedGames)
			stats.GameWinRateCI = wilsonScoreInterval95(acc.wins, a.completedGames)
		}
		agg.Strategies = append(agg.Strategies, stats)
	}

	return agg
}

// Run plays cfg.Games games (seeds cfg.SeedStart..cfg.SeedStart+Games-1) for
// the fixed matchup in cfg.Strategies, up to cfg.Parallelism at a time, and
// returns the aggregated Aggregate.
//
// registry must be the same registry sim was constructed with -- Simulator
// keeps its registry unexported, so Run takes it separately rather than
// reaching into Simulator, to validate cfg up front (an unknown strategy ID
// must fail before any game runs, not get silently misread as a run-time
// failure by the first worker that hits it).
//
// onResult, if non-nil, is called once per game as its Result arrives --
// always from Run's own goroutine (a single-consumer design), so a caller
// can stream raw results out (e.g. to a JSONL file) or drive a progress
// counter with no locking of its own. Games complete in whatever order
// their individual durations dictate, not seed order -- each Result is
// self-contained (carries its own Seed), so callers that care about
// per-game identity should read it from there rather than assuming order.
//
// Each game's Trace is forced to simulation.TraceNone: a tournament only
// consumes Results, and retaining decision/milestone traces for every game
// in a large batch would be pure waste.
//
// ctx cancellation stops handing out new seeds; games already in flight
// still run to their own completion/failure (RunOne observes ctx itself).
// The returned error is ctx.Err() if the tournament was cut short this way,
// nil otherwise -- cfg validation errors are returned immediately, before
// any game runs.
func Run(ctx context.Context, sim *simulation.Simulator, registry bot.StrategyRegistry, cfg Config, onResult func(simulation.Result)) (Aggregate, error) {
	if err := cfg.Validate(registry); err != nil {
		return Aggregate{}, err
	}

	seeds := make(chan int64)
	go func() {
		defer close(seeds)
		for i := 0; i < cfg.Games; i++ {
			select {
			case seeds <- cfg.SeedStart + int64(i):
			case <-ctx.Done():
				return
			}
		}
	}()

	results := make(chan simulation.Result)
	var wg sync.WaitGroup
	for i := 0; i < cfg.Parallelism; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for seed := range seeds {
				gameCfg := simulation.Config{
					Seed:       seed,
					Strategies: cfg.Strategies,
					GameMode:   cfg.GameMode,
					Trace:      simulation.TraceNone,
					Limits:     cfg.Limits,
				}
				// RunOne's error, when non-nil, is always the same value as
				// the returned Result's Failure (see RunOne's doc comment)
				// and Result is populated either way -- absorb reads
				// Result.Completed/Failure directly, so the error itself
				// carries nothing extra worth handling here.
				result, _, _ := sim.RunOne(ctx, gameCfg, nil)
				results <- result
			}
		}()
	}
	go func() {
		defer close(results)
		wg.Wait()
	}()

	agg := newAggregator()
	for result := range results {
		agg.absorb(result)
		if onResult != nil {
			onResult(result)
		}
	}

	return agg.finalize(), ctx.Err()
}
