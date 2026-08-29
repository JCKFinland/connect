package postgres

import (
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FarePricingProfileRepository struct {
	db DBTX
}

var _ repository.FarePricingProfileRepository = (*FarePricingProfileRepository)(nil)

func NewFarePricingProfileRepository(
	db *pgxpool.Pool,
) *FarePricingProfileRepository {
	return &FarePricingProfileRepository{
		db: db,
	}
}

func NewFarePricingProfileRepositoryWithDB(
	db DBTX,
) *FarePricingProfileRepository {
	return &FarePricingProfileRepository{
		db: db,
	}
}
