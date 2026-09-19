package fare_estimate

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/services/fare"
	"github.com/JCKFinland/connect/backend/internal/services/pricing"
	"github.com/JCKFinland/connect/backend/internal/services/routing"
)

func (s *service) Estimate(
	ctx context.Context,
	request EstimateRequest,
) (*EstimateResult, error) {
	if request.ServiceCategoryID == "" {
		return nil, ErrInvalidServiceCategoryID
	}

	if s.bookingCompanyID == "" {
		return nil, ErrInvalidBookingCompanyID
	}

	route, err := s.routing.Route(
		ctx,
		routing.RouteRequest{
			Origin: routing.Coordinate{
				Latitude:  request.PickupLatitude,
				Longitude: request.PickupLongitude,
			},
			Destination: routing.Coordinate{
				Latitude:  request.DestinationLatitude,
				Longitude: request.DestinationLongitude,
			},
		},
	)
	if err != nil {
		return nil, err
	}

	resolvedPricing, err := s.pricing.Resolve(
		ctx,
		pricing.ResolveInput{
			CompanyID: s.bookingCompanyID,

			ServiceCategoryID: request.ServiceCategoryID,

			At: s.now().UTC(),
		},
	)
	if err != nil {
		return nil, err
	}

	calculated, err := s.fare.Estimate(
		fare.EstimateInput{
			DistanceMeters: route.DistanceMeters,

			DurationSeconds: route.DurationSeconds,

			WaitingSeconds: 0,

			Pricing: resolvedPricing.Pricing,
		},
	)
	if err != nil {
		return nil, err
	}

	return &EstimateResult{
		DistanceMeters: calculated.DistanceMeters,

		DurationSeconds: calculated.DurationSeconds,

		BaseFare:     calculated.BaseFare,
		DistanceFare: calculated.DistanceFare,
		TimeFare:     calculated.TimeFare,
		BookingFee:   calculated.BookingFee,
		SurgeAmount:  calculated.SurgeAmount,

		TotalAmount: calculated.TotalAmount,

		Currency: calculated.Currency,

		PricingProfileID: resolvedPricing.ProfileID,

		PricingVersion: calculated.PricingVersion,
	}, nil
}
