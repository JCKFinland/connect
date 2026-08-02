package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type BranchRepository struct {
	db *pgxpool.Pool
}

func NewBranchRepository(
	db *pgxpool.Pool,
) *BranchRepository {

	return &BranchRepository{
		db: db,
	}
}

func (r *BranchRepository) Create(
	ctx context.Context,
	branch *models.Branch,
) error {

	query := `
	INSERT INTO branches
	(
		company_id,
		code,
		name,
		email,
		phone,
		address_line1,
		address_line2,
		city,
		state,
		postal_code,
		latitude,
		longitude,
		is_active
	)
	VALUES
	(
		$1,$2,$3,$4,$5,$6,$7,
		$8,$9,$10,$11,$12,$13
	)
	RETURNING
		id,
		created_at,
		updated_at
	`

	return r.db.QueryRow(
		ctx,
		query,
		branch.CompanyID,
		branch.Code,
		branch.Name,
		branch.Email,
		branch.Phone,
		branch.AddressLine1,
		branch.AddressLine2,
		branch.City,
		branch.State,
		branch.PostalCode,
		branch.Latitude,
		branch.Longitude,
		branch.IsActive,
	).Scan(
		&branch.ID,
		&branch.CreatedAt,
		&branch.UpdatedAt,
	)
}

func (r *BranchRepository) GetByID(
	ctx context.Context,
	id string,
) (*models.Branch, error) {

	query := `
	SELECT
		id,
		company_id,
		code,
		name,
		email,
		phone,
		address_line1,
		address_line2,
		city,
		state,
		postal_code,
		latitude,
		longitude,
		is_active,
		created_at,
		updated_at,
		deleted_at
	FROM branches
	WHERE id = $1
	AND deleted_at IS NULL;
	`

	var branch models.Branch

	err := r.db.QueryRow(
		ctx,
		query,
		id,
	).Scan(
		&branch.ID,
		&branch.CompanyID,
		&branch.Code,
		&branch.Name,
		&branch.Email,
		&branch.Phone,
		&branch.AddressLine1,
		&branch.AddressLine2,
		&branch.City,
		&branch.State,
		&branch.PostalCode,
		&branch.Latitude,
		&branch.Longitude,
		&branch.IsActive,
		&branch.CreatedAt,
		&branch.UpdatedAt,
		&branch.DeletedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, repository.ErrNotFound
	}

	if err != nil {
		return nil, err
	}

	return &branch, nil
}

func (r *BranchRepository) List(
	ctx context.Context,
) ([]*models.Branch, error) {

	query := `
	SELECT
		id,
		company_id,
		code,
		name,
		email,
		phone,
		address_line1,
		address_line2,
		city,
		state,
		postal_code,
		latitude,
		longitude,
		is_active,
		created_at,
		updated_at,
		deleted_at
	FROM branches
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

	var branches []*models.Branch

	for rows.Next() {

		var branch models.Branch

		if err := rows.Scan(
			&branch.ID,
			&branch.CompanyID,
			&branch.Code,
			&branch.Name,
			&branch.Email,
			&branch.Phone,
			&branch.AddressLine1,
			&branch.AddressLine2,
			&branch.City,
			&branch.State,
			&branch.PostalCode,
			&branch.Latitude,
			&branch.Longitude,
			&branch.IsActive,
			&branch.CreatedAt,
			&branch.UpdatedAt,
			&branch.DeletedAt,
		); err != nil {
			return nil, err
		}

		branches = append(
			branches,
			&branch,
		)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return branches, nil
}

func (r *BranchRepository) Update(
	ctx context.Context,
	branch *models.Branch,
) error {

	query := `
	UPDATE branches
	SET
		company_id = $1,
		code = $2,
		name = $3,
		email = $4,
		phone = $5,
		address_line1 = $6,
		address_line2 = $7,
		city = $8,
		state = $9,
		postal_code = $10,
		latitude = $11,
		longitude = $12,
		is_active = $13,
		updated_at = NOW()
	WHERE id = $14
	AND deleted_at IS NULL;
	`

	cmd, err := r.db.Exec(
		ctx,
		query,
		branch.CompanyID,
		branch.Code,
		branch.Name,
		branch.Email,
		branch.Phone,
		branch.AddressLine1,
		branch.AddressLine2,
		branch.City,
		branch.State,
		branch.PostalCode,
		branch.Latitude,
		branch.Longitude,
		branch.IsActive,
		branch.ID,
	)
	if err != nil {
		return err
	}

	if cmd.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	return nil
}

func (r *BranchRepository) Delete(
	ctx context.Context,
	id string,
) error {

	query := `
	UPDATE branches
	SET
		deleted_at = NOW(),
		updated_at = NOW()
	WHERE id = $1
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
