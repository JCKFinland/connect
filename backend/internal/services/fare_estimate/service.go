package fare_estimate

import (
	"context"
	"time"

	"github.com/JCKFinland/connect/backend/internal/services/fare"
	"github.com/JCKFinland/connect/backend/internal/services/pricing"
	"github.com/JCKFinland/connect/backend/internal/services/routing"
)

type Service interface {
	Estimate(
		ctx context.Context,
		request EstimateRequest,
	) (*EstimateResult, error)
}

type Dependencies struct {
	Routing routing.Service
	Pricing pricing.Service
	Fare    fare.Service

	BookingCompanyID string
}

type service struct {
	routing routing.Service
	pricing pricing.Service
	fare    fare.Service

	bookingCompanyID string
	now              func() time.Time
}

func NewService(deps Dependencies) Service {
	return &service{
		routing:          deps.Routing,
		pricing:          deps.Pricing,
		fare:             deps.Fare,
		bookingCompanyID: deps.BookingCompanyID,
		now:              time.Now,
	}
}

var _ Service = (*service)(nil)
