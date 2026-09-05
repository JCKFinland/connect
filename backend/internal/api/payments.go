package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/services/payment"
	"github.com/JCKFinland/connect/backend/pkg/response"
)

type PaymentHandler struct {
	service payment.Service
}

func NewPaymentHandler(
	service payment.Service,
) *PaymentHandler {
	return &PaymentHandler{
		service: service,
	}
}

type CreatePaymentRequest struct {
	PaymentMethod string `json:"payment_method" binding:"required"`
}

// CreateForCompletedTrip handles:
//
//	POST /api/v1/trips/:id/payments
func (h *PaymentHandler) CreateForCompletedTrip(
	c *gin.Context,
) {
	tripID := c.Param("id")

	if tripID == "" {
		response.BadRequest(
			c,
			"Trip ID is required",
		)
		return
	}

	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(
			c,
			"Authenticated user not found",
		)
		return
	}

	var req CreatePaymentRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(
			c,
			"Invalid request body",
		)
		return
	}

	result, err :=
		h.service.CreateForCompletedTripAuthorized(
			c.Request.Context(),
			tripID,
			req.PaymentMethod,
			user.ID,
		)
	if err != nil {
		switch {
		case errors.Is(
			err,
			payment.ErrPaymentAccessDenied,
		):
			response.Forbidden(
				c,
				"You are not authorized to create this payment",
			)

		case errors.Is(err, pgx.ErrNoRows):
			response.NotFound(
				c,
				"Trip or completed fare not found",
			)

		default:
			response.BadRequest(
				c,
				err.Error(),
			)
		}

		return
	}

	c.JSON(
		http.StatusCreated,
		gin.H{
			"success": true,
			"message": "Payment created successfully",
			"data":    result,
		},
	)
}

// GetPayment handles:
//
//	GET /api/v1/payments/:id
func (h *PaymentHandler) GetPayment(
	c *gin.Context,
) {
	paymentID := c.Param("id")

	if paymentID == "" {
		response.BadRequest(
			c,
			"Payment ID is required",
		)
		return
	}

	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(
			c,
			"Authenticated user not found",
		)
		return
	}

	result, err :=
		h.service.GetByIDAuthorized(
			c.Request.Context(),
			paymentID,
			user.ID,
		)
	if err != nil {
		switch {
		case errors.Is(
			err,
			payment.ErrPaymentAccessDenied,
		):
			response.Forbidden(
				c,
				"You are not authorized to view this payment",
			)

		case errors.Is(err, pgx.ErrNoRows):
			response.NotFound(
				c,
				"Payment not found",
			)

		default:
			response.InternalServerError(c)
		}

		return
	}

	c.JSON(
		http.StatusOK,
		gin.H{
			"success": true,
			"data":    result,
		},
	)
}

// GetTripPayment handles:
//
//	GET /api/v1/trips/:id/payment
func (h *PaymentHandler) GetTripPayment(
	c *gin.Context,
) {
	tripID := c.Param("id")

	if tripID == "" {
		response.BadRequest(
			c,
			"Trip ID is required",
		)
		return
	}

	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		response.Unauthorized(
			c,
			"Authenticated user not found",
		)
		return
	}

	result, err :=
		h.service.GetByTripIDAuthorized(
			c.Request.Context(),
			tripID,
			user.ID,
		)
	if err != nil {
		switch {
		case errors.Is(
			err,
			payment.ErrPaymentAccessDenied,
		):
			response.Forbidden(
				c,
				"You are not authorized to view this payment",
			)

		case errors.Is(err, pgx.ErrNoRows):
			response.NotFound(
				c,
				"Payment not found",
			)

		default:
			response.InternalServerError(c)
		}

		return
	}

	c.JSON(
		http.StatusOK,
		gin.H{
			"success": true,
			"data":    result,
		},
	)
}
