package reporting

import (
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// CombatEvent is a decoded and validated combat_roll_resolved domain event row.
type CombatEvent struct {
	// ID is the unique identifier of the underlying event row.
	ID string
	// GameID identifies the game this combat event belongs to.
	GameID string
	// GameSequence is the game-relative, monotonically increasing sequence number of the event.
	GameSequence int64
	// OccurredAt is the timestamp at which the combat roll was resolved.
	OccurredAt time.Time

	// AttackerPlayerID is the ID of the player who initiated the attack.
	AttackerPlayerID string
	// DefenderPlayerID is the ID of the player defending the target territory.
	DefenderPlayerID string
	// SourceTerritoryID is the ID of the territory the attack was launched from.
	SourceTerritoryID string
	// TargetTerritoryID is the ID of the territory being attacked.
	TargetTerritoryID string
	// SourceArmiesBefore is the army count in the source territory before this roll.
	SourceArmiesBefore int
	// TargetArmiesBefore is the army count in the target territory before this roll.
	TargetArmiesBefore int
	// AttackerDice holds the face values rolled by the attacker.
	AttackerDice []int
	// DefenderDice holds the face values rolled by the defender.
	DefenderDice []int
	// Comparisons holds the per-die attacker-vs-defender outcome comparisons for this roll.
	Comparisons []risk.DieComparison
	// AttackerLosses is the number of armies the attacker lost in this roll.
	AttackerLosses int
	// DefenderLosses is the number of armies the defender lost in this roll.
	DefenderLosses int
	// TerritoryCaptured reports whether this roll resulted in the target territory being captured.
	TerritoryCaptured bool
}

// FaceDistribution counts how many times each die face (1–6) appeared.
type FaceDistribution struct {
	Counts map[int]int // keyed by face value 1–6
	// Total is the total number of dice counted across all faces.
	Total int
}

// DiceReport aggregates combat-dice statistics across all combat events for a game.
type DiceReport struct {
	// GameID identifies the game this report covers.
	GameID string
	// CombatRolls is the total number of combat roll events included in the report.
	CombatRolls int
	// Captures is the total number of territory captures across all combat events.
	Captures int

	// AttackerDice is the face-value distribution of all attacker dice rolled.
	AttackerDice FaceDistribution
	// DefenderDice is the face-value distribution of all defender dice rolled.
	DefenderDice FaceDistribution

	// AttackerLosses is the total number of armies lost by attackers across all events.
	AttackerLosses int
	// DefenderLosses is the total number of armies lost by defenders across all events.
	DefenderLosses int

	// SkippedEvents is the count of malformed rows excluded from the calculations.
	SkippedEvents int
}

// PlayerCombatReport aggregates a single player's attack statistics.
type PlayerCombatReport struct {
	// GameID identifies the game this report covers.
	GameID string
	// PlayerID is the ID of the attacking player this report covers.
	PlayerID string
	// PlayerDisplayName is the attacking player's display name.
	PlayerDisplayName string

	// AttackRolls is the total number of combat rolls this player initiated as attacker.
	AttackRolls int
	// TerritoriesCaptured is the number of territories this player captured.
	TerritoriesCaptured int
	// AttackerDiceRolled is the total number of individual dice this player rolled as attacker.
	AttackerDiceRolled int
	// AttackerLosses is the total number of armies this player lost while attacking.
	AttackerLosses int
	// DefenderLossesInflicted is the total number of defender armies destroyed by this player's attacks.
	DefenderLossesInflicted int

	// AverageAttackerDice is the mean number of dice rolled per attack.
	AverageAttackerDice float64
	// AverageSourceArmiesBefore is the mean army count in the attacking territory before each roll.
	AverageSourceArmiesBefore float64
	// AverageTargetArmiesBefore is the mean army count in the defending territory before each roll.
	AverageTargetArmiesBefore float64
	// AverageArmyAdvantage is AverageSourceArmiesBefore minus AverageTargetArmiesBefore.
	AverageArmyAdvantage float64
	CaptureRate          float64 // percentage, e.g. 19.7
}

// PlayerChoice is a display-name / value pair used for Discord autocomplete.
// Value is the username, which ResolvePlayer can look up.
type PlayerChoice struct {
	Name  string // shown in the autocomplete dropdown
	Value string // username sent as the option value
}

// RecentCombatRoll is a single combat-roll event prepared for display.
type RecentCombatRoll struct {
	// GameSequence is the game-relative sequence number of the roll.
	GameSequence int64
	// OccurredAt is the timestamp at which the roll was resolved.
	OccurredAt time.Time

	// AttackerPlayerID is the ID of the attacking player.
	AttackerPlayerID string
	// DefenderPlayerID is the ID of the defending player.
	DefenderPlayerID string
	// AttackerDisplayName is the attacking player's display name.
	AttackerDisplayName string
	// DefenderDisplayName is the defending player's display name.
	DefenderDisplayName string

	// SourceTerritoryID is the ID of the territory the attack was launched from.
	SourceTerritoryID string
	// TargetTerritoryID is the ID of the territory that was attacked.
	TargetTerritoryID string

	// AttackerDice holds the face values rolled by the attacker.
	AttackerDice []int
	// DefenderDice holds the face values rolled by the defender.
	DefenderDice []int
	// AttackerLosses is the number of armies the attacker lost in this roll.
	AttackerLosses int
	// DefenderLosses is the number of armies the defender lost in this roll.
	DefenderLosses int
	// Captured reports whether this roll resulted in the target territory being captured.
	Captured bool
}
