package repository

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrRoleAlreadyAssigned = errors.New("role already assigned")
)

type UserRoleRepository interface {
	AssignRole(ctx context.Context, userID, roleID string) error
	RemoveRole(ctx context.Context, userID, roleID string) error
	UserHasRole(ctx context.Context, userID, roleID string) (bool, error)
	GetUserRoles(ctx context.Context, userID string) ([]string, error)
}

type PostgresUserRoleRepository struct {
	db *pgxpool.Pool
}

func NewUserRoleRepository(db *pgxpool.Pool) UserRoleRepository {
	return &PostgresUserRoleRepository{
		db: db,
	}
}

// AssignRole assigns a role to a user.
func (r *PostgresUserRoleRepository) AssignRole(
	ctx context.Context,
	userID, roleID string,
) error {

	query := `
	INSERT INTO user_roles
	(
		user_id,
		role_id
	)
	VALUES
	($1, $2)
	ON CONFLICT (user_id, role_id)
	DO NOTHING;
	`

	cmd, err := r.db.Exec(ctx, query, userID, roleID)
	if err != nil {
		return err
	}

	if cmd.RowsAffected() == 0 {
		return ErrRoleAlreadyAssigned
	}

	return nil
}

// RemoveRole removes a role from a user.
func (r *PostgresUserRoleRepository) RemoveRole(
	ctx context.Context,
	userID, roleID string,
) error {

	query := `
	DELETE FROM user_roles
	WHERE user_id = $1
	AND role_id = $2;
	`

	_, err := r.db.Exec(ctx, query, userID, roleID)

	return err
}

// UserHasRole checks whether a user has a specific role.
func (r *PostgresUserRoleRepository) UserHasRole(
	ctx context.Context,
	userID, roleID string,
) (bool, error) {

	query := `
	SELECT EXISTS (
		SELECT 1
		FROM user_roles
		WHERE user_id = $1
		AND role_id = $2
	);
	`

	var exists bool

	err := r.db.QueryRow(ctx, query, userID, roleID).Scan(&exists)

	return exists, err
}

// GetUserRoles returns all role names assigned to a user.
func (r *PostgresUserRoleRepository) GetUserRoles(
	ctx context.Context,
	userID string,
) ([]string, error) {

	query := `
	SELECT r.name
	FROM roles r
	INNER JOIN user_roles ur
		ON ur.role_id = r.id
	WHERE ur.user_id = $1
	ORDER BY r.name;
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []string

	for rows.Next() {
		var role string

		if err := rows.Scan(&role); err != nil {
			return nil, err
		}

		roles = append(roles, role)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return roles, nil
}
