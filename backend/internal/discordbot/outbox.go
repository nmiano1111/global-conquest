package discordbot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/nmiano1111/global-conquest/backend/internal/db"
	"github.com/nmiano1111/global-conquest/backend/internal/store"

	"github.com/bwmarrin/discordgo"
)

// MessageSender abstracts Discord message delivery for testability.
type MessageSender interface {
	SendMessage(ctx context.Context, channelID, content string, embeds ...*discordgo.MessageEmbed) error
}

// outboxClaimer is the store interface the worker needs.
type outboxClaimer interface {
	ClaimPendingBatch(ctx context.Context, limit int) ([]store.DiscordOutboxEntry, error)
	MarkDelivered(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id string, attempt int, errMsg string) error
}

// boundOutboxStore binds a PostgresDiscordOutboxStore with a DB for the worker.
type boundOutboxStore struct {
	s *store.PostgresDiscordOutboxStore
	d *db.DB
}

// ClaimPendingBatch claims up to limit pending outbox rows for delivery.
func (b *boundOutboxStore) ClaimPendingBatch(ctx context.Context, limit int) ([]store.DiscordOutboxEntry, error) {
	return b.s.ClaimPending(ctx, b.d, limit)
}

// MarkDelivered marks the outbox row with the given id as successfully delivered.
func (b *boundOutboxStore) MarkDelivered(ctx context.Context, id string) error {
	return b.s.MarkDelivered(ctx, b.d.Queryer(), id)
}

// MarkFailed records a failed delivery attempt for the outbox row with the
// given id, storing the attempt count and error message for diagnostics.
func (b *boundOutboxStore) MarkFailed(ctx context.Context, id string, attempt int, errMsg string) error {
	return b.s.MarkFailed(ctx, b.d.Queryer(), id, attempt, errMsg)
}

// NewBoundOutboxStore wraps the store with a database connection for use in the worker.
func NewBoundOutboxStore(s *store.PostgresDiscordOutboxStore, d *db.DB) outboxClaimer {
	return &boundOutboxStore{s: s, d: d}
}

// Worker polls discord_outbox and delivers pending notifications.
type Worker struct {
	store           outboxClaimer
	sender          MessageSender
	channelID       string
	frontendBaseURL string
	cancel          context.CancelFunc
	stopped         chan struct{}
}

// NewWorker constructs a Worker that claims pending rows from store and
// delivers them via sender to channelID, resolving game links against
// frontendBaseURL.
func NewWorker(store outboxClaimer, sender MessageSender, channelID, frontendBaseURL string) *Worker {
	return &Worker{
		store:           store,
		sender:          sender,
		channelID:       channelID,
		frontendBaseURL: frontendBaseURL,
		stopped:         make(chan struct{}),
	}
}

// Start begins polling and delivering outbox entries in a background
// goroutine, derived from ctx. Call Stop to shut it down.
func (w *Worker) Start(ctx context.Context) {
	ctx, w.cancel = context.WithCancel(ctx)
	go w.run(ctx)
}

// Stop cancels the worker's context and blocks until its run loop has exited.
func (w *Worker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	<-w.stopped
}

func (w *Worker) run(ctx context.Context) {
	defer close(w.stopped)
	log.Println("discord: outbox worker started")

	const batchSize = 10
	idlePoll := time.Second
	dbBackoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		if ctx.Err() != nil {
			log.Println("discord: outbox worker stopped")
			return
		}

		entries, err := w.store.ClaimPendingBatch(ctx, batchSize)
		if err != nil {
			if ctx.Err() != nil {
				log.Println("discord: outbox worker stopped")
				return
			}
			log.Printf("discord: outbox poll error (retry in %s): %v", dbBackoff, err)
			select {
			case <-ctx.Done():
				log.Println("discord: outbox worker stopped")
				return
			case <-time.After(dbBackoff):
			}
			if dbBackoff < maxBackoff {
				dbBackoff *= 2
				if dbBackoff > maxBackoff {
					dbBackoff = maxBackoff
				}
			}
			continue
		}
		dbBackoff = time.Second // reset on success

		log.Printf("discord: outbox claimed %d rows", len(entries))

		for _, entry := range entries {
			w.deliver(ctx, entry)
		}

		if len(entries) == batchSize {
			// Full batch — poll again immediately; more work may be waiting.
			continue
		}

		select {
		case <-ctx.Done():
			log.Println("discord: outbox worker stopped")
			return
		case <-time.After(idlePoll):
		}
	}
}

func (w *Worker) deliver(ctx context.Context, entry store.DiscordOutboxEntry) {
	msg, embeds, err := renderMessage(entry, w.frontendBaseURL)
	if err != nil {
		log.Printf("discord: notification %s (game=%s) render error: %v", entry.ID, entry.GameName, err)
		if markErr := w.store.MarkFailed(ctx, entry.ID, entry.AttemptCount, err.Error()); markErr != nil {
			log.Printf("discord: mark-failed error for %s: %v", entry.ID, markErr)
		}
		return
	}

	if err := w.sender.SendMessage(ctx, w.channelID, msg, embeds...); err != nil {
		log.Printf("discord: notification %s (game=%s) delivery failed (attempt %d): %v",
			entry.ID, entry.GameName, entry.AttemptCount, err)
		errStr := err.Error()
		if markErr := w.store.MarkFailed(ctx, entry.ID, entry.AttemptCount, errStr); markErr != nil {
			log.Printf("discord: mark-failed error for %s: %v", entry.ID, markErr)
		}
		return
	}

	log.Printf("discord: notification %s (game=%s) delivered (attempt %d)", entry.ID, entry.GameName, entry.AttemptCount)
	if markErr := w.store.MarkDelivered(ctx, entry.ID); markErr != nil {
		log.Printf("discord: mark-delivered error for %s: %v", entry.ID, markErr)
	}
}

// gameDiscordColor is the accent color used for the game-name link embed
// (Discord's "blurple").
const gameDiscordColor = 0x5865F2

// gameReference returns the message-content suffix and/or embed used to
// reference the game a notification belongs to.
//
// The "(game `name`)" text suffix is always included, same as before links
// existed. When frontendBaseURL is configured, a separate "Click to view
// game" embed is attached — Discord's plain message content does not render
// `[text](url)` markdown hyperlinks, so a clickable link requires an embed
// regardless of what the content text says.
func gameReference(entry store.DiscordOutboxEntry, frontendBaseURL string) (suffix string, embeds []*discordgo.MessageEmbed) {
	suffix = fmt.Sprintf(" (game `%s`)", entry.GameName)
	if frontendBaseURL == "" {
		return suffix, nil
	}
	url := frontendBaseURL + "/app/game/" + entry.GameID
	return suffix, []*discordgo.MessageEmbed{
		{
			Title: "Click to view game",
			URL:   url,
			Color: gameDiscordColor,
		},
	}
}

// mention returns a Discord @-mention for the player's linked Discord ID,
// falling back to their bolded display name when no Discord ID is linked.
func mention(displayName string, discordID *string) string {
	if discordID != nil && *discordID != "" {
		return fmt.Sprintf("<@%s>", *discordID)
	}
	return fmt.Sprintf("**%s**", displayName)
}

// renderMessage converts an outbox entry into Discord message content and,
// when a frontend URL is configured, an embed linking the game name.
func renderMessage(entry store.DiscordOutboxEntry, frontendBaseURL string) (string, []*discordgo.MessageEmbed, error) {
	gameSuffix, embeds := gameReference(entry, frontendBaseURL)
	switch entry.NotificationType {
	case store.NotificationTypeTurnStarted:
		var p store.TurnStartedPayload
		if err := json.Unmarshal(entry.Payload, &p); err != nil {
			return "", nil, fmt.Errorf("malformed turn_started payload (id=%s): %w", entry.ID, err)
		}
		if p.PlayerDisplayName == "" {
			return "", nil, fmt.Errorf("turn_started payload missing player_display_name (id=%s)", entry.ID)
		}
		prev := mention(p.PreviousPlayerDisplayName, p.PreviousPlayerDiscordName)
		curr := mention(p.PlayerDisplayName, p.PlayerDiscordName)
		return fmt.Sprintf("🎯 %s ended their turn. %s is up.%s", prev, curr, gameSuffix), embeds, nil
	case store.NotificationTypeCardsTrade:
		var p store.CardsTradePayload
		if err := json.Unmarshal(entry.Payload, &p); err != nil {
			return "", nil, fmt.Errorf("malformed cards_trade payload (id=%s): %w", entry.ID, err)
		}
		if p.PlayerDisplayName == "" {
			return "", nil, fmt.Errorf("cards_trade payload missing player_display_name (id=%s)", entry.ID)
		}
		return fmt.Sprintf("🃏 %s traded in cards for %d armies.%s", mention(p.PlayerDisplayName, p.PlayerDiscordName), p.Armies, gameSuffix), embeds, nil
	case store.NotificationTypePlayerEliminated:
		var p store.PlayerEliminatedPayload
		if err := json.Unmarshal(entry.Payload, &p); err != nil {
			return "", nil, fmt.Errorf("malformed player_eliminated payload (id=%s): %w", entry.ID, err)
		}
		if p.AttackerDisplayName == "" || p.EliminatedPlayerDisplayName == "" {
			return "", nil, fmt.Errorf("player_eliminated payload missing display name (id=%s)", entry.ID)
		}
		attacker := mention(p.AttackerDisplayName, p.AttackerDiscordName)
		eliminated := mention(p.EliminatedPlayerDisplayName, p.EliminatedPlayerDiscordName)
		return fmt.Sprintf("⚔️ %s eliminated %s!%s", attacker, eliminated, gameSuffix), embeds, nil
	case store.NotificationTypeGameOver:
		var p store.GameOverPayload
		if err := json.Unmarshal(entry.Payload, &p); err != nil {
			return "", nil, fmt.Errorf("malformed game_over payload (id=%s): %w", entry.ID, err)
		}
		if p.WinnerDisplayName == "" {
			return "", nil, fmt.Errorf("game_over payload missing winner_display_name (id=%s)", entry.ID)
		}
		return fmt.Sprintf("🏆 %s has won the game!%s", mention(p.WinnerDisplayName, p.WinnerDiscordName), gameSuffix), embeds, nil
	case store.NotificationTypeGameStarted:
		var p store.GameStartedPayload
		if err := json.Unmarshal(entry.Payload, &p); err != nil {
			return "", nil, fmt.Errorf("malformed game_started payload (id=%s): %w", entry.ID, err)
		}
		if p.PlayerDisplayName == "" {
			return "", nil, fmt.Errorf("game_started payload missing player_display_name (id=%s)", entry.ID)
		}
		return fmt.Sprintf("🚦 The game has begun! %s goes first.%s", mention(p.PlayerDisplayName, p.PlayerDiscordName), gameSuffix), embeds, nil
	default:
		return "", nil, fmt.Errorf("unknown notification type %q (id=%s)", entry.NotificationType, entry.ID)
	}
}
