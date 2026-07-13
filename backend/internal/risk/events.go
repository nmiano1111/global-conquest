package risk

const (
	// EventTypeCombatRollResolved is the DomainEvent.Type value for a
	// resolved combat roll.
	EventTypeCombatRollResolved = "combat_roll_resolved"
	// EventVersionCombatRollResolved is the DomainEvent.Version value for a
	// resolved combat roll.
	EventVersionCombatRollResolved int16 = 1
	// SchemaVersionCombatRollResolved is the
	// CombatRollResolvedPayload.SchemaVersion value for the current payload
	// shape.
	SchemaVersionCombatRollResolved = 1
)

// DieComparison records a single attacker-vs-defender die pair and who lost.
// Risk ties are awarded to the defender, so the attacker loses on a tie.
type DieComparison struct {
	// AttackerDie is the attacker's die value in this comparison.
	AttackerDie int `json:"attacker_die"`
	// DefenderDie is the defender's die value in this comparison.
	DefenderDie int    `json:"defender_die"`
	Loser       string `json:"loser"` // "attacker" or "defender"
}

// CombatRollResolvedPayload is the typed payload for a combat_roll_resolved event.
type CombatRollResolvedPayload struct {
	// SchemaVersion identifies the shape of this payload for consumers that
	// persist or replay events.
	SchemaVersion int `json:"schema_version"`
	// TurnNumber is the Game.TurnNumber at the time of the attack.
	TurnNumber int `json:"turn_number"`
	// Phase is the game phase the attack occurred in, always PhaseAttack.
	Phase string `json:"phase"`
	// AttackerPlayerID is the ID of the player who initiated the attack.
	AttackerPlayerID string `json:"attacker_player_id"`
	// DefenderPlayerID is the ID of the player who owned the target territory.
	DefenderPlayerID string `json:"defender_player_id"`
	// SourceTerritoryID is the attacking territory.
	SourceTerritoryID string `json:"source_territory_id"`
	// TargetTerritoryID is the defending territory.
	TargetTerritoryID string `json:"target_territory_id"`
	// SourceArmiesBefore is the source territory's army count before this
	// attack round.
	SourceArmiesBefore int `json:"source_armies_before"`
	// TargetArmiesBefore is the target territory's army count before this
	// attack round.
	TargetArmiesBefore int `json:"target_armies_before"`
	// AttackerDice holds the attacker's rolled dice, sorted highest to lowest.
	AttackerDice []int `json:"attacker_dice"`
	// DefenderDice holds the defender's rolled dice, sorted highest to lowest.
	DefenderDice []int `json:"defender_dice"`
	// Comparisons pairs each attacker die with a defender die and records
	// who lost that pairing.
	Comparisons []DieComparison `json:"comparisons"`
	// AttackerLosses is the number of armies the attacker lost in this
	// attack round.
	AttackerLosses int `json:"attacker_losses"`
	// DefenderLosses is the number of armies the defender lost in this
	// attack round.
	DefenderLosses int `json:"defender_losses"`
	// SourceArmiesAfter is the source territory's army count after this
	// attack round.
	SourceArmiesAfter int `json:"source_armies_after"`
	// TargetArmiesAfter is the target territory's army count after this
	// attack round.
	TargetArmiesAfter int `json:"target_armies_after"`
	// TerritoryCaptured reports whether this attack round reduced the
	// target's armies to zero, capturing the territory.
	TerritoryCaptured bool `json:"territory_captured"`
}

// DomainEvent carries semantic event data produced by the engine.
// The application layer is responsible for persisting it.
type DomainEvent struct {
	// Type is the event's type string (see the EventType* constants).
	Type string
	// Version is the event schema version (see the EventVersion* constants).
	Version int16
	// ActorPlayerID is the ID of the player whose action produced this event.
	ActorPlayerID string
	// Payload is the event's typed data, e.g. CombatRollResolvedPayload.
	Payload any
}
