package middleware

import (
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/gin-gonic/gin"
)

const (
	ContextUserKey   = "user"
	ContextClaimsKey = "claims"
)

// SetCurrentUser stores the authenticated user in the Gin context.
func SetCurrentUser(
	c *gin.Context,
	user *models.User,
) {
	c.Set(ContextUserKey, user)
}

// CurrentUser returns the authenticated user from the Gin context.
func CurrentUser(
	c *gin.Context,
) (*models.User, bool) {

	value, exists := c.Get(ContextUserKey)
	if !exists {
		return nil, false
	}

	user, ok := value.(*models.User)
	if !ok {
		return nil, false
	}

	return user, true
}

// MustCurrentUser returns the authenticated user.
//
// It assumes the authentication middleware has already verified
// and stored the user in the context.
func MustCurrentUser(
	c *gin.Context,
) *models.User {

	user, _ := CurrentUser(c)

	return user
}
