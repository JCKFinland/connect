package flex_offer

import (
	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Dependencies contains the infrastructure required for
// transactional FLEX driver offer submission.
type Dependencies struct {
	DB     *pgxpool.Pool
	Config *config.Config
}

// Service coordinates driver offer submission.
//
// It does not create trips, authorize payments,
// or mutate driver availability.
type Service struct {
	db  *pgxpool.Pool
	cfg *config.Config
}

// NewService constructs the FLEX driver offer service.
func NewService(deps Dependencies) *Service {
	return &Service{
		db:  deps.DB,
		cfg: deps.Config,
	}
}
