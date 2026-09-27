package postgres

import "github.com/jackc/pgx/v5/pgxpool"

// DriverEarningRepository provides PostgreSQL persistence
// for authoritative driver earnings.
type DriverEarningRepository struct {
	db DBTX
}

func NewDriverEarningRepository(
	db *pgxpool.Pool,
) *DriverEarningRepository {
	return &DriverEarningRepository{
		db: db,
	}
}

func NewDriverEarningRepositoryWithDB(
	db DBTX,
) *DriverEarningRepository {
	return &DriverEarningRepository{
		db: db,
	}
}
