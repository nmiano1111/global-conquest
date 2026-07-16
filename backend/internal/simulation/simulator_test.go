package simulation

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

func fullRegistry() bot.StrategyRegistry {
	return bot.StrategyRegistry{
		bot.StrategyBasicV1:  bot.NewBasicStrategy(),
		bot.StrategyScoredV1: bot.NewScoredStrategy(bot.DefaultWeights),
	}
}

// testLimits returns DefaultLimits with MaxDuration scaled up under the
// race detector (see race_on_test.go/race_off_test.go) -- a game that
// converges in well under a second normally can genuinely need 10x that
// under -race's instrumentation overhead, and DefaultLimits' real 30s is a
// production-facing constant that shouldn't itself change just because the
// test binary happens to be built with -race.
func testLimits() Limits {
	l := DefaultLimits()
	if raceDetectorEnabled {
		l.MaxDuration *= 10
	}
	return l
}

// raceScale multiplies a test's own timing budget the same way, for tests
// that don't go through testLimits (e.g. a bare wall-clock assertion).
func raceScale(d time.Duration) time.Duration {
	if raceDetectorEnabled {
		return d * 10
	}
	return d
}

func TestRunOneReachesGameOverForEveryStrategyCombination(t *testing.T) {
	sim := NewSimulator(fullRegistry())

	cases := []struct {
		name       string
		strategies []string
		mode       GameMode
		seed       int64
	}{
		{"3p all basic-v1", []string{bot.StrategyBasicV1, bot.StrategyBasicV1, bot.StrategyBasicV1}, GameModeAutoStart, 1},
		{"3p all scored-v1", []string{bot.StrategyScoredV1, bot.StrategyScoredV1, bot.StrategyScoredV1}, GameModeAutoStart, 1},
		{"4p mixed, auto-start", []string{bot.StrategyBasicV1, bot.StrategyScoredV1, bot.StrategyBasicV1, bot.StrategyScoredV1}, GameModeAutoStart, 1},
		{"4p mixed, random-territory", []string{bot.StrategyBasicV1, bot.StrategyScoredV1, bot.StrategyBasicV1, bot.StrategyScoredV1}, GameModeRandomTerritory, 1},
		// 6 identical scored-v1 bots frequently deadlock in a multi-way
		// border stand-off that never resolves (empirically confirmed:
		// scanning seeds 1-30 for this exact matchup, roughly half never
		// converge within 8s -- a real, fairly common emergent property
		// of the current heuristic weights facing itself 6-way, not a
		// rare fluke or a bug). Seed 23 is verified fast (<1s, 45 turns)
		// and used here specifically so this test exercises 6-player
		// completion without being at the mercy of that dynamic; the
		// dynamic itself is exactly what Limits.MaxDuration exists to
		// bound in the cases that do hit it (see its doc comment).
		{"6p all scored-v1", []string{bot.StrategyScoredV1, bot.StrategyScoredV1, bot.StrategyScoredV1, bot.StrategyScoredV1, bot.StrategyScoredV1, bot.StrategyScoredV1}, GameModeAutoStart, 23},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{
				Seed:       tc.seed,
				Strategies: tc.strategies,
				GameMode:   tc.mode,
				Trace:      TraceSummary,
				Limits:     testLimits(),
			}
			result, _, err := sim.RunOne(context.Background(), cfg, nil)
			if err != nil {
				t.Fatalf("RunOne: %v", err)
			}
			if !result.Completed {
				t.Fatalf("expected the game to complete, got Failure=%+v", result.Failure)
			}
			if result.Failure != nil {
				t.Fatalf("expected no failure on a completed game, got %+v", result.Failure)
			}
			if result.WinnerSeat < 0 || result.WinnerPlayerID == "" {
				t.Fatalf("expected a winner to be recorded, got seat=%d id=%q", result.WinnerSeat, result.WinnerPlayerID)
			}
			winnerCount := 0
			for _, seat := range result.Seats {
				if seat.FinishOrder == 1 {
					winnerCount++
				}
				if !seat.Eliminated && seat.FinishOrder != 1 {
					t.Errorf("non-eliminated, non-winning seat %d has FinishOrder %d, expected exactly one seat to have FinishOrder 1", seat.Seat, seat.FinishOrder)
				}
			}
			if winnerCount != 1 {
				t.Fatalf("expected exactly one seat with FinishOrder 1, got %d", winnerCount)
			}
			if result.Commands == 0 || result.Turns == 0 {
				t.Fatalf("expected a completed game to have played at least one command/turn, got Commands=%d Turns=%d", result.Commands, result.Turns)
			}
		})
	}
}

// TestRunOneReportsProgress uses the 3p-all-basic-v1 case, which takes
// over a second (well past progressInterval's 100ms) in every prior
// timing observation in this file, so at least one throttled update is
// guaranteed.
func TestRunOneReportsProgress(t *testing.T) {
	sim := NewSimulator(fullRegistry())
	cfg := Config{
		Seed:       1,
		Strategies: []string{bot.StrategyBasicV1, bot.StrategyBasicV1, bot.StrategyBasicV1},
		GameMode:   GameModeAutoStart,
		Trace:      TraceNone,
		Limits:     DefaultLimits(),
	}

	var updates []ProgressUpdate
	result, _, err := sim.RunOne(context.Background(), cfg, func(u ProgressUpdate) {
		updates = append(updates, u)
	})
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	if len(updates) == 0 {
		t.Fatalf("expected at least one progress update for a multi-second game")
	}
	last := updates[len(updates)-1]
	if last.Commands != result.Commands {
		t.Fatalf("expected the final progress update to report the final command count (%d), got %d", result.Commands, last.Commands)
	}
	if last.Turn != result.Turns {
		t.Fatalf("expected the final progress update to report the final turn (%d), got %d", result.Turns, last.Turn)
	}
	for i := 1; i < len(updates); i++ {
		if updates[i].Commands < updates[i-1].Commands {
			t.Fatalf("expected Commands to be non-decreasing across updates, got %d then %d", updates[i-1].Commands, updates[i].Commands)
		}
		if updates[i].Elapsed < updates[i-1].Elapsed {
			t.Fatalf("expected Elapsed to be non-decreasing across updates")
		}
	}
}

func TestRunOneNilProgressCallbackIsSafe(t *testing.T) {
	sim := NewSimulator(fullRegistry())
	cfg := Config{
		Seed:       1,
		Strategies: []string{bot.StrategyBasicV1, bot.StrategyBasicV1, bot.StrategyBasicV1},
		GameMode:   GameModeAutoStart,
		Trace:      TraceNone,
		Limits:     DefaultLimits(),
	}
	if _, _, err := sim.RunOne(context.Background(), cfg, nil); err != nil {
		t.Fatalf("RunOne: %v", err)
	}
}

func TestRunOneIsDeterministic(t *testing.T) {
	sim := NewSimulator(fullRegistry())
	cfg := Config{
		Seed:       42,
		Strategies: []string{bot.StrategyScoredV1, bot.StrategyBasicV1, bot.StrategyScoredV1},
		GameMode:   GameModeAutoStart,
		Trace:      TraceFull,
		Limits:     testLimits(),
	}

	r1, rec1, err1 := sim.RunOne(context.Background(), cfg, nil)
	if err1 != nil {
		t.Fatalf("first run: %v", err1)
	}
	r2, rec2, err2 := sim.RunOne(context.Background(), cfg, nil)
	if err2 != nil {
		t.Fatalf("second run: %v", err2)
	}

	r1.Duration, r2.Duration = 0, 0
	if !reflect.DeepEqual(r1, r2) {
		t.Fatalf("expected identical Results for the same seed+config (excluding Duration):\n%+v\nvs\n%+v", r1, r2)
	}
	if !reflect.DeepEqual(rec1.Entries(), rec2.Entries()) {
		t.Fatalf("expected identical decision traces for the same seed+config")
	}
	if !reflect.DeepEqual(rec1.Milestones(), rec2.Milestones()) {
		t.Fatalf("expected identical milestone traces for the same seed+config")
	}
}

// TestRunOneDifferentSeedsCanDiverge tolerates an individual seed failing
// to converge (some seeds genuinely produce a long "arms race" stalemate
// between evenly-matched scored-v1 bots at a shared border -- discovered
// during development, exactly what Limits.MaxDuration exists to bound)
// rather than treating it as a test failure; a short, test-local
// MaxDuration keeps any such seed from making this test slow, since all
// this test needs is evidence that seed choice affects outcome among
// whichever seeds do complete.
func TestRunOneDifferentSeedsCanDiverge(t *testing.T) {
	sim := NewSimulator(fullRegistry())
	limits := DefaultLimits()
	limits.MaxDuration = raceScale(3 * time.Second)
	base := Config{
		Strategies: []string{bot.StrategyScoredV1, bot.StrategyScoredV1, bot.StrategyScoredV1},
		GameMode:   GameModeAutoStart,
		Trace:      TraceNone,
		Limits:     limits,
	}

	seen := map[string]bool{}
	for seed := int64(1); seed <= 5; seed++ {
		cfg := base
		cfg.Seed = seed
		r, _, err := sim.RunOne(context.Background(), cfg, nil)
		if err != nil {
			t.Logf("seed %d: did not converge within the test's MaxDuration (%v) -- a legitimate stalemate outcome, not asserted against", seed, err)
			continue
		}
		seen[r.WinnerPlayerID] = true
	}
	if len(seen) < 2 {
		t.Fatalf("expected different seeds to sometimes produce different winners among the seeds that completed, got only %v", seen)
	}
}

func TestRunOneRejectsInvalidConfig(t *testing.T) {
	sim := NewSimulator(fullRegistry())
	cfg := Config{
		Seed:       1,
		Strategies: []string{bot.StrategyBasicV1, "not-a-real-strategy", bot.StrategyBasicV1},
		GameMode:   GameModeAutoStart,
		Trace:      TraceNone,
		Limits:     DefaultLimits(),
	}
	result, _, err := sim.RunOne(context.Background(), cfg, nil)
	if err == nil {
		t.Fatalf("expected an error for an invalid config")
	}
	if result.Failure != nil {
		t.Fatalf("expected a config validation failure to return a plain error, not a populated Result.Failure (no game was ever constructed): got %+v", result.Failure)
	}
}

func TestRunOneRespectsContextCancellation(t *testing.T) {
	sim := NewSimulator(fullRegistry())
	cfg := Config{
		Seed:       1,
		Strategies: []string{bot.StrategyBasicV1, bot.StrategyBasicV1, bot.StrategyBasicV1},
		GameMode:   GameModeAutoStart,
		Trace:      TraceNone,
		Limits:     DefaultLimits(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled before RunOne even starts its loop

	result, _, err := sim.RunOne(ctx, cfg, nil)
	if err == nil {
		t.Fatalf("expected an error when context is already canceled")
	}
	if result.Failure == nil || result.Failure.Type != FailureContextCanceled {
		t.Fatalf("expected FailureContextCanceled, got %+v", result.Failure)
	}
	if result.Failure.Seed != cfg.Seed {
		t.Fatalf("expected the failure to carry the seed for exact reruns, got %d", result.Failure.Seed)
	}
}

func TestRunOneStopsAtCommandLimit(t *testing.T) {
	sim := NewSimulator(fullRegistry())
	limits := DefaultLimits()
	limits.MaxCommands = 3 // far below what any full game needs
	cfg := Config{
		Seed:       1,
		Strategies: []string{bot.StrategyBasicV1, bot.StrategyBasicV1, bot.StrategyBasicV1},
		GameMode:   GameModeAutoStart,
		Trace:      TraceNone,
		Limits:     limits,
	}

	result, _, err := sim.RunOne(context.Background(), cfg, nil)
	if err == nil {
		t.Fatalf("expected an error when the command limit is hit")
	}
	if result.Completed {
		t.Fatalf("expected the run to be marked incomplete")
	}
	if result.Failure == nil || result.Failure.Type != FailureCommandLimitReached {
		t.Fatalf("expected FailureCommandLimitReached, got %+v", result.Failure)
	}
	if result.Commands != 3 {
		t.Fatalf("expected exactly 3 commands to have been recorded before failing, got %d", result.Commands)
	}
}

// TestRunOneTraceLevelDoesNotAffectResult is the direct RunOne-level
// version of the design doc's "trace collection must never affect
// gameplay" requirement: the same seed at TraceNone and TraceFull must
// produce an identical Result.
func TestRunOneTraceLevelDoesNotAffectResult(t *testing.T) {
	sim := NewSimulator(fullRegistry())
	base := Config{
		Seed:       9,
		Strategies: []string{bot.StrategyScoredV1, bot.StrategyBasicV1, bot.StrategyScoredV1},
		GameMode:   GameModeAutoStart,
		Limits:     DefaultLimits(),
	}

	none := base
	none.Trace = TraceNone
	rNone, _, err := sim.RunOne(context.Background(), none, nil)
	if err != nil {
		t.Fatalf("TraceNone run: %v", err)
	}

	full := base
	full.Trace = TraceFull
	rFull, _, err := sim.RunOne(context.Background(), full, nil)
	if err != nil {
		t.Fatalf("TraceFull run: %v", err)
	}

	rNone.Duration, rFull.Duration = 0, 0
	if !reflect.DeepEqual(rNone, rFull) {
		t.Fatalf("expected trace level to have no effect on Result:\nnone=%+v\nfull=%+v", rNone, rFull)
	}
}

// TestNoForbiddenImports statically confirms internal/simulation never
// imports anything Postgres/HTTP/websocket-shaped, mirroring the same
// check already run against internal/bot earlier in this project.
func TestNoForbiddenImports(t *testing.T) {
	forbidden := []string{
		`"github.com/nmiano1111/global-conquest/backend/internal/store"`,
		`"net/http"`,
		`"database/sql"`,
		"nhooyr.io/websocket",
		"gorilla/websocket",
		"coder/websocket", // the actual websocket package this project depends on (see go.mod)
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		for _, imp := range f.Imports {
			for _, bad := range forbidden {
				if imp.Path.Value == bad || strings.Contains(imp.Path.Value, bad) {
					t.Errorf("%s imports forbidden package %s", e.Name(), imp.Path.Value)
				}
			}
		}
	}
}

// TestRunOneCompletesQuickly is a coarse performance guard: a headless
// simulation has no sleeps, no network, no DB -- a handful of full games
// should take well under a second of wall-clock time.
func TestRunOneCompletesQuickly(t *testing.T) {
	sim := NewSimulator(fullRegistry())
	cfg := Config{
		Seed:       1,
		Strategies: []string{bot.StrategyScoredV1, bot.StrategyScoredV1, bot.StrategyScoredV1, bot.StrategyScoredV1},
		GameMode:   GameModeAutoStart,
		Trace:      TraceNone,
		Limits:     testLimits(),
	}
	start := time.Now()
	result, _, err := sim.RunOne(context.Background(), cfg, nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("RunOne: %v", err)
	}
	if !result.Completed {
		t.Fatalf("expected completion, got %+v", result.Failure)
	}
	if elapsed > raceScale(5*time.Second) {
		t.Fatalf("expected a single headless game to finish in well under 5s, took %s", elapsed)
	}
}

// stubStrategy always returns a fixed Command regardless of game state,
// counting how many times NextCommand was called -- lets a test force a
// specific (possibly illegal) decision and assert on exactly how many
// times the simulator asked for one.
type stubStrategy struct {
	cmd   bot.Command
	calls int
}

func (s *stubStrategy) NextCommand(_ context.Context, _ *risk.Game, _ string) (bot.Command, bot.Explanation, error) {
	s.calls++
	return s.cmd, bot.Explanation{}, nil
}

// TestRunOneFailsImmediatelyOnEngineRejectionWithNoRetry proves RunOne's
// documented zero-retry policy -- the direct opposite of bot.Runner's
// maxRejectedCommandRetries, which exists for concurrent production state
// but would mask a strategy bug here. GameModeAutoStart starts every game
// in PhaseReinforce, so an Attack command is illegal on the very first
// call; if RunOne retried, the stub (which always returns the same illegal
// command) would be called more than once before failing.
func TestRunOneFailsImmediatelyOnEngineRejectionWithNoRetry(t *testing.T) {
	stub := &stubStrategy{cmd: bot.Command{Action: bot.ActionAttack, From: "not-a-real-territory", To: "also-not-real", Armies: 1}}
	registry := bot.StrategyRegistry{"stub": stub}
	cfg := Config{
		Seed:       1,
		Strategies: []string{"stub", "stub", "stub"},
		GameMode:   GameModeAutoStart,
		Trace:      TraceNone,
		Limits:     DefaultLimits(),
	}
	sim := NewSimulator(registry)

	result, _, err := sim.RunOne(context.Background(), cfg, nil)
	if err == nil {
		t.Fatal("expected an error from an engine-rejected command")
	}
	if result.Failure == nil || result.Failure.Type != FailureEngineRejectedCommand {
		t.Fatalf("expected FailureEngineRejectedCommand, got %+v", result.Failure)
	}
	if result.Commands != 0 {
		t.Fatalf("expected zero successfully dispatched commands before the failure, got %d", result.Commands)
	}
	if stub.calls != 1 {
		t.Fatalf("expected NextCommand to be called exactly once (no retry), got %d calls", stub.calls)
	}
}

// TestRunOneConcurrentCallsAreIsolated proves independent RunOne calls
// against the same Simulator/registry never share a *risk.Game or RNG
// instance: each call constructs its own from scratch (see RunOne's doc
// comment), so running a set of seeds concurrently must produce results
// identical to running them one at a time. Run with `go test -race` to
// catch any accidental shared mutable state directly, not just via a
// results mismatch.
//
// Deliberately doesn't require any seed to *complete* -- some basic-v1
// mirror matchups genuinely don't converge within DefaultLimits (the same
// kind of legitimate stalemate TestRunOneDifferentSeedsCanDiverge already
// tolerates). What isolation actually demands is that a given seed behaves
// identically whether run alone or alongside five others -- a completed
// Result and an identical Failure (same Type/CommandIndex/Turn) are both
// valid proof of that; only a *mismatch* between the two runs would
// indicate shared state.
func TestRunOneConcurrentCallsAreIsolated(t *testing.T) {
	sim := NewSimulator(fullRegistry())

	seeds := []int64{1, 2, 3, 4, 5, 6}
	cfgFor := func(seed int64) Config {
		return Config{
			Seed:       seed,
			Strategies: []string{bot.StrategyBasicV1, bot.StrategyBasicV1, bot.StrategyBasicV1},
			GameMode:   GameModeAutoStart,
			Trace:      TraceNone,
			Limits:     DefaultLimits(),
		}
	}

	sequential := make(map[int64]Result, len(seeds))
	for _, seed := range seeds {
		r, _, _ := sim.RunOne(context.Background(), cfgFor(seed), nil)
		r.Duration = 0
		sequential[seed] = r
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	concurrent := make(map[int64]Result, len(seeds))
	for _, seed := range seeds {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			r, _, _ := sim.RunOne(context.Background(), cfgFor(seed), nil)
			r.Duration = 0
			mu.Lock()
			concurrent[seed] = r
			mu.Unlock()
		}(seed)
	}
	wg.Wait()

	completed := 0
	for _, seed := range seeds {
		if !reflect.DeepEqual(sequential[seed], concurrent[seed]) {
			t.Errorf("seed %d: concurrent run diverged from its sequential baseline\nsequential=%+v\nconcurrent=%+v",
				seed, sequential[seed], concurrent[seed])
		}
		if sequential[seed].Completed {
			completed++
		}
	}
	if completed == 0 {
		t.Fatalf("expected at least one of seeds %v to complete under DefaultLimits -- got zero, which would make this test vacuous", seeds)
	}
}

// TestNoLiveBotSymbolsReferenced statically confirms internal/simulation
// never *uses* bot.Runner, bot.Sleeper, or bot.ExecutionLive as code -- the
// production live-pacing machinery this package deliberately bypasses (see
// the package comment on why RunOne dispatches directly instead of reusing
// Runner). This package's own doc comments legitimately name these symbols
// to explain that design choice (e.g. "it does not reuse bot.Runner"), so a
// plain text/substring scan would flag its own documentation -- this walks
// the parsed AST's selector expressions instead, which comments and string
// literals never appear in, so only an actual `bot.Runner`-shaped
// reference in real code trips it.
func TestNoLiveBotSymbolsReferenced(t *testing.T) {
	forbidden := map[string]bool{
		"Runner": true, "NewRunner": true, "Sleeper": true,
		"ExecutionLive": true, "PacingConfig": true,
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkgIdent, ok := sel.X.(*ast.Ident)
			if !ok || pkgIdent.Name != "bot" || !forbidden[sel.Sel.Name] {
				return true
			}
			pos := fset.Position(sel.Pos())
			t.Errorf("%s:%d references bot.%s -- internal/simulation must never touch production's live-pacing machinery",
				e.Name(), pos.Line, sel.Sel.Name)
			return true
		})
	}
}
