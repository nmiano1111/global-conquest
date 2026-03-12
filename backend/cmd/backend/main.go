package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"backend/internal/db"
	"backend/internal/game"
	"backend/internal/httpapi"
	"backend/internal/service"
	"backend/internal/store"

	_ "backend/docs"

	"github.com/jackc/pgx/v5/pgxpool"
)

// @title           Backend Game API
// @version         1.0
// @description     API for the game backend server
// @BasePath        /
// @schemes         http

func main() {
	ctx := context.Background()

	// game server
	s := game.NewServer()
	go s.Run()

	// db
	dbCfg, err := db.ConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	pool, err := db.NewPool(ctx, dbCfg)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	// runMigrations(ctx, pool)

	d := db.New(pool)

	// store + service
	usersStore := store.NewPostgresUsersStore()
	usersSvc := service.NewUsersService(d, usersStore)
	gamesStore := store.NewPostgresGamesStore()
	gamesSvc := service.NewGamesService(d, gamesStore)
	gameEventStore := store.NewPostgresGameEventStore()
	gamesSvc.SetGameEventStore(gameEventStore)
	gameActionSvc := service.NewGameActionService(gamesSvc)
	chatStore := store.NewPostgresChatStore()
	chatSvc := service.NewChatService(d, chatStore)
	gameChatStore := store.NewPostgresGameChatStore()
	gameChatSvc := service.NewGameChatService(d, gameChatStore)
	s.SetGameChatLogStore(gameChatSvc)
	s.SetGameActionService(gameActionSvc)

	// http
	h := httpapi.NewHandler(s, usersSvc, gamesSvc, chatSvc)
	r := httpapi.NewRouter(h)

	log.Fatal(r.Run(":8080"))
}

// runMigrations applies any unapplied SQL migration files from the migrations/
// directory. It tracks applied versions in a schema_migrations table and runs
// files in lexicographic order (V1__, V2__, …).
func runMigrations(ctx context.Context, pool *pgxpool.Pool) {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT        PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`)
	if err != nil {
		log.Fatal("migrations: create tracking table:", err)
	}

	entries, err := os.ReadDir("migrations")
	if err != nil {
		log.Fatal("migrations: read dir:", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var applied bool
		row := pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)", name)
		if err := row.Scan(&applied); err != nil {
			log.Fatal("migrations: check:", name, ":", err)
		}
		if applied {
			continue
		}

		sql, err := os.ReadFile(filepath.Join("migrations", name))
		if err != nil {
			log.Fatal("migrations: read:", name, ":", err)
		}

		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			log.Fatal("migrations: apply:", name, ":", err)
		}

		if _, err := pool.Exec(ctx,
			"INSERT INTO schema_migrations(version) VALUES($1)", name); err != nil {
			log.Fatal("migrations: record:", name, ":", err)
		}

		log.Printf("migrations: applied %s", name)
	}
}
