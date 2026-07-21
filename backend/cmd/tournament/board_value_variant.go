package main

import (
	"fmt"
	"strings"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
)

// boardValueVariantFlag collects repeated --board-value-variant
// <strategy-id>=<weights-path> pairs into a flag.Value -- mirrors
// weightsVariantFlag exactly.
type boardValueVariantFlag []boardValueVariantEntry

type boardValueVariantEntry struct {
	StrategyID  string
	WeightsPath string
}

func (f *boardValueVariantFlag) String() string {
	parts := make([]string, len(*f))
	for i, e := range *f {
		parts[i] = e.StrategyID + "=" + e.WeightsPath
	}
	return strings.Join(parts, ",")
}

func (f *boardValueVariantFlag) Set(value string) error {
	id, path, ok := strings.Cut(value, "=")
	if !ok || id == "" || path == "" {
		return fmt.Errorf("invalid --board-value-variant %q, want <strategy-id>=<weights-path>", value)
	}
	*f = append(*f, boardValueVariantEntry{StrategyID: id, WeightsPath: path})
	return nil
}

// registerBoardValueVariants loads each entry's board_fit.py-exported
// weights file (bot.LoadBoardValue) and adds a bot.ValueStrategy to
// registry under its given ID, with Lookahead set from lookahead (see
// --lookahead in main.go). Rejects any ID that collides with an
// already-registered strategy -- a built-in, a --weights-variant, or a
// repeated --board-value-variant ID -- same rationale as
// registerWeightsVariants.
func registerBoardValueVariants(registry bot.StrategyRegistry, variants boardValueVariantFlag, lookahead bool) error {
	for _, v := range variants {
		if _, exists := registry[v.StrategyID]; exists {
			return fmt.Errorf("--board-value-variant %s: strategy ID is already registered (a built-in strategy or a duplicate variant)", v.StrategyID)
		}
		value, err := bot.LoadBoardValue(v.WeightsPath)
		if err != nil {
			return fmt.Errorf("--board-value-variant %s: %w", v.StrategyID, err)
		}
		bvs := bot.NewBoardValueStrategy(value)
		bvs.Lookahead = lookahead
		registry[v.StrategyID] = bvs
	}
	return nil
}
