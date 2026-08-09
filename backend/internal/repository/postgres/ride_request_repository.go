package postgres

import "github.com/jackc/pgx/v5/pgxpool"

// RideRequestRepository provides PostgreSQL persistence for ride requests.
type RideRequestRepository struct {
	db *pgxpool.Pool
}

// NewRideRequestRepository creates a new PostgreSQL ride request repository.
func NewRideRequestRepository(db *pgxpool.Pool) *RideRequestRepository {
	return &RideRequestRepository{
		db: db,
	}
}
