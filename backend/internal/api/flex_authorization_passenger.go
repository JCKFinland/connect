package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/JCKFinland/connect/backend/internal/middleware"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	flexauthorization "github.com/JCKFinland/connect/backend/internal/services/flex_authorization"
	"github.com/JCKFinland/connect/backend/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// PassengerFlexAuthorizationExecutor is the existing financial
// authorization service contract. The HTTP layer must not contact
// Stripe or create financial records independently.
type PassengerFlexAuthorizationExecutor interface {
	Execute(
		ctx context.Context,
		rideRequestID string,
		auctionID string,
		passengerID string,
	) (*flexauthorization.ExecuteResult, error)
}

// PassengerFlexAuthorizationHandler exposes pre-trip authorization.
type PassengerFlexAuthorizationHandler struct {
	confirmationHandler *PassengerFlexConfirmationHandler
	executor            PassengerFlexAuthorizationExecutor
}

func NewPassengerFlexAuthorizationHandler(
	executor PassengerFlexAuthorizationExecutor,
) *PassengerFlexAuthorizationHandler {
	return &PassengerFlexAuthorizationHandler{
		executor: executor,
	}
}

type passengerFlexAuthorizationResponse struct {
	AuthorizationID        string `json:"authorization_id"`
	Status                 string `json:"status"`
	Authorized             bool   `json:"authorized"`
	RequiresCustomerAction bool   `json:"requires_customer_action"`
	ClientSecret           string `json:"client_secret,omitempty"`
}

// Authorize executes or safely resumes the selected offer's
// pre-trip payment authorization.
//
// It does not confirm the ride, capture payment, or create a trip.
func (h *PassengerFlexAuthorizationHandler) Authorize(
	c *gin.Context,
) {
	if h == nil || h.executor == nil {
		response.Error(
			c,
			http.StatusServiceUnavailable,
			"FLEX authorization service unavailable",
			nil,
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

	rideRequestID := c.Param("id")
	auctionID := c.Param("auction_id")

	if _, err := uuid.Parse(rideRequestID); err != nil {
		response.BadRequest(c, "Invalid ride request ID")
		return
	}

	if _, err := uuid.Parse(auctionID); err != nil {
		response.BadRequest(c, "Invalid FLEX auction ID")
		return
	}

	result, err := h.executor.Execute(
		c.Request.Context(),
		rideRequestID,
		auctionID,
		user.ID,
	)

	if err != nil {
		switch {
		case errors.Is(
			err,
			postgresrepo.ErrFlexAuthorizationAccessDenied,
		):
			response.Forbidden(
				c,
				"FLEX authorization access denied",
			)

		case errors.Is(
			err,
			postgresrepo.ErrFlexAuthorizationNotEligible,
		),
			errors.Is(
				err,
				flexauthorization.ErrAuthorizationNotEligible,
			):
			response.Conflict(
				c,
				"FLEX authorization is not eligible",
			)

		case errors.Is(
			err,
			flexauthorization.ErrAuthorizationPending,
		):
			response.Conflict(
				c,
				"FLEX authorization is pending reconciliation",
			)

		case errors.Is(
			err,
			flexauthorization.ErrServiceUnavailable,
		):
			response.Error(
				c,
				http.StatusServiceUnavailable,
				"FLEX authorization service unavailable",
				nil,
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

	public := passengerFlexAuthorizationResponse{
		AuthorizationID:        result.AuthorizationID,
		Status:                 result.Status,
		Authorized:             result.Authorized,
		RequiresCustomerAction: result.RequiresCustomerAction,
	}

	if result.RequiresCustomerAction &&
		!result.Authorized {
		public.ClientSecret = result.ClientSecret
	}

	response.OK(
		c,
		"FLEX authorization processed",
		public,
	)
}

// SetConfirmationHandler attaches the optional passenger confirmation
// dependency during startup, before the router begins serving requests.
func (h *PassengerFlexAuthorizationHandler) SetConfirmationHandler(
	confirmation *PassengerFlexConfirmationHandler,
) {
	if h == nil {
		return
	}
	h.confirmationHandler = confirmation
}
