package httpapi

import (
	"github.com/nmiano1111/global-conquest/backend/internal/auth"
	"github.com/nmiano1111/global-conquest/backend/internal/store"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const authUserContextKey = "auth_user"

// RequireAuth returns a gin middleware that authenticates the request via a
// Bearer session token in the Authorization header, aborting with 401 if the
// header is missing/malformed or the token is invalid or expired. On success
// it stores the resolved store.User on the gin context, retrievable via
// getAuthUser by downstream handlers.
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

// RequireAdmin returns a gin middleware that aborts with 401 if no
// authenticated user is present (RequireAuth must run first in the chain)
// or 403 if the authenticated user's role is not "admin".
func (h *Handler) RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		u, ok := getAuthUser(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}
		if !strings.EqualFold(u.Role, "admin") {
			c.JSON(http.StatusForbidden, gin.H{"error": "admin role required"})
			c.Abort()
			return
		}
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

func getAuthUser(c *gin.Context) (store.User, bool) {
	v, ok := c.Get(authUserContextKey)
	if !ok {
		return store.User{}, false
	}
	u, ok := v.(store.User)
	return u, ok
}
