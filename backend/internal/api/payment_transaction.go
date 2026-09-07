package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/payment"
	"github.com/JCKFinland/connect/backend/internal/services/paymenttransaction"
	"github.com/JCKFinland/connect/backend/pkg/response"
)

type PaymentTransactionHandler struct {
	payments payment.Service

	transactions paymenttransaction.Service
}

func NewPaymentTransactionHandler(
	payments payment.Service,
	transactions paymenttransaction.Service,
) *PaymentTransactionHandler {
	return &PaymentTransactionHandler{
		payments: payments,

		transactions: transactions,
	}
}

type InitiatePaymentTransactionRequest struct {
	Provider string `json:"provider"`

	IdempotencyKey string `json:"idempotency_key"`

	TransactionType string `json:"transaction_type"`

	// Amount is required only for REFUND.
	Amount string `json:"amount,omitempty"`
}

// Initiate handles:
//
//	POST /api/v1/payments/:id/transactions
//
// The authenticated user is authorized against the logical payment before
// a provider-facing operation is persisted.
func (h *PaymentTransactionHandler) Initiate(
	c *gin.Context,
) {
	paymentID :=
		strings.TrimSpace(
			c.Param("id"),
		)

	if paymentID == "" {
		response.BadRequest(
			c,
			"Payment ID is required",
		)
		return
	}

	user, ok :=
		middleware.CurrentUser(c)

	if !ok || user == nil {
		response.Unauthorized(
			c,
			"Authenticated user not found",
		)
		return
	}

	if h.payments == nil ||
		h.transactions == nil {

		response.InternalServerError(c)
		return
	}

	var req InitiatePaymentTransactionRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(
			c,
			"Invalid request body",
		)
		return
	}

	req.Provider =
		strings.ToUpper(
			strings.TrimSpace(
				req.Provider,
			),
		)

	req.TransactionType =
		strings.ToUpper(
			strings.TrimSpace(
				req.TransactionType,
			),
		)

	req.IdempotencyKey =
		strings.TrimSpace(
			req.IdempotencyKey,
		)

	req.Amount =
		strings.TrimSpace(
			req.Amount,
		)

	if req.Provider == "" {
		response.BadRequest(
			c,
			"Payment provider is required",
		)
		return
	}

	if req.IdempotencyKey == "" {
		response.BadRequest(
			c,
			"Idempotency key is required",
		)
		return
	}

	if req.TransactionType == "" {
		response.BadRequest(
			c,
			"Transaction type is required",
		)
		return
	}

	authorizedPayment, err :=
		h.payments.GetByIDAuthorized(
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
				"You are not authorized to create a transaction for this payment",
			)

		case errors.Is(
			err,
			repository.ErrNotFound,
		) || errors.Is(
			err,
			pgx.ErrNoRows,
		):
			response.NotFound(
				c,
				"Payment not found",
			)

		default:
			response.InternalServerError(c)
		}

		return
	}

	if authorizedPayment == nil {
		response.NotFound(
			c,
			"Payment not found",
		)
		return
	}

	transaction, err :=
		h.transactions.InitiateOperation(
			c.Request.Context(),
			paymenttransaction.InitiateOperationRequest{
				PaymentID: authorizedPayment.ID,

				Provider: req.Provider,

				IdempotencyKey: req.IdempotencyKey,

				TransactionType: req.TransactionType,

				Amount: req.Amount,
			},
		)
	if err != nil {
		switch {
		case errors.Is(
			err,
			paymenttransaction.ErrUnsupportedTransactionType,
		):
			response.BadRequest(
				c,
				"Unsupported payment transaction type",
			)

		case errors.Is(
			err,
			paymenttransaction.ErrInvalidPaymentOperation,
		):
			response.BadRequest(
				c,
				"Payment operation is not allowed in the current payment state",
			)

		case errors.Is(
			err,
			paymenttransaction.ErrPaymentOperationIdempotencyConflict,
		):
			response.Error(
				c,
				http.StatusConflict,
				"Idempotency key conflicts with an existing payment operation",
				nil,
			)

		case errors.Is(
			err,
			paymenttransaction.ErrPaymentOperationAmountRequired,
		):
			response.BadRequest(
				c,
				"Amount is required for REFUND",
			)

		case errors.Is(
			err,
			paymenttransaction.ErrPaymentOperationAmountInvalid,
		):
			response.BadRequest(
				c,
				"Invalid payment operation amount",
			)

		default:
			response.InternalServerError(c)
		}

		return
	}

	if transaction == nil {
		response.InternalServerError(c)
		return
	}

	c.JSON(
		http.StatusCreated,
		gin.H{
			"success": true,
			"message": "Payment transaction initiated successfully",
			"data":    transaction,
		},
	)
}
