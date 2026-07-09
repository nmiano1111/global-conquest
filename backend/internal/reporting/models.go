package reporting

import (
	"time"

	"backend/internal/risk"
)

// CombatEvent is a decoded and validated combat_roll_resolved domain event row.
type CombatEvent struct {
	ID           string
	GameID       string
	GameSequence int64
	OccurredAt   time.Time

	AttackerPlayerID   string
	DefenderPlayerID   string
	SourceTerritoryID  string
	TargetTerritoryID  string
	SourceArmiesBefore int
	TargetArmiesBefore int
	AttackerDice       []int
	DefenderDice       []int
	Comparisons        []risk.DieComparison
	AttackerLosses     int
	DefenderLosses     int
	TerritoryCaptured  bool
}

// FaceDistribution counts how many times each die face (1–6) appeared.
type FaceDistribution struct {
	Counts map[int]int // keyed by face value 1–6
	Total  int
}

// DiceReport aggregates combat-dice statistics across all combat events for a game.
type DiceReport struct {
	GameID      string
	CombatRolls int
	Captures    int

	AttackerDice FaceDistribution
	DefenderDice FaceDistribution

	AttackerLosses int
	DefenderLosses int

	// SkippedEvents is the count of malformed rows excluded from the calculations.
	SkippedEvents int
}

// PlayerCombatReport aggregates a single player's attack statistics.
type PlayerCombatReport struct {
	GameID            string
	PlayerID          string
	PlayerDisplayName string

	AttackRolls             int
	TerritoriesCaptured     int
	AttackerDiceRolled      int
	AttackerLosses          int
	DefenderLossesInflicted int

	AverageAttackerDice       float64
	AverageSourceArmiesBefore float64
	AverageTargetArmiesBefore float64
	AverageArmyAdvantage      float64
	CaptureRate               float64 // percentage, e.g. 19.7
}

// PlayerChoice is a display-name / value pair used for Discord autocomplete.
// Value is the username, which ResolvePlayer can look up.
type PlayerChoice struct {
	Name  string // shown in the autocomplete dropdown
	Value string // username sent as the option value
}

// RecentCombatRoll is a single combat-roll event prepared for display.
type RecentCombatRoll struct {
	GameSequence int64
	OccurredAt   time.Time

	AttackerPlayerID    string
	DefenderPlayerID    string
	AttackerDisplayName string
	DefenderDisplayName string

	SourceTerritoryID string
	TargetTerritoryID string

	AttackerDice   []int
	DefenderDice   []int
	AttackerLosses int
	DefenderLosses int
	Captured       bool
}
