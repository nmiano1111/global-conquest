package discordbot

import (
	"fmt"
	"os"
	"strings"
)

// Config holds the environment-derived settings needed to run the Discord bot.
type Config struct {
	// BotToken is the Discord bot token used to authenticate the gateway session.
	BotToken string
	// ApplicationID is the Discord application ID slash commands are registered under.
	ApplicationID string
	// GuildID is the Discord guild (server) slash commands are registered to.
	GuildID string
	// EventsChannelID is the channel game-event notifications are posted to.
	EventsChannelID string
	// FrontendBaseURL is optional. When set, notifications link the game name
	// to <FrontendBaseURL>/app/game/<id>. When unset, notifications fall back
	// to showing the game name as plain text.
	FrontendBaseURL string

	// Trello credentials for the /bug and /feature commands. A single
	// account-level API key + token pair — no per-user OAuth. Never logged.
	TrelloAPIKey string
	// TrelloToken is the Trello API token paired with TrelloAPIKey.
	TrelloToken string
	// TrelloTriageListID is the Trello list new /bug and /feature cards are created in.
	TrelloTriageListID string
	// TrelloBugLabelID is the Trello label applied to cards created via /bug.
	TrelloBugLabelID string
	// TrelloFeatureLabelID is the Trello label applied to cards created via /feature.
	TrelloFeatureLabelID string
}

// ConfigFromEnv builds a Config from environment variables, returning an
// error listing the names of any required variable (every field except
// FrontendBaseURL) that is unset.
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
