package discordbot

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"backend/internal/store"

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

// --- renderMessage tests ---

func makeEntry(notifType string, payloadJSON string) store.DiscordOutboxEntry {
	return store.DiscordOutboxEntry{
		ID:               "outbox-1",
		GameID:           "game-1",
		NotificationType: notifType,
		Payload:          json.RawMessage(payloadJSON),
		AttemptCount:     1,
	}
}

func TestRenderTurnStarted(t *testing.T) {
	entry := makeEntry(store.NotificationTypeTurnStarted, `{
		"schema_version": 1,
		"previous_player_display_name": "Bob",
		"player_id": "player-uuid",
		"player_display_name": "Alice",
		"turn_number": 5
	}`)
	msg, err := renderMessage(entry)
	if err != nil {
		t.Fatalf("renderMessage: %v", err)
	}
	if msg != "@everyone 🎯 **Bob** ended their turn. **Alice** is up. (game `game-1`)" {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func TestRenderTurnStartedWithDiscordNames(t *testing.T) {
	entry := makeEntry(store.NotificationTypeTurnStarted, `{
		"schema_version": 1,
		"previous_player_display_name": "Bob",
		"previous_player_discord_name": "bobsmith",
		"player_id": "player-uuid",
		"player_display_name": "Alice",
		"player_discord_name": "alicewonder",
		"turn_number": 5
	}`)
	msg, err := renderMessage(entry)
	if err != nil {
		t.Fatalf("renderMessage: %v", err)
	}
	if msg != "🎯 **@bobsmith** ended their turn. **@alicewonder** is up. (game `game-1`)" {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func TestRenderTurnStartedOneDiscordNameMissing(t *testing.T) {
	entry := makeEntry(store.NotificationTypeTurnStarted, `{
		"schema_version": 1,
		"previous_player_display_name": "Bob",
		"previous_player_discord_name": "bobsmith",
		"player_id": "player-uuid",
		"player_display_name": "Alice",
		"turn_number": 5
	}`)
	msg, err := renderMessage(entry)
	if err != nil {
		t.Fatalf("renderMessage: %v", err)
	}
	if msg != "@everyone 🎯 **Bob** ended their turn. **Alice** is up. (game `game-1`)" {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func TestRenderTurnStartedMissingName(t *testing.T) {
	entry := makeEntry(store.NotificationTypeTurnStarted, `{
		"schema_version": 1,
		"player_id": "player-uuid",
		"player_display_name": ""
	}`)
	_, err := renderMessage(entry)
	if err == nil {
		t.Fatal("expected error for missing player_display_name")
	}
}

func TestRenderMalformedPayload(t *testing.T) {
	entry := makeEntry(store.NotificationTypeTurnStarted, `not-valid-json`)
	_, err := renderMessage(entry)
	if err == nil {
		t.Fatal("expected error for malformed payload")
	}
}

func TestRenderUnknownNotificationType(t *testing.T) {
	entry := makeEntry("player_eliminated", `{"schema_version":1}`)
	_, err := renderMessage(entry)
	if err == nil {
		t.Fatal("expected error for unknown notification type")
	}
	if !strings.Contains(err.Error(), "unknown notification type") {
		t.Fatalf("error should mention unknown type: %v", err)
	}
}

// --- Worker delivery tests ---

type fakeOutboxClaimer struct {
	claimFn   func(ctx context.Context, limit int) ([]store.DiscordOutboxEntry, error)
	deliverFn func(ctx context.Context, id string) error
	failFn    func(ctx context.Context, id string, attempt int, errMsg string) error
}

func (f *fakeOutboxClaimer) ClaimPendingBatch(ctx context.Context, limit int) ([]store.DiscordOutboxEntry, error) {
	if f.claimFn != nil {
		return f.claimFn(ctx, limit)
	}
	return nil, nil
}

func (f *fakeOutboxClaimer) MarkDelivered(ctx context.Context, id string) error {
	if f.deliverFn != nil {
		return f.deliverFn(ctx, id)
	}
	return nil
}

func (f *fakeOutboxClaimer) MarkFailed(ctx context.Context, id string, attempt int, errMsg string) error {
	if f.failFn != nil {
		return f.failFn(ctx, id, attempt, errMsg)
	}
	return nil
}

type fakeSender struct {
	sendFn func(ctx context.Context, channelID, content string) error
}

func (f *fakeSender) SendMessage(ctx context.Context, channelID, content string) error {
	if f.sendFn != nil {
		return f.sendFn(ctx, channelID, content)
	}
	return nil
}

func TestWorkerDeliverSuccess(t *testing.T) {
	entry := makeEntry(store.NotificationTypeTurnStarted, `{
		"schema_version": 1,
		"previous_player_display_name": "Alice",
		"player_id": "p1",
		"player_display_name": "Bob",
		"turn_number": 3
	}`)
	entry.AttemptCount = 1

	var sentContent string
	var markedDelivered string
	claimer := &fakeOutboxClaimer{
		deliverFn: func(_ context.Context, id string) error {
			markedDelivered = id
			return nil
		},
	}
	sender := &fakeSender{
		sendFn: func(_ context.Context, _, content string) error {
			sentContent = content
			return nil
		},
	}

	w := NewWorker(claimer, sender, "channel-id")
	w.deliver(context.Background(), entry)

	if sentContent != "@everyone 🎯 **Alice** ended their turn. **Bob** is up. (game `game-1`)" {
		t.Fatalf("unexpected message content: %q", sentContent)
	}
	if markedDelivered != "outbox-1" {
		t.Fatalf("expected MarkDelivered called with outbox-1, got %q", markedDelivered)
	}
}

func TestWorkerDeliverSendError(t *testing.T) {
	entry := makeEntry(store.NotificationTypeTurnStarted, `{
		"schema_version": 1,
		"previous_player_display_name": "Bob",
		"player_id": "p1",
		"player_display_name": "Carol",
		"turn_number": 2
	}`)
	entry.AttemptCount = 2

	var failedID string
	var failedAttempt int
	claimer := &fakeOutboxClaimer{
		deliverFn: func(_ context.Context, _ string) error {
			t.Error("MarkDelivered must not be called on send failure")
			return nil
		},
		failFn: func(_ context.Context, id string, attempt int, _ string) error {
			failedID = id
			failedAttempt = attempt
			return nil
		},
	}
	sender := &fakeSender{
		sendFn: func(context.Context, string, string) error {
			return errors.New("discord 429")
		},
	}

	w := NewWorker(claimer, sender, "channel-id")
	w.deliver(context.Background(), entry)

	if failedID != "outbox-1" {
		t.Fatalf("expected MarkFailed called with outbox-1, got %q", failedID)
	}
	if failedAttempt != 2 {
		t.Fatalf("expected attempt=2, got %d", failedAttempt)
	}
}

func TestWorkerDeliverMalformedPayload(t *testing.T) {
	entry := makeEntry(store.NotificationTypeTurnStarted, `not-json`)
	entry.AttemptCount = 1

	failCalled := false
	claimer := &fakeOutboxClaimer{
		failFn: func(_ context.Context, _ string, _ int, _ string) error {
			failCalled = true
			return nil
		},
	}
	sender := &fakeSender{
		sendFn: func(context.Context, string, string) error {
			t.Error("SendMessage must not be called for malformed payload")
			return nil
		},
	}

	w := NewWorker(claimer, sender, "channel-id")
	w.deliver(context.Background(), entry)

	if !failCalled {
		t.Fatal("expected MarkFailed called for malformed payload")
	}
}

func TestWorkerDeliverUnknownType(t *testing.T) {
	entry := makeEntry("cards_traded", `{"schema_version":1}`)
	entry.AttemptCount = 1

	failCalled := false
	claimer := &fakeOutboxClaimer{
		failFn: func(context.Context, string, int, string) error {
			failCalled = true
			return nil
		},
	}
	sender := &fakeSender{
		sendFn: func(context.Context, string, string) error {
			t.Error("SendMessage must not be called for unknown notification type")
			return nil
		},
	}

	w := NewWorker(claimer, sender, "channel-id")
	w.deliver(context.Background(), entry)

	if !failCalled {
		t.Fatal("expected MarkFailed called for unknown notification type")
	}
}
