package repository

import (
	"context"
	"errors"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrRoleNotFound = errors.New("role not found")
)

type RoleRepository interface {
	GetByID(ctx context.Context, id string) (*models.Role, error)
	GetByName(ctx context.Context, name string) (*models.Role, error)
}

type PostgresRoleRepository struct {
	db *pgxpool.Pool
}

func NewRoleRepository(db *pgxpool.Pool) RoleRepository {
	return &PostgresRoleRepository{
		db: db,
	}
}

func (r *PostgresRoleRepository) GetByID(
	ctx context.Context,
	id string,
) (*models.Role, error) {

	query := `
	SELECT
		id,
		name,
		description,
		created_at,
		updated_at
	FROM roles
	WHERE id = $1;
	`

	var role models.Role

	err := r.db.QueryRow(ctx, query, id).Scan(
		&role.ID,
		&role.Name,
		&role.Description,
		&role.CreatedAt,
		&role.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRoleNotFound
	}

	if err != nil {
		return nil, err
	}

	return &role, nil
}

func (r *PostgresRoleRepository) GetByName(
	ctx context.Context,
	name string,
) (*models.Role, error) {

	query := `
	SELECT
		id,
		name,
		description,
		created_at,
		updated_at
	FROM roles
	WHERE name = $1;
	`

	var role models.Role

	err := r.db.QueryRow(ctx, query, name).Scan(
		&role.ID,
		&role.Name,
		&role.Description,
		&role.CreatedAt,
		&role.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRoleNotFound
	}

	if err != nil {
		return nil, err
	}

	return &role, nil
}
