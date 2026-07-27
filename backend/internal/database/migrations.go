package database

import (
	"database/sql"
	"fmt"
	"log/slog"
	"path/filepath"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/JCKFinland/connect/backend/internal/config"
)

// RunMigrations executes all pending Goose migrations.
func RunMigrations(cfg *config.Config, log *slog.Logger) error {

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Name,
		cfg.Database.SSLMode,
	)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	migrationsDir := filepath.Join(".", "migrations")

	if err := goose.Up(db, migrationsDir); err != nil {
		return err
	}

	log.Info("Database migrations completed successfully")

	return nil
}