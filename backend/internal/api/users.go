package api

import (
	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/pkg/response"
	"github.com/gin-gonic/gin"
)

type UserHandler struct{}

// NewUserHandler creates a new user handler.
func NewUserHandler() *UserHandler {
	return &UserHandler{}
}

// Me returns the currently authenticated user.
func (h *UserHandler) Me(c *gin.Context) {

	user, ok := middleware.CurrentUser(c)
	if !ok {
		response.Unauthorized(c, "Authentication required")
		return
	}

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
			"is_verified": user.IsVerified,
			"created_at":  user.CreatedAt,
		},
	)
}
