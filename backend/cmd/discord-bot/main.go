package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"backend/internal/db"
	"backend/internal/discordbot"
	"backend/internal/reporting"
	"backend/internal/store"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Println("warning: .env:", err)
	}

	// Discord config
	cfg, err := discordbot.ConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	// Database
	ctx := context.Background()
	dbCfg, err := db.ConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	pool, err := db.NewPool(ctx, dbCfg)
	if err != nil {
		log.Fatalf("database connect: %v", err)
	}
	defer pool.Close()
	d := db.New(pool)

	// Reporting
	reportSvc := reporting.NewService(reporting.NewRepository(d.Queryer()))

	// Discord bot
	bot, err := discordbot.New(cfg, reportSvc)
	if err != nil {
		log.Fatal(err)
	}
	if err := bot.Start(); err != nil {
		log.Fatal(err)
	}

	// Outbox worker
	outboxStore := discordbot.NewBoundOutboxStore(store.NewPostgresDiscordOutboxStore(), d)
	worker := discordbot.NewWorker(outboxStore, bot.NewMessageSender(), cfg.EventsChannelID)
	worker.Start(ctx)

	// Wait for shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("discord: shutdown signal received")
	worker.Stop()
	if err := bot.Close(); err != nil {
		log.Printf("discord: session close error: %v", err)
	}
}
