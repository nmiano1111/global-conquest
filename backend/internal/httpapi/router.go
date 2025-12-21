package httpapi

import (
	"backend/internal/wsapi"
	"net/http"

	"github.com/gin-gonic/gin"
)

func NewRouter() *gin.Engine {
	r := gin.Default()

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	// WS endpoint
	r.GET("/ws", wsapi.GinHandler(wsapi.Options{
		OriginPatterns: []string{"localhost:*", "127.0.0.1:*"},
		PingInterval:   wsapi.DefaultPingInterval,
		SendBuffer:     16,
	}))

	return r
}
