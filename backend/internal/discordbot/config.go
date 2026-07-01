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
}

func ConfigFromEnv() (Config, error) {
	cfg := Config{
		BotToken:        strings.TrimSpace(os.Getenv("DISCORD_BOT_TOKEN")),
		ApplicationID:   strings.TrimSpace(os.Getenv("DISCORD_APPLICATION_ID")),
		GuildID:         strings.TrimSpace(os.Getenv("DISCORD_GUILD_ID")),
		EventsChannelID: strings.TrimSpace(os.Getenv("DISCORD_EVENTS_CHANNEL_ID")),
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

	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return cfg, nil
}
