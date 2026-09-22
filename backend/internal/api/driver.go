package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/middleware"

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

// Register creates a driver profile for the authenticated user.
func (h *DriverHandler) Register(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "authentication required",
		})
		return
	}

	var req driverservice.RegisterDriverRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	driver, err := h.service.Register(
		c.Request.Context(),
		user,
		req,
	)
	if err != nil {
		switch {
		case errors.Is(err, driverservice.ErrDriverAlreadyExists):
			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"message": "driver already registered",
			})

		case errors.Is(err, driverservice.ErrInvalidDriver):
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "invalid driver registration",
			})

		default:
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "failed to register driver",
			})
		}

		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    driver,
	})
}

// GetRegistration returns the authenticated user's driver registration.
func (h *DriverHandler) GetRegistration(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "authentication required",
		})
		return
	}

	driver, err := h.service.GetRegistration(
		c.Request.Context(),
		user,
	)
	if err != nil {
		switch {
		case errors.Is(err, driverservice.ErrDriverNotFound):
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "driver registration not found",
			})

		case errors.Is(err, driverservice.ErrInvalidDriver):
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "invalid driver registration",
			})

		default:
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "failed to retrieve driver registration",
			})
		}

		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    driver,
	})
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
		"data":    driver,
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
		"data":    driver,
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
		"data":    drivers,
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
