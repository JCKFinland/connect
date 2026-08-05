package postgres

import (
	"github.com/JCKFinland/connect/backend/internal/repository"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DriverRepository implements repository.DriverRepository using PostgreSQL.
type DriverRepository struct {
	db *pgxpool.Pool
}

// Compile-time interface compliance check.
var _ repository.DriverRepository = (*DriverRepository)(nil)

// NewDriverRepository creates a new PostgreSQL driver repository.
func NewDriverRepository(
	db *pgxpool.Pool,
) *DriverRepository {

	return &DriverRepository{
		db: db,
	}
}