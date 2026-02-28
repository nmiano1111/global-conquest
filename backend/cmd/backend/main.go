package main

import (
	"context"
	"log"

	"backend/internal/db"
	"backend/internal/game"
	"backend/internal/httpapi"
	"backend/internal/service"
	"backend/internal/store"

	_ "backend/docs"
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

	d := db.New(pool)

	// store + service
	usersStore := store.NewPostgresUsersStore()
	usersSvc := service.NewUsersService(d, usersStore)
	gamesStore := store.NewPostgresGamesStore()
	gamesSvc := service.NewGamesService(d, gamesStore)
	chatStore := store.NewPostgresChatStore()
	chatSvc := service.NewChatService(d, chatStore)

	// http
	h := httpapi.NewHandler(s, usersSvc, gamesSvc, chatSvc)
	r := httpapi.NewRouter(h)

	log.Fatal(r.Run(":8080"))
}
