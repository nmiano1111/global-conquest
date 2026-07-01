package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"backend/internal/discordbot"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Println("warning: .env:", err)
	}

	cfg, err := discordbot.ConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	bot, err := discordbot.New(cfg)
	if err != nil {
		log.Fatal(err)
	}

	if err := bot.Start(); err != nil {
		log.Fatal(err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("discord: shutdown signal received")
	if err := bot.Close(); err != nil {
		log.Printf("discord: session close error: %v", err)
	}
}
