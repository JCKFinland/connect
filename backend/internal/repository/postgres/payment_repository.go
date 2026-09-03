package postgres

import "github.com/jackc/pgx/v5/pgxpool"

// PaymentRepository provides PostgreSQL persistence
// for payment records.
type PaymentRepository struct {
	db DBTX
}

func NewPaymentRepository(
	db *pgxpool.Pool,
) *PaymentRepository {
	return &PaymentRepository{
		db: db,
	}
}

func NewPaymentRepositoryWithDB(
	db DBTX,
) *PaymentRepository {
	return &PaymentRepository{
		db: db,
	}
}
