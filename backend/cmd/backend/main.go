package main

import (
	"context"
	"log"

	"backend/internal/db"
	"backend/internal/game"
	"backend/internal/httpapi"
	"backend/internal/service"
	"backend/internal/store"
)

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

	// http
	h := httpapi.NewHandler(s, usersSvc)
	r := httpapi.NewRouter(h)

	log.Fatal(r.Run(":8080"))
}
