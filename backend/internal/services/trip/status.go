package trip

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"

	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

// Valid trip lifecycle statuses.
const (
	StatusAssigned      = "ASSIGNED"
	StatusDriverEnRoute = "DRIVER_EN_ROUTE"
	StatusDriverArrived = "DRIVER_ARRIVED"
	StatusInProgress    = "IN_PROGRESS"
	StatusCompleted     = "COMPLETED"
	StatusCancelled     = "CANCELLED"
)

const (
	driverAvailabilityAvailable = "AVAILABLE"

	rideRequestStatusCancelled = "CANCELLED"
)

// UpdateStatus validates and atomically applies a trip lifecycle transition.
//
// Terminal transitions perform the related operational cleanup in the same
// PostgreSQL transaction:
//
//   - COMPLETED:
//
//   - mark trip COMPLETED
//
//   - maintain completed_at through the repository
//
//   - mark trip inactive
//
//   - release driver back to AVAILABLE
//
//   - CANCELLED:
//
//   - mark trip CANCELLED
//
//   - maintain cancelled_at through the repository
//
//   - mark trip inactive
//
//   - mark the source ride request CANCELLED
//
//   - release driver back to AVAILABLE
//
// If any operation fails, the entire transaction is rolled back.
func (s *tripService) UpdateStatus(
	ctx context.Context,
	id string,
	newStatus string,
	performedByUserID string,
) error {

	if id == "" {
		return fmt.Errorf("trip ID is required")
	}

	if !isValidStatus(newStatus) {
		return fmt.Errorf(
			"invalid trip status: %s",
			newStatus,
		)
	}

	if s.db == nil {
		return fmt.Errorf(
			"trip database is not configured",
		)
	}

	return postgresrepo.RunInTransaction(
		ctx,
		s.db,
		func(tx pgx.Tx) error {

			trips := postgresrepo.NewTripRepositoryWithDB(tx)
			rideRequests := postgresrepo.NewRideRequestRepositoryWithDB(tx)
			presence := postgresrepo.NewDriverPresenceRepositoryWithDB(tx)
			tripEvents := postgresrepo.NewTripEventRepositoryWithDB(tx)

			// ---------------------------------------------------------
			// Lock trip row
			// ---------------------------------------------------------

			currentTrip, err := trips.GetByIDForUpdate(
				ctx,
				id,
			)
			if err != nil {
				return fmt.Errorf(
					"get trip for status update: %w",
					err,
				)
			}

			// ---------------------------------------------------------
			// Validate lifecycle transition while row is locked
			// ---------------------------------------------------------

			if err := validateStatusTransition(
				currentTrip.Status,
				newStatus,
			); err != nil {
				return err
			}

			// ---------------------------------------------------------
			// Update trip lifecycle state
			//
			// TripRepository.UpdateStatus also maintains:
			// driver_arrived_at
			// started_at
			// completed_at
			// cancelled_at
			// is_active
			// ---------------------------------------------------------

			if err := trips.UpdateStatus(
				ctx,
				currentTrip.ID,
				newStatus,
			); err != nil {
				return fmt.Errorf(
					"update trip status: %w",
					err,
				)
			}
			eventType, ok := eventTypeForStatus(newStatus)
			if ok {
				metadata, err := json.Marshal(
					map[string]string{
						"previous_status": currentTrip.Status,
						"new_status":      newStatus,
					},
				)
				if err != nil {
					return fmt.Errorf(
						"marshal trip event metadata: %w",
						err,
					)
				}

				event := &models.TripEvent{
					TripID:     currentTrip.ID,
					EventType:  eventType,
					Metadata:   metadata,
					OccurredAt: time.Now().UTC(),
				}

				if performedByUserID != "" {
					event.PerformedByUserID = &performedByUserID
				}

				if err := tripEvents.Create(
					ctx,
					event,
				); err != nil {
					return fmt.Errorf(
						"create trip lifecycle event: %w",
						err,
					)
				}
			}

			// ---------------------------------------------------------
			// Non-terminal states require no resource release.
			// ---------------------------------------------------------

			if newStatus != StatusCompleted &&
				newStatus != StatusCancelled {
				return nil
			}

			// ---------------------------------------------------------
			// Release driver after terminal trip state
			// ---------------------------------------------------------

			if currentTrip.DriverID != "" {
				if err := presence.UpdateAvailability(
					ctx,
					currentTrip.DriverID,
					driverAvailabilityAvailable,
					true,
				); err != nil {
					return fmt.Errorf(
						"release driver after terminal trip: %w",
						err,
					)
				}
			}

			// ---------------------------------------------------------
			// Cancellation also closes the source ride request.
			//
			// COMPLETED does NOT change the ride request to COMPLETED
			// because ride_requests does not have that lifecycle state.
			// A successfully dispatched/completed request remains ACCEPTED
			// while the Trip carries operational completion.
			// ---------------------------------------------------------

			if newStatus == StatusCancelled &&
				currentTrip.RideRequestID != "" {

				if err := rideRequests.UpdateStatus(
					ctx,
					currentTrip.RideRequestID,
					rideRequestStatusCancelled,
				); err != nil {
					return fmt.Errorf(
						"cancel source ride request: %w",
						err,
					)
				}
			}

			return nil
		},
	)
}

func isValidStatus(
	status string,
) bool {

	switch status {

	case StatusAssigned,
		StatusDriverEnRoute,
		StatusDriverArrived,
		StatusInProgress,
		StatusCompleted,
		StatusCancelled:

		return true

	default:
		return false
	}
}

func validateStatusTransition(
	currentStatus string,
	newStatus string,
) error {

	if currentStatus == newStatus {
		return fmt.Errorf(
			"trip is already in status %s",
			currentStatus,
		)
	}

	switch currentStatus {

	case StatusAssigned:
		switch newStatus {
		case StatusDriverEnRoute,
			StatusCancelled:
			return nil
		}

	case StatusDriverEnRoute:
		switch newStatus {
		case StatusDriverArrived,
			StatusCancelled:
			return nil
		}

	case StatusDriverArrived:
		switch newStatus {
		case StatusInProgress,
			StatusCancelled:
			return nil
		}

	case StatusInProgress:
		switch newStatus {
		case StatusCompleted,
			StatusCancelled:
			return nil
		}

	case StatusCompleted:
		// COMPLETED is terminal.

	case StatusCancelled:
		// CANCELLED is terminal.
	}

	return fmt.Errorf(
		"invalid trip status transition: %s -> %s",
		currentStatus,
		newStatus,
	)
}
