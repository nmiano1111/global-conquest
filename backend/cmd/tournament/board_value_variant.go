package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
)

// boardValueVariantFlag collects repeated
// --board-value-variant <strategy-id>=<weights-path>[,search-depth=N][,risky=R]
// entries into a flag.Value -- mirrors gcnVariantFlag exactly.
type boardValueVariantFlag []boardValueVariantEntry

type boardValueVariantEntry struct {
	StrategyID    string
	WeightsPath   string
	SearchDepth   int
	Risky         float64
	SearchBreadth int
}

func (f *boardValueVariantFlag) String() string {
	parts := make([]string, len(*f))
	for i, e := range *f {
		parts[i] = e.StrategyID + "=" + e.WeightsPath
	}
	return strings.Join(parts, ",")
}

// Set parses "<strategy-id>=<weights-path>[,search-depth=N][,risky=R]" --
// see searchVariantOptions for the shared suffix-parsing logic
// gcnVariantFlag.Set also uses.
func (f *boardValueVariantFlag) Set(value string) error {
	fields := strings.Split(value, ",")
	id, path, ok := strings.Cut(fields[0], "=")
	if !ok || id == "" || path == "" {
		return fmt.Errorf("invalid --board-value-variant %q, want <strategy-id>=<weights-path>", value)
	}
	entry := boardValueVariantEntry{StrategyID: id, WeightsPath: path}
	opts, err := searchVariantOptions("--board-value-variant", value, fields[1:])
	if err != nil {
		return err
	}
	entry.SearchDepth, entry.Risky, entry.SearchBreadth = opts.depth, opts.risky, opts.breadth
	*f = append(*f, entry)
	return nil
}

// searchVariantOpts holds the optional bot.ValueStrategy search fields
// --board-value-variant/--gcn-variant can set via their suffix syntax.
type searchVariantOpts struct {
	depth   int
	risky   float64
	breadth int
}

// searchVariantOptions parses the optional
// "search-depth=N"/"risky=R"/"search-breadth=N" suffix fields shared by
// --board-value-variant and --gcn-variant
// (bot.ValueStrategy.AttackSearchDepth/Risky/AttackSearchBreadth -- see
// internal/bot/attack_search.go), so both --gcn-variant and
// --board-value-variant can register an A/B-testable move-sequence
// search variant alongside the original single-ply blend baseline
// (depth 0, the zero value) without a separate flag. flagName/rawValue
// are only used to build a readable error.
func searchVariantOptions(flagName, rawValue string, fields []string) (searchVariantOpts, error) {
	var opts searchVariantOpts
	for _, field := range fields {
		k, v, ok := strings.Cut(field, "=")
		if !ok {
			return searchVariantOpts{}, fmt.Errorf("invalid %s %q: option %q must be key=value", flagName, rawValue, field)
		}
		var err error
		switch k {
		case "search-depth":
			opts.depth, err = strconv.Atoi(v)
			if err != nil {
				return searchVariantOpts{}, fmt.Errorf("invalid %s %q: search-depth must be an integer: %w", flagName, rawValue, err)
			}
		case "risky":
			opts.risky, err = strconv.ParseFloat(v, 64)
			if err != nil {
				return searchVariantOpts{}, fmt.Errorf("invalid %s %q: risky must be a float: %w", flagName, rawValue, err)
			}
		case "search-breadth":
			opts.breadth, err = strconv.Atoi(v)
			if err != nil {
				return searchVariantOpts{}, fmt.Errorf("invalid %s %q: search-breadth must be an integer: %w", flagName, rawValue, err)
			}
		default:
			return searchVariantOpts{}, fmt.Errorf("invalid %s %q: unknown option %q (want search-depth, risky, or search-breadth)", flagName, rawValue, k)
		}
	}
	return opts, nil
}

// registerBoardValueVariants loads each entry's board_fit.py-exported
// weights file (bot.LoadBoardValue) and adds a bot.ValueStrategy to
// registry under its given ID. Rejects any ID that collides with an
// already-registered strategy -- a built-in, a --weights-variant, or a
// repeated --board-value-variant ID -- same rationale as
// registerWeightsVariants.
func registerBoardValueVariants(registry bot.StrategyRegistry, variants boardValueVariantFlag) error {
	for _, v := range variants {
		if _, exists := registry[v.StrategyID]; exists {
			return fmt.Errorf("--board-value-variant %s: strategy ID is already registered (a built-in strategy or a duplicate variant)", v.StrategyID)
		}
		value, err := bot.LoadBoardValue(v.WeightsPath)
		if err != nil {
			return fmt.Errorf("--board-value-variant %s: %w", v.StrategyID, err)
		}
		bvs := bot.NewBoardValueStrategy(value)
		bvs.AttackSearchDepth = v.SearchDepth
		bvs.Risky = v.Risky
		bvs.AttackSearchBreadth = v.SearchBreadth
		registry[v.StrategyID] = bvs
	}
	return nil
}
