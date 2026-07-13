// Package wsconn implements the low-level websocket transport: connection
// accept/upgrade, a background keepalive ping loop, a single writer
// goroutine draining a buffered send channel (delivery is at-most-once — a
// full buffer silently drops the message), and a blocking JSON read loop.
package wsconn

import (
	"github.com/nmiano1111/global-conquest/backend/internal/proto/wsmsg"
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Conn is an accepted websocket connection with a background ping loop, a
// single writer goroutine serializing outbound sends from a buffered send
// channel, and a blocking read loop. Use Accept to create one.
type Conn struct {
	ws        *websocket.Conn
	send      chan wsmsg.Envelope
	done      chan struct{}
	closeOnce sync.Once
	label     string
}

// Options configures Accept.
type Options struct {
	// OriginPatterns lists allowed WebSocket origins, matched against the
	// request's Origin header.
	OriginPatterns []string
	// PingInterval is how often the keepalive ping loop pings the peer. If
	// <= 0, DefaultPingInterval is used.
	PingInterval time.Duration
	// SendBuffer is the capacity of the outbound send channel. If <= 0, a
	// default of 16 is used. Because delivery is at-most-once, a full
	// buffer causes Send to silently drop the message.
	SendBuffer int
	// Label identifies this connection in log output (e.g. the hub-assigned
	// client ID). Optional; logs fall back to "?" when unset.
	Label string
}

// DefaultPingInterval is the keepalive ping interval used when
// Options.PingInterval is unset or non-positive.
const DefaultPingInterval = 20 * time.Second

// Accept upgrades r into a websocket connection and returns a Conn with its
// background ping loop and single writer goroutine already running. Both
// goroutines exit when r's request context is done or the Conn is closed.
func Accept(w http.ResponseWriter, r *http.Request, opts Options) (*Conn, error) {
	if opts.PingInterval <= 0 {
		opts.PingInterval = DefaultPingInterval
	}
	if opts.SendBuffer <= 0 {
		opts.SendBuffer = 16
	}

	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: opts.OriginPatterns,
	})
	if err != nil {
		return nil, err
	}

	label := opts.Label
	if label == "" {
		label = "?"
	}

	c := &Conn{
		ws:    ws,
		send:  make(chan wsmsg.Envelope, opts.SendBuffer),
		done:  make(chan struct{}),
		label: label,
	}

	ctx := r.Context()

	// ping loop
	go func() {
		t := time.NewTicker(opts.PingInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.done:
				return
			case <-t.C:
				if err := ws.Ping(ctx); err != nil {
					log.Printf("ws: ping failed client=%s err=%v", c.label, err)
				}
			}
		}
	}()

	// writer loop (single writer)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.done:
				return
			case msg := <-c.send:
				if err := wsjson.Write(ctx, ws, msg); err != nil {
					log.Printf("ws: write failed client=%s type=%s err=%v", c.label, msg.Type, err)
				}
			}
		}
	}()

	return c, nil
}

// Close signals the writer goroutine to stop and closes the WebSocket.
// Safe to call multiple times.
func (c *Conn) Close(code websocket.StatusCode, reason string) error {
	c.closeOnce.Do(func() { close(c.done) })
	return c.ws.Close(code, reason)
}

// Send enqueues env for delivery. Returns false if the buffer is full or the
// connection is already closed. Never panics.
func (c *Conn) Send(env wsmsg.Envelope) bool {
	select {
	case <-c.done:
		log.Printf("ws: send dropped (closed) client=%s type=%s", c.label, env.Type)
		return false
	default:
	}
	select {
	case c.send <- env:
		return true
	default:
		log.Printf("ws: send dropped (buffer full, cap=%d) client=%s type=%s", cap(c.send), c.label, env.Type)
		return false
	}
}

// ReadLoop reads JSON envelopes and calls fn for each one.
// It returns when the connection closes or ctx is canceled.
func (c *Conn) ReadLoop(ctx context.Context, fn func(env wsmsg.Envelope)) error {
	for {
		var env wsmsg.Envelope
		if err := wsjson.Read(ctx, c.ws, &env); err != nil {
			return err
		}
		fn(env)
	}
}
