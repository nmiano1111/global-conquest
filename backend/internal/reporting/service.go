package reporting

import (
	"context"
	"fmt"
)

// reportingRepository is the data-access interface the Service requires.
// *Repository satisfies it; test fakes can satisfy it without a real DB.
type reportingRepository interface {
	LoadRawCombatEvents(ctx context.Context, gameID string) ([]rawCombatRow, error)
	LoadRawRecentCombatEvents(ctx context.Context, gameID string, limit int) ([]rawCombatRow, error)
	LoadPlayerNames(ctx context.Context, playerIDs []string) (map[string]string, error)
}

// Service orchestrates repository calls and statistical calculations.
// It is the entry point for all Discord report commands.
type Service struct {
	repo reportingRepository
}

// NewService creates a Service. Pass reporting.NewRepository(db.Queryer()) from main.
func NewService(repo reportingRepository) *Service {
	return &Service{repo: repo}
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
