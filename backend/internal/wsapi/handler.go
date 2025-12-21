package wsapi

import (
	"backend/internal/game"
	"backend/internal/proto/wsmsg"
	"backend/internal/wsconn"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

type Options struct {
	OriginPatterns []string
	PingInterval   time.Duration
	SendBuffer     int
}

// GinHandler wires transport to the game hub.
func GinHandler(s *game.Server, opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		w := unwrapWriter(c.Writer)

		conn, err := wsconn.Accept(w, c.Request, wsconn.Options{
			OriginPatterns: opts.OriginPatterns,
			PingInterval:   opts.PingInterval,
			SendBuffer:     opts.SendBuffer,
		})
		if err != nil {
			return
		}
		c.Abort()

		ctx := c.Request.Context()

		// Create client + register with hub.
		// Name is optional for now; you can set it via "hello" later.
		clientID := randClientID()
		cl := &game.Client{
			ID:   clientID,
			Name: "anon",
			Conn: conn,
		}
		s.Inbox() <- game.Register{C: cl}

		// On exit: unregister + close socket
		defer func() {
			s.Inbox() <- game.Unregister{ClientID: clientID}
			_ = conn.Close(websocket.StatusNormalClosure, "bye")
		}()

		// Read pump: each envelope goes into hub
		_ = conn.ReadLoop(ctx, func(env wsmsg.Envelope) {
			s.Inbox() <- game.Incoming{ClientID: clientID, Env: env}
		})
	}
}

func unwrapWriter(w http.ResponseWriter) http.ResponseWriter {
	rw := w
	if uw, ok := any(w).(interface{ Unwrap() http.ResponseWriter }); ok {
		rw = uw.Unwrap()
	}
	return rw
}

// tiny local id helper (or reuse game.newID)
func randClientID() string {
	// keep it simple: reuse game.newID-like approach without exporting
	// (or just export game.NewID if you prefer)
	return "p_" + time.Now().Format("150405.000000000")
}
