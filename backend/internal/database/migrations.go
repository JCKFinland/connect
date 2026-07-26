package database

import (
	"log/slog"
)

// RunMigrations executes all pending database migrations.
//
// Goose integration will be added in Milestone 3.
func RunMigrations(log *slog.Logger) error {

	log.Info("Database migrations skipped (not implemented yet)")

	return nil
}