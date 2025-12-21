package wsapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/gin-gonic/gin"
)

const DefaultPingInterval = 20 * time.Second

type Options struct {
	OriginPatterns []string
	PingInterval   time.Duration
	SendBuffer     int
}

// Envelope is the single wire format used for both client->server and server->client messages.
type Envelope struct {
	Type          string          `json:"type"`
	ID            string          `json:"id,omitempty"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	GameID        string          `json:"game_id,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

// GinHandler upgrades to WS and runs a basic transport loop.
// It reads/writes Envelopes only; game logic should live elsewhere.
func GinHandler(opts Options) gin.HandlerFunc {
	// sane defaults
	if opts.PingInterval <= 0 {
		opts.PingInterval = DefaultPingInterval
	}
	if opts.SendBuffer <= 0 {
		opts.SendBuffer = 16
	}

	return func(c *gin.Context) {
		// IMPORTANT: unwrap gin's ResponseWriter so Hijack works properly
		w := unwrapWriter(c.Writer)

		conn, err := websocket.Accept(w, c.Request, &websocket.AcceptOptions{
			OriginPatterns: opts.OriginPatterns,
		})
		if err != nil {
			// Accept generally writes the HTTP error response already
			return
		}

		// Ensure gin doesn't try to write anything else
		c.Abort()

		defer conn.Close(websocket.StatusNormalClosure, "bye")

		ctx := c.Request.Context()

		// Keepalive ping loop
		pingCtx, cancelPing := context.WithCancel(ctx)
		defer cancelPing()
		go pingLoop(pingCtx, conn, opts.PingInterval)

		// Single-writer loop using a buffered channel (backpressure by drop)
		send := make(chan Envelope, opts.SendBuffer)
		go writeLoop(ctx, conn, send)

		// Initial hello (non-blocking)
		nonBlockingSend(send, Envelope{Type: "hello"})

		// Read loop: read Envelopes, do minimal demo responses.
		for {
			var env Envelope
			if err := wsjson.Read(ctx, conn, &env); err != nil {
				if isNormalClose(err) {
					return
				}
				log.Printf("ws read error: %v", err)
				_ = conn.Close(websocket.StatusInternalError, "read error")
				return
			}

			// TODO: replace this with decoding -> domain commands -> server/hub.
			// For now, keep a tiny demo: reply to "ping", ack everything else.
			switch env.Type {
			case "ping":
				nonBlockingSend(send, Envelope{
					Type:          "pong",
					CorrelationID: env.ID,
					GameID:        env.GameID,
				})
			default:
				nonBlockingSend(send, Envelope{
					Type:          "ack",
					CorrelationID: env.ID,
					GameID:        env.GameID,
				})
			}
		}
	}
}

func unwrapWriter(w http.ResponseWriter) http.ResponseWriter {
	// Gin's ResponseWriter wraps the underlying writer; websocket.Accept needs a hijackable writer.
	rw := w
	if uw, ok := any(w).(interface{ Unwrap() http.ResponseWriter }); ok {
		rw = uw.Unwrap()
	}
	return rw
}

func pingLoop(ctx context.Context, conn *websocket.Conn, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = conn.Ping(ctx)
		}
	}
}

func writeLoop(ctx context.Context, conn *websocket.Conn, send <-chan Envelope) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-send:
			if !ok {
				return
			}
			if err := wsjson.Write(ctx, conn, msg); err != nil {
				return
			}
		}
	}
}

func nonBlockingSend(ch chan<- Envelope, msg Envelope) {
	select {
	case ch <- msg:
	default:
		// drop when buffer is full
	}
}

func isNormalClose(err error) bool {
	var closeErr websocket.CloseError
	if errors.As(err, &closeErr) {
		return true
	}
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
{"type":"ack","correlation_id":"c2"}