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
	db DBTX
}

var _ repository.BranchRepository = (*BranchRepository)(nil)

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
		COALESCE(email, ''),
		COALESCE(phone, ''),
		COALESCE(address_line1, ''),
		COALESCE(address_line2, ''),
		COALESCE(city, ''),
		COALESCE(state, ''),
		COALESCE(postal_code, ''),
		COALESCE(latitude, 0),
		COALESCE(longitude, 0),
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
		COALESCE(email, ''),
		COALESCE(phone, ''),
		COALESCE(address_line1, ''),
		COALESCE(address_line2, ''),
		COALESCE(city, ''),
		COALESCE(state, ''),
		COALESCE(postal_code, ''),
		COALESCE(latitude, 0),
		COALESCE(longitude, 0),
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

func (r *BranchRepository) ListForCompanyMember(
	ctx context.Context,
	userID string,
) ([]*models.Branch, error) {
	const query = `
		SELECT
			b.id,
			b.company_id,
			b.code,
			b.name,
			COALESCE(b.email, ''),
			COALESCE(b.phone, ''),
			COALESCE(b.address_line1, ''),
			COALESCE(b.address_line2, ''),
			COALESCE(b.city, ''),
			COALESCE(b.state, ''),
			COALESCE(b.postal_code, ''),
			COALESCE(b.latitude, 0),
			COALESCE(b.longitude, 0),
			b.is_active,
			b.created_at,
			b.updated_at,
			b.deleted_at
		FROM branches AS b
		WHERE b.deleted_at IS NULL
		  AND EXISTS (
			SELECT 1
			FROM company_memberships AS cm
			WHERE cm.user_id=$1
			  AND cm.company_id=b.company_id
		  )
		ORDER BY b.name
	`

	rows, err := r.db.Query(ctx, query, userID)
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

		branches = append(branches, &branch)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return branches, nil
}

func (r *BranchRepository) ListActiveByCompanyID(
	ctx context.Context,
	companyID string,
) ([]*models.Branch, error) {
	query := `
	SELECT
		id,
		company_id,
		code,
		name,
		COALESCE(email, ''),
		COALESCE(phone, ''),
		COALESCE(address_line1, ''),
		COALESCE(address_line2, ''),
		COALESCE(city, ''),
		COALESCE(state, ''),
		COALESCE(postal_code, ''),
		COALESCE(latitude, 0),
		COALESCE(longitude, 0),
		is_active,
		created_at,
		updated_at,
		deleted_at
	FROM branches
	WHERE company_id = $1
	AND deleted_at IS NULL
	AND is_active = TRUE
	ORDER BY name;
	`

	rows, err := r.db.Query(ctx, query, companyID)
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

		branches = append(branches, &branch)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return branches, nil
}

func (r *BranchRepository) UpdateDetails(
	ctx context.Context,
	branch *models.Branch,
) error {
	const query = `
		UPDATE branches
		SET
			code=$2,
			name=$3,
			email=$4,
			phone=$5,
			address_line1=$6,
			address_line2=$7,
			city=$8,
			state=$9,
			postal_code=$10,
			latitude=$11,
			longitude=$12,
			updated_at=NOW()
		WHERE id=$1
		  AND deleted_at IS NULL
	`

	result, err := r.db.Exec(
		ctx,
		query,
		branch.ID,
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
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *BranchRepository) Archive(
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

func (r *BranchRepository) GetByIDForCompanyMember(
	ctx context.Context,
	userID string,
	id string,
) (*models.Branch, error) {
	const query = `
		SELECT
			b.id, b.company_id, b.code, b.name,
			COALESCE(b.email, ''), COALESCE(b.phone, ''),
			COALESCE(b.address_line1, ''), COALESCE(b.address_line2, ''),
			COALESCE(b.city, ''), COALESCE(b.state, ''),
			COALESCE(b.postal_code, ''),
			COALESCE(b.latitude, 0), COALESCE(b.longitude, 0),
			b.is_active,
			b.created_at, b.updated_at, b.deleted_at
		FROM branches AS b
		WHERE b.id=$2
		  AND b.deleted_at IS NULL
		  AND EXISTS (
			SELECT 1
			FROM company_memberships AS cm
			WHERE cm.user_id=$1
			  AND cm.company_id=b.company_id
		  )
	`

	var branch models.Branch
	err := r.db.QueryRow(ctx, query, userID, id).Scan(
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

func (r *BranchRepository) UpdateDetailsForCompanyMember(
	ctx context.Context,
	userID string,
	branch *models.Branch,
) error {
	const query = `
		UPDATE branches AS b
		SET
			code=$3,
			name=$4,
			email=$5,
			phone=$6,
			address_line1=$7,
			address_line2=$8,
			city=$9,
			state=$10,
			postal_code=$11,
			latitude=$12,
			longitude=$13,
			updated_at=NOW()
		WHERE b.id=$1
		  AND b.deleted_at IS NULL
		  AND EXISTS (
			SELECT 1
			FROM company_memberships AS cm
			WHERE cm.user_id=$2
			  AND cm.company_id=b.company_id
		  )
	`

	result, err := r.db.Exec(
		ctx,
		query,
		branch.ID,
		userID,
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
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *BranchRepository) ArchiveForCompanyMember(
	ctx context.Context,
	userID string,
	id string,
) error {
	const query = `
		UPDATE branches AS b
		SET deleted_at=NOW(), updated_at=NOW()
		WHERE b.id=$1
		  AND b.deleted_at IS NULL
		  AND EXISTS (
			SELECT 1
			FROM company_memberships AS cm
			WHERE cm.user_id=$2
			  AND cm.company_id=b.company_id
		  )
	`

	result, err := r.db.Exec(ctx, query, id, userID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *BranchRepository) Deactivate(
	ctx context.Context,
	id string,
) error {
	const query = `
		UPDATE branches
		SET is_active=FALSE, updated_at=NOW()
		WHERE id=$1
		  AND deleted_at IS NULL
	`

	result, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *BranchRepository) DeactivateForCompanyMember(
	ctx context.Context,
	userID string,
	id string,
) error {
	const query = `
		UPDATE branches AS b
		SET is_active=FALSE, updated_at=NOW()
		WHERE b.id=$1
		  AND b.deleted_at IS NULL
		  AND EXISTS (
			SELECT 1
			FROM company_memberships AS cm
			WHERE cm.user_id=$2
			  AND cm.company_id=b.company_id
		  )
	`

	result, err := r.db.Exec(ctx, query, id, userID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *BranchRepository) Reactivate(
	ctx context.Context,
	id string,
) error {
	const query = `
		UPDATE branches
		SET is_active=TRUE, updated_at=NOW()
		WHERE id=$1
		  AND deleted_at IS NULL
	`

	result, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (r *BranchRepository) ReactivateForCompanyMember(
	ctx context.Context,
	userID string,
	id string,
) error {
	const query = `
		UPDATE branches AS b
		SET is_active=TRUE, updated_at=NOW()
		WHERE b.id=$1
		  AND b.deleted_at IS NULL
		  AND EXISTS (
			SELECT 1
			FROM company_memberships AS cm
			WHERE cm.user_id=$2
			  AND cm.company_id=b.company_id
		  )
	`

	result, err := r.db.Exec(ctx, query, id, userID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}
