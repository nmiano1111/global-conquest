package reporting

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// reportingRepository is the data-access interface the Service requires.
// *Repository satisfies it; test fakes can satisfy it without a real DB.
type reportingRepository interface {
	LoadLatestGame(ctx context.Context) (gameID, gameName string, err error)
	LoadGameByName(ctx context.Context, name string) (gameID, gameName string, err error)
	LoadPlayerByUsername(ctx context.Context, username string) (playerID string, err error)
	LoadCurrentPlayer(ctx context.Context, gameID string) (username string, discordName *string, err error)
	LoadRawCombatEvents(ctx context.Context, gameID string) ([]rawCombatRow, error)
	LoadRawRecentCombatEvents(ctx context.Context, gameID string, limit int) ([]rawCombatRow, error)
	LoadPlayerNames(ctx context.Context, playerIDs []string) (map[string]string, error)
}

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Service orchestrates repository calls and statistical calculations.
// It is the entry point for all Discord report commands.
type Service struct {
	repo reportingRepository
}

// NewService creates a Service. Pass reporting.NewRepository(db.Queryer()) from main.
func NewService(repo reportingRepository) *Service {
	return &Service{repo: repo}
}

// ResolveGame returns the game ID and canonical name for the given game.
// If name is empty the most recently updated non-lobby game is used.
// Returns ErrNoActiveGame or ErrGameNotFound when no match exists.
func (svc *Service) ResolveGame(ctx context.Context, name string) (string, string, error) {
	if name == "" {
		return svc.repo.LoadLatestGame(ctx)
	}
	return svc.repo.LoadGameByName(ctx, name)
}

// ResolvePlayer maps a player identifier (UUID or username) to a player UUID.
// Returns ErrPlayerNotFound when a username lookup finds no match.
func (svc *Service) ResolvePlayer(ctx context.Context, identifier string) (string, error) {
	if uuidPattern.MatchString(identifier) {
		return strings.ToLower(identifier), nil
	}
	return svc.repo.LoadPlayerByUsername(ctx, identifier)
}

// CurrentPlayer returns the username and optional Discord name of the player
// whose turn it currently is in the given game.
func (svc *Service) CurrentPlayer(ctx context.Context, gameID string) (username string, discordName *string, err error) {
	return svc.repo.LoadCurrentPlayer(ctx, gameID)
}

// DiceReport builds an aggregate dice-statistics report for the given game.
// Returns ErrNoEvents when the game has no combat_roll_resolved events.
func (svc *Service) DiceReport(ctx context.Context, gameID string) (DiceReport, error) {
	raw, err := svc.repo.LoadRawCombatEvents(ctx, gameID)
	if err != nil {
		return DiceReport{}, fmt.Errorf("load combat events: %w", err)
	}
	if len(raw) == 0 {
		return DiceReport{}, ErrNoEvents
	}
	events, skipped, err := decodeAll(raw)
	if err != nil {
		return DiceReport{}, err
	}
	return BuildDiceReport(gameID, events, skipped), nil
}

// PlayerReport builds attack statistics for a single player in the given game.
// Returns a zero-attack report (not an error) when the player has no events.
func (svc *Service) PlayerReport(ctx context.Context, gameID, playerID string) (PlayerCombatReport, error) {
	raw, err := svc.repo.LoadRawCombatEvents(ctx, gameID)
	if err != nil {
		return PlayerCombatReport{}, fmt.Errorf("load combat events: %w", err)
	}
	events, _, err := decodeAll(raw)
	if err != nil {
		return PlayerCombatReport{}, err
	}
	ids := uniquePlayerIDs(events)
	// Always include the requested playerID so the name lookup works even if
	// they appear only as a defender or have no events at all.
	ids = appendUnique(ids, playerID)
	names, err := svc.repo.LoadPlayerNames(ctx, ids)
	if err != nil {
		return PlayerCombatReport{}, fmt.Errorf("load player names: %w", err)
	}
	return BuildPlayerReport(gameID, playerID, events, names), nil
}

// RecentRolls returns up to count recent combat rolls in chronological display
// order (oldest → newest). Returns ErrNoEvents when the game has no combat events.
func (svc *Service) RecentRolls(ctx context.Context, gameID string, count int) ([]RecentCombatRoll, error) {
	raw, err := svc.repo.LoadRawRecentCombatEvents(ctx, gameID, count)
	if err != nil {
		return nil, fmt.Errorf("load recent combat events: %w", err)
	}
	if len(raw) == 0 {
		return nil, ErrNoEvents
	}
	// The repository returns DESC order (most-recent first); reverse for display.
	for i, j := 0, len(raw)-1; i < j; i, j = i+1, j-1 {
		raw[i], raw[j] = raw[j], raw[i]
	}
	events, _, err := decodeAll(raw)
	if err != nil {
		return nil, err
	}
	ids := uniquePlayerIDs(events)
	names, err := svc.repo.LoadPlayerNames(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("load player names: %w", err)
	}
	return BuildRecentRolls(events, names), nil
}

func appendUnique(ids []string, id string) []string {
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}
