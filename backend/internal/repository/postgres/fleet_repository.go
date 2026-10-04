package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

// FleetRepository implements repository.FleetRepository.
type FleetRepository struct {
	db DBTX
}

// Compile-time interface check.
var _ repository.FleetRepository = (*FleetRepository)(nil)

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

// GetByIDForCompanyMember retrieves a fleet only when the user has
// explicit membership in the fleet's company.
func (r *FleetRepository) GetByIDForCompanyMember(
	ctx context.Context,
	userID string,
	id string,
) (*models.Fleet, error) {

	query := `
	SELECT
		f.id,
		f.company_id,
		f.branch_id,
		f.code,
		f.name,
		f.description,
		f.is_active,
		f.created_at,
		f.updated_at,
		f.deleted_at
	FROM fleets f
	INNER JOIN company_memberships cm
		ON cm.company_id = f.company_id
	   AND cm.user_id = $1
	WHERE f.id = $2
	  AND f.deleted_at IS NULL;
	`

	var fleet models.Fleet

	err := r.db.QueryRow(
		ctx,
		query,
		userID,
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
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
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

// ListForCompanyMember returns fleets only from companies in which the
// user has explicit membership.
func (r *FleetRepository) ListForCompanyMember(
	ctx context.Context,
	userID string,
) ([]*models.Fleet, error) {

	query := `
	SELECT
		f.id,
		f.company_id,
		f.branch_id,
		f.code,
		f.name,
		f.description,
		f.is_active,
		f.created_at,
		f.updated_at,
		f.deleted_at
	FROM fleets f
	INNER JOIN company_memberships cm
		ON cm.company_id = f.company_id
	   AND cm.user_id = $1
	WHERE f.deleted_at IS NULL
	ORDER BY f.name;
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fleets := make([]*models.Fleet, 0)

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

		fleets = append(fleets, &fleet)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return fleets, nil
}

func (r *FleetRepository) ListActiveByCompanyAndBranch(
	ctx context.Context,
	companyID string,
	branchID string,
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
	WHERE company_id = $1
	  AND branch_id = $2
	  AND is_active = TRUE
	  AND deleted_at IS NULL
	ORDER BY name;
	`

	rows, err := r.db.Query(
		ctx,
		query,
		companyID,
		branchID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fleets := make([]*models.Fleet, 0)

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

		fleets = append(fleets, &fleet)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return fleets, nil
}

// UpdateDetails modifies descriptive fleet fields only. Tenant, branch, and
// activation authority cannot be changed through this repository operation.
func (r *FleetRepository) UpdateDetails(
	ctx context.Context,
	fleet *models.Fleet,
) error {
	const query = `
		UPDATE fleets
		SET
			code=$2,
			name=$3,
			description=$4,
			updated_at=NOW()
		WHERE id=$1
		  AND deleted_at IS NULL
	`

	result, err := r.db.Exec(
		ctx,
		query,
		fleet.ID,
		fleet.Code,
		fleet.Name,
		fleet.Description,
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	return nil
}

// UpdateDetailsForCompanyMember modifies descriptive fleet fields only when
// the user has explicit membership in the fleet's current company. The
// membership check is part of the UPDATE so authorization cannot become stale
// between a preceding read and the mutation.
func (r *FleetRepository) UpdateDetailsForCompanyMember(
	ctx context.Context,
	userID string,
	fleet *models.Fleet,
) error {
	const query = `
		UPDATE fleets AS f
		SET
			code=$3,
			name=$4,
			description=$5,
			updated_at=NOW()
		WHERE f.id=$1
		  AND f.deleted_at IS NULL
		  AND EXISTS (
			SELECT 1
			FROM company_memberships AS cm
			WHERE cm.user_id=$2
			  AND cm.company_id=f.company_id
		  )
	`

	result, err := r.db.Exec(
		ctx,
		query,
		fleet.ID,
		userID,
		fleet.Code,
		fleet.Name,
		fleet.Description,
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	return nil
}

// Archive hides a non-deleted fleet from normal repository reads without
// changing its operational activation state.
func (r *FleetRepository) Archive(
	ctx context.Context,
	id string,
) error {

	const query = `
		UPDATE fleets
		SET
			deleted_at=NOW(),
			updated_at=NOW()
		WHERE id=$1
		  AND deleted_at IS NULL
	`

	result, err := r.db.Exec(
		ctx,
		query,
		id,
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	return nil
}

// ArchiveForCompanyMember hides a non-deleted fleet only when the
// authenticated user has explicit membership in its current company.
func (r *FleetRepository) ArchiveForCompanyMember(
	ctx context.Context,
	userID string,
	id string,
) error {

	const query = `
		UPDATE fleets AS f
		SET
			deleted_at=NOW(),
			updated_at=NOW()
		WHERE f.id=$1
		  AND f.deleted_at IS NULL
		  AND EXISTS (
			SELECT 1
			FROM company_memberships AS cm
			WHERE cm.company_id=f.company_id
			  AND cm.user_id=$2
		  )
	`

	result, err := r.db.Exec(
		ctx,
		query,
		id,
		userID,
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return repository.ErrNotFound
	}

	return nil
}

// Deactivate marks a non-deleted fleet operationally inactive.
func (r *FleetRepository) Deactivate(
	ctx context.Context,
	id string,
) error {
	const query = `
		UPDATE fleets
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

// DeactivateForCompanyMember marks a non-deleted fleet inactive only when the
// authenticated user has explicit membership in its current company.
func (r *FleetRepository) DeactivateForCompanyMember(
	ctx context.Context,
	userID string,
	id string,
) error {
	const query = `
		UPDATE fleets AS f
		SET is_active=FALSE, updated_at=NOW()
		WHERE f.id=$1
		  AND f.deleted_at IS NULL
		  AND EXISTS (
			SELECT 1
			FROM company_memberships AS cm
			WHERE cm.company_id=f.company_id
			  AND cm.user_id=$2
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

// IsOwningBranchActive reports whether a non-deleted fleet's owning branch
// exists, is non-deleted, and is operationally active.
func (r *FleetRepository) IsOwningBranchActive(
	ctx context.Context,
	fleetID string,
) (bool, error) {
	const query = `
		SELECT b.is_active
		FROM fleets AS f
		JOIN branches AS b ON b.id=f.branch_id
		WHERE f.id=$1
		  AND f.deleted_at IS NULL
		  AND b.deleted_at IS NULL
	`

	var active bool
	err := r.db.QueryRow(ctx, query, fleetID).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, repository.ErrNotFound
	}
	if err != nil {
		return false, err
	}
	return active, nil
}

// Reactivate marks a non-deleted fleet operationally active only while its
// owning branch remains active and non-deleted.
func (r *FleetRepository) Reactivate(
	ctx context.Context,
	id string,
) error {
	const query = `
		UPDATE fleets AS f
		SET is_active=TRUE, updated_at=NOW()
		WHERE f.id=$1
		  AND f.deleted_at IS NULL
		  AND EXISTS (
			SELECT 1
			FROM branches AS b
			WHERE b.id=f.branch_id
			  AND b.is_active=TRUE
			  AND b.deleted_at IS NULL
		  )
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

// ReactivateForCompanyMember marks a non-deleted fleet active only when the
// authenticated user has explicit membership in its current company and its
// owning branch remains active and non-deleted.
func (r *FleetRepository) ReactivateForCompanyMember(
	ctx context.Context,
	userID string,
	id string,
) error {
	const query = `
		UPDATE fleets AS f
		SET is_active=TRUE, updated_at=NOW()
		WHERE f.id=$1
		  AND f.deleted_at IS NULL
		  AND EXISTS (
			SELECT 1
			FROM company_memberships AS cm
			WHERE cm.company_id=f.company_id
			  AND cm.user_id=$2
		  )
		  AND EXISTS (
			SELECT 1
			FROM branches AS b
			WHERE b.id=f.branch_id
			  AND b.is_active=TRUE
			  AND b.deleted_at IS NULL
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
