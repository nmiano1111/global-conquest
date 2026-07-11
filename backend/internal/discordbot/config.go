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
	// FrontendBaseURL is optional. When set, notifications link the game name
	// to <FrontendBaseURL>/app/game/<id>. When unset, notifications fall back
	// to showing the game name as plain text.
	FrontendBaseURL string

	// Trello credentials for the /bug and /feature commands. A single
	// account-level API key + token pair — no per-user OAuth. Never logged.
	TrelloAPIKey         string
	TrelloToken          string
	TrelloTriageListID   string
	TrelloBugLabelID     string
	TrelloFeatureLabelID string
}

func ConfigFromEnv() (Config, error) {
	cfg := Config{
		BotToken:        strings.TrimSpace(os.Getenv("DISCORD_BOT_TOKEN")),
		ApplicationID:   strings.TrimSpace(os.Getenv("DISCORD_APPLICATION_ID")),
		GuildID:         strings.TrimSpace(os.Getenv("DISCORD_GUILD_ID")),
		EventsChannelID: strings.TrimSpace(os.Getenv("DISCORD_EVENTS_CHANNEL_ID")),
		FrontendBaseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("FRONTEND_BASE_URL")), "/"),

		TrelloAPIKey:         strings.TrimSpace(os.Getenv("TRELLO_API_KEY")),
		TrelloToken:          strings.TrimSpace(os.Getenv("TRELLO_TOKEN")),
		TrelloTriageListID:   strings.TrimSpace(os.Getenv("TRELLO_TRIAGE_LIST_ID")),
		TrelloBugLabelID:     strings.TrimSpace(os.Getenv("TRELLO_BUG_LABEL_ID")),
		TrelloFeatureLabelID: strings.TrimSpace(os.Getenv("TRELLO_FEATURE_LABEL_ID")),
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
	if cfg.TrelloAPIKey == "" {
		missing = append(missing, "TRELLO_API_KEY")
	}
	if cfg.TrelloToken == "" {
		missing = append(missing, "TRELLO_TOKEN")
	}
	if cfg.TrelloTriageListID == "" {
		missing = append(missing, "TRELLO_TRIAGE_LIST_ID")
	}
	if cfg.TrelloBugLabelID == "" {
		missing = append(missing, "TRELLO_BUG_LABEL_ID")
	}
	if cfg.TrelloFeatureLabelID == "" {
		missing = append(missing, "TRELLO_FEATURE_LABEL_ID")
	}

	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return cfg, nil
}
