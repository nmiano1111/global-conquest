package discordbot

import (
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

const (
	pingCommandName        = "ping"
	pingCommandDescription = "Check whether the Global Conquest bot is online"
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
