package postgres

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/repository"
)

// DriverVehicleAssignmentRepository implements
// repository.DriverVehicleAssignmentRepository.
type DriverVehicleAssignmentRepository struct {
	db *pgxpool.Pool
}

// NewDriverVehicleAssignmentRepository creates a new repository.
func NewDriverVehicleAssignmentRepository(
	db *pgxpool.Pool,
) repository.DriverVehicleAssignmentRepository {

	return &DriverVehicleAssignmentRepository{
		db: db,
	}
}

// Compile-time interface check.
var _ repository.DriverVehicleAssignmentRepository =
	(*DriverVehicleAssignmentRepository)(nil)