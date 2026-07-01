package discordbot

import (
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

type Bot struct {
	session *discordgo.Session
	cfg     Config
}

func New(cfg Config) (*Bot, error) {
	s, err := discordgo.New("Bot " + cfg.BotToken)
	if err != nil {
		return nil, fmt.Errorf("create discord session: %w", err)
	}
	s.Identify.Intents = discordgo.IntentsGuilds
	return &Bot{session: s, cfg: cfg}, nil
}

func (b *Bot) Start() error {
	b.session.AddHandler(b.handleInteraction)

	if err := b.session.Open(); err != nil {
		return fmt.Errorf("open gateway connection: %w", err)
	}
	log.Println("discord: gateway connected")

	if err := ensurePingCommand(b.session, b.cfg.ApplicationID, b.cfg.GuildID); err != nil {
		_ = b.session.Close()
		return err
	}

	if _, err := b.session.ChannelMessageSend(b.cfg.EventsChannelID, "Global Conquest bot connected successfully."); err != nil {
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

func (b *Bot) handleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	if i.ApplicationCommandData().Name != pingCommandName {
		return
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: "Pong"},
	}); err != nil {
		log.Printf("discord: /ping respond error: %v", err)
	}
}
