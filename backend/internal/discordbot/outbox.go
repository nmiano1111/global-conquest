package discordbot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"backend/internal/db"
	"backend/internal/store"
)

// MessageSender abstracts Discord message delivery for testability.
type MessageSender interface {
	SendMessage(ctx context.Context, channelID, content string) error
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

func (b *boundOutboxStore) ClaimPendingBatch(ctx context.Context, limit int) ([]store.DiscordOutboxEntry, error) {
	return b.s.ClaimPending(ctx, b.d, limit)
}

func (b *boundOutboxStore) MarkDelivered(ctx context.Context, id string) error {
	return b.s.MarkDelivered(ctx, b.d.Queryer(), id)
}

func (b *boundOutboxStore) MarkFailed(ctx context.Context, id string, attempt int, errMsg string) error {
	return b.s.MarkFailed(ctx, b.d.Queryer(), id, attempt, errMsg)
}

// NewBoundOutboxStore wraps the store with a database connection for use in the worker.
func NewBoundOutboxStore(s *store.PostgresDiscordOutboxStore, d *db.DB) outboxClaimer {
	return &boundOutboxStore{s: s, d: d}
}

// Worker polls discord_outbox and delivers pending notifications.
type Worker struct {
	store     outboxClaimer
	sender    MessageSender
	channelID string
	cancel    context.CancelFunc
	stopped   chan struct{}
}

func NewWorker(store outboxClaimer, sender MessageSender, channelID string) *Worker {
	return &Worker{
		store:     store,
		sender:    sender,
		channelID: channelID,
		stopped:   make(chan struct{}),
	}
}

func (w *Worker) Start(ctx context.Context) {
	ctx, w.cancel = context.WithCancel(ctx)
	go w.run(ctx)
}

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
	msg, err := renderMessage(entry)
	if err != nil {
		log.Printf("discord: notification %s (game=%s) render error: %v", entry.ID, entry.GameID, err)
		if markErr := w.store.MarkFailed(ctx, entry.ID, entry.AttemptCount, err.Error()); markErr != nil {
			log.Printf("discord: mark-failed error for %s: %v", entry.ID, markErr)
		}
		return
	}

	if err := w.sender.SendMessage(ctx, w.channelID, msg); err != nil {
		log.Printf("discord: notification %s (game=%s) delivery failed (attempt %d): %v",
			entry.ID, entry.GameID, entry.AttemptCount, err)
		errStr := err.Error()
		if markErr := w.store.MarkFailed(ctx, entry.ID, entry.AttemptCount, errStr); markErr != nil {
			log.Printf("discord: mark-failed error for %s: %v", entry.ID, markErr)
		}
		return
	}

	log.Printf("discord: notification %s (game=%s) delivered (attempt %d)", entry.ID, entry.GameID, entry.AttemptCount)
	if markErr := w.store.MarkDelivered(ctx, entry.ID); markErr != nil {
		log.Printf("discord: mark-delivered error for %s: %v", entry.ID, markErr)
	}
}

// renderMessage converts an outbox entry into a Discord message string.
func renderMessage(entry store.DiscordOutboxEntry) (string, error) {
	switch entry.NotificationType {
	case store.NotificationTypeTurnStarted:
		var p store.TurnStartedPayload
		if err := json.Unmarshal(entry.Payload, &p); err != nil {
			return "", fmt.Errorf("malformed turn_started payload (id=%s): %w", entry.ID, err)
		}
		if p.PlayerDisplayName == "" {
			return "", fmt.Errorf("turn_started payload missing player_display_name (id=%s)", entry.ID)
		}
		if p.PreviousPlayerDiscordName != nil && *p.PreviousPlayerDiscordName != "" &&
			p.PlayerDiscordName != nil && *p.PlayerDiscordName != "" {
			return fmt.Sprintf("🎯 **@%s** ended their turn. **@%s** is up. (game `%s`)", *p.PreviousPlayerDiscordName, *p.PlayerDiscordName, entry.GameID), nil
		}
		return fmt.Sprintf("@everyone 🎯 **%s** ended their turn. **%s** is up. (game `%s`)", p.PreviousPlayerDisplayName, p.PlayerDisplayName, entry.GameID), nil
	case store.NotificationTypeCardsTrade:
		var p store.CardsTradePayload
		if err := json.Unmarshal(entry.Payload, &p); err != nil {
			return "", fmt.Errorf("malformed cards_trade payload (id=%s): %w", entry.ID, err)
		}
		if p.PlayerDisplayName == "" {
			return "", fmt.Errorf("cards_trade payload missing player_display_name (id=%s)", entry.ID)
		}
		if p.PlayerDiscordName != nil && *p.PlayerDiscordName != "" {
			return fmt.Sprintf("🃏 **@%s** traded in cards for %d armies. (game `%s`)", *p.PlayerDiscordName, p.Armies, entry.GameID), nil
		}
		return fmt.Sprintf("@everyone 🃏 **%s** traded in cards for %d armies. (game `%s`)", p.PlayerDisplayName, p.Armies, entry.GameID), nil
	default:
		return "", fmt.Errorf("unknown notification type %q (id=%s)", entry.NotificationType, entry.ID)
	}
}
