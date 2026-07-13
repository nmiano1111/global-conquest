package reporting

import "fmt"

// ErrNoEvents is returned when a game has no combat_roll_resolved events.
var ErrNoEvents = fmt.Errorf("no combat events found")

// ErrNoActiveGame is returned when no in-progress or completed game exists.
var ErrNoActiveGame = fmt.Errorf("no active game found")

// ErrGameNotFound is returned when a game name lookup yields no match.
var ErrGameNotFound = fmt.Errorf("game not found")

// ErrPlayerNotFound is returned when a username lookup yields no match.
var ErrPlayerNotFound = fmt.Errorf("player not found")

// ErrNoCurrentPlayer is returned when a game has no resolvable current player.
var ErrNoCurrentPlayer = fmt.Errorf("no current player found")

// ErrUnsupportedEventVersion is returned when an event's event_version is not
// supported. Reports fail hard rather than skip: an unknown version cannot be
// safely decoded and silently ignoring it would produce misleading statistics.
type ErrUnsupportedEventVersion struct {
	// GameSequence is the game-relative sequence number of the offending event.
	GameSequence int64
	// EventVersion is the unsupported event_version value encountered on the event.
	EventVersion int16
}

// Error implements the error interface for ErrUnsupportedEventVersion.
func (e ErrUnsupportedEventVersion) Error() string {
	return fmt.Sprintf(
		"unsupported event_version %d at game_sequence %d (only version 1 is supported)",
		e.EventVersion, e.GameSequence,
	)
}

// ErrUnsupportedSchemaVersion is returned when a payload's schema_version is not
// supported. Same hard-fail policy as ErrUnsupportedEventVersion.
type ErrUnsupportedSchemaVersion struct {
	// GameSequence is the game-relative sequence number of the offending event.
	GameSequence int64
	// SchemaVersion is the unsupported schema_version value encountered in the event's payload.
	SchemaVersion int
}

// Error implements the error interface for ErrUnsupportedSchemaVersion.
func (e ErrUnsupportedSchemaVersion) Error() string {
	return fmt.Sprintf(
		"unsupported schema_version %d at game_sequence %d (only version 1 is supported)",
		e.SchemaVersion, e.GameSequence,
	)
}
