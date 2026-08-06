package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	dvaservice "github.com/JCKFinland/connect/backend/internal/services/driver_vehicle_assignment"
)

// DriverVehicleAssignmentHandler exposes HTTP endpoints
// for operational driver-vehicle assignments.
type DriverVehicleAssignmentHandler struct {
	service *dvaservice.Service
}

// NewDriverVehicleAssignmentHandler creates a new handler.
func NewDriverVehicleAssignmentHandler(
	service *dvaservice.Service,
) *DriverVehicleAssignmentHandler {

	return &DriverVehicleAssignmentHandler{
		service: service,
	}
}

// Assign creates a new driver-vehicle assignment.
func (h *DriverVehicleAssignmentHandler) Assign(c *gin.Context) {

	var req dvaservice.AssignDriverVehicleRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	assignment, err := h.service.Assign(
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
		"data": assignment,
	})
}

// GetByID returns a single assignment.
func (h *DriverVehicleAssignmentHandler) GetByID(c *gin.Context) {

	assignment, err := h.service.GetByID(
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
		"data": assignment,
	})
}

// List returns all assignments.
func (h *DriverVehicleAssignmentHandler) List(c *gin.Context) {

	assignments, err := h.service.List(
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
		"data": assignments,
	})
}

// Release ends an assignment.
func (h *DriverVehicleAssignmentHandler) Release(c *gin.Context) {

	err := h.service.Release(
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
		"message": "Assignment released successfully",
	})
}

// Delete performs a soft delete.
func (h *DriverVehicleAssignmentHandler) Delete(c *gin.Context) {

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
		"message": "Assignment deleted successfully",
	})
}