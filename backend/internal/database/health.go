package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Health checks database connectivity.
func Health(pool *pgxpool.Pool) error {

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return pool.Ping(ctx)
}