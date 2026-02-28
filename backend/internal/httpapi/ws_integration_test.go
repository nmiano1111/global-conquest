package httpapi

import (
	"backend/internal/game"
	"backend/internal/proto/wsmsg"
	"backend/internal/service"
	"backend/internal/store"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func startTestHTTPServer(t *testing.T, h http.Handler) (baseHTTPURL string) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	srv := &http.Server{Handler: h}
	go func() {
		_ = srv.Serve(ln)
	}()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		_ = ln.Close()
	})

	return "http://" + ln.Addr().String()
}

func mustReadEnvelope(t *testing.T, c *websocket.Conn) wsmsg.Envelope {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var env wsmsg.Envelope
	if err := wsjson.Read(ctx, c, &env); err != nil {
		t.Fatalf("read envelope: %v", err)
	}
	return env
}

func readUntilType(t *testing.T, c *websocket.Conn, typ wsmsg.Type, max int) wsmsg.Envelope {
	t.Helper()
	for i := 0; i < max; i++ {
		env := mustReadEnvelope(t, c)
		if env.Type == typ {
			return env
		}
	}
	t.Fatalf("did not receive envelope type %q within %d messages", typ, max)
	return wsmsg.Envelope{}
}

func TestWebSocketHello(t *testing.T) {
	g := game.NewServer()
	go g.Run()

	svc := &fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		getUserFn:    func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:      func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	}
	games := &fakeGamesService{
		createClassicGameFn: func(context.Context, string, int) (store.Game, error) { return store.Game{}, nil },
		joinClassicGameFn:   func(context.Context, string, string) (store.Game, error) { return store.Game{}, nil },
		getGameFn:           func(context.Context, string) (store.Game, error) { return store.Game{}, nil },
		listGamesFn:         func(context.Context, string, string, int, int) ([]store.Game, error) { return nil, nil },
		updateGameStateFn:   func(context.Context, string, string, json.RawMessage) (store.Game, error) { return store.Game{}, nil },
	}
	router := NewRouter(NewHandler(g, svc, games))
	base := startTestHTTPServer(t, router)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws://"+base[len("http://"):]+"/ws", nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "done") }()

	hello := mustReadEnvelope(t, conn)
	if hello.Type != wsmsg.Type("hello") {
		t.Fatalf("expected hello, got %q", hello.Type)
	}

	var payload map[string]any
	if err := json.Unmarshal(hello.Payload, &payload); err != nil {
		t.Fatalf("decode hello payload: %v", err)
	}
	if payload["name"] != "anon" {
		t.Fatalf("expected anon name, got %#v", payload["name"])
	}
	if payload["client_id"] == "" {
		t.Fatalf("expected client_id in payload")
	}
}

func TestWebSocketPingPongAndCreateGame(t *testing.T) {
	g := game.NewServer()
	go g.Run()

	svc := &fakeUsersService{
		createUserFn: func(context.Context, string, string) (store.User, error) { return store.User{}, nil },
		getUserFn:    func(context.Context, string) (store.User, error) { return store.User{}, nil },
		loginFn:      func(context.Context, string, string) (service.LoginResult, error) { return service.LoginResult{}, nil },
	}
	games := &fakeGamesService{
		createClassicGameFn: func(context.Context, string, int) (store.Game, error) { return store.Game{}, nil },
		joinClassicGameFn:   func(context.Context, string, string) (store.Game, error) { return store.Game{}, nil },
		getGameFn:           func(context.Context, string) (store.Game, error) { return store.Game{}, nil },
		listGamesFn:         func(context.Context, string, string, int, int) ([]store.Game, error) { return nil, nil },
		updateGameStateFn:   func(context.Context, string, string, json.RawMessage) (store.Game, error) { return store.Game{}, nil },
	}
	router := NewRouter(NewHandler(g, svc, games))
	base := startTestHTTPServer(t, router)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws://"+base[len("http://"):]+"/ws", nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "done") }()

	_ = mustReadEnvelope(t, conn) // initial hello

	if err := wsjson.Write(context.Background(), conn, wsmsg.Envelope{
		Type: wsmsg.Type("ping"),
		ID:   "c_ping",
	}); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	pong := readUntilType(t, conn, wsmsg.Type("pong"), 3)
	if pong.CorrelationID != "c_ping" {
		t.Fatalf("expected correlation_id c_ping, got %q", pong.CorrelationID)
	}

	if err := wsjson.Write(context.Background(), conn, wsmsg.Envelope{
		Type: wsmsg.Type("create_game"),
		ID:   "c_create",
	}); err != nil {
		t.Fatalf("write create_game: %v", err)
	}
	created := readUntilType(t, conn, wsmsg.Type("game_created"), 6)
	if created.GameID == "" {
		t.Fatalf("expected game_id in game_created envelope")
	}
	if created.CorrelationID != "c_create" {
		t.Fatalf("expected correlation_id c_create, got %q", created.CorrelationID)
	}
}
