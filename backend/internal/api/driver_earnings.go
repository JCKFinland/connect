package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	driverearning "github.com/JCKFinland/connect/backend/internal/services/driver_earning"
)

type DriverEarningsHandler struct {
	service *driverearning.Service
}

func NewDriverEarningsHandler(
	service *driverearning.Service,
) *DriverEarningsHandler {
	return &DriverEarningsHandler{
		service: service,
	}
}

// GetDashboard handles GET /api/v1/driver/earnings.
func (h *DriverEarningsHandler) GetDashboard(
	c *gin.Context,
) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "authenticated user not found",
		})
		return
	}

	dashboard, err := h.service.GetDashboard(
		c.Request.Context(),
		user.ID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed to retrieve driver earnings",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    dashboard,
	})
}
