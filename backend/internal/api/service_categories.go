package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/repository"
)

// ServiceCategoryHandler exposes read-only service-category operations.
type ServiceCategoryHandler struct {
	repository repository.ServiceCategoryRepository
}

// NewServiceCategoryHandler creates a service-category API handler.
func NewServiceCategoryHandler(
	repository repository.ServiceCategoryRepository,
) *ServiceCategoryHandler {
	return &ServiceCategoryHandler{
		repository: repository,
	}
}

// ListActive returns service categories currently available for booking.
func (h *ServiceCategoryHandler) ListActive(c *gin.Context) {
	categories, err := h.repository.List(
		c.Request.Context(),
		true,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to retrieve service categories",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    categories,
	})
}
