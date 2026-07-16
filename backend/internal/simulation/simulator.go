package simulation

import (
	"context"
	"fmt"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// Simulator runs complete bot-vs-bot games headlessly, calling
// Strategy.NextCommand directly against an in-process *risk.Game and
// dispatching the result straight to the matching risk.Game method
// (Dispatch, dispatcher.go) -- it does not use bot.Runner. Runner never
// returns a strategy's Explanation (only logs it as a string), and
// retries a rejected command up to 3 times, a policy built for
// concurrent production state that would mask strategy bugs in this
// single-threaded, single-actor setting. See the simulation framework
// design doc for the full comparison.
type Simulator struct {
	registry bot.StrategyRegistry
}

// NewSimulator returns a Simulator resolving strategy IDs against the
// given registry.
func NewSimulator(registry bot.StrategyRegistry) *Simulator {
	return &Simulator{registry: registry}
}

// RunOne plays exactly one complete game to PhaseGameOver, a safety
// limit, or a hard failure -- never partially, never with retries. The
// same Config (same Seed, same Strategies, same GameMode) against the
// same registry always produces the same Result and the same trace.
//
// The returned *Recorder holds whatever trace data Config.Trace called
// for (see recorder.go); Result itself never embeds trace data, matching
// the design's separation between "what happened" (Result) and "how much
// detail was captured about it" (Recorder) -- a CLI or caller combines
// the two for output.
//
// Whenever Result.Failure is non-nil, the returned error is that same
// *Failure value (it implements error), so both idiomatic `if err != nil`
// handling and inspecting Result.Failure's structured fields work.
//
// onProgress, if non-nil, is called at most every progressInterval with
// the game's current state -- purely an observation side-channel (e.g.
// for a CLI to show a live status line) that never influences any
// decision or the game itself, so it has no bearing on determinism. Pass
// nil to skip it entirely.
//
// "Seat" throughout this function and in every Entry/Milestone/SeatResult
// means the stable 0-indexed position in Config.Strategies -- not
// risk.Game.CurrentPlayer, which indexes risk.Game.Players *after* the
// engine's internal turn-order shuffle and so does not stay aligned with
// any one seat across different seeds. Every lookup goes through
// seatByPlayerID, keyed by the deterministic player IDs Config assigns
// before construction.
func (s *Simulator) RunOne(ctx context.Context, cfg Config, onProgress func(ProgressUpdate)) (Result, *Recorder, error) {
	start := time.Now()
	lastProgress := start
	recorder := NewRecorder(cfg.Trace)

	if err := cfg.Validate(s.registry); err != nil {
		return Result{}, recorder, err
	}

	rng := NewDeterministicRNG(cfg.Seed)
	playerIDs := cfg.PlayerIDs()

	var g *risk.Game
	var err error
	switch cfg.GameMode {
	case GameModeManual:
		g, err = risk.NewClassicRandomTerritoryGame(playerIDs, rng)
	default: // Validate already rejected anything but the two known modes
		g, err = risk.NewClassicAutoStartGame(playerIDs, rng)
	}
	if err != nil {
		return Result{}, recorder, fmt.Errorf("simulation: construct game: %w", err)
	}

	seatByPlayerID := make(map[string]int, len(playerIDs))
	strategyByPlayerID := cfg.StrategyByPlayerID()
	for i, id := range playerIDs {
		seatByPlayerID[id] = i
	}
	for i := range g.Players {
		id := g.Players[i].ID
		g.Players[i].Controller = risk.ControllerBot
		g.Players[i].Strategy = strategyByPlayerID[id]
	}

	result := NewResult(cfg.Seed, cfg.PlayerCount())
	result.Seats = make([]SeatResult, len(playerIDs))
	for i, id := range playerIDs {
		result.Seats[i] = SeatResult{Seat: i, PlayerID: id, StrategyID: cfg.Strategies[i]}
	}

	fail := func(f *Failure) (Result, *Recorder, error) {
		result.Completed = false
		result.Failure = f
		result.Duration = time.Since(start)
		return result, recorder, f
	}

	limits := NewLimitTracker(cfg.Limits)
	var eliminationOrder []int // seat indices, in the order eliminated
	setupTurn := 0             // round-robin cursor for PhaseSetupReinforce, see below

	commandIndex := 0
	for {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fail(&Failure{
				Type: FailureContextCanceled, Message: ctxErr.Error(),
				Phase: string(g.Phase), Turn: g.TurnNumber, CommandIndex: commandIndex, Seed: cfg.Seed,
			})
		}
		if g.Phase == risk.PhaseGameOver {
			break
		}

		// PhaseSetupReinforce is a special case: PlaceInitialArmy lets
		// any player with reserves left act out of strict turn order
		// (checked in engine.go against playerID, not CurrentPlayer),
		// and only resets CurrentPlayer once every reserve is placed --
		// it never advances it per-placement. Reading g.CurrentPlayer
		// during this phase would therefore just keep asking whichever
		// single seat happened to hold it at construction, and once that
		// seat's reserves ran out, every subsequent decision would be
		// asked of a player with no legal action left. The simulator
		// round-robins through seats with remaining reserves itself.
		var current risk.PlayerState
		if g.Phase == risk.PhaseSetupReinforce {
			idx := nextSetupPlayer(g, setupTurn)
			if idx < 0 {
				// Unreachable: setupDone() inside PlaceInitialArmy would
				// already have advanced the phase away from
				// SetupReinforce once every reserve was placed.
				return fail(&Failure{
					Type:    FailureInternalInvariant,
					Message: "no player has setup reserves left but phase is still setup_reinforce",
					Phase:   string(g.Phase), Turn: g.TurnNumber, CommandIndex: commandIndex, Seed: cfg.Seed,
				})
			}
			current = g.Players[idx]
			setupTurn = (idx + 1) % len(g.Players)
		} else {
			if g.CurrentPlayer < 0 || g.CurrentPlayer >= len(g.Players) {
				return fail(&Failure{
					Type:    FailureInternalInvariant,
					Message: fmt.Sprintf("CurrentPlayer %d out of range for %d players", g.CurrentPlayer, len(g.Players)),
					Phase:   string(g.Phase), Turn: g.TurnNumber, CommandIndex: commandIndex, Seed: cfg.Seed,
				})
			}
			current = g.Players[g.CurrentPlayer]
		}
		seat := seatByPlayerID[current.ID]

		strat, ok := s.registry.Get(current.Strategy)
		if !ok {
			// Unreachable if Config.Validate and the Controller/Strategy
			// assignment above did their job -- guarded defensively
			// rather than assumed.
			return fail(&Failure{
				Type:       FailureInternalInvariant,
				Message:    fmt.Sprintf("no registered strategy %q for seat %d", current.Strategy, seat),
				Phase:      string(g.Phase),
				PlayerID:   current.ID,
				StrategyID: current.Strategy,
				Turn:       g.TurnNumber, CommandIndex: commandIndex, Seed: cfg.Seed,
			})
		}

		preDispatchPhase := g.Phase
		preDispatchTurn := g.TurnNumber

		cmd, expl, err := strat.NextCommand(ctx, g, current.ID)
		if err != nil {
			return fail(&Failure{
				Type:       FailureStrategyError,
				Message:    err.Error(),
				Phase:      string(preDispatchPhase),
				PlayerID:   current.ID,
				StrategyID: current.Strategy,
				Turn:       preDispatchTurn, CommandIndex: commandIndex, Seed: cfg.Seed,
			})
		}

		// Captured before Dispatch: a conquering attack transfers
		// ownership as part of the call itself, so the pre-attack owner
		// must be read first to attribute combat losses to the right
		// seat afterward.
		targetOwnerSeat := -1
		if cmd.Action == bot.ActionAttack {
			if ts, ok := g.Territories[risk.Territory(cmd.To)]; ok && ts.Owner >= 0 && ts.Owner < len(g.Players) {
				targetOwnerSeat = seatByPlayerID[g.Players[ts.Owner].ID]
			}
		}

		dispatchResult, err := Dispatch(g, current.ID, cmd)
		if err != nil {
			return fail(&Failure{
				Type:       FailureEngineRejectedCommand,
				Message:    err.Error(),
				Phase:      string(preDispatchPhase),
				PlayerID:   current.ID,
				StrategyID: current.Strategy,
				Command:    cmd.Action,
				Turn:       preDispatchTurn, CommandIndex: commandIndex, Seed: cfg.Seed,
			})
		}

		result.Commands++
		commandIndex++

		var fp StateFingerprint
		var domainEvent *risk.DomainEvent
		if recorder.Level() == TraceFull {
			fp = Fingerprint(g)
			domainEvent = dispatchResult.DomainEvent
		}
		recorder.RecordEntry(Entry{
			CommandIndex: commandIndex - 1,
			Turn:         preDispatchTurn,
			Seat:         seat,
			PlayerID:     current.ID,
			StrategyID:   current.Strategy,
			Phase:        string(preDispatchPhase),
			Command:      cmd,
			Explanation:  expl,
			Fingerprint:  fp,
			DomainEvent:  domainEvent,
		})

		if onProgress != nil && time.Since(lastProgress) >= progressInterval {
			onProgress(ProgressUpdate{Commands: result.Commands, Turn: g.TurnNumber, Elapsed: time.Since(start)})
			lastProgress = time.Now()
		}

		switch cmd.Action {
		case bot.ActionAttack:
			ar := dispatchResult.AttackResult
			result.CombatRolls++
			if targetOwnerSeat >= 0 {
				result.Seats[targetOwnerSeat].CombatLossesTaken += ar.DefenderLoss
			}
			if ar.Conquered {
				result.Captures++
				result.Seats[seat].Captures++
				recorder.RecordMilestone(Milestone{
					Type: MilestoneCapture, Turn: preDispatchTurn, Seat: seat, PlayerID: current.ID,
					Detail: fmt.Sprintf("%s captured %s from %s", current.ID, cmd.To, cmd.From),
				})
			}
			if ar.Eliminated != "" {
				result.Eliminations++
				result.Seats[seat].EliminationsMade++
				if eliminatedSeat, ok := seatByPlayerID[ar.Eliminated]; ok {
					result.Seats[eliminatedSeat].Eliminated = true
					eliminationOrder = append(eliminationOrder, eliminatedSeat)
				}
				recorder.RecordMilestone(Milestone{
					Type: MilestoneElimination, Turn: preDispatchTurn, Seat: seat, PlayerID: current.ID,
					Detail: fmt.Sprintf("%s eliminated %s", current.ID, ar.Eliminated),
				})
			}
		case bot.ActionTradeCards:
			result.CardTurnIns++
			recorder.RecordMilestone(Milestone{
				Type: MilestoneCardTurnIn, Turn: preDispatchTurn, Seat: seat, PlayerID: current.ID,
				Detail: fmt.Sprintf("%s traded cards for %d armies", current.ID, dispatchResult.ReinforcementsGranted),
			})
		case bot.ActionEndTurn:
			recorder.RecordMilestone(Milestone{
				Type: MilestoneTurnTransition, Turn: g.TurnNumber, Seat: seat, PlayerID: current.ID,
				Detail: fmt.Sprintf("%s ended their turn", current.ID),
			})
		}

		if g.Phase == risk.PhaseGameOver {
			break
		}

		if breach := limits.Observe(g); breach != nil {
			return fail(&Failure{
				Type:       breach.Type,
				Message:    breach.Message,
				Phase:      string(g.Phase),
				PlayerID:   current.ID,
				StrategyID: current.Strategy,
				Command:    cmd.Action,
				Turn:       g.TurnNumber, CommandIndex: commandIndex, Seed: cfg.Seed,
			})
		}
	}

	result.Turns = g.TurnNumber
	result.Completed = true
	result.Duration = time.Since(start)

	if onProgress != nil {
		onProgress(ProgressUpdate{Commands: result.Commands, Turn: result.Turns, Elapsed: result.Duration})
	}

	if g.Winner != "" {
		if winnerSeat, ok := seatByPlayerID[g.Winner]; ok {
			result.WinnerSeat = winnerSeat
			result.WinnerPlayerID = g.Winner
			result.WinnerStrategy = result.Seats[winnerSeat].StrategyID
			result.Seats[winnerSeat].FinishOrder = 1
		}
	}
	// FinishOrder among the eliminated: the seat eliminated LAST (survived
	// longest) places 2nd, down to the seat eliminated FIRST placing last
	// (PlayerCount). eliminationOrder is append-ordered first-to-last, so
	// walk it in reverse to hand out places 2, 3, 4, ...
	place := 2
	for i := len(eliminationOrder) - 1; i >= 0; i-- {
		result.Seats[eliminationOrder[i]].FinishOrder = place
		place++
	}

	for i, id := range playerIDs {
		territories, armies := countTerritories(g, id)
		result.Seats[i].FinalTerritories = territories
		result.Seats[i].FinalArmies = armies
	}

	return result, recorder, nil
}

// nextSetupPlayer finds the next seat, starting from and including from
// and wrapping around, with SetupReserves > 0 -- used to round-robin
// through players during PhaseSetupReinforce, since the engine itself
// only advances CurrentPlayer once every reserve across every player has
// been placed, not per-placement (see the comment at its call site in
// RunOne). Returns -1 if no player has reserves left.
func nextSetupPlayer(g *risk.Game, from int) int {
	n := len(g.Players)
	for i := 0; i < n; i++ {
		idx := (from + i) % n
		if g.SetupReserves[idx] > 0 {
			return idx
		}
	}
	return -1
}

// countTerritories sums the territories and armies owned by playerID at
// the end of a simulation. risk.Game has no exported equivalent
// (playerTerritoryCount is unexported), so the simulator computes it
// directly from final state.
func countTerritories(g *risk.Game, playerID string) (territories, armies int) {
	seatIdx := -1
	for i, p := range g.Players {
		if p.ID == playerID {
			seatIdx = i
			break
		}
	}
	if seatIdx < 0 {
		return 0, 0
	}
	for _, t := range g.Board.Order {
		ts := g.Territories[t]
		if ts.Owner == seatIdx {
			territories++
			armies += ts.Armies
		}
	}
	return territories, armies
}
