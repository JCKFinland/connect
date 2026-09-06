package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/JCKFinland/connect/backend/internal/services/payment"
	"github.com/JCKFinland/connect/backend/internal/services/paymentexecution"
	"github.com/JCKFinland/connect/backend/pkg/response"
)

type paymentTransactionReader interface {
	GetByID(
		ctx context.Context,
		id string,
	) (*models.PaymentTransaction, error)
}

type PaymentExecutionHandler struct {
	payments payment.Service

	transactions paymentTransactionReader

	execution paymentexecution.Service
}

func NewPaymentExecutionHandler(
	payments payment.Service,
	transactions paymentTransactionReader,
	execution paymentexecution.Service,
) *PaymentExecutionHandler {
	return &PaymentExecutionHandler{
		payments: payments,

		transactions: transactions,

		execution: execution,
	}
}

type PaymentExecutionResponse struct {
	ID string `json:"id"`

	PaymentID string `json:"payment_id"`

	Provider string `json:"provider"`

	ProviderTransactionID *string `json:"provider_transaction_id,omitempty"`

	TransactionType string `json:"transaction_type"`

	Status string `json:"status"`

	Amount string `json:"amount"`

	Currency string `json:"currency"`

	ClientSecret string `json:"client_secret,omitempty"`

	RequiresCustomerAction bool `json:"requires_customer_action"`
}

// Execute handles:
//
//	POST /api/v1/payments/:id/transactions/:transaction_id/execute
//
// The payment resource is authorized first. The referenced provider-facing
// transaction must then be proven to belong to that same payment before
// execution is delegated to the payment execution service.
func (h *PaymentExecutionHandler) Execute(
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

	transactionID :=
		c.Param("transaction_id")

	if transactionID == "" {
		response.BadRequest(
			c,
			"Payment transaction ID is required",
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
		h.transactions == nil ||
		h.execution == nil {

		response.InternalServerError(c)
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
				"You are not authorized to execute this payment",
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
		h.transactions.GetByID(
			c.Request.Context(),
			transactionID,
		)
	if err != nil {
		if errors.Is(
			err,
			repository.ErrNotFound,
		) || errors.Is(
			err,
			pgx.ErrNoRows,
		) {
			response.NotFound(
				c,
				"Payment transaction not found",
			)
			return
		}

		response.InternalServerError(c)
		return
	}

	if transaction == nil {
		response.NotFound(
			c,
			"Payment transaction not found",
		)
		return
	}

	if transaction.PaymentID !=
		authorizedPayment.ID {

		// Do not disclose that the supplied transaction belongs to a
		// different payment.
		response.NotFound(
			c,
			"Payment transaction not found",
		)
		return
	}

	result, err :=
		h.execution.Execute(
			c.Request.Context(),
			transaction.ID,
		)
	if err != nil {
		switch {
		case errors.Is(
			err,
			paymentexecution.ErrParentProviderTransactionNotFound,
		):
			response.BadRequest(
				c,
				"Required prior payment operation was not found",
			)

		case errors.Is(
			err,
			paymentexecution.ErrUnsupportedProvider,
		):
			response.BadRequest(
				c,
				"Payment provider is not supported",
			)

		default:
			response.InternalServerError(c)
		}

		return
	}

	if result == nil ||
		result.Transaction == nil {

		response.InternalServerError(c)
		return
	}

	executed :=
		result.Transaction

	data :=
		PaymentExecutionResponse{
			ID: executed.ID,

			PaymentID: executed.PaymentID,

			Provider: executed.Provider,

			ProviderTransactionID: executed.ProviderTransactionID,

			TransactionType: executed.TransactionType,

			Status: executed.Status,

			Amount: executed.Amount,

			Currency: executed.Currency,

			RequiresCustomerAction: result.RequiresCustomerAction,
		}

	if result.RequiresCustomerAction &&
		result.ClientSecret != "" {

		data.ClientSecret =
			result.ClientSecret
	}

	c.JSON(
		http.StatusOK,
		gin.H{
			"success": true,

			"message": "Payment transaction execution submitted",

			"data": data,
		},
	)
}
