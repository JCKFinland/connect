package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/repository"
	branchservice "github.com/JCKFinland/connect/backend/internal/services/branch"
	"github.com/JCKFinland/connect/backend/pkg/response"
)

type BranchHandler struct {
	service *branchservice.Service
}

func NewBranchHandler(service *branchservice.Service) *BranchHandler {
	return &BranchHandler{
		service: service,
	}
}

func (h *BranchHandler) Create(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	var req branchservice.CreateBranchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(
			c,
			http.StatusBadRequest,
			"Invalid request body",
			nil,
		)
		return
	}

	createdBranch, err := h.service.Create(
		c.Request.Context(),
		user.ID,
		req,
	)
	if err != nil {
		switch {
		case errors.Is(err, branchservice.ErrBranchCreationAccessDenied):
			response.Error(
				c,
				http.StatusForbidden,
				"Branch creation access denied",
				nil,
			)

		default:
			response.Error(
				c,
				http.StatusInternalServerError,
				"Failed to create branch",
				nil,
			)
		}

		return
	}

	response.Success(
		c,
		http.StatusCreated,
		"Branch created successfully",
		createdBranch,
	)
}

func (h *BranchHandler) GetByID(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	branchObj, err := h.service.GetByID(
		c.Request.Context(),
		user.ID,
		c.Param("id"),
	)
	if err != nil {
		switch {
		case errors.Is(err, branchservice.ErrBranchNotFound),
			errors.Is(err, repository.ErrNotFound):
			response.Error(
				c,
				http.StatusNotFound,
				"Branch not found",
				nil,
			)

		default:
			response.Error(
				c,
				http.StatusInternalServerError,
				"Failed to retrieve branch",
				nil,
			)
		}

		return
	}

	response.Success(
		c,
		http.StatusOK,
		"Branch retrieved successfully",
		branchObj,
	)
}

func (h *BranchHandler) List(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	branches, err := h.service.List(
		c.Request.Context(),
		user.ID,
	)
	if err != nil {
		response.Error(
			c,
			http.StatusInternalServerError,
			"Failed to retrieve branches",
			nil,
		)
		return
	}

	response.Success(
		c,
		http.StatusOK,
		"Branches retrieved successfully",
		branches,
	)
}

func (h *BranchHandler) Update(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	var req branchservice.UpdateBranchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(
			c,
			http.StatusBadRequest,
			"Invalid request body",
			nil,
		)
		return
	}

	err := h.service.Update(
		c.Request.Context(),
		user.ID,
		c.Param("id"),
		req,
	)
	if err != nil {
		switch {
		case errors.Is(err, branchservice.ErrBranchNotFound),
			errors.Is(err, repository.ErrNotFound):
			response.Error(
				c,
				http.StatusNotFound,
				"Branch not found",
				nil,
			)

		default:
			response.Error(
				c,
				http.StatusInternalServerError,
				"Failed to update branch",
				nil,
			)
		}

		return
	}

	response.Success(
		c,
		http.StatusOK,
		"Branch updated successfully",
		nil,
	)
}

// Delete archives a branch after enforcing tenant and lifecycle authority.
func (h *BranchHandler) Delete(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	err := h.service.Delete(
		c.Request.Context(),
		user.ID,
		c.Param("id"),
	)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrNotFound),
			errors.Is(err, branchservice.ErrBranchNotFound):
			response.Error(
				c,
				http.StatusNotFound,
				"Branch not found",
				nil,
			)

		case errors.Is(err, branchservice.ErrBranchHasFleets):
			response.Error(
				c,
				http.StatusConflict,
				"Branch contains non-archived fleets",
				nil,
			)

		default:
			response.InternalServerError(c)
		}

		return
	}

	response.Success(
		c,
		http.StatusOK,
		"Branch archived successfully",
		nil,
	)
}

// Deactivate makes a branch operationally inactive when it has no active,
// non-archived fleets.
func (h *BranchHandler) Deactivate(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	err := h.service.Deactivate(
		c.Request.Context(),
		user.ID,
		c.Param("id"),
	)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrNotFound),
			errors.Is(err, branchservice.ErrBranchNotFound):
			response.Error(
				c,
				http.StatusNotFound,
				"Branch not found",
				nil,
			)

		case errors.Is(err, branchservice.ErrBranchHasActiveFleets):
			response.Error(
				c,
				http.StatusConflict,
				"Branch cannot be deactivated while it contains active fleets",
				nil,
			)

		default:
			response.InternalServerError(c)
		}

		return
	}

	response.Success(
		c,
		http.StatusOK,
		"Branch deactivated successfully",
		nil,
	)
}

// Reactivate restores operational eligibility for a non-archived branch.
func (h *BranchHandler) Reactivate(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(c, "Authenticated user not found")
		return
	}

	err := h.service.Reactivate(
		c.Request.Context(),
		user.ID,
		c.Param("id"),
	)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrNotFound),
			errors.Is(err, branchservice.ErrBranchNotFound):
			response.Error(
				c,
				http.StatusNotFound,
				"Branch not found",
				nil,
			)

		default:
			response.InternalServerError(c)
		}

		return
	}

	response.Success(
		c,
		http.StatusOK,
		"Branch reactivated successfully",
		nil,
	)
}
