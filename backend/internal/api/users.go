package api

import (
	// Injects the middleware layer to retrieve contextual identity pointers.
	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/pkg/response"
	"github.com/gin-gonic/gin"
)

// UserHandler contains endpoint functions dealing with user records.
type UserHandler struct{}

// NewUserHandler is a simple constructor function invoked inside main.go.
func NewUserHandler() *UserHandler {
	return &UserHandler{}
}

// Me extracts and returns the currently logged-in user or driver session details.
func (h *UserHandler) Me(c *gin.Context) {

	// Extracts the fully hydration-parsed user entity stored inside the Gin Context by AuthMiddleware.
	user, ok := middleware.CurrentUser(c)
	if !ok {
		// Safeguard fallback: if the route was improperly configured without auth protection, block access.
		response.Unauthorized(c, "Authentication required")
		return
	}

	// Returns an HTTP 200 OK along with an explicitly limited public data dictionary map.
	response.OK(
		c,
		"User profile",
		gin.H{
			"id":          user.ID,
			"email":       user.Email,
			"first_name":  user.FirstName,
			"last_name":   user.LastName,
			"phone":       user.Phone,
			"is_active":   user.IsActive,
			"is_verified": user.IsVerified, // Crucial field determining if a taxi driver is cleared to accept jobs.
			"created_at":  user.CreatedAt,
		},
	)
}
