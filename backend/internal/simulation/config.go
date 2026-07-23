// Package simulation drives complete bot-vs-bot Global Conquest games
// headlessly: no Postgres, no WebSocket, no Discord, no HTTP, no live
// pacing. It calls internal/risk directly and resolves decisions through
// internal/bot's Strategy interface, translating each returned Command to
// the matching risk.Game method itself rather than reusing bot.Runner —
// see simulator.go for why.
package simulation

import (
	"errors"
	"fmt"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// TraceLevel controls how much detail a simulation records beyond its
// final Result. See recorder.go for exactly what each level captures.
type TraceLevel string

const (
	TraceNone     TraceLevel = "none"
	TraceSummary  TraceLevel = "summary"
	TraceDecision TraceLevel = "decision"
	TraceFull     TraceLevel = "full"
)

// GameMode selects which risk constructor builds the simulated game.
// AutoStart skips straight to PhaseReinforce with armies already
// distributed; Manual stops at PhaseSetupReinforce and requires bots to
// place their own initial armies via place_initial_army commands -- named
// to match internal/service's own "manual" setup-mode terminology for the
// identical underlying risk.NewClassicRandomTerritoryGame constructor
// (see that function's doc comment for why its own name doesn't match).
// Both are fully supported by the command dispatcher; which one to use is
// a per-experiment choice, not a fixed default.
type GameMode string

const (
	GameModeAutoStart GameMode = "auto_start"
	GameModeManual    GameMode = "manual"
)

// ErrInvalidConfig is returned by Config.Validate (and Limits.Validate) for
// any configuration mistake caught before a risk.Game is ever constructed.
// A simulation must never surface this kind of problem as a mid-run
// Failure — an unresolvable strategy ID or a bad player count is a setup
// error, not a game event.
var ErrInvalidConfig = errors.New("simulation: invalid config")

// Limits bounds a single simulation so a strategy bug can't hang forever.
type Limits struct {
	// MaxCommands stops the simulation after this many total dispatched
	// commands, across every seat and phase.
	MaxCommands int
	// MaxTurns stops the simulation once risk.Game.TurnNumber reaches
	// this value.
	MaxTurns int
	// MaxCommandsWithoutProgress stops the simulation if this many
	// consecutive commands pass with neither CurrentPlayer nor Phase
	// changing — a cheap first-line stall detector, checked before the
	// more expensive repeated-state-hash check (see statehash.go).
	//
	// This is a coarse heuristic, not a precise "nothing is happening"
	// signal: within a single attack phase, every individual dice round
	// is its own command, and CurrentPlayer/Phase legitimately stay
	// unchanged for as many commands as the fight takes to resolve.
	// Confirmed by tracing an actual failure this heuristic caused: a
	// real, steadily-progressing fight between two armies in the
	// hundreds (accumulated over 200+ turns) needed on the order of a
	// thousand consecutive same-phase commands to resolve, well past the
	// old default of 500 — MaxRepeatedStates (below) is the check that
	// actually detects genuine non-progress (an *identical* state
	// recurring), since real combat changes army counts, hence the state
	// fingerprint, every round; this value only needs to be generous
	// enough not to misfire on legitimately long fights before that
	// check (or MaxCommands) would catch a truly pathological case.
	MaxCommandsWithoutProgress int
	// MaxRepeatedStates stops the simulation once the same state
	// fingerprint (statehash.go) has recurred this many times.
	MaxRepeatedStates int
	// MaxDuration is a wall-clock backstop, checked alongside the other
	// limits every command. It is deliberately NOT part of the
	// deterministic Result/trace story (Result.Duration is explicitly
	// excluded from determinism comparisons) -- it exists only as a
	// practical safety valve for scenarios the other limits can't catch:
	// e.g. two bots locked in an "arms race" at a shared border,
	// indefinitely reinforcing without ever attacking, where the state
	// fingerprint changes every turn (army counts keep climbing) so
	// MaxCommandsWithoutProgress/MaxRepeatedStates never fire, while
	// ForecastAttack's cost grows with army count and quietly turns each
	// decision more expensive than the last. Confirmed necessary by an
	// actual such stalemate hanging a test during development.
	MaxDuration time.Duration
}

// DefaultLimits are generous enough not to cut off any real game while
// still bounding a runaway strategy bug (or a genuine gameplay stalemate)
// to a fast, cheap failure.
func DefaultLimits() Limits {
	return Limits{
		MaxCommands:                20000,
		MaxTurns:                   2000,
		MaxCommandsWithoutProgress: 5000,
		MaxRepeatedStates:          3,
		MaxDuration:                30 * time.Second,
	}
}

// Validate checks the limits are usable before a simulation starts.
func (l Limits) Validate() error {
	if l.MaxCommands <= 0 {
		return fmt.Errorf("%w: MaxCommands must be positive, got %d", ErrInvalidConfig, l.MaxCommands)
	}
	if l.MaxTurns <= 0 {
		return fmt.Errorf("%w: MaxTurns must be positive, got %d", ErrInvalidConfig, l.MaxTurns)
	}
	if l.MaxCommandsWithoutProgress <= 0 {
		return fmt.Errorf("%w: MaxCommandsWithoutProgress must be positive, got %d", ErrInvalidConfig, l.MaxCommandsWithoutProgress)
	}
	if l.MaxRepeatedStates <= 0 {
		return fmt.Errorf("%w: MaxRepeatedStates must be positive, got %d", ErrInvalidConfig, l.MaxRepeatedStates)
	}
	if l.MaxDuration <= 0 {
		return fmt.Errorf("%w: MaxDuration must be positive, got %s", ErrInvalidConfig, l.MaxDuration)
	}
	return nil
}

// TurnBoundary describes one completed player turn, passed to
// Config.OnTurnBoundary (if set) right after the corresponding
// command dispatches -- either a normal end_turn, or, less commonly, any
// other action that ends the game directly (a conquering attack or
// occupation that eliminates the second-to-last player checks for a
// winner immediately, without ever reaching another end_turn -- see
// risk.Game.OccupyTerritory/Attack). Game reflects the resulting board:
// the start of the next player's turn, or the final PhaseGameOver state.
//
// Game is the simulation's own live, mutable *risk.Game -- valid only for
// the duration of the callback. A caller that needs to retain anything
// must copy/encode what it needs synchronously within the callback (e.g.
// via internal/tdstate.Encode, which never aliases back into Game); it
// must not store the pointer itself for later use.
type TurnBoundary struct {
	Game     *risk.Game
	Seat     int
	PlayerID string
	Turn     int
}

// Config fully specifies one reproducible simulation: the same Config
// (same Seed, same Strategies, same GameMode) against the same strategy
// registry must always produce the same game.
type Config struct {
	Seed int64

	// OnTurnBoundary, if set, is called once per completed player turn
	// (see TurnBoundary) -- purely an observation side-channel like
	// onProgress in RunOne, never influencing the game or its
	// determinism. Left nil, it costs nothing: RunOne only ever calls it
	// when non-nil.
	OnTurnBoundary func(TurnBoundary)

	// Strategies holds one strategy ID per seat, in seat order; its
	// length is the player count (risk.NewClassicGame requires 3-6). Seat
	// i is assigned player ID SeatPlayerID(i) before risk's internal
	// turn-order shuffle runs, so this mapping stays correct regardless
	// of where that player ends up sitting after shuffling.
	Strategies []string

	GameMode GameMode
	Trace    TraceLevel
	Limits   Limits
}

// PlayerCount is the number of seats this config configures, derived from
// Strategies rather than tracked separately so the two can't drift apart.
func (c Config) PlayerCount() int {
	return len(c.Strategies)
}

// SeatPlayerID returns the deterministic player ID assigned to a seat
// before construction: simple and reproducible, unlike production's
// crypto/rand-backed UUID-format bot IDs (internal/service's unexported
// newBotPlayerID). Nothing in internal/risk or internal/bot parses or
// validates player ID format, so there's no reason to match that shape.
func SeatPlayerID(seat int) string {
	return fmt.Sprintf("p%d", seat)
}

// PlayerIDs returns the ordered player IDs this config passes to
// risk.NewClassicGame / NewClassicAutoStartGame / NewClassicRandomTerritoryGame.
func (c Config) PlayerIDs() []string {
	ids := make([]string, len(c.Strategies))
	for i := range c.Strategies {
		ids[i] = SeatPlayerID(i)
	}
	return ids
}

// StrategyByPlayerID builds the playerID -> strategyID mapping this config
// implies. Intended to be called once at simulation setup and consulted
// by player ID thereafter (risk's internal turn-order shuffle reorders
// risk.Game.Players, so an index-based lookup after construction would be
// wrong; a player's ID is stable through the shuffle, its index is not).
func (c Config) StrategyByPlayerID() map[string]string {
	m := make(map[string]string, len(c.Strategies))
	for i, strategyID := range c.Strategies {
		m[SeatPlayerID(i)] = strategyID
	}
	return m
}

// Validate checks a Config against the given strategy registry, catching
// configuration mistakes -- an unknown strategy ID, a player count outside
// risk's supported 3-6 range, an unrecognized trace level or game mode --
// before any risk.Game is constructed. These must never surface as a
// mid-run Failure, only as a setup-time error the caller can report
// immediately.
func (c Config) Validate(registry bot.StrategyRegistry) error {
	if len(c.Strategies) < 3 || len(c.Strategies) > 6 {
		return fmt.Errorf("%w: player count must be between 3 and 6, got %d", ErrInvalidConfig, len(c.Strategies))
	}
	for i, strategyID := range c.Strategies {
		if _, ok := registry.Get(strategyID); !ok {
			return fmt.Errorf("%w: seat %d: unknown strategy %q", ErrInvalidConfig, i, strategyID)
		}
	}
	switch c.Trace {
	case TraceNone, TraceSummary, TraceDecision, TraceFull:
	default:
		return fmt.Errorf("%w: unknown trace level %q", ErrInvalidConfig, c.Trace)
	}
	switch c.GameMode {
	case GameModeAutoStart, GameModeManual:
	default:
		return fmt.Errorf("%w: unknown game mode %q", ErrInvalidConfig, c.GameMode)
	}
	return c.Limits.Validate()
}
