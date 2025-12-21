package httpapi

import (
	"backend/internal/game"
	"backend/internal/wsapi"
	"net/http"

	"github.com/gin-gonic/gin"
)

func NewRouter() *gin.Engine {
	r := gin.Default()

	// start hub
	s := game.NewServer()
	go s.Run()

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	r.GET("/ws", wsapi.GinHandler(s, wsapi.Options{
		OriginPatterns: []string{"localhost:*", "127.0.0.1:*"},
		SendBuffer:     16,
	}))

	return r
}
