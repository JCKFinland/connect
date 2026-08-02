package api

import (
	"net/http"

	// Imports Gin to extract incoming JSON payloads and handle request contexts.
	"github.com/gin-gonic/gin"

	// Accesses the core driver matching and assignment business workflows.
	assignment "github.com/JCKFinland/connect/backend/internal/services/assignment"
	"github.com/JCKFinland/connect/backend/pkg/response"
)

// DriverAssignmentHandler bundles endpoints used to link or detach drivers and tasks.
type DriverAssignmentHandler struct {
	// Holds a reference to the active logical allocation engine.
	service *assignment.Service
}

// NewDriverAssignmentHandler is the factory constructor invoked during startup in main.go.
func NewDriverAssignmentHandler(
	service *assignment.Service,
) *DriverAssignmentHandler {

	return &DriverAssignmentHandler{
		service: service,
	}
}

// Assign binds a driver to an assignment structure (e.g., matching a ride dispatch or a specific fleet vehicle).
func (h *DriverAssignmentHandler) Assign(
	c *gin.Context,
) {

	var req assignment.AssignDriverRequest

	// Validates and decodes the JSON payload sent by the user application.
	if err := c.ShouldBindJSON(&req); err != nil {

		response.BadRequest(
			c,
			"Invalid request body",
		)

		return
	}

	// Forwards the pairing transaction down to the core assignment logic engine.
	driverAssignment, err := h.service.Assign(
		c.Request.Context(),
		req,
	)

	// Handles assignment constraints failures (e.g., driver already busy, vehicle unavailable).
	if err != nil {

		response.Error(
			c,
			http.StatusBadRequest,
			err.Error(),
			nil,
		)

		return
	}

	// Returns an HTTP 200 OK along with the metadata confirming the newly minted pairing details.
	response.Success(
		c,
		http.StatusOK,
		"Driver assigned successfully",
		driverAssignment,
	)
}

// Unassign breaks an active link (e.g., when a driver goes off-shift or cancels/completes a dispatch task).
func (h *DriverAssignmentHandler) Unassign(
	c *gin.Context,
) {

	var req assignment.UnassignDriverRequest

	// Validates that the unassignment request structure matches expected properties.
	if err := c.ShouldBindJSON(&req); err != nil {

		response.BadRequest(
			c,
			"Invalid request body",
		)

		return
	}

	// Commands the logic engine to sever the operational linkage.
	if err := h.service.Unassign(
		c.Request.Context(),
		req,
	); err != nil {

		response.Error(
			c,
			http.StatusBadRequest,
			err.Error(),
			nil,
		)

		return
	}

	// Sends an HTTP 200 OK indicating successful isolation of driver from task/vehicle.
	response.Success(
		c,
		http.StatusOK,
		"Driver unassigned successfully",
		nil,
	)
}
