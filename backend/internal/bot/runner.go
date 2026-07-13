package bot

import (
	"context"
	"errors"
	"log"
	"time"

	"backend/internal/game"
	"backend/internal/risk"
)

// maxRejectedCommandRetries bounds how many times the runner will reload
// state and retry after the engine rejects a bot command. Repeated
// rejection indicates a bug in the strategy or legal-action helpers, not a
// transient condition, so this must stay small.
const maxRejectedCommandRetries = 3

// GameLoader loads the current authoritative game state without locking or
// mutating it. Reads are non-locking because the engine re-validates every
// command transactionally when it is applied; a stale read only costs a
// wasted decision and a bounded retry, never a rule violation.
type GameLoader interface {
	LoadGame(ctx context.Context, gameID string) (g *risk.Game, status string, err error)
}

// ActionSubmitter submits a command through the same normal application
// command path human WebSocket clients use.
type ActionSubmitter interface {
	SubmitGameAction(ctx context.Context, in game.GameActionInput) (game.GameActionUpdate, error)
}

// StopReason explains why RunTurn returned, so a Manager can decide whether
// it is worth checking for a follow-on bot turn.
type StopReason string

const (
	// StopTurnEnded means a bot completed its turn; control passed to
	// someone else.
	StopTurnEnded StopReason = "turn_ended" // a bot completed its turn; control passed to someone else
	// StopNotBotTurn means the current player was never bot-controlled;
	// there was nothing for RunTurn to do.
	StopNotBotTurn StopReason = "not_bot_turn" // current player was never bot-controlled; nothing to do
	// StopGameOver means the game reached PhaseGameOver.
	StopGameOver StopReason = "game_over"
	// StopGameInactive means the game's status isn't in_progress, or its
	// state is otherwise unusable.
	StopGameInactive StopReason = "game_inactive" // status isn't in_progress, or state is otherwise unusable
	// StopLoadError means loading the authoritative game state failed.
	StopLoadError StopReason = "load_error"
	// StopStrategyError means the bot's strategy was unknown or returned an
	// error.
	StopStrategyError StopReason = "strategy_error"
	// StopMaxRetriesExceeded means the engine repeatedly rejected the bot's
	// command, indicating a likely bug rather than a transient condition.
	StopMaxRetriesExceeded StopReason = "max_retries_exceeded" // repeated engine rejection; likely a bug
	// StopCanceled means the run was stopped because its context was
	// canceled.
	StopCanceled StopReason = "canceled"
)

// TurnRunner drives one bot-controlled player's turn to completion, one
// committed command at a time, always against freshly loaded authoritative
// state. It stops as soon as the current player is no longer the bot it
// started with (see StopReason), leaving any hand-off to the next player to
// whoever triggers the next RunTurn call.
type TurnRunner interface {
	RunTurn(ctx context.Context, gameID string, mode ExecutionMode) (StopReason, error)
}

// Runner is the concrete TurnRunner implementation.
type Runner struct {
	loader     GameLoader
	submitter  ActionSubmitter
	strategies StrategyRegistry
	sleeper    Sleeper
	pacing     PacingConfig
}

// NewRunner creates a Runner that loads state via loader, submits commands
// via submitter, resolves strategies from strategies, and paces committed
// actions using sleeper and pacing (in ExecutionLive mode).
func NewRunner(loader GameLoader, submitter ActionSubmitter, strategies StrategyRegistry, sleeper Sleeper, pacing PacingConfig) *Runner {
	return &Runner{
		loader:     loader,
		submitter:  submitter,
		strategies: strategies,
		sleeper:    sleeper,
		pacing:     pacing,
	}
}

// pace sleeps for a duration sampled from [min, max] in live mode. It is a
// no-op in simulation mode and never sleeps inside a transaction — it is
// only ever called after a command has already committed and its result
// broadcast.
func (r *Runner) pace(ctx context.Context, mode ExecutionMode, min, max time.Duration) error {
	if mode != ExecutionLive {
		return nil
	}
	return r.sleeper.Sleep(ctx, randomDuration(min, max))
}

// RunTurn drives one bot-controlled player's turn to completion, one
// committed command at a time, always against freshly reloaded
// authoritative state. It stops as soon as the current player is no longer
// the bot it started with, returning a StopReason explaining why.
func (r *Runner) RunTurn(ctx context.Context, gameID string, mode ExecutionMode) (reason StopReason, err error) {
	var botPlayerID string
	first := true
	retries := 0
	lastAttackTarget := ""

	defer func() {
		if botPlayerID != "" {
			log.Printf("bot: runner stopped game_id=%s player_id=%s reason=%s err=%v", gameID, botPlayerID, reason, err)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return StopCanceled, nil
		default:
		}

		g, status, err := r.loader.LoadGame(ctx, gameID)
		if err != nil {
			log.Printf("bot: load failed game_id=%s err=%v", gameID, err)
			return StopLoadError, err
		}
		if status != "in_progress" {
			return StopGameInactive, nil
		}
		if g.Phase == risk.PhaseGameOver {
			return StopGameOver, nil
		}
		if g.CurrentPlayer < 0 || g.CurrentPlayer >= len(g.Players) {
			return StopGameInactive, nil
		}

		current := g.Players[g.CurrentPlayer]
		if !current.IsBot() {
			if first {
				return StopNotBotTurn, nil
			}
			return StopTurnEnded, nil
		}
		if first {
			botPlayerID = current.ID
			first = false
			log.Printf("bot: runner started game_id=%s player_id=%s strategy=%s phase=%s", gameID, botPlayerID, current.Strategy, g.Phase)
			if err := r.pace(ctx, mode, r.pacing.TurnStartMin, r.pacing.TurnStartMax); err != nil {
				return StopCanceled, nil
			}
		} else if current.ID != botPlayerID {
			return StopTurnEnded, nil
		}

		strat, ok := r.strategies.Get(current.Strategy)
		if !ok || strat == nil {
			log.Printf("bot: unknown strategy=%q game_id=%s player_id=%s", current.Strategy, gameID, botPlayerID)
			return StopStrategyError, errors.New("bot: unknown strategy " + current.Strategy)
		}

		cmd, expl, err := strat.NextCommand(ctx, g, botPlayerID)
		if err != nil {
			log.Printf("bot: strategy error game_id=%s player_id=%s phase=%s err=%v", gameID, botPlayerID, g.Phase, err)
			return StopStrategyError, err
		}

		update, err := r.submitter.SubmitGameAction(ctx, cmd.toGameActionInput(gameID, botPlayerID))
		if err != nil {
			if ctx.Err() != nil {
				return StopCanceled, nil
			}
			retries++
			log.Printf("bot: command rejected game_id=%s player_id=%s action=%s phase=%s retry=%d err=%v", gameID, botPlayerID, cmd.Action, g.Phase, retries, err)
			if retries > maxRejectedCommandRetries {
				return StopMaxRetriesExceeded, err
			}
			continue
		}
		retries = 0

		repeatTarget := cmd.Action == ActionAttack && cmd.To == lastAttackTarget
		if cmd.Action == ActionAttack {
			lastAttackTarget = cmd.To
		} else {
			lastAttackTarget = ""
		}
		decision := r.pacing.classifyAction(cmd, update, repeatTarget)
		log.Printf("bot: command committed game_id=%s player_id=%s action=%s phase=%s pace=%s %s", gameID, botPlayerID, cmd.Action, update.Phase, decision.category, expl)

		if err := r.pace(ctx, mode, decision.min, decision.max); err != nil {
			return StopCanceled, nil
		}

		if update.Phase == string(risk.PhaseGameOver) {
			return StopGameOver, nil
		}
	}
}
