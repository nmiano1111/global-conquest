package httpapi

import (
	"backend/internal/wsapi"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"net/http"

	"github.com/gin-gonic/gin"
)

func NewRouter(h *Handler) *gin.Engine {
	r := gin.Default()

	r.Use(gin.Logger(), gin.Recovery())

	api := r.Group("/api")
	{
		api.GET("/ping", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "pong"})
		})
		users := api.Group("/users")
		{
			users.POST("/", h.CreateUser)
			users.GET("/", h.ListUsers)
			users.GET("/:username", h.GetUser)
		}
		auth := api.Group("/auth")
		{
			auth.POST("/login", h.Login)
		}
		games := api.Group("/games")
		{
			games.GET("/", h.ListGames)
			games.POST("/", h.CreateGame)
			games.GET("/:id", h.GetGame)
			games.PUT("/:id/state", h.UpdateGameState)
		}

	}

	r.GET("/ws", wsapi.GinHandler(h.gameServer, wsapi.Options{
		OriginPatterns: []string{"localhost:*", "127.0.0.1:*"},
		SendBuffer:     16,
	}))

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return r
}
