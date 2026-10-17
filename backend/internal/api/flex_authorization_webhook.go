package api

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/JCKFinland/connect/backend/internal/payments/stripe"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	flexauthorization "github.com/JCKFinland/connect/backend/internal/services/flex_authorization"
)

const maximumFlexWebhookBodyBytes = 1 << 20

// FlexWebhookVerifier verifies Stripe signatures and produces
// a trusted, structurally validated FLEX authorization event.
type FlexWebhookVerifier interface {
	Verify(
		ctx context.Context,
		headers http.Header,
		rawBody []byte,
	) (*stripe.VerifiedFlexAuthorizationEvent, error)
}

// FlexWebhookReconciler performs authoritative provider
// reconciliation after successful signature verification.
type FlexWebhookReconciler interface {
	ReconcileWebhook(
		ctx context.Context,
		event *stripe.VerifiedFlexAuthorizationEvent,
	) (*postgresrepo.FlexAuthorization, error)
}

// FlexAuthorizationWebhookHandler exposes the Stripe-to-CONNECT
// FLEX authorization reconciliation boundary.
type FlexAuthorizationWebhookHandler struct {
	verifier   FlexWebhookVerifier
	reconciler FlexWebhookReconciler
}

func NewFlexAuthorizationWebhookHandler(
	verifier FlexWebhookVerifier,
	reconciler FlexWebhookReconciler,
) *FlexAuthorizationWebhookHandler {
	return &FlexAuthorizationWebhookHandler{
		verifier:   verifier,
		reconciler: reconciler,
	}
}

func (h *FlexAuthorizationWebhookHandler) Handle(
	c *gin.Context,
) {
	if h == nil ||
		h.verifier == nil ||
		h.reconciler == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "FLEX webhook service unavailable",
		})
		return
	}

	bodyReader := http.MaxBytesReader(
		c.Writer,
		c.Request.Body,
		maximumFlexWebhookBodyBytes,
	)

	rawBody, err := io.ReadAll(bodyReader)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"success": false,
				"message": "FLEX webhook payload too large",
			})
			return
		}

		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid FLEX webhook body",
		})
		return
	}

	verified, err := h.verifier.Verify(
		c.Request.Context(),
		c.Request.Header,
		rawBody,
	)
	if err != nil || verified == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "FLEX webhook verification failed",
		})
		return
	}

	authorization, err := h.reconciler.ReconcileWebhook(
		c.Request.Context(),
		verified,
	)
	if err != nil {
		switch {
		case errors.Is(
			err,
			flexauthorization.ErrInvalidFlexWebhook,
		):
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Invalid FLEX webhook event",
			})

		case errors.Is(
			err,
			flexauthorization.ErrFlexWebhookNotReady,
		),
			errors.Is(
				err,
				flexauthorization.ErrFlexWebhookProcessing,
			):
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"message": "FLEX webhook processing pending",
			})

		default:
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"message": "FLEX webhook processing unavailable",
			})
		}
		return
	}

	if authorization == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"message": "FLEX webhook processing unavailable",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "FLEX webhook processed",
	})
}
