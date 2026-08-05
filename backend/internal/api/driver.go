package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	driverservice "github.com/JCKFinland/connect/backend/internal/services/driver"
)

// DriverHandler exposes HTTP endpoints for driver management.
type DriverHandler struct {
	service *driverservice.Service
}

// NewDriverHandler creates a new DriverHandler.
func NewDriverHandler(
	service *driverservice.Service,
) *DriverHandler {

	return &DriverHandler{
		service: service,
	}
}

// Create registers a new driver.
func (h *DriverHandler) Create(c *gin.Context) {

	var req driverservice.CreateDriverRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	driver, err := h.service.Create(
		c.Request.Context(),
		req,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data": driver,
	})
}

// GetByID returns a driver by ID.
func (h *DriverHandler) GetByID(c *gin.Context) {

	driver, err := h.service.GetByID(
		c.Request.Context(),
		c.Param("id"),
	)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": driver,
	})
}

// List returns all drivers.
func (h *DriverHandler) List(c *gin.Context) {

	drivers, err := h.service.List(
		c.Request.Context(),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": drivers,
	})
}

// Update modifies an existing driver.
func (h *DriverHandler) Update(c *gin.Context) {

	var req driverservice.UpdateDriverRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	err := h.service.Update(
		c.Request.Context(),
		c.Param("id"),
		req,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Driver updated successfully",
	})
}

// Delete performs a soft delete.
func (h *DriverHandler) Delete(c *gin.Context) {

	err := h.service.Delete(
		c.Request.Context(),
		c.Param("id"),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Driver deleted successfully",
	})
}