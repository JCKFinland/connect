package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	stripelinking "github.com/JCKFinland/connect/backend/internal/services/stripe_linking"
)

// StripeProvisioningService is deliberately narrow: the HTTP layer cannot
// supply company identity, a Stripe account ID or an idempotency key.
var stripeProvisioningHTTPAccountID = regexp.MustCompile(
	`^acct_[A-Za-z0-9]{8,64}$`,
)

type StripeProvisioningService interface {
	ProvisionAccount(context.Context, string, string) (string, error)
}

type StripeProvisioningHandler struct {
	service StripeProvisioningService
}

func NewStripeProvisioningHandler(
	service StripeProvisioningService,
) *StripeProvisioningHandler {
	return &StripeProvisioningHandler{service: service}
}

// Provision is not registered by the production router yet.
func (h *StripeProvisioningHandler) Provision(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok || user == nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "authentication required",
		})
		return
	}

	actorID, err := uuid.Parse(user.ID)
	if err != nil || actorID.String() != user.ID {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "access denied",
		})
		return
	}

	driverID := c.Param("id")
	parsedDriverID, err := uuid.Parse(driverID)
	if err != nil || parsedDriverID.String() != driverID {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid driver profile ID",
		})
		return
	}

	// This endpoint has no request DTO. In particular, caller-supplied
	// Stripe IDs and idempotency keys are never accepted.
	// Inspect actual bytes even when ContentLength is unknown.
	// Read no more than one byte from an untrusted body.
	if c.Request.ContentLength > 0 ||
		len(c.Request.TransferEncoding) != 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "request body is not permitted",
		})
		return
	}
	if c.Request.Body != nil {
		firstByte, readErr := io.ReadAll(
			io.LimitReader(c.Request.Body, 1),
		)
		if readErr != nil || len(firstByte) != 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "request body is not permitted",
			})
			return
		}
	}

	if h == nil || h.service == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "provisioning unavailable",
		})
		return
	}

	accountID, err := h.service.ProvisionAccount(
		c.Request.Context(), user.ID, driverID,
	)
	switch {
	case errors.Is(err, stripelinking.ErrStripeProvisioningReconciliationRequired):
		c.JSON(http.StatusConflict, gin.H{
			"error": "provisioning requires reconciliation",
		})
	case errors.Is(err, stripelinking.ErrStripeProvisioningDenied):
		c.JSON(http.StatusForbidden, gin.H{
			"error": "access denied",
		})
	case err != nil:
		// Do not expose internal provider or database failures.
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "provisioning unavailable",
		})
	case !stripeProvisioningHTTPAccountID.MatchString(accountID):
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "provisioning unavailable",
		})
	default:
		// A provisioned account is not ownership or payout authorization.
		c.JSON(http.StatusOK, gin.H{
			"stripe_account_id": accountID,
			"payouts_enabled":   false,
		})
	}
}
