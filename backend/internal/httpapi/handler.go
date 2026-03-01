package httpapi

import (
	"backend/internal/auth"
	"backend/internal/game"
	"backend/internal/risk"
	"backend/internal/service"
	"backend/internal/store"
	"context"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"time"
)

type userService interface {
	CreateUser(ctx context.Context, userName, password string) (store.User, error)
	ListUsers(ctx context.Context) ([]store.User, error)
	ListAdminUsers(ctx context.Context) ([]store.AdminUser, error)
	GetUser(ctx context.Context, userName string) (store.User, error)
	UpdateUserAccess(ctx context.Context, userID, accessStatus string) (store.User, error)
	RevokeUserSessions(ctx context.Context, userID string) (int64, error)
	AuthenticateSession(ctx context.Context, token string) (store.User, error)
	Login(ctx context.Context, userName, password string) (service.LoginResult, error)
}

type gameService interface {
	CreateClassicGame(ctx context.Context, ownerUserID string, playerCount int) (store.Game, error)
	JoinClassicGame(ctx context.Context, gameID, playerID string) (store.Game, error)
	GetGame(ctx context.Context, gameID string) (store.Game, error)
	GetGameBootstrap(ctx context.Context, gameID, requesterUserID string) (service.GameBootstrap, error)
	ListGames(ctx context.Context, ownerUserID, status string, limit, offset int) ([]store.Game, error)
	UpdateGameState(ctx context.Context, gameID, status string, state json.RawMessage) (store.Game, error)
}

type chatService interface {
	ListLobbyMessages(ctx context.Context, limit int) ([]store.ChatMessage, error)
	PostLobbyMessage(ctx context.Context, userID, body string) (store.ChatMessage, error)
}

type Handler struct {
	gameServer *game.Server
	users      userService
	games      gameService
	chats      chatService
}

func NewHandler(gameServer *game.Server, users userService, games gameService, chats chatService) *Handler {
	return &Handler{gameServer: gameServer, users: users, games: games, chats: chats}
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

type createGameReq struct {
	PlayerCount int `json:"player_count" binding:"required,min=3,max=6"`
}

type updateGameStateReq struct {
	Status string          `json:"status" binding:"required"`
	State  json.RawMessage `json:"state" binding:"required" swaggertype:"object"`
}

type postLobbyMessageReq struct {
	Body string `json:"body" binding:"required,min=1,max=1000"`
}

type updateUserAccessReq struct {
	AccessStatus string `json:"access_status" binding:"required,oneof=active blocked"`
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

// ListUsers godoc
// @Summary      List all users
// @Description  Retrieves all users
// @Tags         users
// @Produce      json
// @Success      200 {array} store.User
// @Failure      500 {object} map[string]string
// @Router       /api/users [get]
func (h *Handler) ListUsers(c *gin.Context) {
	users, err := h.users.ListUsers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list users"})
		return
	}

	c.JSON(http.StatusOK, users)
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
		case errors.Is(err, service.ErrUserAccessDenied):
			c.JSON(http.StatusForbidden, gin.H{"error": "account is not allowed to login"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "login failed"})
		}
		return
	}

	c.JSON(http.StatusOK, loginResp{
		Token:     out.Token,
		ExpiresAt: out.ExpiresAt.Format(time.RFC3339),
		User: gin.H{
			"id":            out.User.ID,
			"username":      out.User.UserName,
			"role":          out.User.Role,
			"access_status": out.User.AccessStatus,
			"created_at":    out.User.CreatedAt,
			"updated_at":    out.User.UpdatedAt,
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

// CreateGame godoc
// @Summary      Create a new game
// @Description  Creates a new game with an initial Risk state
// @Tags         games
// @Accept       json
// @Produce      json
// @Param        request body createGameReq true "Create game request"
// @Success      201 {object} store.Game
// @Failure      400 {object} map[string]string
// @Failure      500 {object} map[string]string
// @Router       /api/games [post]
func (h *Handler) CreateGame(c *gin.Context) {
	var req createGameReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	authUser, ok := getAuthUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	g, err := h.games.CreateClassicGame(c.Request.Context(), authUser.ID, req.PlayerCount)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidGameInput),
			errors.Is(err, service.ErrUnknownPlayerIDs),
			errors.Is(err, risk.ErrInvalidPlayerCount):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create game"})
		}
		return
	}

	c.JSON(http.StatusCreated, g)
}

// JoinGame godoc
// @Summary      Join a lobby game
// @Description  Adds the authenticated player to a lobby game. When the target count is reached, game transitions to in_progress.
// @Tags         games
// @Produce      json
// @Param        id path string true "Game ID"
// @Success      200 {object} store.Game
// @Failure      400 {object} map[string]string
// @Failure      404 {object} map[string]string
// @Failure      409 {object} map[string]string
// @Failure      500 {object} map[string]string
// @Router       /api/games/{id}/join [post]
func (h *Handler) JoinGame(c *gin.Context) {
	gameID := c.Param("id")
	authUser, ok := getAuthUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	g, err := h.games.JoinClassicGame(c.Request.Context(), gameID, authUser.ID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidGameInput):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrGameNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "game not found"})
		case errors.Is(err, service.ErrGameNotJoinable), errors.Is(err, service.ErrGamePlayerCountFull):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to join game"})
		}
		return
	}

	c.JSON(http.StatusOK, g)
}

// GetGame godoc
// @Summary      Get game state
// @Description  Retrieves stored game state by id
// @Tags         games
// @Produce      json
// @Param        id path string true "Game ID"
// @Success      200 {object} store.Game
// @Failure      404 {object} map[string]string
// @Failure      500 {object} map[string]string
// @Router       /api/games/{id} [get]
func (h *Handler) GetGame(c *gin.Context) {
	gameID := c.Param("id")
	g, err := h.games.GetGame(c.Request.Context(), gameID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrGameNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "game not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch game"})
		}
		return
	}
	c.JSON(http.StatusOK, g)
}

// GetGameBootstrap godoc
// @Summary      Get game bootstrap state
// @Description  Retrieves normalized game engine state for authenticated participants
// @Tags         games
// @Produce      json
// @Param        id path string true "Game ID"
// @Success      200 {object} service.GameBootstrap
// @Failure      401 {object} map[string]string
// @Failure      403 {object} map[string]string
// @Failure      404 {object} map[string]string
// @Failure      500 {object} map[string]string
// @Router       /api/games/{id}/bootstrap [get]
func (h *Handler) GetGameBootstrap(c *gin.Context) {
	gameID := c.Param("id")
	authUser, ok := getAuthUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	out, err := h.games.GetGameBootstrap(c.Request.Context(), gameID, authUser.ID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrGameNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "game not found"})
		case errors.Is(err, service.ErrGameForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		case errors.Is(err, service.ErrInvalidGameInput):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch game bootstrap"})
		}
		return
	}
	c.JSON(http.StatusOK, out)
}

// ListGames godoc
// @Summary      List games
// @Description  Lists stored games with optional filters
// @Tags         games
// @Produce      json
// @Param        owner_user_id query string false "Owner user ID"
// @Param        status query string false "Game status"
// @Param        limit query int false "Max games returned"
// @Param        offset query int false "Pagination offset"
// @Success      200 {array} store.Game
// @Failure      400 {object} map[string]string
// @Failure      500 {object} map[string]string
// @Router       /api/games [get]
func (h *Handler) ListGames(c *gin.Context) {
	owner := c.Query("owner_user_id")
	status := c.Query("status")

	limit := 0
	offset := 0
	var err error
	if v := c.Query("limit"); v != "" {
		limit, err = strconv.Atoi(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be an integer"})
			return
		}
	}
	if v := c.Query("offset"); v != "" {
		offset, err = strconv.Atoi(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "offset must be an integer"})
			return
		}
	}

	games, err := h.games.ListGames(c.Request.Context(), owner, status, limit, offset)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidGameInput):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list games"})
		}
		return
	}
	c.JSON(http.StatusOK, games)
}

// UpdateGameState godoc
// @Summary      Update game state
// @Description  Persists a new serialized game state
// @Tags         games
// @Accept       json
// @Produce      json
// @Param        id path string true "Game ID"
// @Param        request body updateGameStateReq true "Update game state request"
// @Success      200 {object} store.Game
// @Failure      400 {object} map[string]string
// @Failure      404 {object} map[string]string
// @Failure      500 {object} map[string]string
// @Router       /api/games/{id}/state [put]
func (h *Handler) UpdateGameState(c *gin.Context) {
	gameID := c.Param("id")
	var req updateGameStateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	g, err := h.games.UpdateGameState(c.Request.Context(), gameID, req.Status, req.State)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidGameInput):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrGameNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "game not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update game state"})
		}
		return
	}
	c.JSON(http.StatusOK, g)
}

// ListLobbyMessages godoc
// @Summary      List lobby chat messages
// @Description  Returns recent chat messages from the global lobby room
// @Tags         chat
// @Produce      json
// @Param        limit query int false "Max messages returned (default 50)"
// @Success      200 {array} store.ChatMessage
// @Failure      400 {object} map[string]string
// @Failure      500 {object} map[string]string
// @Router       /api/chat/lobby/messages [get]
func (h *Handler) ListLobbyMessages(c *gin.Context) {
	limit := 0
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be an integer"})
			return
		}
		limit = n
	}

	out, err := h.chats.ListLobbyMessages(c.Request.Context(), limit)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidChatInput):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list lobby messages"})
		}
		return
	}
	c.JSON(http.StatusOK, out)
}

// PostLobbyMessage godoc
// @Summary      Post a lobby chat message
// @Description  Creates a message in the global lobby room for the authenticated user
// @Tags         chat
// @Accept       json
// @Produce      json
// @Param        request body postLobbyMessageReq true "Lobby message request"
// @Success      201 {object} store.ChatMessage
// @Failure      400 {object} map[string]string
// @Failure      401 {object} map[string]string
// @Failure      500 {object} map[string]string
// @Router       /api/chat/lobby/messages [post]
func (h *Handler) PostLobbyMessage(c *gin.Context) {
	authUser, ok := getAuthUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req postLobbyMessageReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	msg, err := h.chats.PostLobbyMessage(c.Request.Context(), authUser.ID, req.Body)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidChatInput):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to post lobby message"})
		}
		return
	}

	if h.gameServer != nil {
		h.gameServer.Inbox() <- game.PublishLobbyChat{
			Message: map[string]any{
				"id":         msg.ID,
				"room":       msg.Room,
				"user_id":    msg.UserID,
				"user_name":  msg.UserName,
				"body":       msg.Body,
				"created_at": msg.CreatedAt,
			},
		}
	}

	c.JSON(http.StatusCreated, msg)
}

func (h *Handler) ListAdminUsers(c *gin.Context) {
	users, err := h.users.ListAdminUsers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list users"})
		return
	}
	c.JSON(http.StatusOK, users)
}

func (h *Handler) UpdateUserAccess(c *gin.Context) {
	userID := c.Param("id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user id is required"})
		return
	}
	var req updateUserAccessReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	u, err := h.users.UpdateUserAccess(c.Request.Context(), userID, req.AccessStatus)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidAccessState):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user access"})
		}
		return
	}
	c.JSON(http.StatusOK, u)
}

func (h *Handler) RevokeUserSessions(c *gin.Context) {
	userID := c.Param("id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user id is required"})
		return
	}

	revoked, err := h.users.RevokeUserSessions(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke sessions"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"revoked": revoked})
}
