package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
)

type CompanyMembershipRepository struct {
	db repository.DBTX
}

func NewCompanyMembershipRepository(
	db repository.DBTX,
) *CompanyMembershipRepository {
	return &CompanyMembershipRepository{
		db: db,
	}
}

func (r *CompanyMembershipRepository) Create(
	ctx context.Context,
	membership *models.CompanyMembership,
) error {
	if membership == nil {
		return errors.New("company membership is required")
	}

	query := `
		INSERT INTO company_memberships (
			user_id,
			company_id
		)
		VALUES ($1, $2)
		ON CONFLICT (user_id, company_id)
		DO NOTHING
		RETURNING created_at;
	`

	err := r.db.QueryRow(
		ctx,
		query,
		membership.UserID,
		membership.CompanyID,
	).Scan(&membership.CreatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return repository.ErrCompanyMembershipAlreadyExists
	}
	if err != nil {
		return err
	}

	return nil
}

func (r *CompanyMembershipRepository) Exists(
	ctx context.Context,
	userID string,
	companyID string,
) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1
			FROM company_memberships
			WHERE user_id = $1
			  AND company_id = $2
		);
	`

	var exists bool

	err := r.db.QueryRow(
		ctx,
		query,
		userID,
		companyID,
	).Scan(&exists)

	return exists, err
}

func (r *CompanyMembershipRepository) ListByUserID(
	ctx context.Context,
	userID string,
) ([]*models.CompanyMembership, error) {
	query := `
		SELECT
			user_id,
			company_id,
			created_at
		FROM company_memberships
		WHERE user_id = $1
		ORDER BY created_at, company_id;
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	memberships := make([]*models.CompanyMembership, 0)

	for rows.Next() {
		membership := &models.CompanyMembership{}

		if err := rows.Scan(
			&membership.UserID,
			&membership.CompanyID,
			&membership.CreatedAt,
		); err != nil {
			return nil, err
		}

		memberships = append(
			memberships,
			membership,
		)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return memberships, nil
}
