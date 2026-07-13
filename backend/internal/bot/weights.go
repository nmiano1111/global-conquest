package bot

// Weights holds every tunable coefficient a scored strategy's feature
// functions multiply against their raw signal. Difficulty and personality
// are both meant to be "construct a different Weights value" — no new
// branching logic — once that work begins; for now only DefaultWeights
// exists.
type Weights struct {
	// ArmyAdvantage scales (source armies - target armies).
	ArmyAdvantage float64
	// CaptureProbability scales the estimated probability of eventually
	// winning the attack (see ForecastAttack).
	CaptureProbability float64
	// ExpectedLossCost scales the estimated attacker armies lost fighting
	// to a conclusion; applied as a cost, so it should stay negative.
	ExpectedLossCost float64
	// CompletesContinent is added once, flat, when capturing the target
	// would complete a continent for the attacker.
	CompletesContinent float64
	// BreaksEnemyContinent is added once, flat, when capturing the target
	// would break the defender's currently-completed continent.
	BreaksEnemyContinent float64
	// CardOpportunity scales the value of securing this turn's
	// card-earning conquest, weighted by capture probability.
	CardOpportunity float64
	// EliminatesPlayer is added once, flat, when capturing the target
	// would eliminate the defending player.
	EliminatesPlayer float64
	// ExposurePenalty scales the enemy armies already adjacent to the
	// attacking source territory; applied as a cost, so it should stay
	// negative.
	ExposurePenalty float64
	// EndPhaseBias is added only to the synthetic "end this phase"
	// candidate — the lever aggression/difficulty tunes later.
	EndPhaseBias float64
}

// DefaultWeights are starting values, not derived from any formal tuning
// process: rough magnitudes matching the doc's own worked example
// (army advantage ~8, continent break ~6, card opportunity ~4, expected
// losses ~-2), refined by the accompanying tests' expected outcomes.
var DefaultWeights = Weights{
	ArmyAdvantage:        1.5,
	CaptureProbability:   10.0,
	ExpectedLossCost:     -1.5,
	CompletesContinent:   6.0,
	BreaksEnemyContinent: 4.0,
	CardOpportunity:      4.0,
	EliminatesPlayer:     8.0,
	ExposurePenalty:      -0.75,
	EndPhaseBias:         0.0,
}
