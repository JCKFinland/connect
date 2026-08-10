package postgres

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

type TripRepository struct {
	db *pgxpool.Pool
}

func NewTripRepository(db *pgxpool.Pool) *TripRepository {
	return &TripRepository{
		db: db,
	}
}
