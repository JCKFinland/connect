package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type FleetRepository struct {
	db *pgxpool.Pool
}

func NewFleetRepository(
	db *pgxpool.Pool,
) *FleetRepository {

	return &FleetRepository{
		db: db,
	}
}

func (r *FleetRepository) Create(
	ctx context.Context,
	fleet *models.Fleet,
) error {

	query := `
	INSERT INTO fleets
	(
		company_id,
		branch_id,
		code,
		name,
		description,
		is_active
	)
	VALUES
	(
		$1,$2,$3,$4,$5,$6
	)
	RETURNING
		id,
		created_at,
		updated_at;
	`

	return r.db.QueryRow(
		ctx,
		query,
		fleet.CompanyID,
		fleet.BranchID,
		fleet.Code,
		fleet.Name,
		fleet.Description,
		fleet.IsActive,
	).Scan(
		&fleet.ID,
		&fleet.CreatedAt,
		&fleet.UpdatedAt,
	)
}

func (r *FleetRepository) GetByID(
	ctx context.Context,
	id string,
) (*models.Fleet, error) {

	query := `
	SELECT
		id,
		company_id,
		branch_id,
		code,
		name,
		description,
		is_active,
		created_at,
		updated_at,
		deleted_at
	FROM fleets
	WHERE id=$1
	AND deleted_at IS NULL;
	`

	var fleet models.Fleet

	err := r.db.QueryRow(
		ctx,
		query,
		id,
	).Scan(
		&fleet.ID,
		&fleet.CompanyID,
		&fleet.BranchID,
		&fleet.Code,
		&fleet.Name,
		&fleet.Description,
		&fleet.IsActive,
		&fleet.CreatedAt,
		&fleet.UpdatedAt,
		&fleet.DeletedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, repository.ErrNotFound
	}

	if err != nil {
		return nil, err
	}

	return &fleet, nil
}

func (r *FleetRepository) List(
	ctx context.Context,
) ([]*models.Fleet, error) {

	query := `
	SELECT
		id,
		company_id,
		branch_id,
		code,
		name,
		description,
		is_active,
		created_at,
		updated_at,
		deleted_at
	FROM fleets
	WHERE deleted_at IS NULL
	ORDER BY name;
	`

	rows, err := r.db.Query(
		ctx,
		query,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fleets []*models.Fleet

	for rows.Next() {

		var fleet models.Fleet

		if err := rows.Scan(
			&fleet.ID,
			&fleet.CompanyID,
			&fleet.BranchID,
			&fleet.Code,
			&fleet.Name,
			&fleet.Description,
			&fleet.IsActive,
			&fleet.CreatedAt,
			&fleet.UpdatedAt,
			&fleet.DeletedAt,
		); err != nil {
			return nil, err
		}

		fleets = append(
			fleets,
			&fleet,
		)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return fleets, nil
}

func (r *FleetRepository) Update(
	ctx context.Context,
	fleet *models.Fleet,
) error {

	query := `
	UPDATE fleets
	SET
		company_id=$1,
		branch_id=$2,
		code=$3,
		name=$4,
		description=$5,
		is_active=$6,
		updated_at=NOW()
	WHERE id=$7
	AND deleted_at IS NULL;
	`

	cmd, err := r.db.Exec(
		ctx,
		query,
		fleet.CompanyID,
		fleet.BranchID,
		fleet.Code,
		fleet.Name,
		fleet.Description,
		fleet.IsActive,
		fleet.ID,
	)
	if err != nil {
		return err
	}

	if cmd.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	return nil
}

func (r *FleetRepository) Delete(
	ctx context.Context,
	id string,
) error {

	query := `
	UPDATE fleets
	SET
		deleted_at=NOW(),
		updated_at=NOW()
	WHERE id=$1
	AND deleted_at IS NULL;
	`

	cmd, err := r.db.Exec(
		ctx,
		query,
		id,
	)
	if err != nil {
		return err
	}

	if cmd.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	return nil
}
