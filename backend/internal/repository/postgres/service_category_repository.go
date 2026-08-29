package postgres

import (
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ServiceCategoryRepository implements
// repository.ServiceCategoryRepository.
type ServiceCategoryRepository struct {
	db DBTX
}

var _ repository.ServiceCategoryRepository = (*ServiceCategoryRepository)(nil)

// NewServiceCategoryRepository creates a PostgreSQL-backed
// service category repository.
func NewServiceCategoryRepository(
	db *pgxpool.Pool,
) *ServiceCategoryRepository {
	return &ServiceCategoryRepository{
		db: db,
	}
}

// NewServiceCategoryRepositoryWithDB allows the repository to use
// either the connection pool or an active transaction.
func NewServiceCategoryRepositoryWithDB(
	db DBTX,
) *ServiceCategoryRepository {
	return &ServiceCategoryRepository{
		db: db,
	}
}
