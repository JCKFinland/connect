package postgres

import "github.com/jackc/pgx/v5/pgxpool"

// PaymentTransactionRepository provides PostgreSQL persistence
// for payment transaction records.
type PaymentTransactionRepository struct {
	db DBTX
}

func NewPaymentTransactionRepository(
	db *pgxpool.Pool,
) *PaymentTransactionRepository {
	return &PaymentTransactionRepository{
		db: db,
	}
}

func NewPaymentTransactionRepositoryWithDB(
	db DBTX,
) *PaymentTransactionRepository {
	return &PaymentTransactionRepository{
		db: db,
	}
}
