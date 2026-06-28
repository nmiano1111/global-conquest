package risk

const (
	EventTypeCombatRollResolved         = "combat_roll_resolved"
	EventVersionCombatRollResolved int16 = 1
	SchemaVersionCombatRollResolved     = 1
)

// DieComparison records a single attacker-vs-defender die pair and who lost.
// Risk ties are awarded to the defender, so the attacker loses on a tie.
type DieComparison struct {
	AttackerDie int    `json:"attacker_die"`
	DefenderDie int    `json:"defender_die"`
	Loser       string `json:"loser"` // "attacker" or "defender"
}

// CombatRollResolvedPayload is the typed payload for a combat_roll_resolved event.
type CombatRollResolvedPayload struct {
	SchemaVersion      int            `json:"schema_version"`
	TurnNumber         int            `json:"turn_number"`
	Phase              string         `json:"phase"`
	AttackerPlayerID   string         `json:"attacker_player_id"`
	DefenderPlayerID   string         `json:"defender_player_id"`
	SourceTerritoryID  string         `json:"source_territory_id"`
	TargetTerritoryID  string         `json:"target_territory_id"`
	SourceArmiesBefore int            `json:"source_armies_before"`
	TargetArmiesBefore int            `json:"target_armies_before"`
	AttackerDice       []int          `json:"attacker_dice"`
	DefenderDice       []int          `json:"defender_dice"`
	Comparisons        []DieComparison `json:"comparisons"`
	AttackerLosses     int            `json:"attacker_losses"`
	DefenderLosses     int            `json:"defender_losses"`
	SourceArmiesAfter  int            `json:"source_armies_after"`
	TargetArmiesAfter  int            `json:"target_armies_after"`
	TerritoryCaptured  bool           `json:"territory_captured"`
}

// DomainEvent carries semantic event data produced by the engine.
// The application layer is responsible for persisting it.
type DomainEvent struct {
	Type          string
	Version       int16
	ActorPlayerID string
	Payload       any
}
