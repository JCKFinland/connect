package dispatch

import (
	"log/slog"

	"github.com/JCKFinland/connect/backend/internal/config"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Dependencies struct {
	DB     *pgxpool.Pool
	Config *config.Config
	Logger *slog.Logger

	RideRequests repository.RideRequestRepository
	Assignments  repository.DriverAssignmentRepository
	Presence     repository.DriverPresenceRepository
	Trips        repository.TripRepository
	Vehicles     repository.VehicleRepository
	Drivers      repository.DriverRepository
	Offers       repository.DispatchOfferRepository
}

type Service struct {
	db  *pgxpool.Pool
	cfg *config.Config
	log *slog.Logger

	rideRequests repository.RideRequestRepository
	assignments  repository.DriverAssignmentRepository
	presence     repository.DriverPresenceRepository
	trips        repository.TripRepository
	vehicles     repository.VehicleRepository
	drivers      repository.DriverRepository
	offers       repository.DispatchOfferRepository

	// beforeClaimCandidate is an internal test seam used to coordinate
	// deterministic concurrency tests between candidate ranking and the
	// final locked eligibility recheck.
	//
	// It is nil in normal production operation.
	beforeClaimCandidate func(driverID string)
}

func NewService(
	deps Dependencies,
) *Service {

	log := deps.Logger
	if log == nil {
		log = slog.Default()
	}

	return &Service{
		db:  deps.DB,
		cfg: deps.Config,
		log: log,

		rideRequests: deps.RideRequests,
		assignments:  deps.Assignments,
		presence:     deps.Presence,
		trips:        deps.Trips,
		vehicles:     deps.Vehicles,
		drivers:      deps.Drivers,
		offers:       deps.Offers,
	}
}
