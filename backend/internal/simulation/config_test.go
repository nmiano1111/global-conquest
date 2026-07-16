package simulation

import (
	"errors"
	"strings"
	"testing"

	"github.com/nmiano1111/global-conquest/backend/internal/bot"
)

func testRegistry() bot.StrategyRegistry {
	return bot.StrategyRegistry{
		bot.StrategyBasicV1:  bot.NewBasicStrategy(),
		bot.StrategyScoredV1: bot.NewScoredStrategy(bot.DefaultWeights),
	}
}

func validConfig() Config {
	return Config{
		Seed:       1,
		Strategies: []string{bot.StrategyBasicV1, bot.StrategyScoredV1, bot.StrategyScoredV1},
		GameMode:   GameModeAutoStart,
		Trace:      TraceSummary,
		Limits:     DefaultLimits(),
	}
}

func TestConfigPlayerCountMatchesStrategies(t *testing.T) {
	c := validConfig()
	if c.PlayerCount() != 3 {
		t.Fatalf("expected player count 3, got %d", c.PlayerCount())
	}
}

func TestSeatPlayerIDDeterministic(t *testing.T) {
	if SeatPlayerID(0) != "p0" || SeatPlayerID(5) != "p5" {
		t.Fatalf("expected deterministic p<seat> IDs, got %q and %q", SeatPlayerID(0), SeatPlayerID(5))
	}
}

func TestConfigPlayerIDsOrdered(t *testing.T) {
	c := validConfig()
	ids := c.PlayerIDs()
	want := []string{"p0", "p1", "p2"}
	if len(ids) != len(want) {
		t.Fatalf("expected %d ids, got %d", len(want), len(ids))
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("expected ids[%d] = %q, got %q", i, want[i], ids[i])
		}
	}
}

func TestConfigStrategyByPlayerID(t *testing.T) {
	c := validConfig()
	m := c.StrategyByPlayerID()
	want := map[string]string{
		"p0": bot.StrategyBasicV1,
		"p1": bot.StrategyScoredV1,
		"p2": bot.StrategyScoredV1,
	}
	if len(m) != len(want) {
		t.Fatalf("expected %d entries, got %d: %+v", len(want), len(m), m)
	}
	for id, strategyID := range want {
		if got := m[id]; got != strategyID {
			t.Errorf("expected %s -> %s, got %s", id, strategyID, got)
		}
	}
}

func TestConfigValidateAccepts(t *testing.T) {
	if err := validConfig().Validate(testRegistry()); err != nil {
		t.Fatalf("expected a valid config to pass, got %v", err)
	}
}

func TestConfigValidateRejectsTooFewPlayers(t *testing.T) {
	c := validConfig()
	c.Strategies = []string{bot.StrategyBasicV1, bot.StrategyBasicV1}
	err := c.Validate(testRegistry())
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig for 2 players, got %v", err)
	}
}

func TestConfigValidateRejectsTooManyPlayers(t *testing.T) {
	c := validConfig()
	c.Strategies = []string{
		bot.StrategyBasicV1, bot.StrategyBasicV1, bot.StrategyBasicV1,
		bot.StrategyBasicV1, bot.StrategyBasicV1, bot.StrategyBasicV1, bot.StrategyBasicV1,
	}
	err := c.Validate(testRegistry())
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig for 7 players, got %v", err)
	}
}

func TestConfigValidateRejectsUnknownStrategy(t *testing.T) {
	c := validConfig()
	c.Strategies[1] = "not-a-real-strategy"
	err := c.Validate(testRegistry())
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig for an unknown strategy, got %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "seat 1") {
		t.Fatalf("expected the error to name the offending seat, got %v", err)
	}
}

func TestConfigValidateRejectsUnknownTraceLevel(t *testing.T) {
	c := validConfig()
	c.Trace = "verbose"
	if err := c.Validate(testRegistry()); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig for an unknown trace level, got %v", err)
	}
}

func TestConfigValidateRejectsUnknownGameMode(t *testing.T) {
	c := validConfig()
	c.GameMode = "classic_claim"
	if err := c.Validate(testRegistry()); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig for an unknown game mode, got %v", err)
	}
}

func TestDefaultLimitsAreValid(t *testing.T) {
	if err := DefaultLimits().Validate(); err != nil {
		t.Fatalf("expected DefaultLimits to be valid, got %v", err)
	}
}

func TestLimitsValidateRejectsNonPositiveFields(t *testing.T) {
	base := DefaultLimits()

	cases := []struct {
		name   string
		mutate func(*Limits)
	}{
		{"MaxCommands", func(l *Limits) { l.MaxCommands = 0 }},
		{"MaxTurns", func(l *Limits) { l.MaxTurns = -1 }},
		{"MaxCommandsWithoutProgress", func(l *Limits) { l.MaxCommandsWithoutProgress = 0 }},
		{"MaxRepeatedStates", func(l *Limits) { l.MaxRepeatedStates = 0 }},
		{"MaxDuration", func(l *Limits) { l.MaxDuration = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := base
			tc.mutate(&l)
			if err := l.Validate(); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("expected ErrInvalidConfig when %s is non-positive, got %v", tc.name, err)
			}
		})
	}
}
