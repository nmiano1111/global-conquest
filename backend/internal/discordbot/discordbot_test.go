package discordbot

import (
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

// --- ConfigFromEnv tests ---

func TestConfigFromEnv_AllPresent(t *testing.T) {
	t.Setenv("DISCORD_BOT_TOKEN", "tok")
	t.Setenv("DISCORD_APPLICATION_ID", "appid")
	t.Setenv("DISCORD_GUILD_ID", "guildid")
	t.Setenv("DISCORD_EVENTS_CHANNEL_ID", "chanid")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BotToken != "tok" || cfg.ApplicationID != "appid" ||
		cfg.GuildID != "guildid" || cfg.EventsChannelID != "chanid" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestConfigFromEnv_OneMissing(t *testing.T) {
	t.Setenv("DISCORD_BOT_TOKEN", "tok")
	t.Setenv("DISCORD_APPLICATION_ID", "appid")
	t.Setenv("DISCORD_GUILD_ID", "guildid")
	t.Setenv("DISCORD_EVENTS_CHANNEL_ID", "")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for missing DISCORD_EVENTS_CHANNEL_ID")
	}
	if !strings.Contains(err.Error(), "DISCORD_EVENTS_CHANNEL_ID") {
		t.Fatalf("error should name missing variable, got: %v", err)
	}
}

func TestConfigFromEnv_MultipleMissing(t *testing.T) {
	t.Setenv("DISCORD_BOT_TOKEN", "")
	t.Setenv("DISCORD_APPLICATION_ID", "")
	t.Setenv("DISCORD_GUILD_ID", "guildid")
	t.Setenv("DISCORD_EVENTS_CHANNEL_ID", "chanid")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for multiple missing variables")
	}
	if !strings.Contains(err.Error(), "DISCORD_BOT_TOKEN") {
		t.Fatalf("error should name DISCORD_BOT_TOKEN, got: %v", err)
	}
	if !strings.Contains(err.Error(), "DISCORD_APPLICATION_ID") {
		t.Fatalf("error should name DISCORD_APPLICATION_ID, got: %v", err)
	}
}

func TestConfigFromEnv_WhitespaceTrimmed(t *testing.T) {
	t.Setenv("DISCORD_BOT_TOKEN", "  tok  ")
	t.Setenv("DISCORD_APPLICATION_ID", "\tappid\t")
	t.Setenv("DISCORD_GUILD_ID", " guildid ")
	t.Setenv("DISCORD_EVENTS_CHANNEL_ID", " chanid ")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BotToken != "tok" {
		t.Errorf("BotToken not trimmed: %q", cfg.BotToken)
	}
	if cfg.ApplicationID != "appid" {
		t.Errorf("ApplicationID not trimmed: %q", cfg.ApplicationID)
	}
	if cfg.GuildID != "guildid" {
		t.Errorf("GuildID not trimmed: %q", cfg.GuildID)
	}
	if cfg.EventsChannelID != "chanid" {
		t.Errorf("EventsChannelID not trimmed: %q", cfg.EventsChannelID)
	}
}

func TestConfigFromEnv_ErrorDoesNotExposeToken(t *testing.T) {
	secret := "super-secret-bot-token"
	t.Setenv("DISCORD_BOT_TOKEN", secret)
	t.Setenv("DISCORD_APPLICATION_ID", "")
	t.Setenv("DISCORD_GUILD_ID", "")
	t.Setenv("DISCORD_EVENTS_CHANNEL_ID", "")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error message must not contain the bot token")
	}
}

// --- decidePingAction tests ---

func TestDecidePingAction_NoneExist_Create(t *testing.T) {
	dec := decidePingAction([]*discordgo.ApplicationCommand{
		{Name: "other", Description: "something else"},
	})
	if dec.action != commandCreate {
		t.Fatalf("expected commandCreate, got %v", dec.action)
	}
}

func TestDecidePingAction_EmptyList_Create(t *testing.T) {
	dec := decidePingAction(nil)
	if dec.action != commandCreate {
		t.Fatalf("expected commandCreate for empty list, got %v", dec.action)
	}
}

func TestDecidePingAction_MatchingExists_Reuse(t *testing.T) {
	dec := decidePingAction([]*discordgo.ApplicationCommand{
		{ID: "123", Name: pingCommandName, Description: pingCommandDescription},
	})
	if dec.action != commandReuse {
		t.Fatalf("expected commandReuse, got %v", dec.action)
	}
	if dec.existingID != "123" {
		t.Fatalf("expected existingID=123, got %q", dec.existingID)
	}
}

func TestDecidePingAction_MismatchedDescription_Update(t *testing.T) {
	dec := decidePingAction([]*discordgo.ApplicationCommand{
		{ID: "456", Name: pingCommandName, Description: "old description"},
	})
	if dec.action != commandUpdate {
		t.Fatalf("expected commandUpdate, got %v", dec.action)
	}
	if dec.existingID != "456" {
		t.Fatalf("expected existingID=456, got %q", dec.existingID)
	}
}
