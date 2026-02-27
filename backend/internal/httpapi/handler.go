package httpapi

import (
	"backend/internal/auth"
	"backend/internal/game"
	"backend/internal/service"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
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
	UserName string `json:"username" binding:"required,min=3,max=24"`
	Password string `json:"password" binding:"required,min=8,max=128"`
}

type loginReq struct {
	UserName string `json:"username" binding:"required,min=3,max=24"`
	Password string `json:"password" binding:"required,min=8,max=128"`
}

type loginResp struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
	User      any    `json:"user"`
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
// @Failure      409 {object} map[string]string
// @Failure      500 {object} map[string]string
// @Router       /api/users [post]
func (h *Handler) CreateUser(c *gin.Context) {
	var req createUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	u, err := h.users.CreateUser(c.Request.Context(), req.UserName, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrUsernameInvalid), errors.Is(err, auth.ErrPasswordTooShort):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrUsernameTaken):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
		}
		return
	}

	c.JSON(http.StatusCreated, u)
}

// Login godoc
// @Summary      Login
// @Description  Authenticates a user and returns a new session token
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request body loginReq true "Login request"
// @Success      200 {object} loginResp
// @Failure      400 {object} map[string]string
// @Failure      401 {object} map[string]string
// @Failure      500 {object} map[string]string
// @Router       /api/auth/login [post]
func (h *Handler) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	out, err := h.users.Login(c.Request.Context(), req.UserName, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidUsernameOrPassword):
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "login failed"})
		}
		return
	}

	c.JSON(http.StatusOK, loginResp{
		Token:     out.Token,
		ExpiresAt: out.ExpiresAt.Format(time.RFC3339),
		User: gin.H{
			"id":         out.User.ID,
			"username":   out.User.UserName,
			"role":       out.User.Role,
			"created_at": out.User.CreatedAt,
			"updated_at": out.User.UpdatedAt,
		},
	})
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
