package main

import (
	"fmt"
	"strings"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
)

// gbtVariantFlag collects repeated --gbt-variant <strategy-id>=<model-dir>
// pairs into a flag.Value -- mirrors weightsVariantFlag exactly.
type gbtVariantFlag []gbtVariantEntry

type gbtVariantEntry struct {
	StrategyID string
	ModelDir   string
}

func (f *gbtVariantFlag) String() string {
	parts := make([]string, len(*f))
	for i, e := range *f {
		parts[i] = e.StrategyID + "=" + e.ModelDir
	}
	return strings.Join(parts, ",")
}

func (f *gbtVariantFlag) Set(value string) error {
	id, dir, ok := strings.Cut(value, "=")
	if !ok || id == "" || dir == "" {
		return fmt.Errorf("invalid --gbt-variant %q, want <strategy-id>=<model-dir>", value)
	}
	*f = append(*f, gbtVariantEntry{StrategyID: id, ModelDir: dir})
	return nil
}

// registerGBTVariants loads each entry's four phase model files
// (bot.LoadGBTModels) and adds a bot.GBTStrategy to registry under its
// given ID. Rejects any ID that collides with an already-registered
// strategy -- a built-in, a --weights-variant, or a repeated
// --gbt-variant ID -- same rationale as registerWeightsVariants.
func registerGBTVariants(registry bot.StrategyRegistry, variants gbtVariantFlag) error {
	for _, v := range variants {
		if _, exists := registry[v.StrategyID]; exists {
			return fmt.Errorf("--gbt-variant %s: strategy ID is already registered (a built-in strategy or a duplicate variant)", v.StrategyID)
		}
		models, err := bot.LoadGBTModels(v.ModelDir)
		if err != nil {
			return fmt.Errorf("--gbt-variant %s: %w", v.StrategyID, err)
		}
		registry[v.StrategyID] = bot.NewGBTStrategy(models)
	}
	return nil
}
