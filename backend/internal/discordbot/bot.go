package discordbot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"time"

	"backend/internal/reporting"

	"github.com/bwmarrin/discordgo"
)

// reportingService is the interface the Bot uses to generate reports.
// *reporting.Service satisfies it; test fakes can satisfy it without a real DB.
type reportingService interface {
	LatestGameID(ctx context.Context) (string, error)
	DiceReport(ctx context.Context, gameID string) (reporting.DiceReport, error)
	PlayerReport(ctx context.Context, gameID, playerID string) (reporting.PlayerCombatReport, error)
	RecentRolls(ctx context.Context, gameID string, count int) ([]reporting.RecentCombatRoll, error)
}

// uuidRegexp matches a standard lower-case UUID.
var uuidRegexp = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Bot struct {
	session   *discordgo.Session
	cfg       Config
	reporting reportingService
}

func New(cfg Config, svc reportingService) (*Bot, error) {
	s, err := discordgo.New("Bot " + cfg.BotToken)
	if err != nil {
		return nil, fmt.Errorf("create discord session: %w", err)
	}
	s.Identify.Intents = discordgo.IntentsGuilds
	return &Bot{session: s, cfg: cfg, reporting: svc}, nil
}

func (b *Bot) Start() error {
	b.session.AddHandler(b.handleInteraction)

	if err := b.session.Open(); err != nil {
		return fmt.Errorf("open gateway connection: %w", err)
	}
	log.Println("discord: gateway connected")

	if err := ensureGuildCommands(b.session, b.cfg.ApplicationID, b.cfg.GuildID); err != nil {
		_ = b.session.Close()
		return err
	}

	if _, err := b.session.ChannelMessageSend(b.cfg.EventsChannelID, "Game update released."); err != nil {
		_ = b.session.Close()
		return fmt.Errorf("send startup message: %w", err)
	}
	log.Println("discord: startup message sent")

	return nil
}

func (b *Bot) Close() error {
	if err := b.session.Close(); err != nil {
		return fmt.Errorf("close discord session: %w", err)
	}
	return nil
}

// NewMessageSender returns a MessageSender backed by this bot's Discord session.
func (b *Bot) NewMessageSender() MessageSender {
	return &discordMessageSender{session: b.session}
}

type discordMessageSender struct {
	session *discordgo.Session
}

func (s *discordMessageSender) SendMessage(_ context.Context, channelID, content string) error {
	_, err := s.session.ChannelMessageSend(channelID, content)
	return err
}

func (b *Bot) handleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	switch i.ApplicationCommandData().Name {
	case pingCommandName:
		b.handlePing(s, i)
	case lastRollsCommandName:
		b.handleLastRolls(s, i)
	case diceReportCommandName:
		b.handleDiceReport(s, i)
	case playerStatsCommandName:
		b.handlePlayerStats(s, i)
	}
}

func (b *Bot) handlePing(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: "Pong"},
	}); err != nil {
		log.Printf("discord: /ping respond error: %v", err)
	}
}

func (b *Bot) handleLastRolls(s *discordgo.Session, i *discordgo.InteractionCreate) {
	count := defaultLastRollsCount
	for _, o := range i.ApplicationCommandData().Options {
		if o.Name == "count" {
			v := int(o.IntValue())
			if v < 1 {
				v = 1
			}
			if v > maxLastRollsCount {
				v = maxLastRollsCount
			}
			count = v
		}
	}

	if err := deferResponse(s, i); err != nil {
		log.Printf("discord: /last-rolls defer error: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gameID, ok := b.resolveGameID(ctx, s, i)
	if !ok {
		return
	}

	rolls, err := b.reporting.RecentRolls(ctx, gameID, count)
	if err != nil {
		log.Printf("discord: /last-rolls report error: %v", err)
		msg := "I couldn't generate that report."
		if errors.Is(err, reporting.ErrNoEvents) {
			msg = "No combat events found for this game yet."
		}
		editResponse(s, i, msg)
		return
	}
	editResponse(s, i, formatLastRolls(rolls))
}

func (b *Bot) handleDiceReport(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if err := deferResponse(s, i); err != nil {
		log.Printf("discord: /dice-report defer error: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gameID, ok := b.resolveGameID(ctx, s, i)
	if !ok {
		return
	}

	report, err := b.reporting.DiceReport(ctx, gameID)
	if err != nil {
		log.Printf("discord: /dice-report report error: %v", err)
		msg := "I couldn't generate that report."
		if errors.Is(err, reporting.ErrNoEvents) {
			msg = "No combat events found for this game yet."
		}
		editResponse(s, i, msg)
		return
	}
	editResponse(s, i, formatDiceReport(report))
}

func (b *Bot) handlePlayerStats(s *discordgo.Session, i *discordgo.InteractionCreate) {
	playerID := ""
	for _, o := range i.ApplicationCommandData().Options {
		if o.Name == "player" {
			playerID = o.StringValue()
		}
	}

	// Validate UUID format before deferring so we can respond ephemerally.
	if !uuidRegexp.MatchString(playerID) {
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("Invalid player UUID %q. Use `/player-stats player:<uuid>`.", playerID),
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	if err := deferResponse(s, i); err != nil {
		log.Printf("discord: /player-stats defer error: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gameID, ok := b.resolveGameID(ctx, s, i)
	if !ok {
		return
	}

	report, err := b.reporting.PlayerReport(ctx, gameID, playerID)
	if err != nil {
		log.Printf("discord: /player-stats report error: %v", err)
		editResponse(s, i, "I couldn't generate that report.")
		return
	}
	editResponse(s, i, formatPlayerReport(report))
}

// resolveGameID fetches the latest active game ID, editing the interaction
// response with a user-facing error if none is found. Returns ("", false) on failure.
func (b *Bot) resolveGameID(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) (string, bool) {
	gameID, err := b.reporting.LatestGameID(ctx)
	if err != nil {
		log.Printf("discord: resolveGameID error: %v", err)
		msg := "No active game found. Start a game first!"
		if !errors.Is(err, reporting.ErrNoActiveGame) {
			msg = "I couldn't look up the current game."
		}
		editResponse(s, i, msg)
		return "", false
	}
	return gameID, true
}

// deferResponse sends a deferred (non-ephemeral) channel-message response.
func deferResponse(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	return s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
}

// editResponse fills in a previously deferred interaction response.
func editResponse(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	if _, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &content,
	}); err != nil {
		log.Printf("discord: edit interaction response error: %v", err)
	}
}
