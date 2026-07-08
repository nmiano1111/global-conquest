package wsapi

import (
	"backend/internal/auth"
	"backend/internal/game"
	"backend/internal/proto/wsmsg"
	"backend/internal/wsconn"
	"context"
	"errors"
	"log"
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

		remoteAddr := c.ClientIP()
		userAgent := c.Request.UserAgent()
		authLabel := "anon"
		if authUser.ID != "" {
			authLabel = authUser.UserName + "(" + authUser.ID + ")"
		}

		// Assign the client ID before Accept so the connection's ping/write/send
		// logs can be correlated with the hub-level register/disconnect logs.
		clientID := randClientID()

		w := unwrapWriter(c.Writer)

		conn, err := wsconn.Accept(w, c.Request, wsconn.Options{
			OriginPatterns: opts.OriginPatterns,
			PingInterval:   opts.PingInterval,
			SendBuffer:     opts.SendBuffer,
			Label:          clientID,
		})
		if err != nil {
			log.Printf("ws: accept failed client=%s remote=%s ua=%q auth=%s err=%v", clientID, remoteAddr, userAgent, authLabel, err)
			return
		}
		c.Abort()

		log.Printf("ws: connected client=%s remote=%s ua=%q auth=%s", clientID, remoteAddr, userAgent, authLabel)
		connectedAt := time.Now()

		// Create client + register with hub.
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
		readErr := conn.ReadLoop(ctx, func(env wsmsg.Envelope) {
			s.Inbox() <- game.Incoming{ClientID: clientID, Env: env}
		})
		closeCode := websocket.CloseStatus(readErr)
		log.Printf("ws: disconnected client=%s auth=%s duration=%s closeCode=%d err=%v",
			clientID, authLabel, time.Since(connectedAt), closeCode, readErr)
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
