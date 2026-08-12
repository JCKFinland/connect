package dispatch

import (
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Dependencies struct {
	DB *pgxpool.Pool

	RideRequests repository.RideRequestRepository
	Assignments  repository.DriverAssignmentRepository
	Presence     repository.DriverPresenceRepository
	Trips        repository.TripRepository
}

type Service struct {
	db *pgxpool.Pool

	rideRequests repository.RideRequestRepository
	assignments  repository.DriverAssignmentRepository
	presence     repository.DriverPresenceRepository
	trips        repository.TripRepository
}

func NewService(
	deps Dependencies,
) *Service {
	return &Service{
		db: deps.DB,

		rideRequests: deps.RideRequests,
		assignments:  deps.Assignments,
		presence:     deps.Presence,
		trips:        deps.Trips,
	}
}
