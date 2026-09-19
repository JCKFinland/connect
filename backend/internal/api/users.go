package api

import (
	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/pkg/response"
	"github.com/gin-gonic/gin"
)

// UserHandler contains endpoint functions dealing with user records.
type UserHandler struct {
	userRoles repository.UserRoleRepository
}

// NewUserHandler creates a user handler.
func NewUserHandler(
	userRoles repository.UserRoleRepository,
) *UserHandler {
	return &UserHandler{
		userRoles: userRoles,
	}
}

// Me returns the authenticated user's browser-safe profile and roles.
func (h *UserHandler) Me(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authentication required")
		return
	}

	roles, err := h.userRoles.GetUserRoles(
		c.Request.Context(),
		user.ID,
	)
	if err != nil {
		response.InternalServerError(c)
		return
	}

	if roles == nil {
		roles = []string{}
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
			"roles":       roles,
			"created_at":  user.CreatedAt,
		},
	)
}
