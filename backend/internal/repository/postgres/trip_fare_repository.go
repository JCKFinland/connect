package postgres

import "github.com/jackc/pgx/v5/pgxpool"

// TripFareRepository provides PostgreSQL persistence
// for trip fare records.
type TripFareRepository struct {
	db DBTX
}

func NewTripFareRepository(
	db *pgxpool.Pool,
) *TripFareRepository {
	return &TripFareRepository{
		db: db,
	}
}

func NewTripFareRepositoryWithDB(
	db DBTX,
) *TripFareRepository {
	return &TripFareRepository{
		db: db,
	}
}
