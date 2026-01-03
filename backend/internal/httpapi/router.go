package httpapi

import (
	"backend/internal/wsapi"
	"net/http"

	"github.com/gin-gonic/gin"
)

func NewRouter(h *Handler) *gin.Engine {
	r := gin.Default()

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	r.POST("/users", h.CreateUser)

	r.GET("/ws", wsapi.GinHandler(h.gameServer, wsapi.Options{
		OriginPatterns: []string{"localhost:*", "127.0.0.1:*"},
		SendBuffer:     16,
	}))

	return r
}
