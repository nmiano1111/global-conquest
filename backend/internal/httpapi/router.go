package httpapi

import (
	"backend/internal/wsapi"
	"context"
	"net/http"
	"os"
	"strings"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/gin-gonic/gin"
)

func NewRouter(h *Handler) *gin.Engine {
	r := gin.Default()

	r.Use(gin.Logger(), gin.Recovery())

	allowedOrigin := os.Getenv("CORS_ALLOWED_ORIGIN")
	r.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin != "" && origin == allowedOrigin {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	api := r.Group("/api")
	{
		api.GET("/ping", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "pong"})
		})
		users := api.Group("/users")
		{
			users.POST("/", h.CreateUser)
			protectedUsers := users.Group("")
			protectedUsers.Use(h.RequireAuth())
			{
				protectedUsers.GET("/", h.ListUsers)
				protectedUsers.GET("/:username", h.GetUser)
			}
		}
		auth := api.Group("/auth")
		{
			auth.POST("/login", h.Login)
		}
		games := api.Group("/games")
		{
			games.Use(h.RequireAuth())
			games.GET("/", h.ListGames)
			games.POST("/", h.CreateGame)
			games.POST("/:id/join", h.JoinGame)
			games.GET("/:id", h.GetGame)
			games.GET("/:id/bootstrap", h.GetGameBootstrap)
			games.PUT("/:id/state", h.UpdateGameState)
		}
		chat := api.Group("/chat")
		{
			chat.Use(h.RequireAuth())
			chat.GET("/lobby/messages", h.ListLobbyMessages)
			chat.POST("/lobby/messages", h.PostLobbyMessage)
		}
		admin := api.Group("/admin")
		{
			admin.Use(h.RequireAuth(), h.RequireAdmin())
			admin.GET("/users", h.ListAdminUsers)
			admin.PUT("/users/:id/access", h.UpdateUserAccess)
			admin.POST("/users/:id/revoke-sessions", h.RevokeUserSessions)
		}

	}

	origins := []string{"localhost:*", "127.0.0.1:*"}
	if v := os.Getenv("WS_ALLOWED_ORIGINS"); v != "" {
		origins = strings.Split(v, ",")
	}

	r.GET("/ws", wsapi.GinHandler(h.gameServer, wsapi.Options{
		OriginPatterns: origins,
		SendBuffer:     16,
		Authenticate: func(ctx context.Context, token string) (wsapi.AuthUser, error) {
			u, err := h.users.AuthenticateSession(ctx, token)
			if err != nil {
				return wsapi.AuthUser{}, err
			}
			return wsapi.AuthUser{ID: u.ID, UserName: u.UserName}, nil
		},
	}))

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return r
}
