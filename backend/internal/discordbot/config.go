package discordbot

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	BotToken        string
	ApplicationID   string
	GuildID         string
	EventsChannelID string

	// DefaultGameID is the Global Conquest game UUID used by all report commands
	// (/last-rolls, /dice-report, /player-stats). Set via DISCORD_DEFAULT_GAME_ID.
	DefaultGameID string
}

func ConfigFromEnv() (Config, error) {
	cfg := Config{
		BotToken:        strings.TrimSpace(os.Getenv("DISCORD_BOT_TOKEN")),
		ApplicationID:   strings.TrimSpace(os.Getenv("DISCORD_APPLICATION_ID")),
		GuildID:         strings.TrimSpace(os.Getenv("DISCORD_GUILD_ID")),
		EventsChannelID: strings.TrimSpace(os.Getenv("DISCORD_EVENTS_CHANNEL_ID")),
		DefaultGameID:   strings.TrimSpace(os.Getenv("DISCORD_DEFAULT_GAME_ID")),
	}

	var missing []string
	if cfg.BotToken == "" {
		missing = append(missing, "DISCORD_BOT_TOKEN")
	}
	if cfg.ApplicationID == "" {
		missing = append(missing, "DISCORD_APPLICATION_ID")
	}
	if cfg.GuildID == "" {
		missing = append(missing, "DISCORD_GUILD_ID")
	}
	if cfg.EventsChannelID == "" {
		missing = append(missing, "DISCORD_EVENTS_CHANNEL_ID")
	}
	if cfg.DefaultGameID == "" {
		missing = append(missing, "DISCORD_DEFAULT_GAME_ID")
	}

	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return cfg, nil
}
