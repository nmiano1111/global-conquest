package bot

import (
	"context"
	"log"
	"sync"
)

// Manager ensures at most one runner is active per game, and chains into
// the next bot's turn (if any) once a runner finishes, without ever
// busy-looping on a human's turn: it only re-triggers itself when the
// previous runner reports that a bot turn actually ended, not on every
// no-op check.
//
// For the current single-instance architecture, this in-memory registry is
// sufficient; a multi-instance deployment would need a distributed lock.
type Manager struct {
	mu     sync.Mutex
	active map[string]struct{}

	baseCtx context.Context
	cancel  context.CancelFunc
	runner  TurnRunner
	mode    ExecutionMode
}

func NewManager(parent context.Context, runner TurnRunner, mode ExecutionMode) *Manager {
	ctx, cancel := context.WithCancel(parent)
	return &Manager{
		active:  make(map[string]struct{}),
		baseCtx: ctx,
		cancel:  cancel,
		runner:  runner,
		mode:    mode,
	}
}

// Trigger ensures a runner is running for gameID. It is safe to call
// repeatedly and from any goroutine (including from inside game.Server's
// Run loop, since it never blocks). If a runner is already active for this
// game, the call is a suppressed no-op.
func (m *Manager) Trigger(gameID string) {
	m.mu.Lock()
	if _, exists := m.active[gameID]; exists {
		m.mu.Unlock()
		log.Printf("bot: duplicate runner trigger suppressed game_id=%s", gameID)
		return
	}
	m.active[gameID] = struct{}{}
	m.mu.Unlock()

	go m.run(gameID)
}

func (m *Manager) run(gameID string) {
	reason, err := m.runner.RunTurn(m.baseCtx, gameID, m.mode)

	m.mu.Lock()
	delete(m.active, gameID)
	m.mu.Unlock()

	if err != nil {
		log.Printf("bot: runner error game_id=%s reason=%s err=%v", gameID, reason, err)
	}

	// Only chain when a bot's turn genuinely ended and control passed to a
	// new current player; every other reason (not a bot's turn, game over,
	// canceled, strategy/engine failure) must not trigger another spawn, or
	// a human-controlled game would busy-loop forever re-checking itself.
	if err == nil && reason == StopTurnEnded {
		m.Trigger(gameID)
	}
}

// Shutdown cancels every runner started by this Manager.
func (m *Manager) Shutdown() {
	m.cancel()
}
