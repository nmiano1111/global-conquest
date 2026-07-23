package main

import (
	"fmt"
	"strings"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
	"github.com/nmiano1111/global-conquest/backend/internal/bot/gcnmodel"
)

// gcnVariantFlag collects repeated
// --gcn-variant <strategy-id>=<path>[,search-depth=N][,risky=R] entries
// into a flag.Value -- mirrors boardValueVariantFlag exactly.
type gcnVariantFlag []gcnVariantEntry

type gcnVariantEntry struct {
	StrategyID    string
	WeightsPath   string
	SearchDepth   int
	Risky         float64
	SearchBreadth int
}

func (f *gcnVariantFlag) String() string {
	parts := make([]string, len(*f))
	for i, e := range *f {
		parts[i] = e.StrategyID + "=" + e.WeightsPath
	}
	return strings.Join(parts, ",")
}

// Set parses "<strategy-id>=<weights-path>[,search-depth=N][,risky=R]" --
// see searchVariantOptions for the shared suffix-parsing logic
// boardValueVariantFlag.Set also uses.
func (f *gcnVariantFlag) Set(value string) error {
	fields := strings.Split(value, ",")
	id, path, ok := strings.Cut(fields[0], "=")
	if !ok || id == "" || path == "" {
		return fmt.Errorf("invalid --gcn-variant %q, want <strategy-id>=<weights-path>", value)
	}
	entry := gcnVariantEntry{StrategyID: id, WeightsPath: path}
	opts, err := searchVariantOptions("--gcn-variant", value, fields[1:])
	if err != nil {
		return err
	}
	entry.SearchDepth, entry.Risky, entry.SearchBreadth = opts.depth, opts.risky, opts.breadth
	*f = append(*f, entry)
	return nil
}

// registerGCNVariants loads each entry's gcn_fit.py-exported weights file
// (gcnmodel.LoadModel) and adds a bot.ValueStrategy scored by it to
// registry under its given ID -- the same strategy shell
// --board-value-variant uses, since bot.ValueStrategy is generic
// over bot.ValueFunction and gcnmodel.Model implements it. Rejects any ID
// that collides with an already-registered strategy -- a built-in, a
// --weights-variant/--board-value-variant, or a repeated --gcn-variant
// ID -- same rationale as the other variant registrars.
func registerGCNVariants(registry bot.StrategyRegistry, variants gcnVariantFlag) error {
	for _, v := range variants {
		if _, exists := registry[v.StrategyID]; exists {
			return fmt.Errorf("--gcn-variant %s: strategy ID is already registered (a built-in strategy or a duplicate variant)", v.StrategyID)
		}
		model, err := gcnmodel.LoadModel(v.WeightsPath)
		if err != nil {
			return fmt.Errorf("--gcn-variant %s: %w", v.StrategyID, err)
		}
		bvs := bot.NewBoardValueStrategy(model)
		bvs.AttackSearchDepth = v.SearchDepth
		bvs.Risky = v.Risky
		bvs.AttackSearchBreadth = v.SearchBreadth
		registry[v.StrategyID] = bvs
	}
	return nil
}
