package presence

import (
	"context"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ComplianceEvaluator interface {
	IsEligible(
		ctx context.Context,
		driverID string,
		now time.Time,
	) (bool, error)
}

type Dependencies struct {
	DB     *pgxpool.Pool
	Config *config.Config

	Users       repository.UserRepository
	Drivers     repository.DriverRepository
	Presence    repository.DriverPresenceRepository
	Assignments repository.DriverAssignmentRepository
	Compliance  ComplianceEvaluator
}

type Service struct {
	db  *pgxpool.Pool
	cfg *config.Config

	users       repository.UserRepository
	drivers     repository.DriverRepository
	presence    repository.DriverPresenceRepository
	assignments repository.DriverAssignmentRepository
	compliance  ComplianceEvaluator

	expireStaleIdle func(
		ctx context.Context,
		driverID string,
		staleBefore time.Time,
	) (bool, error)

	expireAllStaleIdle func(
		ctx context.Context,
		staleBefore time.Time,
	) (int64, error)
}

func NewService(
	deps Dependencies,
) *Service {
	service := &Service{
		db:  deps.DB,
		cfg: deps.Config,

		users:       deps.Users,
		drivers:     deps.Drivers,
		presence:    deps.Presence,
		assignments: deps.Assignments,
		compliance:  deps.Compliance,
	}

	if deps.Presence != nil {
		service.expireStaleIdle = deps.Presence.ExpireStaleIdle
		service.expireAllStaleIdle = deps.Presence.ExpireAllStaleIdle
	}

	return service
}
