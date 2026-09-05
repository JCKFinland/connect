package api

import (
	"errors"
	"io"
	"net/http"
	"strings"

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

				ProviderTransactionID: verified.ProviderTransactionID,

				ProviderStatus: verified.ProviderStatus,

				RawPayload: rawBody,
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

	c.JSON(
		http.StatusOK,
		gin.H{
			"success": true,
			"message": "Payment callback processed",
			"data":    result,
		},
	)
}
