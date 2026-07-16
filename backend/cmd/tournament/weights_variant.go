package main

import (
	"fmt"
	"strings"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
)

// weightsVariantFlag collects repeated --weights-variant <strategy-id>=<path>
// pairs into a flag.Value -- Go's flag package has no native support for a
// repeatable flag.
type weightsVariantFlag []weightsVariantEntry

type weightsVariantEntry struct {
	StrategyID string
	Path       string
}

func (f *weightsVariantFlag) String() string {
	parts := make([]string, len(*f))
	for i, e := range *f {
		parts[i] = e.StrategyID + "=" + e.Path
	}
	return strings.Join(parts, ",")
}

func (f *weightsVariantFlag) Set(value string) error {
	id, path, ok := strings.Cut(value, "=")
	if !ok || id == "" || path == "" {
		return fmt.Errorf("invalid --weights-variant %q, want <strategy-id>=<path>", value)
	}
	*f = append(*f, weightsVariantEntry{StrategyID: id, Path: path})
	return nil
}

// registerWeightsVariants loads each entry's weights file
// (bot.LoadWeights) and adds a custom-weighted bot.ScoredStrategy to
// registry under its given ID. Rejects any ID that collides with an
// already-registered strategy -- a built-in ("basic-v1"/"scored-v1") or a
// repeated --weights-variant ID -- since silently shadowing a built-in
// (or letting two variants overwrite each other) would change tournament
// behavior in a way that's easy to miss on the command line.
func registerWeightsVariants(registry bot.StrategyRegistry, variants weightsVariantFlag) error {
	for _, v := range variants {
		if _, exists := registry[v.StrategyID]; exists {
			return fmt.Errorf("--weights-variant %s: strategy ID is already registered (a built-in strategy or a duplicate --weights-variant)", v.StrategyID)
		}
		w, err := bot.LoadWeights(v.Path)
		if err != nil {
			return fmt.Errorf("--weights-variant %s: %w", v.StrategyID, err)
		}
		registry[v.StrategyID] = bot.NewScoredStrategy(w)
	}
	return nil
}
