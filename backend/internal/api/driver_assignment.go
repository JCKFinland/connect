package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	assignment "github.com/JCKFinland/connect/backend/internal/services/assignment"
	"github.com/JCKFinland/connect/backend/pkg/response"
)

type DriverAssignmentHandler struct {
	service *assignment.Service
}

func NewDriverAssignmentHandler(
	service *assignment.Service,
) *DriverAssignmentHandler {

	return &DriverAssignmentHandler{
		service: service,
	}
}

func (h *DriverAssignmentHandler) Assign(
	c *gin.Context,
) {

	var req assignment.AssignDriverRequest

	if err := c.ShouldBindJSON(&req); err != nil {

		response.BadRequest(
			c,
			"Invalid request body",
		)

		return
	}

	driverAssignment, err := h.service.Assign(
		c.Request.Context(),
		req,
	)

	if err != nil {

		response.Error(
			c,
			http.StatusBadRequest,
			err.Error(),
			nil,
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

func (h *DriverAssignmentHandler) Unassign(
	c *gin.Context,
) {

	var req assignment.UnassignDriverRequest

	if err := c.ShouldBindJSON(&req); err != nil {

		response.BadRequest(
			c,
			"Invalid request body",
		)

		return
	}

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

	response.Success(
		c,
		http.StatusOK,
		"Driver unassigned successfully",
		nil,
	)
}
