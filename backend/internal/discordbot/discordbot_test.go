package discordbot

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"backend/internal/reporting"
	"backend/internal/store"

	"github.com/bwmarrin/discordgo"
)

// --- ConfigFromEnv tests ---

const testGameID = "12345678-1234-1234-1234-123456789abc"

func TestConfigFromEnv_AllPresent(t *testing.T) {
	t.Setenv("DISCORD_BOT_TOKEN", "tok")
	t.Setenv("DISCORD_APPLICATION_ID", "appid")
	t.Setenv("DISCORD_GUILD_ID", "guildid")
	t.Setenv("DISCORD_EVENTS_CHANNEL_ID", "chanid")
	t.Setenv("DISCORD_DEFAULT_GAME_ID", testGameID)

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BotToken != "tok" || cfg.ApplicationID != "appid" ||
		cfg.GuildID != "guildid" || cfg.EventsChannelID != "chanid" ||
		cfg.DefaultGameID != testGameID {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestConfigFromEnv_OneMissing(t *testing.T) {
	t.Setenv("DISCORD_BOT_TOKEN", "tok")
	t.Setenv("DISCORD_APPLICATION_ID", "appid")
	t.Setenv("DISCORD_GUILD_ID", "guildid")
	t.Setenv("DISCORD_EVENTS_CHANNEL_ID", "")
	t.Setenv("DISCORD_DEFAULT_GAME_ID", testGameID)

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
	t.Setenv("DISCORD_DEFAULT_GAME_ID", " "+testGameID+" ")

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
	if cfg.DefaultGameID != testGameID {
		t.Errorf("DefaultGameID not trimmed: %q", cfg.DefaultGameID)
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

func TestRenderCardsTrade(t *testing.T) {
	entry := makeEntry(store.NotificationTypeCardsTrade, `{
		"schema_version": 1,
		"player_id": "player-uuid",
		"player_display_name": "Alice",
		"armies": 8
	}`)
	msg, err := renderMessage(entry)
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
	msg, err := renderMessage(entry)
	if err != nil {
		t.Fatalf("renderMessage: %v", err)
	}
	if msg != "🃏 **@alicewonder** traded in cards for 4 armies. (game `game-1`)" {
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
	_, err := renderMessage(entry)
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
	msg, err := renderMessage(entry)
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
	msg, err := renderMessage(entry)
	if err != nil {
		t.Fatalf("renderMessage: %v", err)
	}
	if msg != "⚔️ **@alicewonder** eliminated **@bobsmith**! (game `game-1`)" {
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
	msg, err := renderMessage(entry)
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
	msg, err := renderMessage(entry)
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
	msg, err := renderMessage(entry)
	if err != nil {
		t.Fatalf("renderMessage: %v", err)
	}
	if msg != "🏆 **@alicewonder** has won the game! (game `game-1`)" {
		t.Fatalf("unexpected message: %q", msg)
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
	entry := makeEntry("player_surrendered", `{"schema_version":1}`)
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

// --- allCommandDefs tests ---

func TestAllCommandDefs_Count(t *testing.T) {
	defs := allCommandDefs()
	if len(defs) != 4 {
		t.Errorf("expected 4 command defs, got %d", len(defs))
	}
}

func TestAllCommandDefs_Names(t *testing.T) {
	defs := allCommandDefs()
	names := make(map[string]bool, len(defs))
	for _, d := range defs {
		names[d.Name] = true
	}
	for _, want := range []string{pingCommandName, lastRollsCommandName, diceReportCommandName, playerStatsCommandName} {
		if !names[want] {
			t.Errorf("missing command def: %q", want)
		}
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
	if len(playerStats.Options) != 1 {
		t.Fatalf("expected 1 option, got %d", len(playerStats.Options))
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
	if len(lastRolls.Options) != 1 {
		t.Fatalf("expected 1 option, got %d", len(lastRolls.Options))
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
	out := formatLastRolls(nil)
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
	out := formatLastRolls(rolls)
	if !strings.Contains(out, "Alice") {
		t.Errorf("expected attacker name, got: %q", out)
	}
	if !strings.Contains(out, "Bob") {
		t.Errorf("expected defender name, got: %q", out)
	}
	if !strings.Contains(out, "#7") {
		t.Errorf("expected sequence number, got: %q", out)
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
	out := formatLastRolls(rolls)
	if !strings.Contains(out, "captured") {
		t.Errorf("expected capture indicator, got: %q", out)
	}
}

func TestFormatDiceReport_Empty(t *testing.T) {
	r := DiceReport{GameID: "g1", CombatRolls: 0}
	out := formatDiceReport(r)
	if !strings.Contains(out, "Dice Report") {
		t.Errorf("expected header, got: %q", out)
	}
	if !strings.Contains(out, "Combat rolls: 0") {
		t.Errorf("expected zero rolls, got: %q", out)
	}
	if !strings.Contains(out, "descriptive results") {
		t.Errorf("expected disclaimer, got: %q", out)
	}
}

func TestFormatPlayerReport_NoRolls(t *testing.T) {
	r := PlayerCombatReport{PlayerID: "p1", PlayerDisplayName: "Alice", AttackRolls: 0}
	out := formatPlayerReport(r)
	if !strings.Contains(out, "No attack rolls") {
		t.Errorf("expected no-rolls message, got: %q", out)
	}
	if !strings.Contains(out, "Alice") {
		t.Errorf("expected player name in message, got: %q", out)
	}
}

func TestFormatPlayerReport_WithRolls(t *testing.T) {
	r := PlayerCombatReport{
		PlayerID:               "p1",
		PlayerDisplayName:      "Alice",
		AttackRolls:            3,
		TerritoriesCaptured:    1,
		CaptureRate:            33.3,
		AttackerDiceRolled:     9,
		AverageAttackerDice:    3.0,
		AttackerLosses:         1,
		DefenderLossesInflicted: 3,
		AverageSourceArmiesBefore: 7.0,
		AverageTargetArmiesBefore: 2.0,
		AverageArmyAdvantage:    5.0,
	}
	out := formatPlayerReport(r)
	if !strings.Contains(out, "Alice") {
		t.Errorf("expected player name, got: %q", out)
	}
	if !strings.Contains(out, "Attack rolls: 3") {
		t.Errorf("expected attack roll count, got: %q", out)
	}
	if !strings.Contains(out, "33.3") {
		t.Errorf("expected capture rate, got: %q", out)
	}
}

// Expose reporting types for formatting tests (they live in the same package).
type RecentCombatRoll = reporting.RecentCombatRoll
type DiceReport = reporting.DiceReport
type PlayerCombatReport = reporting.PlayerCombatReport
type FaceDistribution = reporting.FaceDistribution
