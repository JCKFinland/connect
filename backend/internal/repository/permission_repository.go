package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Permission struct {
	ID          string
	Name        string
	Description string
}

type PermissionRepository interface {
	GetByUserID(
		ctx context.Context,
		userID string,
	) ([]Permission, error)

	HasPermission(
		ctx context.Context,
		userID string,
		permission string,
	) (bool, error)
}

type PostgresPermissionRepository struct {
	db *pgxpool.Pool
}

func NewPermissionRepository(
	db *pgxpool.Pool,
) PermissionRepository {

	return &PostgresPermissionRepository{
		db: db,
	}
}

func (r *PostgresPermissionRepository) GetByUserID(
	ctx context.Context,
	userID string,
) ([]Permission, error) {

	query := `
SELECT
	p.id,
	p.name,
	p.description
FROM permissions p
JOIN role_permissions rp
	ON rp.permission_id = p.id
JOIN user_roles ur
	ON ur.role_id = rp.role_id
WHERE ur.user_id = $1
ORDER BY p.name;
`

	rows, err := r.db.Query(
		ctx,
		query,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var permissions []Permission

	for rows.Next() {

		var permission Permission

		if err := rows.Scan(
			&permission.ID,
			&permission.Name,
			&permission.Description,
		); err != nil {
			return nil, err
		}

		permissions = append(
			permissions,
			permission,
		)
	}

	return permissions, rows.Err()
}

func (r *PostgresPermissionRepository) HasPermission(
	ctx context.Context,
	userID string,
	permission string,
) (bool, error) {

	query := `
SELECT EXISTS(
	SELECT 1
	FROM permissions p
	JOIN role_permissions rp
		ON rp.permission_id = p.id
	JOIN user_roles ur
		ON ur.role_id = rp.role_id
	WHERE
		ur.user_id = $1
	AND p.name = $2
);
`

	var exists bool

	err := r.db.QueryRow(
		ctx,
		query,
		userID,
		permission,
	).Scan(&exists)

	return exists, err
}
