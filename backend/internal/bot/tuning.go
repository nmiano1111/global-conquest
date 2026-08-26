package bot

import (
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// DefaultBotTuning returns killbot-v2's compiled-in tuning values -- what
// it does today, out of the box, before any admin override. This is also
// the row an admin tuning-config table is seeded with, and the fallback
// effectiveBotTuning uses for any game whose Game.BotTuning is nil (games
// that predate this feature, or were created via a path that never
// snapshots one, e.g. most simulation/tournament configs).
func DefaultBotTuning() risk.BotTuning {
	return risk.BotTuning{
		KillAdvantageRatio:        killbotV2KillAdvantageRatio,
		KillClusterMaxSize:        killTargetDefaultClusterMaxSize,
		MinAttackWinProbability:   killbotV2MinAttackWinProbability,
		HogwildArmiesPerTerritory: killbotV2HogWildArmiesPerTerritory,
		Pacing:                    toBotPacing(DefaultPacingConfig()),
	}
}

// effectiveBotTuning returns g.BotTuning if the game has one snapshotted,
// else DefaultBotTuning() -- see Game.BotTuning's doc comment
// (internal/risk/engine.go) for why a nil snapshot must fall back rather
// than error.
func effectiveBotTuning(g *risk.Game) risk.BotTuning {
	if g.BotTuning != nil {
		return *g.BotTuning
	}
	return DefaultBotTuning()
}

// toBotPacing converts a PacingConfig (time.Duration bounds) to its
// risk.BotPacing equivalent (whole-millisecond bounds, JSON/DB friendly) --
// the shape Game.BotTuning.Pacing is snapshotted and persisted in.
func toBotPacing(p PacingConfig) risk.BotPacing {
	ms := func(d time.Duration) int { return int(d / time.Millisecond) }
	return risk.BotPacing{
		TurnStartMinMs:     ms(p.TurnStartMin),
		TurnStartMaxMs:     ms(p.TurnStartMax),
		CardTurnInMinMs:    ms(p.CardTurnInMin),
		CardTurnInMaxMs:    ms(p.CardTurnInMax),
		ReinforcementMinMs: ms(p.ReinforcementMin),
		ReinforcementMaxMs: ms(p.ReinforcementMax),
		FirstAttackMinMs:   ms(p.FirstAttackMin),
		FirstAttackMaxMs:   ms(p.FirstAttackMax),
		RepeatAttackMinMs:  ms(p.RepeatAttackMin),
		RepeatAttackMaxMs:  ms(p.RepeatAttackMax),
		CaptureMinMs:       ms(p.CaptureMin),
		CaptureMaxMs:       ms(p.CaptureMax),
		OccupationMinMs:    ms(p.OccupationMin),
		OccupationMaxMs:    ms(p.OccupationMax),
		FortifyMinMs:       ms(p.FortifyMin),
		FortifyMaxMs:       ms(p.FortifyMax),
		EndAttackMinMs:     ms(p.EndAttackMin),
		EndAttackMaxMs:     ms(p.EndAttackMax),
		DramaticMinMs:      ms(p.DramaticMin),
		DramaticMaxMs:      ms(p.DramaticMax),
	}
}

// fromBotPacing is toBotPacing's inverse -- converts a snapshotted
// risk.BotPacing back into the time.Duration-based PacingConfig
// bot.Runner actually paces with.
func fromBotPacing(p risk.BotPacing) PacingConfig {
	d := func(ms int) time.Duration { return time.Duration(ms) * time.Millisecond }
	return PacingConfig{
		TurnStartMin:     d(p.TurnStartMinMs),
		TurnStartMax:     d(p.TurnStartMaxMs),
		CardTurnInMin:    d(p.CardTurnInMinMs),
		CardTurnInMax:    d(p.CardTurnInMaxMs),
		ReinforcementMin: d(p.ReinforcementMinMs),
		ReinforcementMax: d(p.ReinforcementMaxMs),
		FirstAttackMin:   d(p.FirstAttackMinMs),
		FirstAttackMax:   d(p.FirstAttackMaxMs),
		RepeatAttackMin:  d(p.RepeatAttackMinMs),
		RepeatAttackMax:  d(p.RepeatAttackMaxMs),
		CaptureMin:       d(p.CaptureMinMs),
		CaptureMax:       d(p.CaptureMaxMs),
		OccupationMin:    d(p.OccupationMinMs),
		OccupationMax:    d(p.OccupationMaxMs),
		FortifyMin:       d(p.FortifyMinMs),
		FortifyMax:       d(p.FortifyMaxMs),
		EndAttackMin:     d(p.EndAttackMinMs),
		EndAttackMax:     d(p.EndAttackMaxMs),
		DramaticMin:      d(p.DramaticMinMs),
		DramaticMax:      d(p.DramaticMaxMs),
	}
}
