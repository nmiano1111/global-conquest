package discordbot

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"backend/internal/reporting"
	"backend/internal/store"
	"backend/internal/trello"

	"github.com/bwmarrin/discordgo"
)

// --- ConfigFromEnv tests ---

func TestConfigFromEnv_AllPresent(t *testing.T) {
	t.Setenv("DISCORD_BOT_TOKEN", "tok")
	t.Setenv("DISCORD_APPLICATION_ID", "appid")
	t.Setenv("DISCORD_GUILD_ID", "guildid")
	t.Setenv("DISCORD_EVENTS_CHANNEL_ID", "chanid")
	t.Setenv("TRELLO_API_KEY", "trellokey")
	t.Setenv("TRELLO_TOKEN", "trellotoken")
	t.Setenv("TRELLO_TRIAGE_LIST_ID", "listid")
	t.Setenv("TRELLO_BUG_LABEL_ID", "buglabelid")
	t.Setenv("TRELLO_FEATURE_LABEL_ID", "featurelabelid")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BotToken != "tok" || cfg.ApplicationID != "appid" ||
		cfg.GuildID != "guildid" || cfg.EventsChannelID != "chanid" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.TrelloAPIKey != "trellokey" || cfg.TrelloToken != "trellotoken" ||
		cfg.TrelloTriageListID != "listid" || cfg.TrelloBugLabelID != "buglabelid" ||
		cfg.TrelloFeatureLabelID != "featurelabelid" {
		t.Fatalf("unexpected trello config: %+v", cfg)
	}
}

func TestConfigFromEnv_TrelloVarMissing(t *testing.T) {
	t.Setenv("DISCORD_BOT_TOKEN", "tok")
	t.Setenv("DISCORD_APPLICATION_ID", "appid")
	t.Setenv("DISCORD_GUILD_ID", "guildid")
	t.Setenv("DISCORD_EVENTS_CHANNEL_ID", "chanid")
	t.Setenv("TRELLO_API_KEY", "trellokey")
	t.Setenv("TRELLO_TOKEN", "trellotoken")
	t.Setenv("TRELLO_TRIAGE_LIST_ID", "listid")
	t.Setenv("TRELLO_BUG_LABEL_ID", "buglabelid")
	t.Setenv("TRELLO_FEATURE_LABEL_ID", "")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for missing TRELLO_FEATURE_LABEL_ID")
	}
	if !strings.Contains(err.Error(), "TRELLO_FEATURE_LABEL_ID") {
		t.Fatalf("error should name missing variable, got: %v", err)
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
	t.Setenv("TRELLO_API_KEY", "trellokey")
	t.Setenv("TRELLO_TOKEN", "trellotoken")
	t.Setenv("TRELLO_TRIAGE_LIST_ID", "listid")
	t.Setenv("TRELLO_BUG_LABEL_ID", "buglabelid")
	t.Setenv("TRELLO_FEATURE_LABEL_ID", "featurelabelid")

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

func TestConfigFromEnv_ErrorDoesNotExposeTrelloToken(t *testing.T) {
	secret := "super-secret-trello-token"
	t.Setenv("DISCORD_BOT_TOKEN", "tok")
	t.Setenv("DISCORD_APPLICATION_ID", "appid")
	t.Setenv("DISCORD_GUILD_ID", "guildid")
	t.Setenv("DISCORD_EVENTS_CHANNEL_ID", "chanid")
	t.Setenv("TRELLO_API_KEY", "trellokey")
	t.Setenv("TRELLO_TOKEN", secret)
	t.Setenv("TRELLO_TRIAGE_LIST_ID", "")
	t.Setenv("TRELLO_BUG_LABEL_ID", "")
	t.Setenv("TRELLO_FEATURE_LABEL_ID", "")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error message must not contain the trello token")
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
		GameName:         "game-1",
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
	msg, _, err := renderMessage(entry, "")
	if err != nil {
		t.Fatalf("renderMessage: %v", err)
	}
	if msg != "@everyone 🎯 <@Bob> ended their turn. <@Alice> is up. (game `game-1`)" {
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
	msg, _, err := renderMessage(entry, "")
	if err != nil {
		t.Fatalf("renderMessage: %v", err)
	}
	if msg != "🎯 <@bobsmith> ended their turn. <@alicewonder> is up. (game `game-1`)" {
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
	msg, _, err := renderMessage(entry, "")
	if err != nil {
		t.Fatalf("renderMessage: %v", err)
	}
	if msg != "@everyone 🎯 <@Bob> ended their turn. <@Alice> is up. (game `game-1`)" {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func TestRenderTurnStartedMissingName(t *testing.T) {
	entry := makeEntry(store.NotificationTypeTurnStarted, `{
		"schema_version": 1,
		"player_id": "player-uuid",
		"player_display_name": ""
	}`)
	_, _, err := renderMessage(entry, "")
	if err == nil {
		t.Fatal("expected error for missing player_display_name")
	}
}

func TestRenderCardsTrade(t *testing.T) {
	entry := makeEntry(store.NotificationTypeCardsTrade, `{
		"schema_version": 1,
		"player_id": "player-uuid",
		"player_display_name": "Alice",
		"armies": 8
	}`)
	msg, _, err := renderMessage(entry, "")
	if err != nil {
		t.Fatalf("renderMessage: %v", err)
	}
	if msg != "@everyone 🃏 **Alice** traded in cards for 8 armies. (game `game-1`)" {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func TestRenderCardsTradeWithDiscordName(t *testing.T) {
	entry := makeEntry(store.NotificationTypeCardsTrade, `{
		"schema_version": 1,
		"player_id": "player-uuid",
		"player_display_name": "Alice",
		"player_discord_name": "alicewonder",
		"armies": 4
	}`)
	msg, _, err := renderMessage(entry, "")
	if err != nil {
		t.Fatalf("renderMessage: %v", err)
	}
	if msg != "🃏 <@alicewonder> traded in cards for 4 armies. (game `game-1`)" {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func TestRenderCardsTradesMissingName(t *testing.T) {
	entry := makeEntry(store.NotificationTypeCardsTrade, `{
		"schema_version": 1,
		"player_id": "player-uuid",
		"player_display_name": "",
		"armies": 4
	}`)
	_, _, err := renderMessage(entry, "")
	if err == nil {
		t.Fatal("expected error for missing player_display_name")
	}
}

func TestRenderPlayerEliminated(t *testing.T) {
	entry := makeEntry(store.NotificationTypePlayerEliminated, `{
		"schema_version": 1,
		"attacker_id": "a1",
		"attacker_display_name": "Alice",
		"eliminated_player_id": "b1",
		"eliminated_player_display_name": "Bob"
	}`)
	msg, _, err := renderMessage(entry, "")
	if err != nil {
		t.Fatalf("renderMessage: %v", err)
	}
	if msg != "@everyone ⚔️ **Alice** eliminated **Bob**! (game `game-1`)" {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func TestRenderPlayerEliminatedWithDiscordNames(t *testing.T) {
	entry := makeEntry(store.NotificationTypePlayerEliminated, `{
		"schema_version": 1,
		"attacker_id": "a1",
		"attacker_display_name": "Alice",
		"attacker_discord_name": "alicewonder",
		"eliminated_player_id": "b1",
		"eliminated_player_display_name": "Bob",
		"eliminated_player_discord_name": "bobsmith"
	}`)
	msg, _, err := renderMessage(entry, "")
	if err != nil {
		t.Fatalf("renderMessage: %v", err)
	}
	if msg != "⚔️ <@alicewonder> eliminated <@bobsmith>! (game `game-1`)" {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func TestRenderPlayerEliminatedOneDiscordNameMissing(t *testing.T) {
	entry := makeEntry(store.NotificationTypePlayerEliminated, `{
		"schema_version": 1,
		"attacker_id": "a1",
		"attacker_display_name": "Alice",
		"attacker_discord_name": "alicewonder",
		"eliminated_player_id": "b1",
		"eliminated_player_display_name": "Bob"
	}`)
	msg, _, err := renderMessage(entry, "")
	if err != nil {
		t.Fatalf("renderMessage: %v", err)
	}
	if msg != "@everyone ⚔️ **Alice** eliminated **Bob**! (game `game-1`)" {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func TestRenderGameOver(t *testing.T) {
	entry := makeEntry(store.NotificationTypeGameOver, `{
		"schema_version": 1,
		"winner_id": "a1",
		"winner_display_name": "Alice"
	}`)
	msg, _, err := renderMessage(entry, "")
	if err != nil {
		t.Fatalf("renderMessage: %v", err)
	}
	if msg != "@everyone 🏆 **Alice** has won the game! (game `game-1`)" {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func TestRenderGameOverWithDiscordName(t *testing.T) {
	entry := makeEntry(store.NotificationTypeGameOver, `{
		"schema_version": 1,
		"winner_id": "a1",
		"winner_display_name": "Alice",
		"winner_discord_name": "alicewonder"
	}`)
	msg, _, err := renderMessage(entry, "")
	if err != nil {
		t.Fatalf("renderMessage: %v", err)
	}
	if msg != "🏆 <@alicewonder> has won the game! (game `game-1`)" {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func TestRenderGameOverWithFrontendURL_KeepsGameNameAndAddsLinkEmbed(t *testing.T) {
	entry := makeEntry(store.NotificationTypeGameOver, `{
		"schema_version": 1,
		"winner_id": "a1",
		"winner_display_name": "Alice"
	}`)
	msg, embeds, err := renderMessage(entry, "https://play.example.com")
	if err != nil {
		t.Fatalf("renderMessage: %v", err)
	}
	// The "(game `name`)" text suffix stays exactly as it was before links
	// existed; the link is a separate embed (Discord content does not render
	// [text](url) markdown, so a clickable link requires an embed either way).
	if msg != "@everyone 🏆 **Alice** has won the game! (game `game-1`)" {
		t.Fatalf("unexpected message: %q", msg)
	}
	if len(embeds) != 1 {
		t.Fatalf("expected 1 embed, got %d", len(embeds))
	}
	if embeds[0].Title != "Click to view game" {
		t.Errorf("expected embed title %q, got %q", "Click to view game", embeds[0].Title)
	}
	if embeds[0].URL != "https://play.example.com/app/game/game-1" {
		t.Errorf("unexpected embed URL: %q", embeds[0].URL)
	}
}

func TestRenderGameStarted(t *testing.T) {
	entry := makeEntry(store.NotificationTypeGameStarted, `{
		"schema_version": 1,
		"player_id": "a1",
		"player_display_name": "Alice"
	}`)
	msg, _, err := renderMessage(entry, "")
	if err != nil {
		t.Fatalf("renderMessage: %v", err)
	}
	if msg != "@everyone 🚦 The game has begun! **Alice** goes first. (game `game-1`)" {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func TestRenderGameStartedWithDiscordName(t *testing.T) {
	entry := makeEntry(store.NotificationTypeGameStarted, `{
		"schema_version": 1,
		"player_id": "a1",
		"player_display_name": "Alice",
		"player_discord_name": "alicewonder"
	}`)
	msg, _, err := renderMessage(entry, "")
	if err != nil {
		t.Fatalf("renderMessage: %v", err)
	}
	if msg != "🚦 The game has begun! <@alicewonder> goes first. (game `game-1`)" {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func TestRenderGameStartedMissingName(t *testing.T) {
	entry := makeEntry(store.NotificationTypeGameStarted, `{
		"schema_version": 1,
		"player_id": "a1",
		"player_display_name": ""
	}`)
	_, _, err := renderMessage(entry, "")
	if err == nil {
		t.Fatal("expected error for missing player_display_name")
	}
}

func TestRenderTurnStartedWithFrontendURL_KeepsGameNameAndAddsLinkEmbed(t *testing.T) {
	entry := makeEntry(store.NotificationTypeTurnStarted, `{
		"schema_version": 1,
		"previous_player_display_name": "Bob",
		"player_id": "player-uuid",
		"player_display_name": "Alice",
		"turn_number": 5
	}`)
	msg, embeds, err := renderMessage(entry, "https://play.example.com")
	if err != nil {
		t.Fatalf("renderMessage: %v", err)
	}
	if !strings.Contains(msg, "(game `game-1`)") {
		t.Errorf("expected game name suffix to stay in content, got: %q", msg)
	}
	if len(embeds) != 1 || embeds[0].URL != "https://play.example.com/app/game/game-1" {
		t.Fatalf("expected a linking embed, got: %v", embeds)
	}
}

func TestRenderWithoutFrontendURL_NoEmbed(t *testing.T) {
	entry := makeEntry(store.NotificationTypeGameOver, `{
		"schema_version": 1,
		"winner_id": "a1",
		"winner_display_name": "Alice"
	}`)
	_, embeds, err := renderMessage(entry, "")
	if err != nil {
		t.Fatalf("renderMessage: %v", err)
	}
	if embeds != nil {
		t.Fatalf("expected no embeds when frontendBaseURL is unset, got: %v", embeds)
	}
}

func TestRenderMalformedPayload(t *testing.T) {
	entry := makeEntry(store.NotificationTypeTurnStarted, `not-valid-json`)
	_, _, err := renderMessage(entry, "")
	if err == nil {
		t.Fatal("expected error for malformed payload")
	}
}

func TestRenderUnknownNotificationType(t *testing.T) {
	entry := makeEntry("player_surrendered", `{"schema_version":1}`)
	_, _, err := renderMessage(entry, "")
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
	sendFn func(ctx context.Context, channelID, content string, embeds ...*discordgo.MessageEmbed) error
}

func (f *fakeSender) SendMessage(ctx context.Context, channelID, content string, embeds ...*discordgo.MessageEmbed) error {
	if f.sendFn != nil {
		return f.sendFn(ctx, channelID, content, embeds...)
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
		sendFn: func(_ context.Context, _, content string, _ ...*discordgo.MessageEmbed) error {
			sentContent = content
			return nil
		},
	}

	w := NewWorker(claimer, sender, "channel-id", "")
	w.deliver(context.Background(), entry)

	if sentContent != "@everyone 🎯 <@Alice> ended their turn. <@Bob> is up. (game `game-1`)" {
		t.Fatalf("unexpected message content: %q", sentContent)
	}
	if markedDelivered != "outbox-1" {
		t.Fatalf("expected MarkDelivered called with outbox-1, got %q", markedDelivered)
	}
}

func TestWorkerDeliverWithFrontendURL_PassesEmbedToSender(t *testing.T) {
	entry := makeEntry(store.NotificationTypeGameOver, `{
		"schema_version": 1,
		"winner_id": "a1",
		"winner_display_name": "Alice"
	}`)
	entry.AttemptCount = 1

	var sentEmbeds []*discordgo.MessageEmbed
	claimer := &fakeOutboxClaimer{
		deliverFn: func(context.Context, string) error { return nil },
	}
	sender := &fakeSender{
		sendFn: func(_ context.Context, _, _ string, embeds ...*discordgo.MessageEmbed) error {
			sentEmbeds = embeds
			return nil
		},
	}

	w := NewWorker(claimer, sender, "channel-id", "https://play.example.com")
	w.deliver(context.Background(), entry)

	if len(sentEmbeds) != 1 {
		t.Fatalf("expected 1 embed passed to sender, got %d", len(sentEmbeds))
	}
	if sentEmbeds[0].URL != "https://play.example.com/app/game/game-1" {
		t.Errorf("unexpected embed URL: %q", sentEmbeds[0].URL)
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
		sendFn: func(context.Context, string, string, ...*discordgo.MessageEmbed) error {
			return errors.New("discord 429")
		},
	}

	w := NewWorker(claimer, sender, "channel-id", "")
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
		sendFn: func(context.Context, string, string, ...*discordgo.MessageEmbed) error {
			t.Error("SendMessage must not be called for malformed payload")
			return nil
		},
	}

	w := NewWorker(claimer, sender, "channel-id", "")
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
		sendFn: func(context.Context, string, string, ...*discordgo.MessageEmbed) error {
			t.Error("SendMessage must not be called for unknown notification type")
			return nil
		},
	}

	w := NewWorker(claimer, sender, "channel-id", "")
	w.deliver(context.Background(), entry)

	if !failCalled {
		t.Fatal("expected MarkFailed called for unknown notification type")
	}
}

// --- allCommandDefs tests ---

func TestAllCommandDefs_Count(t *testing.T) {
	defs := allCommandDefs()
	if len(defs) != 8 {
		t.Errorf("expected 8 command defs, got %d", len(defs))
	}
}

func TestAllCommandDefs_Names(t *testing.T) {
	defs := allCommandDefs()
	names := make(map[string]bool, len(defs))
	for _, d := range defs {
		names[d.Name] = true
	}
	for _, want := range []string{
		pingCommandName, lastRollsCommandName, diceReportCommandName, playerStatsCommandName,
		playerUpCommandName, rollStreaksCommandName, bugCommandName, featureCommandName,
	} {
		if !names[want] {
			t.Errorf("missing command def: %q", want)
		}
	}
}

func TestAllCommandDefs_PlayerUpHasOptionalGameOption(t *testing.T) {
	var playerUp *discordgo.ApplicationCommand
	for _, d := range allCommandDefs() {
		if d.Name == playerUpCommandName {
			playerUp = d
			break
		}
	}
	if playerUp == nil {
		t.Fatal("/player-up not found in allCommandDefs")
	}
	if len(playerUp.Options) != 1 {
		t.Fatalf("expected 1 option, got %d", len(playerUp.Options))
	}
	opt := playerUp.Options[0]
	if opt.Name != "game" {
		t.Errorf("option name: want %q, got %q", "game", opt.Name)
	}
	if opt.Required {
		t.Error("game option must not be required")
	}
}

func TestAllCommandDefs_PlayerStatsHasRequiredPlayerOption(t *testing.T) {
	var playerStats *discordgo.ApplicationCommand
	for _, d := range allCommandDefs() {
		if d.Name == playerStatsCommandName {
			playerStats = d
			break
		}
	}
	if playerStats == nil {
		t.Fatal("/player-stats not found in allCommandDefs")
	}
	if len(playerStats.Options) != 2 {
		t.Fatalf("expected 2 options (player + game), got %d", len(playerStats.Options))
	}
	opt := playerStats.Options[0]
	if opt.Name != "player" {
		t.Errorf("option name: want %q, got %q", "player", opt.Name)
	}
	if opt.Type != discordgo.ApplicationCommandOptionString {
		t.Errorf("option type: want String, got %v", opt.Type)
	}
	if !opt.Required {
		t.Error("player option must be required")
	}
	gameOpt := playerStats.Options[1]
	if gameOpt.Name != "game" {
		t.Errorf("second option name: want %q, got %q", "game", gameOpt.Name)
	}
	if gameOpt.Required {
		t.Error("game option must not be required")
	}
}

func TestAllCommandDefs_LastRollsHasOptionalCountOption(t *testing.T) {
	var lastRolls *discordgo.ApplicationCommand
	for _, d := range allCommandDefs() {
		if d.Name == lastRollsCommandName {
			lastRolls = d
			break
		}
	}
	if lastRolls == nil {
		t.Fatal("/last-rolls not found in allCommandDefs")
	}
	if len(lastRolls.Options) != 2 {
		t.Fatalf("expected 2 options (count + game), got %d", len(lastRolls.Options))
	}
	opt := lastRolls.Options[0]
	if opt.Name != "count" {
		t.Errorf("option name: want %q, got %q", "count", opt.Name)
	}
	if opt.Type != discordgo.ApplicationCommandOptionInteger {
		t.Errorf("option type: want Integer, got %v", opt.Type)
	}
	if opt.Required {
		t.Error("count option must not be required")
	}
	if opt.MaxValue != float64(maxLastRollsCount) {
		t.Errorf("MaxValue: want %v, got %v", float64(maxLastRollsCount), opt.MaxValue)
	}
}

// --- decideCommandAction tests ---

func TestDecideCommandAction_Create(t *testing.T) {
	def := &discordgo.ApplicationCommand{Name: "ping", Description: "desc"}
	result := decideCommandAction(nil, def)
	if result.action != commandCreate {
		t.Errorf("expected commandCreate, got %v", result.action)
	}
}

func TestDecideCommandAction_Reuse(t *testing.T) {
	existing := []*discordgo.ApplicationCommand{
		{ID: "cmd-1", Name: "ping", Description: "desc"},
	}
	def := &discordgo.ApplicationCommand{Name: "ping", Description: "desc"}
	result := decideCommandAction(existing, def)
	if result.action != commandReuse {
		t.Errorf("expected commandReuse, got %v", result.action)
	}
	if result.existingID != "cmd-1" {
		t.Errorf("existingID: want %q, got %q", "cmd-1", result.existingID)
	}
}

func TestDecideCommandAction_Update(t *testing.T) {
	existing := []*discordgo.ApplicationCommand{
		{ID: "cmd-2", Name: "ping", Description: "old description"},
	}
	def := &discordgo.ApplicationCommand{Name: "ping", Description: "new description"}
	result := decideCommandAction(existing, def)
	if result.action != commandUpdate {
		t.Errorf("expected commandUpdate, got %v", result.action)
	}
	if result.existingID != "cmd-2" {
		t.Errorf("existingID: want %q, got %q", "cmd-2", result.existingID)
	}
}

// --- report formatting tests ---

func TestFormatLastRolls_Empty(t *testing.T) {
	out := formatLastRolls(nil, "Test Game")
	if !strings.Contains(out, "No combat events") {
		t.Errorf("expected empty message, got: %q", out)
	}
}

func TestFormatLastRolls_SingleRoll(t *testing.T) {
	rolls := []RecentCombatRoll{
		{
			GameSequence:        7,
			AttackerDisplayName: "Alice",
			DefenderDisplayName: "Bob",
			SourceTerritoryID:   "alaska",
			TargetTerritoryID:   "kamchatka",
			AttackerDice:        []int{6, 5},
			DefenderDice:        []int{3},
			AttackerLosses:      0,
			DefenderLosses:      1,
			Captured:            false,
		},
	}
	out := formatLastRolls(rolls, "Test Game")
	if !strings.Contains(out, "Alice") {
		t.Errorf("expected attacker name, got: %q", out)
	}
	if !strings.Contains(out, "Bob") {
		t.Errorf("expected defender name, got: %q", out)
	}
	if !strings.Contains(out, "Alaska") {
		t.Errorf("expected title-cased territory, got: %q", out)
	}
	if !strings.Contains(out, "Kamchatka") {
		t.Errorf("expected title-cased territory, got: %q", out)
	}
	if !strings.Contains(out, "6  5") {
		t.Errorf("expected attacker dice, got: %q", out)
	}
}

func TestFormatLastRolls_Captured(t *testing.T) {
	rolls := []RecentCombatRoll{
		{
			GameSequence:        1,
			AttackerDisplayName: "Carol",
			DefenderDisplayName: "Dave",
			AttackerDice:        []int{6},
			DefenderDice:        []int{1},
			Captured:            true,
		},
	}
	out := formatLastRolls(rolls, "Test Game")
	if !strings.Contains(out, "✅") {
		t.Errorf("expected capture indicator, got: %q", out)
	}
}

func TestFormatDiceReport_Empty(t *testing.T) {
	r := DiceReport{GameID: "g1", CombatRolls: 0}
	out := formatDiceReport(r, "Test Game")
	if !strings.Contains(out, "Dice Report") {
		t.Errorf("expected header, got: %q", out)
	}
	if !strings.Contains(out, "Combat rolls") {
		t.Errorf("expected zero rolls, got: %q", out)
	}
	if !strings.Contains(out, "Descriptive results") {
		t.Errorf("expected disclaimer, got: %q", out)
	}
}

func TestFormatPlayerReport_NoRolls(t *testing.T) {
	r := PlayerCombatReport{PlayerID: "p1", PlayerDisplayName: "Alice", AttackRolls: 0}
	out := formatPlayerReport(r, "Test Game")
	if !strings.Contains(out, "No attack rolls") {
		t.Errorf("expected no-rolls message, got: %q", out)
	}
	if !strings.Contains(out, "Alice") {
		t.Errorf("expected player name in message, got: %q", out)
	}
}

func TestFormatPlayerReport_WithRolls(t *testing.T) {
	r := PlayerCombatReport{
		PlayerID:                  "p1",
		PlayerDisplayName:         "Alice",
		AttackRolls:               3,
		TerritoriesCaptured:       1,
		CaptureRate:               33.3,
		AttackerDiceRolled:        9,
		AverageAttackerDice:       3.0,
		AttackerLosses:            1,
		DefenderLossesInflicted:   3,
		AverageSourceArmiesBefore: 7.0,
		AverageTargetArmiesBefore: 2.0,
		AverageArmyAdvantage:      5.0,
	}
	out := formatPlayerReport(r, "Test Game")
	if !strings.Contains(out, "Alice") {
		t.Errorf("expected player name, got: %q", out)
	}
	if !strings.Contains(out, "Attack rolls") {
		t.Errorf("expected attack roll count, got: %q", out)
	}
	if !strings.Contains(out, "33.3") {
		t.Errorf("expected capture rate, got: %q", out)
	}
}

func TestFormatRollStreaks_Empty(t *testing.T) {
	out := formatRollStreaks(reporting.RollStreakReport{}, "Test Game", 5)
	if !strings.Contains(out, "No attack rolls found") {
		t.Errorf("expected empty message, got: %q", out)
	}
}

func TestFormatRollStreaks_PartialHistoryWarning(t *testing.T) {
	r := reporting.RollStreakReport{
		PartialHistory: true,
		SummaryByAttacker: []reporting.PlayerStreakSummary{
			{PlayerID: "p1", PlayerName: "Alice", AttackRollsCaptured: 5},
		},
	}
	out := formatRollStreaks(r, "Test Game", 5)
	if !strings.Contains(out, "partial event history") {
		t.Errorf("expected partial history warning, got: %q", out)
	}
}

func TestFormatRollStreaks_NoPartialHistoryWarningWhenComplete(t *testing.T) {
	r := reporting.RollStreakReport{
		PartialHistory: false,
		SummaryByAttacker: []reporting.PlayerStreakSummary{
			{PlayerID: "p1", PlayerName: "Alice", AttackRollsCaptured: 5},
		},
	}
	out := formatRollStreaks(r, "Test Game", 5)
	if strings.Contains(out, "partial event history") {
		t.Errorf("did not expect partial history warning, got: %q", out)
	}
}

func TestFormatRollStreaks_SummaryIncludesPlayerName(t *testing.T) {
	r := reporting.RollStreakReport{
		SummaryByAttacker: []reporting.PlayerStreakSummary{
			{PlayerID: "p1", PlayerName: "Tucker", AttackRollsCaptured: 8, AttackerLossCount: 5},
		},
	}
	out := formatRollStreaks(r, "Test Game", 5)
	if !strings.Contains(out, "Tucker") {
		t.Errorf("expected player name in summary, got: %q", out)
	}
}

func TestFormatRollStreaks_TopNLimitsStreaksShown(t *testing.T) {
	streaks := make([]reporting.Streak, 3)
	for i := range streaks {
		streaks[i] = reporting.Streak{AttackerName: "Alice", Length: 3 - i, StartSeq: int64(i), EndSeq: int64(i), RollTrace: "1-0, 1-0"}
	}
	r := reporting.RollStreakReport{
		SummaryByAttacker:    []reporting.PlayerStreakSummary{{PlayerID: "p1", PlayerName: "Alice", AttackRollsCaptured: 6}},
		AttackingLossStreaks: streaks,
	}
	out := formatRollStreaks(r, "Test Game", 2)
	if !strings.Contains(out, "1 more not shown") {
		t.Errorf("expected truncation notice for top=2 with 3 streaks, got: %q", out)
	}
}

func TestFormatRollStreaks_RollTraceShown(t *testing.T) {
	r := reporting.RollStreakReport{
		SummaryByAttacker: []reporting.PlayerStreakSummary{{PlayerID: "p1", PlayerName: "Alice", AttackRollsCaptured: 2}},
		AttackingLossStreaks: []reporting.Streak{
			{AttackerName: "Alice", Length: 2, StartSeq: 1, EndSeq: 2, RollTrace: "2-0, 1-0"},
		},
	}
	out := formatRollStreaks(r, "Test Game", 5)
	if !strings.Contains(out, "2-0, 1-0") {
		t.Errorf("expected roll trace, got: %q", out)
	}
}

// Expose reporting types for formatting tests (they live in the same package).
type RecentCombatRoll = reporting.RecentCombatRoll
type DiceReport = reporting.DiceReport
type PlayerCombatReport = reporting.PlayerCombatReport
type FaceDistribution = reporting.FaceDistribution

// --- ticket.go: parseTicketType / custom ID tests ---

func TestParseTicketType_Bug(t *testing.T) {
	got, ok := parseTicketType("ticket_modal:bug")
	if !ok || got != ticketTypeBug {
		t.Fatalf("expected (bug, true), got (%q, %v)", got, ok)
	}
}

func TestParseTicketType_Feature(t *testing.T) {
	got, ok := parseTicketType("ticket_modal:feature")
	if !ok || got != ticketTypeFeature {
		t.Fatalf("expected (feature, true), got (%q, %v)", got, ok)
	}
}

func TestParseTicketType_UnknownType(t *testing.T) {
	_, ok := parseTicketType("ticket_modal:surprise")
	if ok {
		t.Fatal("expected ok=false for an unrecognized ticket type")
	}
}

func TestParseTicketType_WrongPrefix_Ignored(t *testing.T) {
	// Some other, unrelated modal — must be ignored (ok=false), not error.
	_, ok := parseTicketType("some_other_modal:whatever")
	if ok {
		t.Fatal("expected ok=false for a non-ticket modal custom ID")
	}
}

func TestTicketModalCustomID_RoundTripsWithParseTicketType(t *testing.T) {
	for _, want := range []ticketType{ticketTypeBug, ticketTypeFeature} {
		id := ticketModalCustomID(want)
		got, ok := parseTicketType(id)
		if !ok || got != want {
			t.Errorf("round trip failed for %q: got (%q, %v)", want, got, ok)
		}
	}
}

// --- ticket.go: label mapping ---

func TestTrelloLabelIDFor_Bug(t *testing.T) {
	cfg := Config{TrelloBugLabelID: "bug-label", TrelloFeatureLabelID: "feature-label"}
	if got := trelloLabelIDFor(ticketTypeBug, cfg); got != "bug-label" {
		t.Errorf("expected bug-label, got %q", got)
	}
}

func TestTrelloLabelIDFor_Feature(t *testing.T) {
	cfg := Config{TrelloBugLabelID: "bug-label", TrelloFeatureLabelID: "feature-label"}
	if got := trelloLabelIDFor(ticketTypeFeature, cfg); got != "feature-label" {
		t.Errorf("expected feature-label, got %q", got)
	}
}

func TestLabelIDsOrEmpty_NonEmpty(t *testing.T) {
	got := labelIDsOrEmpty("abc")
	if len(got) != 1 || got[0] != "abc" {
		t.Errorf("unexpected result: %v", got)
	}
}

func TestLabelIDsOrEmpty_Empty(t *testing.T) {
	if got := labelIDsOrEmpty(""); got != nil {
		t.Errorf("expected nil for empty label ID, got %v", got)
	}
}

// --- ticket.go: card title/description formatting ---

func TestTicketCardTitle_Bug(t *testing.T) {
	got := ticketCardTitle(ticketTypeBug, "Login button is broken")
	if got != "[bug] Login button is broken" {
		t.Errorf("unexpected title: %q", got)
	}
}

func TestTicketCardTitle_Feature(t *testing.T) {
	got := ticketCardTitle(ticketTypeFeature, "Dark mode")
	if got != "[feature] Dark mode" {
		t.Errorf("unexpected title: %q", got)
	}
}

func TestTicketCardName_FallsBackWhenTitleBlank(t *testing.T) {
	got := ticketCardName(ticketTypeBug, map[string]string{fieldTitle: "   "})
	if got != "[bug] Untitled bug report" {
		t.Errorf("unexpected fallback title: %q", got)
	}
}

func TestTicketCardName_UsesSubmittedTitle(t *testing.T) {
	got := ticketCardName(ticketTypeFeature, map[string]string{fieldTitle: "Dark mode"})
	if got != "[feature] Dark mode" {
		t.Errorf("unexpected title: %q", got)
	}
}

func TestTicketCardDescription_Bug(t *testing.T) {
	reporter := ticketReporter{
		DisplayName: "Tucker",
		UserID:      "123456789",
		GuildID:     "987654321",
		ChannelID:   "555555555",
	}
	fields := map[string]string{
		fieldWhatHappened:     "The login button does nothing.",
		fieldExpectedBehavior: "It should log me in.",
		fieldStepsToReproduce: "1. Click login\n2. Nothing happens",
	}
	got := ticketCardDescription(ticketTypeBug, reporter, fields)

	want := strings.Join([]string{
		"Reported by: Tucker",
		"Discord user ID: 123456789",
		"Discord guild ID: 987654321",
		"Discord channel ID: 555555555",
		"Ticket type: bug",
		"Source: Discord /bug command",
		"",
		"What happened:",
		"The login button does nothing.",
		"",
		"Expected behavior:",
		"It should log me in.",
		"",
		"Steps to reproduce:",
		"1. Click login\n2. Nothing happens",
	}, "\n")
	if got != want {
		t.Errorf("unexpected description.\ngot:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestTicketCardDescription_Feature(t *testing.T) {
	reporter := ticketReporter{DisplayName: "Alice", UserID: "u1", GuildID: "g1", ChannelID: "c1"}
	fields := map[string]string{
		fieldWhatShouldBeAdded: "A dark mode theme.",
		fieldWhyUseful:         "Easier on the eyes at night.",
	}
	got := ticketCardDescription(ticketTypeFeature, reporter, fields)

	want := strings.Join([]string{
		"Reported by: Alice",
		"Discord user ID: u1",
		"Discord guild ID: g1",
		"Discord channel ID: c1",
		"Ticket type: feature",
		"Source: Discord /feature command",
		"",
		"What should be added:",
		"A dark mode theme.",
		"",
		"Why would this be useful:",
		"Easier on the eyes at night.",
	}, "\n")
	if got != want {
		t.Errorf("unexpected description.\ngot:\n%s\n\nwant:\n%s", got, want)
	}
}

// --- ticket.go: modal field extraction (no live Discord connection needed) ---

func TestExtractModalFields(t *testing.T) {
	data := discordgo.ModalSubmitInteractionData{
		CustomID: "ticket_modal:bug",
		Components: []discordgo.MessageComponent{
			&discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				&discordgo.TextInput{CustomID: fieldTitle, Value: "Login broken"},
			}},
			&discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				&discordgo.TextInput{CustomID: fieldWhatHappened, Value: "Nothing happens on click"},
			}},
		},
	}
	fields := extractModalFields(data)
	if fields[fieldTitle] != "Login broken" {
		t.Errorf("title: got %q", fields[fieldTitle])
	}
	if fields[fieldWhatHappened] != "Nothing happens on click" {
		t.Errorf("what_happened: got %q", fields[fieldWhatHappened])
	}
}

func TestExtractModalFields_IgnoresNonActionsRowComponents(t *testing.T) {
	data := discordgo.ModalSubmitInteractionData{
		Components: []discordgo.MessageComponent{
			&discordgo.TextInput{CustomID: "stray", Value: "should be ignored"},
		},
	}
	fields := extractModalFields(data)
	if len(fields) != 0 {
		t.Errorf("expected no fields extracted, got %v", fields)
	}
}

func TestExtractModalFields_Empty(t *testing.T) {
	fields := extractModalFields(discordgo.ModalSubmitInteractionData{})
	if len(fields) != 0 {
		t.Errorf("expected empty map, got %v", fields)
	}
}

// --- ticket.go: reporter extraction ---

func TestTicketReporterFromInteraction_UsesMemberNickWhenPresent(t *testing.T) {
	i := &discordgo.Interaction{
		GuildID:   "g1",
		ChannelID: "c1",
		Member: &discordgo.Member{
			Nick: "Tuck",
			User: &discordgo.User{ID: "u1", Username: "tuckerreu"},
		},
	}
	r := ticketReporterFromInteraction(i)
	if r.DisplayName != "Tuck" {
		t.Errorf("expected nickname to be preferred, got %q", r.DisplayName)
	}
	if r.UserID != "u1" || r.GuildID != "g1" || r.ChannelID != "c1" {
		t.Errorf("unexpected reporter: %+v", r)
	}
}

func TestTicketReporterFromInteraction_FallsBackToUsernameWithoutNick(t *testing.T) {
	i := &discordgo.Interaction{
		Member: &discordgo.Member{User: &discordgo.User{ID: "u1", Username: "tuckerreu"}},
	}
	r := ticketReporterFromInteraction(i)
	if r.DisplayName != "tuckerreu" {
		t.Errorf("expected username fallback, got %q", r.DisplayName)
	}
}

func TestTicketReporterFromInteraction_FallsBackToUserForDMs(t *testing.T) {
	i := &discordgo.Interaction{
		User: &discordgo.User{ID: "u2", Username: "dmuser"},
	}
	r := ticketReporterFromInteraction(i)
	if r.DisplayName != "dmuser" || r.UserID != "u2" {
		t.Errorf("unexpected reporter: %+v", r)
	}
}

// --- ticket.go: Trello creation result -> ephemeral message ---

func TestTicketSubmitResultMessage_Success(t *testing.T) {
	card := &trello.CreatedCard{ID: "c1", URL: "https://trello.com/c/abc123"}
	got := ticketSubmitResultMessage(ticketTypeBug, card, nil)
	want := "Created bug report: https://trello.com/c/abc123"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTicketSubmitResultMessage_Failure(t *testing.T) {
	got := ticketSubmitResultMessage(ticketTypeFeature, nil, errors.New("trello: create card failed with status 401: unauthorized"))
	if !strings.Contains(got, "couldn't create") {
		t.Errorf("expected a user-friendly failure message, got %q", got)
	}
	// Must not leak the underlying error (which could contain response bodies).
	if strings.Contains(got, "401") || strings.Contains(got, "unauthorized") {
		t.Errorf("failure message should not leak internal error details, got %q", got)
	}
}

// --- ticket.go: modal builders ---

func TestBugReportModal_HasExpectedFieldsAndCustomID(t *testing.T) {
	modal := bugReportModal()
	if modal.CustomID != "ticket_modal:bug" {
		t.Errorf("unexpected custom ID: %q", modal.CustomID)
	}
	fieldIDs := modalFieldCustomIDs(t, modal.Components)
	want := []string{fieldTitle, fieldWhatHappened, fieldExpectedBehavior, fieldStepsToReproduce}
	if !reflect.DeepEqual(fieldIDs, want) {
		t.Errorf("unexpected field IDs: got %v, want %v", fieldIDs, want)
	}
}

func TestFeatureRequestModal_HasExpectedFieldsAndCustomID(t *testing.T) {
	modal := featureRequestModal()
	if modal.CustomID != "ticket_modal:feature" {
		t.Errorf("unexpected custom ID: %q", modal.CustomID)
	}
	fieldIDs := modalFieldCustomIDs(t, modal.Components)
	want := []string{fieldTitle, fieldWhatShouldBeAdded, fieldWhyUseful}
	if !reflect.DeepEqual(fieldIDs, want) {
		t.Errorf("unexpected field IDs: got %v, want %v", fieldIDs, want)
	}
}

func modalFieldCustomIDs(t *testing.T, components []discordgo.MessageComponent) []string {
	t.Helper()
	ids := make([]string, 0, len(components))
	for _, comp := range components {
		row, ok := comp.(discordgo.ActionsRow)
		if !ok {
			t.Fatalf("expected a discordgo.ActionsRow, got %T", comp)
		}
		if len(row.Components) != 1 {
			t.Fatalf("expected 1 text input per row, got %d", len(row.Components))
		}
		input, ok := row.Components[0].(discordgo.TextInput)
		if !ok {
			t.Fatalf("expected a discordgo.TextInput, got %T", row.Components[0])
		}
		ids = append(ids, input.CustomID)
	}
	return ids
}

// --- Fake Trello client for handler-level tests ---

type fakeTrelloClient struct {
	createCardFn func(ctx context.Context, input trello.CreateCardInput) (*trello.CreatedCard, error)
	lastInput    trello.CreateCardInput
}

func (f *fakeTrelloClient) CreateCard(ctx context.Context, input trello.CreateCardInput) (*trello.CreatedCard, error) {
	f.lastInput = input
	if f.createCardFn != nil {
		return f.createCardFn(ctx, input)
	}
	return &trello.CreatedCard{ID: "c1", URL: "https://trello.com/c/c1"}, nil
}

func TestNew_WiresTrelloClient(t *testing.T) {
	trelloClient := &fakeTrelloClient{}
	bot, err := New(Config{BotToken: "tok"}, nil, trelloClient)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if bot.trello != trelloClient {
		t.Error("expected the Bot to store the provided trello client")
	}
}
