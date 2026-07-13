package bot

import (
	"math/rand"
	"time"

	"backend/internal/game"
	"backend/internal/risk"
)

// PaceCategory is a coarse, loggable significance label for a committed
// bot action. The runner picks the actual delay from PacingConfig based on
// the finer-grained situation (e.g. "first attack" vs "repeat attack" both
// log as PaceNormal but use different ranges); the category exists purely
// for observability, not as the thing that determines the sleep duration.
type PaceCategory string

const (
	// PaceMinor labels a low-significance action such as placing
	// reinforcements or occupying after a conquest.
	PaceMinor PaceCategory = "minor"
	// PaceNormal labels an ordinary action such as an attack that doesn't
	// conquer, or a fortify.
	PaceNormal PaceCategory = "normal"
	// PaceSignificant labels a noteworthy action such as a card trade-in or
	// a successful conquest.
	PaceSignificant PaceCategory = "significant"
	// PaceDramatic labels a high-stakes action such as a player elimination
	// or the game ending.
	PaceDramatic PaceCategory = "dramatic"
)

// PacingConfig holds the bounded random delay range applied after each
// distinct kind of committed bot action in live mode. Ranges are sampled
// uniformly rather than using an identical fixed pause, so turns don't feel
// mechanical.
type PacingConfig struct {
	// TurnStartMin and TurnStartMax bound the delay before a bot's first
	// action of its turn.
	TurnStartMin, TurnStartMax time.Duration
	// CardTurnInMin and CardTurnInMax bound the delay after trading in a
	// card set.
	CardTurnInMin, CardTurnInMax time.Duration
	// ReinforcementMin and ReinforcementMax bound the delay after placing
	// reinforcements.
	ReinforcementMin, ReinforcementMax time.Duration
	// FirstAttackMin and FirstAttackMax bound the delay after attacking a
	// target for the first time this turn.
	FirstAttackMin, FirstAttackMax time.Duration
	// RepeatAttackMin and RepeatAttackMax bound the delay after attacking
	// the same target again.
	RepeatAttackMin, RepeatAttackMax time.Duration
	// CaptureMin and CaptureMax bound the delay after an attack conquers a
	// territory.
	CaptureMin, CaptureMax time.Duration
	// OccupationMin and OccupationMax bound the delay after the occupy
	// move following a conquest.
	OccupationMin, OccupationMax time.Duration
	// FortifyMin and FortifyMax bound the delay after a fortify action.
	FortifyMin, FortifyMax time.Duration
	// EndAttackMin and EndAttackMax bound the delay after ending the attack
	// phase.
	EndAttackMin, EndAttackMax time.Duration
	// DramaticMin and DramaticMax bound the delay after a high-stakes event
	// such as a player elimination or the game ending.
	DramaticMin, DramaticMax time.Duration
}

// DefaultPacingConfig returns the live pacing ranges. Roughly 1.5x the
// original first-pass values across the board (per user feedback that bot
// turns felt a little too quick), keeping the same relative proportions.
func DefaultPacingConfig() PacingConfig {
	return PacingConfig{
		TurnStartMin:     1200 * time.Millisecond,
		TurnStartMax:     2200 * time.Millisecond,
		CardTurnInMin:    1500 * time.Millisecond,
		CardTurnInMax:    2700 * time.Millisecond,
		ReinforcementMin: 600 * time.Millisecond,
		ReinforcementMax: 1200 * time.Millisecond,
		FirstAttackMin:   1000 * time.Millisecond,
		FirstAttackMax:   1800 * time.Millisecond,
		RepeatAttackMin:  500 * time.Millisecond,
		RepeatAttackMax:  1000 * time.Millisecond,
		CaptureMin:       1500 * time.Millisecond,
		CaptureMax:       2700 * time.Millisecond,
		OccupationMin:    750 * time.Millisecond,
		OccupationMax:    1500 * time.Millisecond,
		FortifyMin:       1000 * time.Millisecond,
		FortifyMax:       2000 * time.Millisecond,
		// end_attack is a phase transition with no visible board change;
		// end_turn intentionally has no extra pacing here — the pause that
		// matters happens before the *next* bot's first action (TurnStart),
		// not after the current player relinquishes control.
		EndAttackMin: 450 * time.Millisecond,
		EndAttackMax: 900 * time.Millisecond,
		DramaticMin:  2200 * time.Millisecond,
		DramaticMax:  3500 * time.Millisecond,
	}
}

type paceDecision struct {
	category PaceCategory
	min, max time.Duration
}

// classifyAction determines the pacing for a just-committed command. It
// relies on the typed Result the engine already produced (risk.AttackResult
// for attacks) and the resulting Phase, rather than diffing arbitrary JSON —
// both are already available on game.GameActionUpdate without any new
// domain event type.
func (cfg PacingConfig) classifyAction(cmd Command, update game.GameActionUpdate, repeatTarget bool) paceDecision {
	if update.Phase == string(risk.PhaseGameOver) {
		return paceDecision{PaceDramatic, cfg.DramaticMin, cfg.DramaticMax}
	}
	switch cmd.Action {
	case ActionAttack:
		if ar, ok := update.Result.(risk.AttackResult); ok {
			if ar.Eliminated != "" {
				return paceDecision{PaceDramatic, cfg.DramaticMin, cfg.DramaticMax}
			}
			if ar.Conquered {
				return paceDecision{PaceSignificant, cfg.CaptureMin, cfg.CaptureMax}
			}
		}
		if repeatTarget {
			return paceDecision{PaceNormal, cfg.RepeatAttackMin, cfg.RepeatAttackMax}
		}
		return paceDecision{PaceNormal, cfg.FirstAttackMin, cfg.FirstAttackMax}
	case ActionTradeCards:
		return paceDecision{PaceSignificant, cfg.CardTurnInMin, cfg.CardTurnInMax}
	case ActionPlaceReinforcement, ActionPlaceInitialArmy:
		return paceDecision{PaceMinor, cfg.ReinforcementMin, cfg.ReinforcementMax}
	case ActionOccupy:
		return paceDecision{PaceMinor, cfg.OccupationMin, cfg.OccupationMax}
	case ActionFortify:
		return paceDecision{PaceNormal, cfg.FortifyMin, cfg.FortifyMax}
	case ActionEndAttack:
		return paceDecision{PaceMinor, cfg.EndAttackMin, cfg.EndAttackMax}
	default: // end_turn and anything else: no extra pacing
		return paceDecision{PaceMinor, 0, 0}
	}
}

// randomDuration samples uniformly from [min, max]. Jitter itself is not
// worth making deterministic/injectable — tests only assert the resulting
// delay falls within the expected bounds for a given situation.
func randomDuration(min, max time.Duration) time.Duration {
	if max <= min {
		return min
	}
	delta := int64(max - min)
	return min + time.Duration(rand.Int63n(delta+1))
}
