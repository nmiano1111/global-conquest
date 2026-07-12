package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"backend/internal/bot"
	"backend/internal/db"
	"backend/internal/game"
	"backend/internal/httpapi"
	"backend/internal/service"
	"backend/internal/store"
	"github.com/joho/godotenv"

	_ "backend/docs"

	"github.com/jackc/pgx/v5/pgxpool"
)

// @title           Backend Game API
// @version         1.0
// @description     API for the game backend server
// @BasePath        /
// @schemes         http

func main() {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Println("warning: .env:", err)
	}

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

	if os.Getenv("SKIP_MIGRATIONS") != "true" {
		runMigrations(ctx, pool)
	}

	d := db.New(pool)

	// store + service
	usersStore := store.NewPostgresUsersStore()
	usersSvc := service.NewUsersService(d, usersStore)
	gamesStore := store.NewPostgresGamesStore()
	gamesSvc := service.NewGamesService(d, gamesStore)
	gameEventStore := store.NewPostgresGameEventStore()
	gamesSvc.SetGameEventStore(gameEventStore)
	gamePlayersStore := store.NewPostgresGamePlayersStore()
	gamesSvc.SetGamePlayersStore(gamePlayersStore)
	gameDomainEventStore := store.NewPostgresGameDomainEventStore()
	gamesSvc.SetGameDomainEventStore(gameDomainEventStore)
	discordOutboxStore := store.NewPostgresDiscordOutboxStore()
	gamesSvc.SetDiscordOutboxStore(discordOutboxStore)
	gameActionSvc := service.NewGameActionService(gamesSvc)
	chatStore := store.NewPostgresChatStore()
	chatSvc := service.NewChatService(d, chatStore)
	gameChatStore := store.NewPostgresGameChatStore()
	gameChatSvc := service.NewGameChatService(d, gameChatStore)
	s.SetGameChatLogStore(gameChatSvc)
	s.SetGameActionService(gameActionSvc)

	// bots
	botLoader := service.NewBotGameLoader(gamesSvc)
	strategies := bot.StrategyRegistry{bot.StrategyBasicV1: bot.NewBasicStrategy()}
	botRunner := bot.NewRunner(botLoader, s, strategies, bot.RealSleeper{}, bot.DefaultLiveDelay)
	botManager := bot.NewManager(context.Background(), botRunner, bot.ExecutionLive)
	s.SetBotTrigger(botManager.Trigger)
	recoverBotGames(ctx, gamesSvc, botManager)

	// http
	h := httpapi.NewHandler(s, usersSvc, gamesSvc, chatSvc)
	r := httpapi.NewRouter(h)

	log.Fatal(r.Run(":8080"))
}

// recoverBotGames restarts a runner for every in_progress game after a
// backend restart. It triggers unconditionally for every active game
// rather than pre-filtering by controller: RunTurn's own state load is a
// cheap no-op (StopNotBotTurn) when the current player isn't bot-controlled,
// and duplicating that check here would just be the same read twice. No
// in-memory bot plan is ever persisted — resuming means loading the
// authoritative JSONB state and continuing from the current phase.
func recoverBotGames(ctx context.Context, gamesSvc *service.GamesService, botManager *bot.Manager) {
	const pageSize = 200
	offset := 0
	total := 0
	for {
		games, err := gamesSvc.ListGames(ctx, "", "in_progress", pageSize, offset)
		if err != nil {
			log.Printf("bot: startup recovery: list in_progress games: %v", err)
			return
		}
		for _, g := range games {
			botManager.Trigger(g.ID)
			total++
		}
		if len(games) < pageSize {
			break
		}
		offset += pageSize
	}
	log.Printf("bot: startup recovery: checked %d in_progress game(s)", total)
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
