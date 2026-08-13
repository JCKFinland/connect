package postgres

import (
	"github.com/JCKFinland/connect/backend/internal/repository"
)

// NewTripRepositoryWithDB creates a trip repository using either
// a normal connection pool or an active PostgreSQL transaction.
func NewTripRepositoryWithDB(
	db DBTX,
) *TripRepository {
	return &TripRepository{
		db: db,
	}
}

// NewVehicleRepositoryWithDB creates a vehicle repository
// using either a normal connection pool or an active transaction.
func NewVehicleRepositoryWithDB(
	db DBTX,
) repository.VehicleRepository {
	return &VehicleRepository{
		db: db,
	}
}

// NewRideRequestRepositoryWithDB creates a ride request repository
// using either a normal connection pool or an active transaction.
func NewRideRequestRepositoryWithDB(
	db DBTX,
) *RideRequestRepository {
	return &RideRequestRepository{
		db: db,
	}
}

// NewDriverPresenceRepositoryWithDB creates a driver presence
// repository using either a normal connection pool or an active transaction.
func NewDriverPresenceRepositoryWithDB(
	db DBTX,
) repository.DriverPresenceRepository {
	return &DriverPresenceRepository{
		db: db,
	}
}

// NewDriverAssignmentRepositoryWithDB creates a driver assignment
// repository using either a normal connection pool or an active transaction.
func NewDriverAssignmentRepositoryWithDB(
	db DBTX,
) repository.DriverAssignmentRepository {
	return &DriverAssignmentRepository{
		db: db,
	}
}
