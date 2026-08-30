package ride_request

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/JCKFinland/connect/backend/internal/models"
)

var ErrInvalidRideRequestExpiry = errors.New(
	"invalid ride request expiry",
)

// Create creates a new ride request.
//
// If expires_at is omitted, CONNECT applies the configured default matching
// lifetime. An explicitly supplied expires_at must be strictly in the future.
func (s *Service) Create(
	ctx context.Context,
	req CreateRideRequestRequest,
) (*models.RideRequest, error) {

	if s == nil {
		return nil, errors.New(
			"ride request service is required",
		)
	}

	if s.repo == nil {
		return nil, errors.New(
			"ride request repository is not configured",
		)
	}

	if s.cfg == nil {
		return nil, errors.New(
			"ride request configuration is not configured",
		)
	}

	if s.cfg.RideRequest.DefaultMatchingLifetime <= 0 {
		return nil, errors.New(
			"ride request default matching lifetime must be greater than zero",
		)
	}

	if s.serviceCategories == nil {
		return nil, errors.New(
			"service category repository is not configured",
		)
	}

	if req.ServiceCategoryID == "" {
		return nil, errors.New(
			"service category ID is required",
		)
	}

	category, err := s.serviceCategories.GetByID(
		ctx,
		req.ServiceCategoryID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"get service category: %w",
			err,
		)
	}

	if !category.IsActive {
		return nil, errors.New(
			"service category is inactive",
		)
	}

	now := time.Now().UTC()

	var expiresAt time.Time

	if req.ExpiresAt != nil {

		requestedExpiry := req.ExpiresAt.UTC()

		if !requestedExpiry.After(now) {
			return nil, fmt.Errorf(
				"%w: expires_at must be in the future",
				ErrInvalidRideRequestExpiry,
			)
		}

		expiresAt = requestedExpiry

	} else {

		expiresAt = now.Add(
			s.cfg.RideRequest.DefaultMatchingLifetime,
		)
	}

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
		ServiceCategoryID:    &req.ServiceCategoryID,
		PassengerCount:       req.PassengerCount,

		Status: StatusPending,

		Notes: req.Notes,

		RequestedAt: now,
		ExpiresAt:   &expiresAt,
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
