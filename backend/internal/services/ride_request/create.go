package ride_request

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// Create creates a new ride request.
func (s *Service) Create(
	ctx context.Context,
	req CreateRideRequestRequest,
) (*models.RideRequest, error) {
	now := time.Now().UTC()

	request := &models.RideRequest{
		BaseModel: models.BaseModel{
			ID:        uuid.NewString(),
			CreatedAt: now,
			UpdatedAt: now,
		},

		CustomerID: req.CustomerID,

		PickupAddress:   req.PickupAddress,
		PickupLatitude:  req.PickupLatitude,
		PickupLongitude: req.PickupLongitude,

		DestinationAddress:   req.DestinationAddress,
		DestinationLatitude:  req.DestinationLatitude,
		DestinationLongitude: req.DestinationLongitude,

		RequestedVehicleType: req.RequestedVehicleType,
		PassengerCount:       req.PassengerCount,

		Status: "PENDING",

		Notes: req.Notes,

		RequestedAt: now,
		ExpiresAt:   req.ExpiresAt,
	}

	if request.RequestedVehicleType == "" {
		request.RequestedVehicleType = "STANDARD"
	}

	if request.PassengerCount <= 0 {
		request.PassengerCount = 1
	}

	if err := s.repo.Create(
		ctx,
		request,
	); err != nil {
		return nil, err
	}

	return request, nil
}
