package httpapi

import (
	"backend/internal/game"
	"backend/internal/service"
	"backend/internal/store"
	"github.com/gin-gonic/gin"
	"net/http"
)

type Handler struct {
	gameServer *game.Server
	users      *service.UsersService
}

func NewHandler(gameServer *game.Server, users *service.UsersService) *Handler {
	return &Handler{gameServer: gameServer, users: users}
}

type createUserReq struct {
	Email       string `json:"email" binding:"required,email"`
	DisplayName string `json:"display_name" binding:"required"`
}

func (h *Handler) CreateUser(c *gin.Context) {
	var req createUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	u, err := h.users.CreateUser(c.Request.Context(), store.NewUser{
		Email:       req.Email,
		DisplayName: req.DisplayName,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, u)
}
