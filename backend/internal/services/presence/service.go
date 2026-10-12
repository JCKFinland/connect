package presence

import (
	"context"
	"sync"
	"time"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/repository"
	postgresrepo "github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ComplianceEvaluator interface {
	IsEligible(
		ctx context.Context,
		driverID string,
		now time.Time,
	) (bool, error)
}

type ComplianceEvaluatorFactory func(
	db postgresrepo.DBTX,
) ComplianceEvaluator

type Dependencies struct {
	DB     *pgxpool.Pool
	Config *config.Config

	Users           repository.UserRepository
	Drivers         repository.DriverRepository
	Presence        repository.DriverPresenceRepository
	Assignments     repository.DriverAssignmentRepository
	Compliance      ComplianceEvaluator
	ComplianceForDB ComplianceEvaluatorFactory
}

type Service struct {
	db  *pgxpool.Pool
	cfg *config.Config

	// Serializes reconciliation cycles and protects the keyset cursor.
	staleIdleCursorMu sync.Mutex
	staleIdleCursor   string

	// Independent cursor for regulatory compliance reconciliation.
	regulatoryCursorMu sync.Mutex
	regulatoryCursor   string

	users           repository.UserRepository
	drivers         repository.DriverRepository
	presence        repository.DriverPresenceRepository
	assignments     repository.DriverAssignmentRepository
	compliance      ComplianceEvaluator
	complianceForDB ComplianceEvaluatorFactory

	expireStaleIdle func(
		ctx context.Context,
		driverID string,
		staleBefore time.Time,
	) (bool, error)

	expireAllStaleIdle func(
		ctx context.Context,
		staleBefore time.Time,
	) (int64, error)

	// Production worker uses transactional bounded reconciliation.
	// Tests may inject a deterministic callback.
	reconcileStaleIdleBounded func(
		ctx context.Context,
		now time.Time,
		batchSize int,
	) (int64, error)

	// Independent regulatory reconciliation pass.
	reconcileRegulatoryBounded func(
		ctx context.Context,
		now time.Time,
		batchSize int,
	) (int64, error)
}

func NewService(
	deps Dependencies,
) *Service {
	service := &Service{
		db:  deps.DB,
		cfg: deps.Config,

		users:           deps.Users,
		drivers:         deps.Drivers,
		presence:        deps.Presence,
		assignments:     deps.Assignments,
		compliance:      deps.Compliance,
		complianceForDB: deps.ComplianceForDB,
	}

	if deps.Presence != nil {
		service.expireStaleIdle = deps.Presence.ExpireStaleIdle
		service.expireAllStaleIdle = deps.Presence.ExpireAllStaleIdle
	}

	service.reconcileStaleIdleBounded = service.ReconcileStaleIdleBounded
	service.reconcileRegulatoryBounded = service.ReconcileRegulatoryBounded
	return service
}
