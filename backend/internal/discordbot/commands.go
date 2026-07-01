package discordbot

import (
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

const (
	pingCommandName        = "ping"
	pingCommandDescription = "Check whether the Global Conquest bot is online"

	lastRollsCommandName        = "last-rolls"
	lastRollsCommandDescription = "Show the most recent combat rolls"

	diceReportCommandName        = "dice-report"
	diceReportCommandDescription = "Show combat dice statistics for the current game"

	playerStatsCommandName        = "player-stats"
	playerStatsCommandDescription = "Show combat statistics for a player"

	defaultLastRollsCount = 5
	maxLastRollsCount     = 20
)

type commandAction int

const (
	commandCreate commandAction = iota
	commandReuse
	commandUpdate
)

type pingDecision struct {
	action     commandAction
	existingID string
}

// decidePingAction inspects the existing guild command list and returns what
// registration action is needed for /ping.
func decidePingAction(existing []*discordgo.ApplicationCommand) pingDecision {
	for _, cmd := range existing {
		if cmd.Name == pingCommandName {
			if cmd.Description == pingCommandDescription {
				return pingDecision{action: commandReuse, existingID: cmd.ID}
			}
			return pingDecision{action: commandUpdate, existingID: cmd.ID}
		}
	}
	return pingDecision{action: commandCreate}
}

func ensurePingCommand(s *discordgo.Session, appID, guildID string) error {
	existing, err := s.ApplicationCommands(appID, guildID)
	if err != nil {
		return fmt.Errorf("list guild commands: %w", err)
	}

	def := &discordgo.ApplicationCommand{
		Name:        pingCommandName,
		Description: pingCommandDescription,
	}

	dec := decidePingAction(existing)
	switch dec.action {
	case commandReuse:
		log.Printf("discord: /ping already registered (id=%s), reusing", dec.existingID)
		return nil
	case commandUpdate:
		if _, err := s.ApplicationCommandEdit(appID, guildID, dec.existingID, def); err != nil {
			return fmt.Errorf("update /ping command: %w", err)
		}
		log.Printf("discord: /ping updated (id=%s)", dec.existingID)
		return nil
	default: // commandCreate
		cmd, err := s.ApplicationCommandCreate(appID, guildID, def)
		if err != nil {
			return fmt.Errorf("create /ping command: %w", err)
		}
		log.Printf("discord: /ping registered (id=%s)", cmd.ID)
		return nil
	}
}

// decideCommandAction returns the registration action needed for a single command
// definition against the set of existing guild commands.
func decideCommandAction(existing []*discordgo.ApplicationCommand, def *discordgo.ApplicationCommand) pingDecision {
	for _, cmd := range existing {
		if cmd.Name != def.Name {
			continue
		}
		if commandsMatch(cmd, def) {
			return pingDecision{action: commandReuse, existingID: cmd.ID}
		}
		return pingDecision{action: commandUpdate, existingID: cmd.ID}
	}
	return pingDecision{action: commandCreate}
}

// commandsMatch returns true when two command definitions have the same description
// and the same option names, types, and required flags.
func commandsMatch(a, b *discordgo.ApplicationCommand) bool {
	if a.Description != b.Description {
		return false
	}
	if len(a.Options) != len(b.Options) {
		return false
	}
	for i, opt := range b.Options {
		ea := a.Options[i]
		if ea.Name != opt.Name || ea.Type != opt.Type || ea.Required != opt.Required {
			return false
		}
	}
	return true
}

// allCommandDefs returns the definitions for every guild-scoped slash command.
// Used by ensureGuildCommands and tests.
func allCommandDefs() []*discordgo.ApplicationCommand {
	minCount := float64(1)
	maxCount := float64(maxLastRollsCount)
	return []*discordgo.ApplicationCommand{
		{
			Name:        pingCommandName,
			Description: pingCommandDescription,
		},
		{
			Name:        lastRollsCommandName,
			Description: lastRollsCommandDescription,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "count",
					Description: fmt.Sprintf("Number of rolls to show (default %d, max %d)", defaultLastRollsCount, maxLastRollsCount),
					Required:    false,
					MinValue:    &minCount,
					MaxValue:    maxCount,
				},
			},
		},
		{
			Name:        diceReportCommandName,
			Description: diceReportCommandDescription,
		},
		{
			Name:        playerStatsCommandName,
			Description: playerStatsCommandDescription,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "player",
					Description: "Global Conquest player UUID",
					Required:    true,
				},
			},
		},
	}
}

// ensureGuildCommands reconciles all command definitions against what is currently
// registered for the guild, issuing create or update calls only when needed.
func ensureGuildCommands(s *discordgo.Session, appID, guildID string) error {
	existing, err := s.ApplicationCommands(appID, guildID)
	if err != nil {
		return fmt.Errorf("list guild commands: %w", err)
	}
	for _, def := range allCommandDefs() {
		dec := decideCommandAction(existing, def)
		switch dec.action {
		case commandReuse:
			log.Printf("discord: /%s already registered (id=%s), reusing", def.Name, dec.existingID)
		case commandUpdate:
			if _, err := s.ApplicationCommandEdit(appID, guildID, dec.existingID, def); err != nil {
				return fmt.Errorf("update /%s command: %w", def.Name, err)
			}
			log.Printf("discord: /%s updated (id=%s)", def.Name, dec.existingID)
		default: // commandCreate
			cmd, err := s.ApplicationCommandCreate(appID, guildID, def)
			if err != nil {
				return fmt.Errorf("create /%s command: %w", def.Name, err)
			}
			log.Printf("discord: /%s registered (id=%s)", def.Name, cmd.ID)
		}
	}
	return nil
}
