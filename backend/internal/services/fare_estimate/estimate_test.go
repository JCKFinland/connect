package fare_estimate

import (
	"context"
	"testing"
	"time"

	"github.com/JCKFinland/connect/backend/internal/services/fare"
	"github.com/JCKFinland/connect/backend/internal/services/pricing"
	"github.com/JCKFinland/connect/backend/internal/services/routing"
)

type routingStub struct {
	request routing.RouteRequest
	result  *routing.RouteMetrics
	err     error
}

func (s *routingStub) Route(
	_ context.Context,
	request routing.RouteRequest,
) (*routing.RouteMetrics, error) {
	s.request = request

	return s.result, s.err
}

type pricingStub struct {
	input  pricing.ResolveInput
	result *pricing.ResolveResult
	err    error
}

func (s *pricingStub) Resolve(
	_ context.Context,
	input pricing.ResolveInput,
) (*pricing.ResolveResult, error) {
	s.input = input

	return s.result, s.err
}

func TestEstimateUsesBookingCompanyAndCanonicalFare(
	t *testing.T,
) {
	router := &routingStub{
		result: &routing.RouteMetrics{
			DistanceMeters:  8420,
			DurationSeconds: 900,
		},
	}

	pricer := &pricingStub{
		result: &pricing.ResolveResult{
			ProfileID:         "profile-1",
			ServiceCategoryID: "category-1",
			Pricing: fare.PricingSnapshot{
				BaseFare:             4.90,
				DistanceRatePerKM:    1.50,
				TimeRatePerMinute:    0.25,
				WaitingRatePerMinute: 0.50,
				BookingFee:           2.00,
				SurgeMultiplier:      1.00,
				Currency:             "EUR",
				PricingVersion:       "v1",
			},
		},
	}

	svc := NewService(
		Dependencies{
			Routing: router,
			Pricing: pricer,
			Fare:    fare.NewService(),

			BookingCompanyID: "booking-company-1",
		},
	)

	internalService := svc.(*service)

	fixedTime := time.Date(
		2026,
		time.September,
		18,
		20,
		0,
		0,
		0,
		time.UTC,
	)

	internalService.now = func() time.Time {
		return fixedTime
	}

	result, err := svc.Estimate(
		context.Background(),
		EstimateRequest{
			PickupLatitude:  60.2055,
			PickupLongitude: 24.6559,

			DestinationLatitude:  60.1719,
			DestinationLongitude: 24.9414,

			ServiceCategoryID: "category-1",
		},
	)
	if err != nil {
		t.Fatalf(
			"Estimate() unexpected error: %v",
			err,
		)
	}

	if pricer.input.CompanyID !=
		"booking-company-1" {
		t.Fatalf(
			"pricing company = %q, want %q",
			pricer.input.CompanyID,
			"booking-company-1",
		)
	}

	if pricer.input.BranchID != nil {
		t.Fatalf(
			"pricing branch = %v, want nil",
			pricer.input.BranchID,
		)
	}

	if pricer.input.ServiceCategoryID !=
		"category-1" {
		t.Fatalf(
			"pricing category = %q, want %q",
			pricer.input.ServiceCategoryID,
			"category-1",
		)
	}

	if !pricer.input.At.Equal(fixedTime) {
		t.Fatalf(
			"pricing time = %v, want %v",
			pricer.input.At,
			fixedTime,
		)
	}

	if result.DistanceMeters != 8420 {
		t.Fatalf(
			"distance = %d, want 8420",
			result.DistanceMeters,
		)
	}

	if result.DurationSeconds != 900 {
		t.Fatalf(
			"duration = %d, want 900",
			result.DurationSeconds,
		)
	}

	if result.TotalAmount != 23.28 {
		t.Fatalf(
			"total = %.2f, want 23.28",
			result.TotalAmount,
		)
	}

	if result.Currency != "EUR" {
		t.Fatalf(
			"currency = %q, want EUR",
			result.Currency,
		)
	}

	if result.PricingProfileID != "profile-1" {
		t.Fatalf(
			"pricing profile = %q, want profile-1",
			result.PricingProfileID,
		)
	}

	if result.PricingVersion != "v1" {
		t.Fatalf(
			"pricing version = %q, want v1",
			result.PricingVersion,
		)
	}
}

func TestEstimateRejectsMissingServiceCategory(
	t *testing.T,
) {
	svc := NewService(
		Dependencies{
			BookingCompanyID: "booking-company-1",
		},
	)

	_, err := svc.Estimate(
		context.Background(),
		EstimateRequest{},
	)

	if err != ErrInvalidServiceCategoryID {
		t.Fatalf(
			"Estimate() error = %v, want %v",
			err,
			ErrInvalidServiceCategoryID,
		)
	}
}

func TestEstimateRejectsMissingBookingCompany(
	t *testing.T,
) {
	svc := NewService(
		Dependencies{},
	)

	_, err := svc.Estimate(
		context.Background(),
		EstimateRequest{
			ServiceCategoryID: "category-1",
		},
	)

	if err != ErrInvalidBookingCompanyID {
		t.Fatalf(
			"Estimate() error = %v, want %v",
			err,
			ErrInvalidBookingCompanyID,
		)
	}
}
