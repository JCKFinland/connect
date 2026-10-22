package api

import "github.com/gin-gonic/gin"

// RegisterPassengerFlexConfirmationRoute attaches confirmation
// to an already-authenticated passenger ride-request group.
// It must only be called with a group protected by RequireAuth.
func RegisterPassengerFlexConfirmationRoute(
	rideRequests *gin.RouterGroup,
	handler *PassengerFlexConfirmationHandler,
) {
	if rideRequests == nil || handler == nil {
		return
	}

	rideRequests.POST(
		"/:id/flex-auctions/:auction_id/confirm",
		handler.Confirm,
	)
}
