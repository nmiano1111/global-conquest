package risk

// BotTuning holds every admin-tunable bot AI parameter, snapshotted onto
// Game.BotTuning at creation time (see that field's doc comment). Defined
// here, not in internal/bot (which actually reads/uses these values),
// because internal/bot already imports internal/risk -- the reverse
// import would cycle.
type BotTuning struct {
	// KillAdvantageRatio is the card-adjusted army advantage a bot must
	// have over the weakest reachable rival before committing to hunt
	// them down (internal/bot's killTarget). Lux's own Vulture AI, and
	// this project's killbot-v1, use 2.0 unconditionally.
	KillAdvantageRatio float64 `json:"kill_advantage_ratio"`
	// KillClusterMaxSize caps how large (in territory count) a kill
	// target's connected landmass may be for a bot to commit to
	// eliminating them outright (internal/bot's
	// targetHasSingleReachableCluster). Lux's own Vulture AI, and this
	// project's killbot-v1, use 20.
	KillClusterMaxSize int `json:"kill_cluster_max_size"`
	// MinAttackWinProbability is the minimum risk.ForecastAttack win
	// probability a bot requires before taking an attack it's evaluating
	// with real combat-odds math rather than a raw army-count comparison.
	MinAttackWinProbability float64 `json:"min_attack_win_probability"`
	// HogwildArmiesPerTerritory is the per-territory army threshold (armies
	// owned divided across the board's territory count) past which a bot
	// broadens its attack scan to every owned territory regardless of
	// individual matchup odds.
	HogwildArmiesPerTerritory int `json:"hogwild_armies_per_territory"`
	// Pacing bounds the random delay applied after each bot action in a
	// live (human-involved) game -- see internal/bot's PacingConfig,
	// which this mirrors field-for-field in whole milliseconds (JSON/DB
	// friendly; internal/bot converts to time.Duration). Ignored entirely
	// for a game with no human players (internal/bot.Runner always runs
	// those at full simulation speed regardless of this config).
	Pacing BotPacing `json:"pacing"`
}

// BotPacing mirrors internal/bot.PacingConfig's shape in whole
// milliseconds rather than time.Duration, so it round-trips cleanly
// through JSON and a Postgres jsonb column without a custom
// (Un)MarshalJSON. See PacingConfig's own field docs (internal/bot/pacing.go)
// for what each pair of bounds governs.
type BotPacing struct {
	TurnStartMinMs     int `json:"turn_start_min_ms"`
	TurnStartMaxMs     int `json:"turn_start_max_ms"`
	CardTurnInMinMs    int `json:"card_turn_in_min_ms"`
	CardTurnInMaxMs    int `json:"card_turn_in_max_ms"`
	ReinforcementMinMs int `json:"reinforcement_min_ms"`
	ReinforcementMaxMs int `json:"reinforcement_max_ms"`
	FirstAttackMinMs   int `json:"first_attack_min_ms"`
	FirstAttackMaxMs   int `json:"first_attack_max_ms"`
	RepeatAttackMinMs  int `json:"repeat_attack_min_ms"`
	RepeatAttackMaxMs  int `json:"repeat_attack_max_ms"`
	CaptureMinMs       int `json:"capture_min_ms"`
	CaptureMaxMs       int `json:"capture_max_ms"`
	OccupationMinMs    int `json:"occupation_min_ms"`
	OccupationMaxMs    int `json:"occupation_max_ms"`
	FortifyMinMs       int `json:"fortify_min_ms"`
	FortifyMaxMs       int `json:"fortify_max_ms"`
	EndAttackMinMs     int `json:"end_attack_min_ms"`
	EndAttackMaxMs     int `json:"end_attack_max_ms"`
	DramaticMinMs      int `json:"dramatic_min_ms"`
	DramaticMaxMs      int `json:"dramatic_max_ms"`
}
