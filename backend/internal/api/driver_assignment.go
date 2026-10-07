package api

import (
	"errors"
	"net/http"

	// Imports Gin to extract incoming JSON payloads and handle request contexts.
	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/middleware"
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

func handleAssignmentError(
	c *gin.Context,
	err error,
) {
	switch {
	case errors.Is(err, assignment.ErrDriverNotFound),
		errors.Is(err, assignment.ErrVehicleNotFound),
		errors.Is(err, assignment.ErrAssignmentNotFound):

		response.NotFound(c, err.Error())

	case errors.Is(err, assignment.ErrDriverInactive),
		errors.Is(err, assignment.ErrCompanyInactive),
		errors.Is(err, assignment.ErrBranchInactive),
		errors.Is(err, assignment.ErrFleetInactive),
		errors.Is(err, assignment.ErrVehicleOutsideDriverScope),
		errors.Is(err, assignment.ErrVehicleInactive),
		errors.Is(err, assignment.ErrDriverAlreadyAssigned),
		errors.Is(err, assignment.ErrVehicleAlreadyAssigned),
		errors.Is(err, assignment.ErrDriverOffline),
		errors.Is(err, assignment.ErrDriverUnavailable):

		response.Conflict(c, err.Error())

	default:
		response.InternalServerError(c)
	}
}

// Assign binds a driver to an assignment structure (e.g., matching a ride dispatch or a specific fleet vehicle).
func (h *DriverAssignmentHandler) Assign(
	c *gin.Context,
) {

	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(
			c,
			"authenticated user not found",
		)
		return
	}

	var req assignment.AssignDriverRequest

	// Only vehicle selection and notes come from the client. Driver and
	// organizational identity are resolved authoritatively by the service.
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(
			c,
			"Invalid request body",
		)
		return
	}

	driverAssignment, err := h.service.Assign(
		c.Request.Context(),
		user.ID,
		req,
	)

	if err != nil {
		handleAssignmentError(
			c,
			err,
		)
		return
	}

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

	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(
			c,
			"authenticated user not found",
		)
		return
	}

	// Self-unassignment has no client-controlled driver identity. The
	// authenticated users.id is the operational driver identity.
	if err := h.service.Unassign(
		c.Request.Context(),
		user.ID,
	); err != nil {
		handleAssignmentError(
			c,
			err,
		)
		return
	}

	response.Success(
		c,
		http.StatusOK,
		"Driver unassigned successfully",
		nil,
	)
}
