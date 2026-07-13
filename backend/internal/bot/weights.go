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

	// --- reinforce / setup_reinforce ---

	// ReinforceEnemyThreat scales the enemy armies already adjacent to a
	// candidate reinforcement territory.
	ReinforceEnemyThreat float64
	// ReinforceEnemyTerritoryCount scales the number of distinct adjacent
	// enemy-owned territories — a separate signal from summed threat: a
	// border facing 3 weak enemies is a bigger multi-front risk than 1
	// strong one with the same total armies.
	ReinforceEnemyTerritoryCount float64
	// ReinforceWeakness scales (adjacent enemy threat - current armies);
	// negative once a territory is already well-defended relative to its
	// threat, which is what discourages further reinforcing a strong spot
	// without a separate penalty term.
	ReinforceWeakness float64
	// ReinforceContinentValue scales continentReinforceValue: defending a
	// continent pi already fully owns, or pushing toward one pi is close
	// to completing.
	ReinforceContinentValue float64
	// ReinforceConcentrationPenalty scales the armies already present at
	// a candidate territory; applied as a cost, so it should stay
	// negative — discourages repeatedly stacking one spot turn after turn
	// once it's already strong.
	ReinforceConcentrationPenalty float64
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

	ReinforceEnemyThreat:          1.0,
	ReinforceEnemyTerritoryCount:  1.5,
	ReinforceWeakness:             1.0,
	ReinforceContinentValue:       2.0,
	ReinforceConcentrationPenalty: -0.3,
}
