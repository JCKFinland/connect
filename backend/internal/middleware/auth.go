package middleware

import (
	"net/http"
	"strings"

	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/security"
	"github.com/JCKFinland/connect/backend/pkg/response"
	"github.com/gin-gonic/gin"
)

type AuthMiddleware struct {
	jwt   *security.JWTService
	users repository.UserRepository
}

// NewAuthMiddleware creates a new authentication middleware.
func NewAuthMiddleware(
	jwt *security.JWTService,
	users repository.UserRepository,
) *AuthMiddleware {

	return &AuthMiddleware{
		jwt:   jwt,
		users: users,
	}
}

// RequireAuth validates the JWT and loads the current user.
func (m *AuthMiddleware) RequireAuth() gin.HandlerFunc {

	return func(c *gin.Context) {

		authHeader := c.GetHeader("Authorization")

		if authHeader == "" {
			response.Unauthorized(c, "Authorization header is required")
			c.Abort()
			return
		}

		const bearer = "Bearer "

		if !strings.HasPrefix(authHeader, bearer) {
			response.Unauthorized(c, "Invalid authorization header")
			c.Abort()
			return
		}

		token := strings.TrimPrefix(authHeader, bearer)

		claims, err := m.jwt.ValidateToken(token)
		if err != nil {
			response.Unauthorized(c, err.Error())
			c.Abort()
			return
		}

		user, err := m.users.GetByID(
			c.Request.Context(),
			claims.UserID,
		)

		if err != nil {
			response.Unauthorized(c, "User not found")
			c.Abort()
			return
		}

		if !user.IsActive {
			response.Error(
				c,
				http.StatusForbidden,
				"Account is disabled",
				nil,
			)
			c.Abort()
			return
		}

		SetCurrentUser(c, user)

		c.Next()
	}
}
