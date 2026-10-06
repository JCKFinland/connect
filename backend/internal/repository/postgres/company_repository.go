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
	db DBTX
}

var _ repository.CompanyRepository = (*CompanyRepository)(nil)

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
	return r.getByID(ctx, "", id, false)
}

func (r *CompanyRepository) GetByIDForCompanyMember(
	ctx context.Context,
	userID string,
	id string,
) (*models.Company, error) {
	return r.getByID(ctx, userID, id, true)
}

func (r *CompanyRepository) getByID(
	ctx context.Context,
	userID string,
	id string,
	requireMembership bool,
) (*models.Company, error) {
	query := `
		SELECT
			c.id,
			c.name,
			COALESCE(c.legal_name, ''),
			COALESCE(c.business_id, ''),
			COALESCE(c.email, ''),
			COALESCE(c.phone, ''),
			COALESCE(c.website, ''),
			COALESCE(c.country_code, ''),
			COALESCE(c.timezone, ''),
			COALESCE(c.address_line1, ''),
			COALESCE(c.address_line2, ''),
			COALESCE(c.city, ''),
			COALESCE(c.state, ''),
			COALESCE(c.postal_code, ''),
			COALESCE(c.logo_url, ''),
			c.is_active,
			c.created_at,
			c.updated_at,
			c.deleted_at
		FROM companies c
		WHERE c.id = $1
		  AND c.deleted_at IS NULL
	`

	args := []any{id}

	if requireMembership {
		query += `
		  AND EXISTS (
			SELECT 1
			FROM company_memberships cm
			WHERE cm.user_id = $2
			  AND cm.company_id = c.id
		  )
		`
		args = append(args, userID)
	}

	var company models.Company

	err := r.db.QueryRow(
		ctx,
		query,
		args...,
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
	return r.list(ctx, "", false, false)
}

func (r *CompanyRepository) ListForCompanyMember(
	ctx context.Context,
	userID string,
) ([]*models.Company, error) {
	return r.list(ctx, userID, true, false)
}

func (r *CompanyRepository) ListActive(
	ctx context.Context,
) ([]*models.Company, error) {
	return r.list(ctx, "", false, true)
}

func (r *CompanyRepository) list(
	ctx context.Context,
	userID string,
	requireMembership bool,
	activeOnly bool,
) ([]*models.Company, error) {
	query := `
		SELECT
			c.id,
			c.name,
			COALESCE(c.legal_name, ''),
			COALESCE(c.business_id, ''),
			COALESCE(c.email, ''),
			COALESCE(c.phone, ''),
			COALESCE(c.website, ''),
			COALESCE(c.country_code, ''),
			COALESCE(c.timezone, ''),
			COALESCE(c.address_line1, ''),
			COALESCE(c.address_line2, ''),
			COALESCE(c.city, ''),
			COALESCE(c.state, ''),
			COALESCE(c.postal_code, ''),
			COALESCE(c.logo_url, ''),
			c.is_active,
			c.created_at,
			c.updated_at,
			c.deleted_at
		FROM companies c
		WHERE c.deleted_at IS NULL
	`

	args := []any{}

	if requireMembership {
		query += `
		  AND EXISTS (
			SELECT 1
			FROM company_memberships cm
			WHERE cm.user_id = $1
			  AND cm.company_id = c.id
		  )
		`
		args = append(args, userID)
	}

	if activeOnly {
		query += `
		  AND c.is_active = TRUE
		`
	}

	query += `
		ORDER BY c.name;
	`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	companies := make([]*models.Company, 0)

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

func (r *CompanyRepository) UpdateDetails(
	ctx context.Context,
	company *models.Company,
) error {
	return r.updateDetails(ctx, "", company, false)
}

func (r *CompanyRepository) UpdateDetailsForCompanyMember(
	ctx context.Context,
	userID string,
	company *models.Company,
) error {
	return r.updateDetails(ctx, userID, company, true)
}

func (r *CompanyRepository) updateDetails(
	ctx context.Context,
	userID string,
	company *models.Company,
	requireMembership bool,
) error {
	query := `
		UPDATE companies c
		SET
			name = $1,
			legal_name = $2,
			business_id = $3,
			email = $4,
			phone = $5,
			website = $6,
			country_code = $7,
			timezone = $8,
			address_line1 = $9,
			address_line2 = $10,
			city = $11,
			state = $12,
			postal_code = $13,
			logo_url = $14,
			updated_at = NOW()
		WHERE c.id = $15
		  AND c.deleted_at IS NULL
	`

	args := []any{
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
		company.ID,
	}

	if requireMembership {
		query += `
		  AND EXISTS (
			SELECT 1
			FROM company_memberships cm
			WHERE cm.user_id = $16
			  AND cm.company_id = c.id
		  )
		`
		args = append(args, userID)
	}

	cmd, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	return nil
}

func (r *CompanyRepository) Archive(
	ctx context.Context,
	id string,
) error {
	return r.archive(ctx, "", id, false)
}

func (r *CompanyRepository) ArchiveForCompanyMember(
	ctx context.Context,
	userID string,
	id string,
) error {
	return r.archive(ctx, userID, id, true)
}

func (r *CompanyRepository) archive(
	ctx context.Context,
	userID string,
	id string,
	requireMembership bool,
) error {
	query := `
		UPDATE companies c
		SET
			deleted_at = NOW(),
			updated_at = NOW()
		WHERE c.id = $1
		  AND c.deleted_at IS NULL
	`

	args := []any{id}

	if requireMembership {
		query += `
		  AND EXISTS (
			SELECT 1
			FROM company_memberships cm
			WHERE cm.user_id = $2
			  AND cm.company_id = c.id
		  )
		`
		args = append(args, userID)
	}

	return execCompanyMutation(ctx, r.db, query, args...)
}

func (r *CompanyRepository) Deactivate(
	ctx context.Context,
	id string,
) error {
	return r.setActive(ctx, "", id, false, false)
}

func (r *CompanyRepository) DeactivateForCompanyMember(
	ctx context.Context,
	userID string,
	id string,
) error {
	return r.setActive(ctx, userID, id, false, true)
}

func (r *CompanyRepository) Reactivate(
	ctx context.Context,
	id string,
) error {
	return r.setActive(ctx, "", id, true, false)
}

func (r *CompanyRepository) ReactivateForCompanyMember(
	ctx context.Context,
	userID string,
	id string,
) error {
	return r.setActive(ctx, userID, id, true, true)
}

func (r *CompanyRepository) setActive(
	ctx context.Context,
	userID string,
	id string,
	active bool,
	requireMembership bool,
) error {
	query := `
		UPDATE companies c
		SET
			is_active = $2,
			updated_at = NOW()
		WHERE c.id = $1
		  AND c.deleted_at IS NULL
	`

	args := []any{id, active}

	if requireMembership {
		query += `
		  AND EXISTS (
			SELECT 1
			FROM company_memberships cm
			WHERE cm.user_id = $3
			  AND cm.company_id = c.id
		  )
		`
		args = append(args, userID)
	}

	return execCompanyMutation(ctx, r.db, query, args...)
}

func execCompanyMutation(
	ctx context.Context,
	db DBTX,
	query string,
	args ...any,
) error {
	cmd, err := db.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	return nil
}
