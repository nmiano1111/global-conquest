// Command bvcalibrate fits ValueStrategy's AttackMargin/FortifyMargin
// empirically against any bot.ValueFunction (a linear BoardValue or a GCN
// gcnmodel.Model): it runs many headless games with a *zero-margin*
// wrapper around a given weights file (so the strategy acts on any
// positive-delta candidate, observing the natural, unfiltered score-delta
// distribution for each phase via ValueStrategy.Observer), then
// writes a copy of the input file with attack_margin/fortify_margin set
// to a chosen percentile of each phase's *positive* observed deltas.
//
// This exists because a first live tournament eval found attack and
// fortify move BoardValue's score on completely different scales (attack
// changes ownership -- many features at once; fortify only reallocates
// armies between the acting player's own territories -- at most two
// per-territory army_fraction coefficients) -- see
// internal/bot/linear_value.go's BoardValue.AttackMargin/FortifyMargin
// doc comment. A single shared margin calibrated to attack's scale
// suppressed fortify almost entirely. The same reasoning applies
// regardless of model class, so this tool works identically for
// gcn_fit.py's exported GCN weights.
//
// Usage:
//
//	go run ./cmd/bvcalibrate --input value.json --output calibrated.json --games 200
//	go run ./cmd/bvcalibrate --input gcn.json --model-type gcn --output calibrated.json --games 200
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
	"github.com/nmiano1111/global-conquest/backend/internal/bot/gcnmodel"
	"github.com/nmiano1111/global-conquest/backend/internal/simulation"
)

// zeroMarginValueFunction wraps a bot.ValueFunction, always reporting
// zero margins while delegating Score unchanged -- lets ValueStrategy
// act on any positive-delta candidate during calibration's observation
// pass, regardless of the wrapped value function's concrete type.
type zeroMarginValueFunction struct {
	bot.ValueFunction
}

func (zeroMarginValueFunction) AttackMargin() float64  { return 0 }
func (zeroMarginValueFunction) FortifyMargin() float64 { return 0 }

// loadValueFunction loads path as either a linear BoardValue or a GCN
// model, per modelType ("linear" or "gcn").
func loadValueFunction(path, modelType string) (bot.ValueFunction, error) {
	switch modelType {
	case "linear":
		return bot.LoadBoardValue(path)
	case "gcn":
		return gcnmodel.LoadModel(path)
	default:
		return nil, fmt.Errorf("unknown --model-type %q (want linear or gcn)", modelType)
	}
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("bvcalibrate", flag.ExitOnError)
	input := fs.String("input", "", "Path to a board_fit.py-exported weights JSON file (required)")
	output := fs.String("output", "", "Destination for the calibrated weights JSON (required)")
	strategies := fs.String("strategies", "basic-v1,scored-v1,board-value-candidate", "Comma-separated strategy ID per seat -- exactly one seat must be board-value-candidate")
	games := fs.Int("games", 200, "Number of calibration games to run")
	seedStart := fs.Int64("seed-start", 1, "First seed used")
	parallel := fs.Int("parallel", runtime.NumCPU(), "Number of games to run concurrently")
	gameMode := fs.String("game-mode", "auto_start", "Game construction mode: auto_start|manual")
	maxTurns := fs.Int("max-turns", 0, "Override the default turn safety limit per game (0 = use the default)")
	maxDuration := fs.Duration("max-duration", 0, "Override the default wall-clock safety limit per game (0 = use the default)")
	percentile := fs.Float64("percentile", 50, "Percentile (0-100) of each phase's positive score-delta distribution to use as its margin")
	modelType := fs.String("model-type", "linear", "Value function model type to calibrate: linear|gcn")
	searchDepth := fs.Int("search-depth", 0, "If > 0, calibrate margins under an N-ply attack sequence search (bot.ValueStrategy.AttackSearchDepth) instead of the single-ply blend -- the calibrated margin only applies to whatever depth/risky/breadth it was calibrated at, the same reason a weights file needs its own calibration run (see internal/bot/attack_search.go)")
	risky := fs.Float64("risky", 0.3, "Attack Handler Risky threshold (bot.ValueStrategy.Risky), only consulted when --search-depth > 0")
	searchBreadth := fs.Int("search-breadth", 0, "If > 0 and --search-depth > 0, cap each search level to this many top-scoring candidates (bot.ValueStrategy.AttackSearchBreadth) -- unpruned search at depth >= 2 is too slow to finish enough games inside the default per-game duration limit for a meaningful sample; see internal/bot/attack_search.go")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *input == "" || *output == "" {
		return fmt.Errorf("--input and --output are required")
	}
	if *games <= 0 {
		return fmt.Errorf("--games must be positive")
	}

	value, err := loadValueFunction(*input, *modelType)
	if err != nil {
		return err
	}
	// Zero-margin wrapper: lets the strategy act on any positive-delta
	// candidate, so the Observer sees the full natural distribution
	// rather than one already filtered by a margin.
	observedValue := zeroMarginValueFunction{value}

	seatStrategies := strings.Split(*strategies, ",")
	boardValueSeat := -1
	for i, s := range seatStrategies {
		if s == "board-value-candidate" {
			boardValueSeat = i
		}
	}
	if boardValueSeat == -1 {
		return fmt.Errorf("--strategies must include exactly one board-value-candidate seat")
	}

	var mu sync.Mutex
	deltas := map[string][]float64{"attack": nil, "fortify": nil}
	observe := func(phase string, bestScore, currentScore float64) {
		mu.Lock()
		deltas[phase] = append(deltas[phase], bestScore-currentScore)
		mu.Unlock()
	}

	limits := simulation.DefaultLimits()
	if *maxTurns > 0 {
		limits.MaxTurns = *maxTurns
	}
	if *maxDuration > 0 {
		limits.MaxDuration = *maxDuration
	}

	sem := make(chan struct{}, *parallel)
	var wg sync.WaitGroup
	for g := range *games {
		seed := *seedStart + int64(g)
		wg.Add(1)
		sem <- struct{}{}
		go func(seed int64) {
			defer wg.Done()
			defer func() { <-sem }()

			bvs := bot.NewBoardValueStrategy(observedValue)
			bvs.AttackSearchDepth = *searchDepth
			bvs.Risky = *risky
			bvs.AttackSearchBreadth = *searchBreadth
			bvs.Observer = observe
			registry := bot.StrategyRegistry{
				bot.StrategyBasicV1:     bot.NewBasicStrategy(),
				bot.StrategyScoredV1:    bot.NewScoredStrategy(bot.DefaultWeights),
				"board-value-candidate": bvs,
			}
			sim := simulation.NewSimulator(registry)
			cfg := simulation.Config{
				Seed:       seed,
				Strategies: seatStrategies,
				GameMode:   simulation.GameMode(*gameMode),
				Trace:      simulation.TraceNone,
				Limits:     limits,
			}
			_, _, _ = sim.RunOne(context.Background(), cfg, nil)
		}(seed)
	}
	wg.Wait()

	rawMargins := map[string]float64{}
	for _, phase := range []string{"attack", "fortify"} {
		rawMargins[phase] = positivePercentile(deltas[phase], *percentile)
		fmt.Printf("%s: n=%d observed decisions, margin=%.4f (p%.0f of positive deltas)\n", phase, len(deltas[phase]), rawMargins[phase], *percentile)
	}

	if err := writeCalibrated(*input, *output, rawMargins["attack"], rawMargins["fortify"]); err != nil {
		return err
	}
	fmt.Printf("Calibrated margins written -> %s\n", *output)
	return nil
}

// positivePercentile returns the given percentile (0-100) of only the
// positive values in xs -- negative/zero deltas already correctly end the
// phase under any margin >= 0 and aren't informative about how large a
// genuine improvement looks for this phase.
func positivePercentile(xs []float64, percentile float64) float64 {
	var positive []float64
	for _, x := range xs {
		if x > 0 {
			positive = append(positive, x)
		}
	}
	if len(positive) == 0 {
		return 0
	}
	sort.Float64s(positive)
	rank := percentile / 100 * float64(len(positive)-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return positive[lo]
	}
	frac := rank - float64(lo)
	return positive[lo]*(1-frac) + positive[hi]*frac
}

// writeCalibrated reads inputPath's JSON, overwrites its attack_margin/
// fortify_margin fields, and writes the result to outputPath -- every
// other field (weights, intercept, mean, std, feature_names) passes
// through unchanged.
func writeCalibrated(inputPath, outputPath string, attackMargin, fortifyMargin float64) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", inputPath, err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("parse %s: %w", inputPath, err)
	}
	payload["attack_margin"] = attackMargin
	payload["fortify_margin"] = fortifyMargin

	out, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, out, 0o644)
}
