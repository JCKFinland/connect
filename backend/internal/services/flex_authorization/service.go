package flex_authorization

import (
	"errors"

	"github.com/JCKFinland/connect/backend/internal/payments/stripe"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrServiceUnavailable = errors.New(
		"FLEX authorization service unavailable",
	)

	ErrAuthorizationPending = errors.New(
		"FLEX authorization processing is pending recovery",
	)

	ErrAuthorizationNotEligible = errors.New(
		"FLEX authorization is not eligible for execution",
	)
)

// Dependencies are supplied by the application composition layer.
type Dependencies struct {
	DB      *pgxpool.Pool
	Adapter stripe.FlexAuthorizationAdapter
}

// Service coordinates FLEX pre-trip authorization.
//
// It does not capture payments, confirm auctions, or create trips.
type Service struct {
	db      *pgxpool.Pool
	adapter stripe.FlexAuthorizationAdapter
}

func NewService(deps Dependencies) *Service {
	return &Service{
		db:      deps.DB,
		adapter: deps.Adapter,
	}
}
