package discordbot

import (
	"fmt"
	"strings"

	"backend/internal/trello"

	"github.com/bwmarrin/discordgo"
)

// ticketType identifies which kind of Trello card a modal submission creates.
type ticketType string

const (
	ticketTypeBug     ticketType = "bug"
	ticketTypeFeature ticketType = "feature"
)

// ticketModalCustomIDPrefix namespaces modal custom IDs so handleModalSubmit
// can recognize ticket modals (as opposed to any other modal the bot might
// grow in the future) and ignore anything it doesn't recognize.
const ticketModalCustomIDPrefix = "ticket_modal:"

// Modal field custom IDs. Shared between modal construction (bugReportModal /
// featureRequestModal) and field extraction (extractModalFields) so the two
// stay in sync.
const (
	fieldTitle             = "title"
	fieldWhatHappened      = "what_happened"
	fieldExpectedBehavior  = "expected_behavior"
	fieldStepsToReproduce  = "steps_to_reproduce"
	fieldWhatShouldBeAdded = "what_should_be_added"
	fieldWhyUseful         = "why_useful"
)

const (
	ticketTitleMaxLength = 100
	ticketBodyMaxLength  = 1000
)

// ticketModalCustomID builds the modal custom ID that encodes the ticket
// type, e.g. "ticket_modal:bug".
func ticketModalCustomID(t ticketType) string {
	return ticketModalCustomIDPrefix + string(t)
}

// parseTicketType extracts the ticket type from a modal custom ID. ok is
// false if the custom ID isn't a ticket modal (some other modal) or names an
// unrecognized type — callers should silently ignore it in either case
// rather than error, since new unrelated modals may be added later.
func parseTicketType(customID string) (t ticketType, ok bool) {
	raw, found := strings.CutPrefix(customID, ticketModalCustomIDPrefix)
	if !found {
		return "", false
	}
	switch ticketType(raw) {
	case ticketTypeBug, ticketTypeFeature:
		return ticketType(raw), true
	default:
		return "", false
	}
}

// bugReportModal builds the modal shown in response to /bug.
func bugReportModal() *discordgo.InteractionResponseData {
	return &discordgo.InteractionResponseData{
		CustomID: ticketModalCustomID(ticketTypeBug),
		Title:    "Report a Bug",
		Components: []discordgo.MessageComponent{
			textInputRow(fieldTitle, "Title", discordgo.TextInputShort, true, ticketTitleMaxLength),
			textInputRow(fieldWhatHappened, "What happened?", discordgo.TextInputParagraph, true, ticketBodyMaxLength),
			textInputRow(fieldExpectedBehavior, "Expected behavior", discordgo.TextInputParagraph, true, ticketBodyMaxLength),
			textInputRow(fieldStepsToReproduce, "Steps to reproduce", discordgo.TextInputParagraph, false, ticketBodyMaxLength),
		},
	}
}

// featureRequestModal builds the modal shown in response to /feature.
func featureRequestModal() *discordgo.InteractionResponseData {
	return &discordgo.InteractionResponseData{
		CustomID: ticketModalCustomID(ticketTypeFeature),
		Title:    "Request a Feature",
		Components: []discordgo.MessageComponent{
			textInputRow(fieldTitle, "Title", discordgo.TextInputShort, true, ticketTitleMaxLength),
			textInputRow(fieldWhatShouldBeAdded, "What should be added?", discordgo.TextInputParagraph, true, ticketBodyMaxLength),
			textInputRow(fieldWhyUseful, "Why would this be useful?", discordgo.TextInputParagraph, false, ticketBodyMaxLength),
		},
	}
}

func textInputRow(customID, label string, style discordgo.TextInputStyle, required bool, maxLength int) discordgo.MessageComponent {
	return discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.TextInput{
				CustomID:  customID,
				Label:     label,
				Style:     style,
				Required:  required,
				MaxLength: maxLength,
			},
		},
	}
}

// extractModalFields flattens a modal submission's action rows into a
// customID -> value map. Components that aren't text inputs inside action
// rows (there aren't any in our modals, but Discord's type is general) are
// silently skipped.
func extractModalFields(data discordgo.ModalSubmitInteractionData) map[string]string {
	fields := make(map[string]string, len(data.Components))
	for _, comp := range data.Components {
		row, ok := comp.(*discordgo.ActionsRow)
		if !ok {
			continue
		}
		for _, inner := range row.Components {
			input, ok := inner.(*discordgo.TextInput)
			if !ok {
				continue
			}
			fields[input.CustomID] = input.Value
		}
	}
	return fields
}

// ticketReporter carries the Discord identity of whoever submitted a ticket.
type ticketReporter struct {
	DisplayName string
	UserID      string
	GuildID     string
	ChannelID   string
}

// ticketReporterFromInteraction extracts reporter identity from a modal
// submit interaction. Guild-installed slash commands populate i.Member; a
// nil Member (e.g. DM context) falls back to i.User.
func ticketReporterFromInteraction(i *discordgo.Interaction) ticketReporter {
	var userID, displayName string
	switch {
	case i.Member != nil && i.Member.User != nil:
		userID = i.Member.User.ID
		displayName = i.Member.User.Username
		if i.Member.Nick != "" {
			displayName = i.Member.Nick
		}
	case i.User != nil:
		userID = i.User.ID
		displayName = i.User.Username
	}
	return ticketReporter{
		DisplayName: displayName,
		UserID:      userID,
		GuildID:     i.GuildID,
		ChannelID:   i.ChannelID,
	}
}

// ticketCardTitle formats a Trello card title, e.g. "[bug] Login button is broken".
func ticketCardTitle(t ticketType, title string) string {
	return fmt.Sprintf("[%s] %s", t, title)
}

// ticketCardDescription formats a Trello card description with reporter
// metadata followed by the submitted modal fields, in the field order the
// corresponding modal presented them.
func ticketCardDescription(t ticketType, reporter ticketReporter, fields map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Reported by: %s\n", reporter.DisplayName)
	fmt.Fprintf(&b, "Discord user ID: %s\n", reporter.UserID)
	fmt.Fprintf(&b, "Discord guild ID: %s\n", reporter.GuildID)
	fmt.Fprintf(&b, "Discord channel ID: %s\n", reporter.ChannelID)
	fmt.Fprintf(&b, "Ticket type: %s\n", t)
	fmt.Fprintf(&b, "Source: Discord /%s command\n", t)

	switch t {
	case ticketTypeBug:
		fmt.Fprintf(&b, "\nWhat happened:\n%s\n", fields[fieldWhatHappened])
		fmt.Fprintf(&b, "\nExpected behavior:\n%s\n", fields[fieldExpectedBehavior])
		fmt.Fprintf(&b, "\nSteps to reproduce:\n%s\n", fields[fieldStepsToReproduce])
	case ticketTypeFeature:
		fmt.Fprintf(&b, "\nWhat should be added:\n%s\n", fields[fieldWhatShouldBeAdded])
		fmt.Fprintf(&b, "\nWhy would this be useful:\n%s\n", fields[fieldWhyUseful])
	}

	return strings.TrimRight(b.String(), "\n")
}

// trelloLabelIDFor returns the configured Trello label ID for a ticket type.
func trelloLabelIDFor(t ticketType, cfg Config) string {
	switch t {
	case ticketTypeBug:
		return cfg.TrelloBugLabelID
	case ticketTypeFeature:
		return cfg.TrelloFeatureLabelID
	default:
		return ""
	}
}

// labelIDsOrEmpty wraps a possibly-empty label ID into the slice shape
// trello.CreateCardInput wants.
func labelIDsOrEmpty(labelID string) []string {
	if labelID == "" {
		return nil
	}
	return []string{labelID}
}

// ticketSubmitResultMessage is the ephemeral message shown to the user after
// a Trello create-card attempt. Kept separate from the Discord/Trello I/O so
// it's testable with plain values.
func ticketSubmitResultMessage(t ticketType, card *trello.CreatedCard, err error) string {
	if err != nil {
		return fmt.Sprintf("Sorry, I couldn't create that %s report. Please try again or contact an admin.", t)
	}
	return fmt.Sprintf("Created %s report: %s", t, card.URL)
}

// ticketCardName returns the submitted title, or a fallback if it was left
// blank (Discord's Required flag should prevent this, but modal validation
// is enforced client-side, not guaranteed).
func ticketCardName(t ticketType, fields map[string]string) string {
	title := strings.TrimSpace(fields[fieldTitle])
	if title == "" {
		title = fmt.Sprintf("Untitled %s report", t)
	}
	return ticketCardTitle(t, title)
}
