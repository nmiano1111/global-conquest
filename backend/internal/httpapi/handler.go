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

// createUserReq represents the payload for creating a user
type createUserReq struct {
	UserName string `json:"username" binding:"required,min=3,max=32,alphanum"`
}

// CreateUser godoc
// @Summary      Create a new user
// @Description  Creates a new user with a unique username
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        request body createUserReq true "Create user request"
// @Success      201 {object} store.User
// @Failure      400 {object} map[string]string
// @Failure      500 {object} map[string]string
// @Router       /api/users [post]
func (h *Handler) CreateUser(c *gin.Context) {
	var req createUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	u, err := h.users.CreateUser(c.Request.Context(), store.NewUser{
		UserName: req.UserName,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, u)
}

// GetUser godoc
// @Summary      Get a user by username
// @Description  Retrieves a user by their username
// @Tags         users
// @Produce      json
// @Param        username path string true "Username"
// @Success      200 {object} store.User
// @Failure      400 {object} map[string]string
// @Failure      404 {object} map[string]string
// @Failure      500 {object} map[string]string
// @Router       /api/users/{username} [get]
func (h *Handler) GetUser(c *gin.Context) {
	userName := c.Param("username")
	if userName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username is required"})
		return
	}

	u, err := h.users.GetUser(c.Request.Context(), userName)
	if err != nil {
		// If your store returns pgx.ErrNoRows, you should map it to 404
		// instead of 500. This is the correct API behavior.
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, u)
}
