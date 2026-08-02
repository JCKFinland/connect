package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	branch "github.com/JCKFinland/connect/backend/internal/services/branch"

	"github.com/JCKFinland/connect/backend/pkg/response"
)

type BranchHandler struct {
	service *branch.Service
}

func NewBranchHandler(
	service *branch.Service,
) *BranchHandler {

	return &BranchHandler{
		service: service,
	}
}

func (h *BranchHandler) Create(c *gin.Context) {

	var req branch.CreateBranchRequest

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
		req,
	)
	if err != nil {

		response.Error(
			c,
			http.StatusInternalServerError,
			err.Error(),
			nil,
		)

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

	id := c.Param("id")

	branchObj, err := h.service.GetByID(
		c.Request.Context(),
		id,
	)
	if err != nil {

		response.Error(
			c,
			http.StatusNotFound,
			err.Error(),
			nil,
		)

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

	branches, err := h.service.List(
		c.Request.Context(),
	)
	if err != nil {

		response.Error(
			c,
			http.StatusInternalServerError,
			err.Error(),
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

	id := c.Param("id")

	var req branch.UpdateBranchRequest

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
		id,
		req,
	)
	if err != nil {

		response.Error(
			c,
			http.StatusInternalServerError,
			err.Error(),
			nil,
		)

		return
	}

	response.Success(
		c,
		http.StatusOK,
		"Branch updated successfully",
		nil,
	)
}

func (h *BranchHandler) Delete(c *gin.Context) {

	id := c.Param("id")

	err := h.service.Delete(
		c.Request.Context(),
		id,
	)
	if err != nil {

		response.Error(
			c,
			http.StatusInternalServerError,
			err.Error(),
			nil,
		)

		return
	}

	response.Success(
		c,
		http.StatusOK,
		"Branch deleted successfully",
		nil,
	)
}