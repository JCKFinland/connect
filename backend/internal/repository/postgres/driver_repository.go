package postgres

import (
	"github.com/JCKFinland/connect/backend/internal/repository"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DriverRepository implements repository.DriverRepository using PostgreSQL.
type DriverRepository struct {
	db DBTX
}

// Compile-time interface compliance check.
var _ repository.DriverRepository = (*DriverRepository)(nil)

// NewDriverRepository creates a new PostgreSQL driver repository
// backed by the connection pool.
func NewDriverRepository(
	db *pgxpool.Pool,
) *DriverRepository {

	return &DriverRepository{
		db: db,
	}
}

// NewDriverRepositoryWithDB creates a driver repository backed by
// either the connection pool or an active PostgreSQL transaction.
func NewDriverRepositoryWithDB(
	db DBTX,
) *DriverRepository {

	return &DriverRepository{
		db: db,
	}
}
