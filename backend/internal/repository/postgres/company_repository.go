package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type CompanyRepository struct {
	db *pgxpool.Pool
}

func NewCompanyRepository(
	db *pgxpool.Pool,
) *CompanyRepository {

	return &CompanyRepository{
		db: db,
	}
}

func (r *CompanyRepository) Create(
	ctx context.Context,
	company *models.Company,
) error {

	query := `
	INSERT INTO companies
	(
		name,
		legal_name,
		business_id,
		email,
		phone,
		website,
		country_code,
		timezone,
		address_line1,
		address_line2,
		city,
		state,
		postal_code,
		logo_url,
		is_active
	)
	VALUES
	(
		$1,$2,$3,$4,$5,$6,$7,$8,
		$9,$10,$11,$12,$13,$14,$15
	)
	RETURNING
		id,
		created_at,
		updated_at
	`

	return r.db.QueryRow(
		ctx,
		query,
		company.Name,
		company.LegalName,
		company.BusinessID,
		company.Email,
		company.Phone,
		company.Website,
		company.CountryCode,
		company.Timezone,
		company.AddressLine1,
		company.AddressLine2,
		company.City,
		company.State,
		company.PostalCode,
		company.LogoURL,
		company.IsActive,
	).Scan(
		&company.ID,
		&company.CreatedAt,
		&company.UpdatedAt,
	)
}

func (r *CompanyRepository) GetByID(
	ctx context.Context,
	id string,
) (*models.Company, error) {

	query := `
	SELECT
		id,
		name,
		legal_name,
		business_id,
		email,
		phone,
		website,
		country_code,
		timezone,
		address_line1,
		address_line2,
		city,
		state,
		postal_code,
		logo_url,
		is_active,
		created_at,
		updated_at,
		deleted_at
	FROM companies
	WHERE id=$1
	AND deleted_at IS NULL;
	`

	var company models.Company

	err := r.db.QueryRow(
		ctx,
		query,
		id,
	).Scan(
		&company.ID,
		&company.Name,
		&company.LegalName,
		&company.BusinessID,
		&company.Email,
		&company.Phone,
		&company.Website,
		&company.CountryCode,
		&company.Timezone,
		&company.AddressLine1,
		&company.AddressLine2,
		&company.City,
		&company.State,
		&company.PostalCode,
		&company.LogoURL,
		&company.IsActive,
		&company.CreatedAt,
		&company.UpdatedAt,
		&company.DeletedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, repository.ErrNotFound
	}

	if err != nil {
		return nil, err
	}

	return &company, nil
}

func (r *CompanyRepository) List(
	ctx context.Context,
) ([]*models.Company, error) {

	query := `
	SELECT
		id,
		name,
		legal_name,
		business_id,
		email,
		phone,
		website,
		country_code,
		timezone,
		address_line1,
		address_line2,
		city,
		state,
		postal_code,
		logo_url,
		is_active,
		created_at,
		updated_at,
		deleted_at
	FROM companies
	WHERE deleted_at IS NULL
	ORDER BY name;
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var companies []*models.Company

	for rows.Next() {

		var company models.Company

		if err := rows.Scan(
			&company.ID,
			&company.Name,
			&company.LegalName,
			&company.BusinessID,
			&company.Email,
			&company.Phone,
			&company.Website,
			&company.CountryCode,
			&company.Timezone,
			&company.AddressLine1,
			&company.AddressLine2,
			&company.City,
			&company.State,
			&company.PostalCode,
			&company.LogoURL,
			&company.IsActive,
			&company.CreatedAt,
			&company.UpdatedAt,
			&company.DeletedAt,
		); err != nil {
			return nil, err
		}

		companies = append(companies, &company)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return companies, nil
}

func (r *CompanyRepository) Update(
	ctx context.Context,
	company *models.Company,
) error {

	query := `
	UPDATE companies
	SET
		name=$1,
		legal_name=$2,
		business_id=$3,
		email=$4,
		phone=$5,
		website=$6,
		country_code=$7,
		timezone=$8,
		address_line1=$9,
		address_line2=$10,
		city=$11,
		state=$12,
		postal_code=$13,
		logo_url=$14,
		is_active=$15,
		updated_at=NOW()
	WHERE id=$16
	AND deleted_at IS NULL;
	`

	cmd, err := r.db.Exec(
		ctx,
		query,
		company.Name,
		company.LegalName,
		company.BusinessID,
		company.Email,
		company.Phone,
		company.Website,
		company.CountryCode,
		company.Timezone,
		company.AddressLine1,
		company.AddressLine2,
		company.City,
		company.State,
		company.PostalCode,
		company.LogoURL,
		company.IsActive,
		company.ID,
	)

	if err != nil {
		return err
	}

	if cmd.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	return nil
}

func (r *CompanyRepository) Delete(
	ctx context.Context,
	id string,
) error {

	query := `
	UPDATE companies
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