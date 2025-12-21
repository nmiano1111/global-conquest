package wsapi

import (
	"context"
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

// GinHandler returns a gin.HandlerFunc that upgrades to WS and runs a basic loop.
// This stays "transport-y" (upgrade, read/write, keepalive). Game logic should move out later.
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
		send := make(chan ServerMsg, opts.SendBuffer)
		go writeLoop(ctx, conn, send)

		// Initial hello (non-blocking)
		nonBlockingSend(send, ServerMsg{Type: "hello"})

		// Read loop
		for {
			var m ClientMsg
			if err := wsjson.Read(ctx, conn, &m); err != nil {
				if isNormalClose(err) {
					return
				}
				log.Printf("ws read error: %v", err)
				_ = conn.Close(websocket.StatusInternalError, "read error")
				return
			}

			// TODO: replace this switch with decoding -> domain commands -> server/hub
			switch m.Type {
			case "ping":
				nonBlockingSend(send, ServerMsg{Type: "pong"})
			default:
				nonBlockingSend(send, ServerMsg{Type: "ack"})
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

func writeLoop(ctx context.Context, conn *websocket.Conn, send <-chan ServerMsg) {
	// NOTE: we do NOT close(send) here. The producer side should close if needed.
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

func nonBlockingSend(ch chan<- ServerMsg, msg ServerMsg) {
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
