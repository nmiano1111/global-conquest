//go:build e2e

package e2e

import (
	"backend/internal/db"
	"backend/internal/game"
	"backend/internal/httpapi"
	"backend/internal/service"
	"backend/internal/store"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type testEnv struct {
	baseURL string
	dbPool  *pgxpool.Pool
	admin   *pgxpool.Pool
	dbName  string
	server  *http.Server
	ln      net.Listener
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()

	adminDSN := os.Getenv("E2E_ADMIN_DSN")
	if adminDSN == "" {
		t.Skip("set E2E_ADMIN_DSN to run DB-backed e2e tests")
	}

	ctx := context.Background()
	admin, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		t.Fatalf("connect admin db: %v", err)
	}
	t.Cleanup(admin.Close)

	dbName := fmt.Sprintf("e2e_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE DATABASE "`+dbName+`"`); err != nil {
		t.Fatalf("create test db: %v", err)
	}

	dsn := withDatabase(adminDSN, dbName)
	appPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}

	if err := applyMigration(ctx, appPool); err != nil {
		t.Fatalf("apply migration: %v", err)
	}

	appDB := db.New(appPool)
	usersStore := store.NewPostgresUsersStore()
	usersSvc := service.NewUsersService(appDB, usersStore)
	gamesStore := store.NewPostgresGamesStore()
	gamesSvc := service.NewGamesService(appDB, gamesStore)
	g := game.NewServer()
	go g.Run()

	handler := httpapi.NewHandler(g, usersSvc, gamesSvc)
	router := httpapi.NewRouter(handler)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: router}
	go func() {
		_ = srv.Serve(ln)
	}()

	env := &testEnv{
		baseURL: "http://" + ln.Addr().String(),
		dbPool:  appPool,
		admin:   admin,
		dbName:  dbName,
		server:  srv,
		ln:      ln,
	}

	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = env.server.Shutdown(shutdownCtx)
		_ = env.ln.Close()
		env.dbPool.Close()

		_, _ = env.admin.Exec(context.Background(),
			`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=$1 AND pid <> pg_backend_pid()`, env.dbName)
		_, _ = env.admin.Exec(context.Background(), `DROP DATABASE IF EXISTS "`+env.dbName+`"`)
	})

	return env
}

func withDatabase(dsn, dbName string) string {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return dsn
	}
	cfg.ConnConfig.Database = dbName
	return cfg.ConnString()
}

func applyMigration(ctx context.Context, pool *pgxpool.Pool) error {
	roots := []string{
		"../../migrations",
		"migrations",
	}
	var files []string
	for _, root := range roots {
		matches, _ := filepath.Glob(filepath.Join(root, "V*.sql"))
		files = append(files, matches...)
	}
	if len(files) == 0 {
		return fmt.Errorf("no migration files found")
	}
	sort.Strings(files)

	for _, p := range files {
		sqlBytes, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", p, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply migration %s: %w", p, err)
		}
	}
	return nil
}

func postJSON(t *testing.T, baseURL, path string, body any) (*http.Response, map[string]any) {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+path, bytes.NewReader(b))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("http post %s: %v", path, err)
	}

	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	_ = resp.Body.Close()
	return resp, out
}

func TestRegisterLoginPersistsSession(t *testing.T) {
	env := setupTestEnv(t)

	username := fmt.Sprintf("player_%d", time.Now().UnixNano())
	password := "correct-horse-battery-staple"

	createResp, _ := postJSON(t, env.baseURL, "/api/users/", map[string]string{
		"username": username,
		"password": password,
	})
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 from create user, got %d", createResp.StatusCode)
	}

	loginResp, body := postJSON(t, env.baseURL, "/api/auth/login", map[string]string{
		"username": username,
		"password": password,
	})
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from login, got %d", loginResp.StatusCode)
	}

	token, _ := body["token"].(string)
	if strings.TrimSpace(token) == "" {
		t.Fatalf("expected non-empty login token")
	}

	var sessions int
	err := env.dbPool.QueryRow(context.Background(), `
		SELECT count(*)
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE u.username = $1
	`, username).Scan(&sessions)
	if err != nil {
		t.Fatalf("query sessions: %v", err)
	}
	if sessions != 1 {
		t.Fatalf("expected 1 session row, got %d", sessions)
	}
}

func TestDuplicateUsernameReturnsConflict(t *testing.T) {
	env := setupTestEnv(t)

	username := fmt.Sprintf("dupe_%d", time.Now().UnixNano())
	password := "safe-password-123"

	firstResp, _ := postJSON(t, env.baseURL, "/api/users/", map[string]string{
		"username": username,
		"password": password,
	})
	if firstResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 from first create, got %d", firstResp.StatusCode)
	}

	secondResp, _ := postJSON(t, env.baseURL, "/api/users/", map[string]string{
		"username": username,
		"password": password,
	})
	if secondResp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 from duplicate create, got %d", secondResp.StatusCode)
	}
}

func TestBadLoginReturnsUnauthorized(t *testing.T) {
	env := setupTestEnv(t)

	username := fmt.Sprintf("login_%d", time.Now().UnixNano())
	password := "real-password-123"

	createResp, _ := postJSON(t, env.baseURL, "/api/users/", map[string]string{
		"username": username,
		"password": password,
	})
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 from create user, got %d", createResp.StatusCode)
	}

	loginResp, _ := postJSON(t, env.baseURL, "/api/auth/login", map[string]string{
		"username": username,
		"password": "wrong-password",
	})
	if loginResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 from bad login, got %d", loginResp.StatusCode)
	}
}
