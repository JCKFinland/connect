package api

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/services/paymentcallback"
	"github.com/JCKFinland/connect/backend/pkg/response"
)

const maximumPaymentCallbackBodyBytes = 1 << 20 // 1 MiB

type PaymentCallbackHandler struct {
	service  paymentcallback.Service
	registry *paymentcallback.VerifierRegistry
}

func NewPaymentCallbackHandler(
	service paymentcallback.Service,
	registry *paymentcallback.VerifierRegistry,
) *PaymentCallbackHandler {
	return &PaymentCallbackHandler{
		service:  service,
		registry: registry,
	}
}

// PaymentCallbackResponse is the intentionally restricted public response
// returned after a provider callback is processed.
//
// Provider request/response payloads, idempotency keys, and other gateway
// internals must never be exposed through this callback endpoint.
type PaymentCallbackResponse struct {
	ID string `json:"id"`

	PaymentID string `json:"payment_id"`

	Provider string `json:"provider"`

	ProviderTransactionID *string `json:"provider_transaction_id,omitempty"`

	TransactionType string `json:"transaction_type"`

	Status string `json:"status"`

	Amount   string `json:"amount"`
	Currency string `json:"currency"`

	ProcessedAt *time.Time `json:"processed_at,omitempty"`
}

func (h *PaymentCallbackHandler) Handle(
	c *gin.Context,
) {
	provider := c.Param("provider")
	if provider == "" {
		response.BadRequest(
			c,
			"Payment provider is required",
		)
		return
	}

	if h.registry == nil {
		response.InternalServerError(c)
		return
	}

	if h.service == nil {
		response.InternalServerError(c)
		return
	}

	verifier, err :=
		h.registry.Get(provider)
	if err != nil {
		if errors.Is(
			err,
			paymentcallback.ErrUnsupportedCallbackProvider,
		) {
			response.NotFound(
				c,
				"Payment callback provider not found",
			)
			return
		}

		response.InternalServerError(c)
		return
	}

	bodyReader := http.MaxBytesReader(
		c.Writer,
		c.Request.Body,
		maximumPaymentCallbackBodyBytes,
	)

	rawBody, err :=
		io.ReadAll(bodyReader)
	if err != nil {
		response.BadRequest(
			c,
			"Invalid payment callback body",
		)
		return
	}

	verified, err :=
		verifier.Verify(
			c.Request.Context(),
			c.Request.Header,
			rawBody,
		)
	if err != nil {
		response.Unauthorized(
			c,
			"Payment callback verification failed",
		)
		return
	}

	if verified == nil {
		response.Unauthorized(
			c,
			"Payment callback verification failed",
		)
		return
	}

	routeProvider :=
		strings.TrimSpace(provider)

	verifiedProvider :=
		strings.TrimSpace(verified.Provider)

	if verifiedProvider == "" {
		verified.Provider = routeProvider
	} else if !strings.EqualFold(
		verifiedProvider,
		routeProvider,
	) {
		response.Unauthorized(
			c,
			"Payment callback verification failed",
		)
		return
	}

	result, err :=
		h.service.ApplyProviderCallback(
			c.Request.Context(),
			paymentcallback.ApplyProviderCallbackRequest{
				Provider: verified.Provider,

				TransactionID: verified.TransactionID,

				PaymentID: verified.PaymentID,

				ProviderTransactionID: verified.ProviderTransactionID,

				ProviderStatus: verified.ProviderStatus,

				RawPayload: verified.RawPayload,
			},
		)
	if err != nil {
		switch {
		case errors.Is(
			err,
			paymentcallback.ErrCallbackTransactionNotFound,
		):
			response.NotFound(
				c,
				"Payment transaction not found",
			)

		case errors.Is(
			err,
			paymentcallback.ErrInvalidCallback,
		):
			response.BadRequest(
				c,
				"Invalid payment callback",
			)

		default:
			response.InternalServerError(c)
		}

		return
	}

	if result == nil {
		response.InternalServerError(c)
		return
	}

	data :=
		PaymentCallbackResponse{
			ID: result.ID,

			PaymentID: result.PaymentID,

			Provider: result.Provider,

			ProviderTransactionID: result.ProviderTransactionID,

			TransactionType: result.TransactionType,

			Status: result.Status,

			Amount: result.Amount,

			Currency: result.Currency,

			ProcessedAt: result.ProcessedAt,
		}

	c.JSON(
		http.StatusOK,
		gin.H{
			"success": true,
			"message": "Payment callback processed",
			"data":    data,
		},
	)
}
