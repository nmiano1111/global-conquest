package main

import (
	"backend/internal/httpapi"
	"log"
)

/*

type ClientMsg struct {
	Type string `json:"type"`
	// Add fields as needed (e.g. room_id, move, etc.)
}

type ServerMsg struct {
	Type string `json:"type"`
}

func ws(c *gin.Context) {
	fmt.Println("WS")

	// IMPORTANT: unwrap gin's ResponseWriter so Hijack works properly
	w := http.ResponseWriter(c.Writer)
	if uw, ok := any(c.Writer).(interface{ Unwrap() http.ResponseWriter }); ok {
		w = uw.Unwrap()
	}

	// Upgrade
	conn, err := websocket.Accept(w, c.Request, &websocket.AcceptOptions{
		// IMPORTANT: lock this down. Example allows same-host only:
		OriginPatterns: []string{"localhost:*", "127.0.0.1:*"},
		// Or implement your own origin check using OriginPatterns and/or other middleware.
	})
	if err != nil {
		fmt.Println(err)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "bye")

	ctx := c.Request.Context()

	// Optional: keepalive (some proxies/timeouts benefit)
	// You can do ping/pong, or set deadlines depending on your design.
	pingCtx, cancelPing := context.WithCancel(ctx)
	defer cancelPing()
	go func() {
		t := time.NewTicker(20 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-pingCtx.Done():
				return
			case <-t.C:
				// Ignore ping errors; the read loop will notice closure too.
				_ = conn.Ping(pingCtx)
			}
		}
	}()

	// Writer goroutine to ensure one writer (nice + simple).
	send := make(chan ServerMsg, 16)
	go func() {
		defer close(send)
		for msg := range send {
			// Use wsjson for convenience. You can also use conn.Write directly.
			if err := wsjson.Write(ctx, conn, msg); err != nil {
				fmt.Println(err)
				return
			}
		}
	}()

	// Initial hello
	select {
	case send <- ServerMsg{Type: "hello"}:
	default:
		// send buffer full; you may choose to close
		fmt.Println("default")
	}

	// Read loop
	for {
		var m ClientMsg
		err := wsjson.Read(ctx, conn, &m)
		if err != nil {
			fmt.Println(err)
			// Normal close vs error
			var closeErr websocket.CloseError
			if errors.As(err, &closeErr) {
				// Client closed or protocol close
				return
			}
			// Context canceled (client went away) is also normal-ish
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			log.Printf("ws read error: %v", err)
			_ = conn.Close(websocket.StatusInternalError, "read error")
			return
		}

		switch m.Type {
		case "ping":
			// app-level ping; protocol ping handled above
			select {
			case send <- ServerMsg{Type: "pong"}:
			default:
				fmt.Println("default pong")
			}
		default:
			// TODO: route your game messages here
			select {
			case send <- ServerMsg{Type: "ack"}:
			default:
				fmt.Println("default ack")
			}
		}
	}
}
*/

func main() {
	r := httpapi.NewRouter()

	// default gin port is :8080 if you pass no addr; I prefer explicit
	if err := r.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
