package main

import (
	"fmt"
	"strings"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
	"github.com/nmiano1111/global-conquest/backend/internal/bot/gcnmodel"
)

// gcnVariantFlag collects repeated --gcn-variant <strategy-id>=<path>
// pairs into a flag.Value -- mirrors boardValueVariantFlag exactly.
type gcnVariantFlag []gcnVariantEntry

type gcnVariantEntry struct {
	StrategyID  string
	WeightsPath string
}

func (f *gcnVariantFlag) String() string {
	parts := make([]string, len(*f))
	for i, e := range *f {
		parts[i] = e.StrategyID + "=" + e.WeightsPath
	}
	return strings.Join(parts, ",")
}

func (f *gcnVariantFlag) Set(value string) error {
	id, path, ok := strings.Cut(value, "=")
	if !ok || id == "" || path == "" {
		return fmt.Errorf("invalid --gcn-variant %q, want <strategy-id>=<weights-path>", value)
	}
	*f = append(*f, gcnVariantEntry{StrategyID: id, WeightsPath: path})
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
		registry[v.StrategyID] = bot.NewBoardValueStrategy(model)
	}
	return nil
}
