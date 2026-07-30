package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/services/rbac"
)

type RBACMiddleware struct {
	rbac *rbac.Service
}

func NewRBACMiddleware(
	rbacService *rbac.Service,
) *RBACMiddleware {

	return &RBACMiddleware{
		rbac: rbacService,
	}
}

// RequirePermission ensures the authenticated user has the required permission.
func (m *RBACMiddleware) RequirePermission(
	permission string,
) gin.HandlerFunc {

	return func(c *gin.Context) {

		user, ok := CurrentUser(c)
		if !ok {
			c.AbortWithStatusJSON(
				http.StatusUnauthorized,
				gin.H{
					"success": false,
					"message": "Authentication required",
				},
			)
			return
		}

		allowed, err := m.rbac.HasPermission(
			c.Request.Context(),
			user.ID,
			permission,
		)
		if err != nil {
			c.AbortWithStatusJSON(
				http.StatusInternalServerError,
				gin.H{
					"success": false,
					"message": "Authorization failed",
				},
			)
			return
		}

		if !allowed {
			c.AbortWithStatusJSON(
				http.StatusForbidden,
				gin.H{
					"success": false,
					"message": "Insufficient permissions",
				},
			)
			return
		}

		c.Next()
	}
}