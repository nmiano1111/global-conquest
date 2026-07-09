package discordbot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"backend/internal/reporting"

	"github.com/bwmarrin/discordgo"
)

// reportingService is the interface the Bot uses to generate reports.
// *reporting.Service satisfies it; test fakes can satisfy it without a real DB.
type reportingService interface {
	ResolveGame(ctx context.Context, name string) (gameID, gameName string, err error)
	ResolvePlayer(ctx context.Context, identifier string) (playerID string, err error)
	CurrentPlayer(ctx context.Context, gameID string) (username string, discordName *string, err error)
	ActiveGameChoices(ctx context.Context, prefix string) ([]string, error)
	PlayerChoices(ctx context.Context, gameName, prefix string) ([]reporting.PlayerChoice, error)
	DiceReport(ctx context.Context, gameID string) (reporting.DiceReport, error)
	PlayerReport(ctx context.Context, gameID, playerID string) (reporting.PlayerCombatReport, error)
	RecentRolls(ctx context.Context, gameID string, count int) ([]reporting.RecentCombatRoll, error)
	RollStreakReport(ctx context.Context, gameID, gameName string, thresholds reporting.StreakThresholds) (reporting.RollStreakReport, error)
}

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
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		switch i.ApplicationCommandData().Name {
		case pingCommandName:
			b.handlePing(s, i)
		case lastRollsCommandName:
			b.handleLastRolls(s, i)
		case diceReportCommandName:
			b.handleDiceReport(s, i)
		case playerStatsCommandName:
			b.handlePlayerStats(s, i)
		case playerUpCommandName:
			b.handlePlayerUp(s, i)
		case rollStreaksCommandName:
			b.handleRollStreaks(s, i)
		}
	case discordgo.InteractionApplicationCommandAutocomplete:
		b.handleAutocomplete(s, i)
	}
}

func (b *Bot) handleAutocomplete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	data := i.ApplicationCommandData()
	for _, opt := range data.Options {
		if !opt.Focused {
			continue
		}
		switch opt.Name {
		case "game":
			b.respondGameAutocomplete(ctx, s, i, opt.StringValue())
		case "player":
			gameName := ""
			for _, o := range data.Options {
				if o.Name == "game" {
					gameName = o.StringValue()
				}
			}
			b.respondPlayerAutocomplete(ctx, s, i, gameName, opt.StringValue())
		}
		return
	}
}

func (b *Bot) respondGameAutocomplete(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate, prefix string) {
	names, err := b.reporting.ActiveGameChoices(ctx, prefix)
	if err != nil {
		log.Printf("discord: game autocomplete error: %v", err)
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionApplicationCommandAutocompleteResult,
			Data: &discordgo.InteractionResponseData{},
		})
		return
	}
	choices := make([]*discordgo.ApplicationCommandOptionChoice, len(names))
	for idx, name := range names {
		choices[idx] = &discordgo.ApplicationCommandOptionChoice{Name: name, Value: name}
	}
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionApplicationCommandAutocompleteResult,
		Data: &discordgo.InteractionResponseData{Choices: choices},
	})
}

func (b *Bot) respondPlayerAutocomplete(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate, gameName, prefix string) {
	players, err := b.reporting.PlayerChoices(ctx, gameName, prefix)
	if err != nil {
		log.Printf("discord: player autocomplete error: %v", err)
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionApplicationCommandAutocompleteResult,
			Data: &discordgo.InteractionResponseData{},
		})
		return
	}
	choices := make([]*discordgo.ApplicationCommandOptionChoice, len(players))
	for idx, p := range players {
		choices[idx] = &discordgo.ApplicationCommandOptionChoice{Name: p.Name, Value: p.Value}
	}
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionApplicationCommandAutocompleteResult,
		Data: &discordgo.InteractionResponseData{Choices: choices},
	})
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
	gameName := ""
	for _, o := range i.ApplicationCommandData().Options {
		switch o.Name {
		case "count":
			v := int(o.IntValue())
			if v < 1 {
				v = 1
			}
			if v > maxLastRollsCount {
				v = maxLastRollsCount
			}
			count = v
		case "game":
			gameName = o.StringValue()
		}
	}

	if err := deferResponse(s, i); err != nil {
		log.Printf("discord: /last-rolls defer error: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gameID, resolvedName, ok := b.resolveGame(ctx, s, i, gameName)
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
	editResponse(s, i, formatLastRolls(rolls, resolvedName))
}

func (b *Bot) handleDiceReport(s *discordgo.Session, i *discordgo.InteractionCreate) {
	gameName := ""
	for _, o := range i.ApplicationCommandData().Options {
		if o.Name == "game" {
			gameName = o.StringValue()
		}
	}

	if err := deferResponse(s, i); err != nil {
		log.Printf("discord: /dice-report defer error: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gameID, resolvedName, ok := b.resolveGame(ctx, s, i, gameName)
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
	editResponse(s, i, formatDiceReport(report, resolvedName))
}

func (b *Bot) handlePlayerStats(s *discordgo.Session, i *discordgo.InteractionCreate) {
	identifier := ""
	gameName := ""
	for _, o := range i.ApplicationCommandData().Options {
		switch o.Name {
		case "player":
			identifier = o.StringValue()
		case "game":
			gameName = o.StringValue()
		}
	}

	if err := deferResponse(s, i); err != nil {
		log.Printf("discord: /player-stats defer error: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gameID, resolvedName, ok := b.resolveGame(ctx, s, i, gameName)
	if !ok {
		return
	}

	playerID, ok := b.resolvePlayer(ctx, s, i, identifier)
	if !ok {
		return
	}

	report, err := b.reporting.PlayerReport(ctx, gameID, playerID)
	if err != nil {
		log.Printf("discord: /player-stats report error: %v", err)
		editResponse(s, i, "I couldn't generate that report.")
		return
	}
	editResponse(s, i, formatPlayerReport(report, resolvedName))
}

func (b *Bot) handlePlayerUp(s *discordgo.Session, i *discordgo.InteractionCreate) {
	gameName := ""
	for _, o := range i.ApplicationCommandData().Options {
		if o.Name == "game" {
			gameName = o.StringValue()
		}
	}

	if err := deferResponse(s, i); err != nil {
		log.Printf("discord: /player-up defer error: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gameID, resolvedName, ok := b.resolveGame(ctx, s, i, gameName)
	if !ok {
		return
	}

	username, discordName, err := b.reporting.CurrentPlayer(ctx, gameID)
	if err != nil {
		log.Printf("discord: /player-up current player error: %v", err)
		msg := "I couldn't determine whose turn it is."
		if errors.Is(err, reporting.ErrNoCurrentPlayer) {
			msg = fmt.Sprintf("No current player found for **%s**.", resolvedName)
		}
		editResponse(s, i, msg)
		return
	}

	playerRef := fmt.Sprintf("<@%s>", username)
	if discordName != nil {
		playerRef = fmt.Sprintf("<@%s>", *discordName)
	}
	editResponse(s, i, fmt.Sprintf("%s, play your turn in **%s**! ⚔️", playerRef, resolvedName))
}

func (b *Bot) handleRollStreaks(s *discordgo.Session, i *discordgo.InteractionCreate) {
	gameName := ""
	top := defaultStreakTopN
	thresholds := reporting.DefaultStreakThresholds()
	for _, o := range i.ApplicationCommandData().Options {
		switch o.Name {
		case "game":
			gameName = o.StringValue()
		case "top":
			top = min(max(int(o.IntValue()), 1), maxStreakTopN)
		case "min-loss-streak":
			thresholds.MinLossStreakLength = int(o.IntValue())
		case "min-win-streak":
			thresholds.MinWinStreakLength = int(o.IntValue())
		case "min-drought":
			thresholds.MinDroughtLength = int(o.IntValue())
		}
	}

	if err := deferResponse(s, i); err != nil {
		log.Printf("discord: /roll-streaks defer error: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	gameID, resolvedName, ok := b.resolveGame(ctx, s, i, gameName)
	if !ok {
		return
	}

	report, err := b.reporting.RollStreakReport(ctx, gameID, resolvedName, thresholds)
	if err != nil {
		log.Printf("discord: /roll-streaks report error: %v", err)
		msg := "I couldn't generate that report."
		if errors.Is(err, reporting.ErrNoEvents) {
			msg = "No combat events found for this game yet."
		}
		editResponse(s, i, msg)
		return
	}
	editResponse(s, i, formatRollStreaks(report, resolvedName, top))
}

// resolveGame fetches the game ID and canonical name for the given name string
// (empty = most recent active game). Edits the deferred response on failure.
func (b *Bot) resolveGame(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate, name string) (gameID, gameName string, ok bool) {
	gid, gname, err := b.reporting.ResolveGame(ctx, name)
	if err != nil {
		log.Printf("discord: resolveGame error: %v", err)
		var msg string
		switch {
		case errors.Is(err, reporting.ErrGameNotFound):
			msg = fmt.Sprintf("No game named %q found. Check the name and try again.", name)
		case errors.Is(err, reporting.ErrNoActiveGame):
			msg = "No active game found. Start a game first!"
		default:
			msg = "I couldn't look up the current game."
		}
		editResponse(s, i, msg)
		return "", "", false
	}
	return gid, gname, true
}

// resolvePlayer maps a username or UUID to a player UUID.
// Edits the deferred response on failure.
func (b *Bot) resolvePlayer(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate, identifier string) (playerID string, ok bool) {
	pid, err := b.reporting.ResolvePlayer(ctx, identifier)
	if err != nil {
		log.Printf("discord: resolvePlayer error: %v", err)
		msg := fmt.Sprintf("No player %q found.", identifier)
		if !errors.Is(err, reporting.ErrPlayerNotFound) {
			msg = "I couldn't look up that player."
		}
		editResponse(s, i, msg)
		return "", false
	}
	return pid, true
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
