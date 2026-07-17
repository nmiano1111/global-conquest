package main

import (
	"fmt"
	"strings"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
	"github.com/nmiano1111/global-conquest/backend/internal/simulation"
)

// trainingRow is one labeled example: the raw (unweighted) feature values
// scored-v1 saw for the command it actually chose, plus whether the player
// who made that decision went on to win the game.
type trainingRow struct {
	// GameID identifies which game this row came from, safe to use as a
	// join/group key even across separate cmd/traindata invocations whose
	// output files get combined later -- see computeGameID. Seed alone is
	// NOT safe for that: two different invocations (different --strategies
	// or --game-mode) can both use seed 1 to produce two entirely
	// different games.
	GameID       string
	Seed         int64
	Phase        string
	StrategyID   string
	PlayerID     string
	Seat         int
	Turn         int
	CommandIndex int
	Won          bool
	// Features is the raw (unweighted) signal per named feature -- one key
	// for every feature phaseFeatures[Phase] knows about, defaulting to 0.0
	// when that feature wasn't present on this particular candidate (see
	// rowsFromEntries), never a missing key.
	Features map[string]float64
}

// computeGameID deterministically identifies one game, safe to use across
// separate cmd/traindata invocations. Composed from exactly the inputs
// that determine a game's actual trajectory -- the same seed, strategies,
// and game mode always reproduce an identical game (internal/simulation's
// own determinism contract) -- so the same config plus the same seed
// always yields the same GameID, and two different configs sharing a seed
// never collide.
//
// MaxTurns/MaxCommands are deliberately not part of this: two runs
// differing only in those limits still produce identical decisions up to
// whichever point the tighter one cuts the game off -- not a genuinely
// different game, just a shorter look at the same one. Rows from both
// would legitimately describe the same decisions if ever combined, not
// colliding data from unrelated games.
func computeGameID(strategies []string, gameMode string, seed int64) string {
	return fmt.Sprintf("%s@%s@%d", strings.Join(strategies, ","), gameMode, seed)
}

// phaseFeatures lists, per phase, every feature name this tool knows how to
// invert back to a raw signal, and the bot.DefaultWeights field it was
// multiplied by -- read directly from every Feature{Name: "...", ...} line
// in internal/bot/strategy_scored*.go, not derived by reflection (the Go
// field names don't mechanically match the feature-name strings, e.g.
// ReinforceEnemyThreat -> "enemy_threat").
//
// Hardcoded against bot.DefaultWeights specifically: this tool's job is
// producing training data from games played under today's baseline
// weights, not an arbitrary candidate's -- extending it to a parameterized
// Weights value is future work once 10_Bot_Weight_Tuning.md's
// LoadWeights/--weights-variant pieces exist.
//
// Deliberately excludes "end_phase_bias" (attack) and "end_turn_bias"
// (fortify): both are flat constants added only to the synthetic "end this
// phase"/"end turn" candidate (Value == weight directly, no signal to
// recover), and both are 0.0 in DefaultWeights today, which would divide
// by zero on inversion.
//
// "continent_value" means two different weights depending on phase
// (ReinforceContinentValue vs FortifyContinentValue) -- every lookup here
// is keyed by (phase, name), never name alone.
var phaseFeatures = map[string]map[string]float64{
	"attack": {
		"army_advantage":         bot.DefaultWeights.ArmyAdvantage,
		"capture_probability":    bot.DefaultWeights.CaptureProbability,
		"expected_loss_cost":     bot.DefaultWeights.ExpectedLossCost,
		"completes_continent":    bot.DefaultWeights.CompletesContinent,
		"breaks_enemy_continent": bot.DefaultWeights.BreaksEnemyContinent,
		"card_opportunity":       bot.DefaultWeights.CardOpportunity,
		"eliminates_player":      bot.DefaultWeights.EliminatesPlayer,
		"exposure_penalty":       bot.DefaultWeights.ExposurePenalty,
	},
	"reinforce": {
		"enemy_threat":          bot.DefaultWeights.ReinforceEnemyThreat,
		"enemy_territory_count": bot.DefaultWeights.ReinforceEnemyTerritoryCount,
		"weakness":              bot.DefaultWeights.ReinforceWeakness,
		"continent_value":       bot.DefaultWeights.ReinforceContinentValue,
		"concentration_penalty": bot.DefaultWeights.ReinforceConcentrationPenalty,
	},
	"occupy": {
		"defense_coverage": bot.DefaultWeights.OccupyDefenseCoverage,
		"momentum":         bot.DefaultWeights.OccupyMomentum,
		"momentum_surplus": bot.DefaultWeights.OccupyMomentumSurplus,
	},
	"fortify": {
		"destination_threat":   bot.DefaultWeights.FortifyDestinationThreat,
		"continent_value":      bot.DefaultWeights.FortifyContinentValue,
		"source_exposure_cost": bot.DefaultWeights.FortifySourceExposureCost,
	},
}

func init() {
	// setup_reinforce shares reinforce's exact feature set --
	// reinforceFeatures in internal/bot/strategy_scored_reinforce.go is
	// literally the same function for both -- same weight mapping, but
	// kept as a distinct Phase value in output rows so the Python fitting
	// step can decide whether to merge them; generation time shouldn't
	// pre-judge that.
	phaseFeatures["setup_reinforce"] = phaseFeatures["reinforce"]
}

// rowsFromEntries converts one game's decision trace into training rows.
// gameID should come from computeGameID for the same seed/config. winnerPlayerID
// is empty for a game that didn't complete -- no reliable win/loss label
// exists for it, so every entry from that game is skipped.
//
// An entry produces a row only if at least one of its phase's known
// features was actually present on the chosen candidate -- this is what
// naturally excludes basic-v1's always-empty Explanation, card-trade-in
// entries (an arbitrary "reason" feature name never present in
// phaseFeatures), and end-phase/end-turn-only entries (only the excluded
// bias feature), with no need to filter by StrategyID or action type.
func rowsFromEntries(seed int64, gameID string, entries []simulation.Entry, winnerPlayerID string) []trainingRow {
	if winnerPlayerID == "" {
		return nil
	}

	var rows []trainingRow
	for _, e := range entries {
		known, ok := phaseFeatures[e.Phase]
		if !ok {
			continue
		}

		present := make(map[string]float64, len(e.Explanation.Features))
		for _, f := range e.Explanation.Features {
			present[f.Name] = f.Value
		}

		features := make(map[string]float64, len(known))
		recognized := 0
		for name, weight := range known {
			raw := 0.0
			if v, ok := present[name]; ok {
				raw = v / weight
				recognized++
			}
			features[name] = raw
		}
		if recognized == 0 {
			continue
		}

		rows = append(rows, trainingRow{
			GameID:       gameID,
			Seed:         seed,
			Phase:        e.Phase,
			StrategyID:   e.StrategyID,
			PlayerID:     e.PlayerID,
			Seat:         e.Seat,
			Turn:         e.Turn,
			CommandIndex: e.CommandIndex,
			Won:          e.PlayerID == winnerPlayerID,
			Features:     features,
		})
	}
	return rows
}
