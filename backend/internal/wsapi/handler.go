package wsapi

import (
	"backend/internal/auth"
	"backend/internal/game"
	"backend/internal/proto/wsmsg"
	"backend/internal/wsconn"
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

type Options struct {
	OriginPatterns []string
	PingInterval   time.Duration
	SendBuffer     int
	Authenticate   func(ctx context.Context, token string) (AuthUser, error)
}

type AuthUser struct {
	ID       string
	UserName string
}

// GinHandler wires transport to the game hub.
func GinHandler(s *game.Server, opts Options) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		authUser := AuthUser{}
		if opts.Authenticate != nil {
			token := c.Query("token")
			if token != "" {
				u, err := opts.Authenticate(ctx, token)
				switch {
				case err == nil:
					authUser = u
				case errors.Is(err, auth.ErrInvalidSession):
					// Token is genuinely invalid/expired/blocked: fall through
					// as anon, same as REST's 401 for a bad credential.
				default:
					// Transient failure (e.g. DB hiccup) resolving an otherwise
					// valid token. Reject the upgrade instead of silently
					// downgrading a valid session to anon for the connection's
					// whole lifetime — the client's reconnect backoff will retry.
					c.AbortWithStatus(http.StatusServiceUnavailable)
					return
				}
			}
		}

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

		// Create client + register with hub.
		clientID := randClientID()
		name := "anon"
		if authUser.UserName != "" {
			name = authUser.UserName
		}
		cl := &game.Client{
			ID:     clientID,
			UserID: authUser.ID,
			Name:   name,
			Conn:   conn,
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
