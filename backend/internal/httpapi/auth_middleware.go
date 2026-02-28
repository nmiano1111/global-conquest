package httpapi

import (
	"backend/internal/auth"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const authUserContextKey = "auth_user"

func (h *Handler) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := parseBearerToken(c.GetHeader("Authorization"))
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid authorization header"})
			c.Abort()
			return
		}

		u, err := h.users.AuthenticateSession(c.Request.Context(), token)
		if err != nil {
			switch {
			case errors.Is(err, auth.ErrInvalidSession):
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired session"})
			default:
				c.JSON(http.StatusInternalServerError, gin.H{"error": "authentication failed"})
			}
			c.Abort()
			return
		}

		c.Set(authUserContextKey, u)
		c.Next()
	}
}

func parseBearerToken(header string) (string, bool) {
	parts := strings.SplitN(strings.TrimSpace(header), " ", 2)
	if len(parts) != 2 {
		return "", false
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", false
	}
	return token, true
}
