package wsconn

import (
	"backend/internal/proto/wsmsg"
	"context"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type Conn struct {
	ws   *websocket.Conn
	send chan wsmsg.Envelope
}

type Options struct {
	OriginPatterns []string
	PingInterval   time.Duration
	SendBuffer     int
}

const DefaultPingInterval = 20 * time.Second

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

	c := &Conn{
		ws:   ws,
		send: make(chan wsmsg.Envelope, opts.SendBuffer),
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
			case <-t.C:
				_ = ws.Ping(ctx)
			}
		}
	}()

	// writer loop (single writer)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-c.send:
				if !ok {
					return
				}
				_ = wsjson.Write(ctx, ws, msg)
			}
		}
	}()

	return c, nil
}

func (c *Conn) Close(code websocket.StatusCode, reason string) error {
	close(c.send)
	return c.ws.Close(code, reason)
}

func (c *Conn) Send(env wsmsg.Envelope) bool {
	select {
	case c.send <- env:
		return true
	default:
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
