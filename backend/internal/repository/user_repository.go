package repository

import (
	"context"
	"errors"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresUserRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) UserRepository {
	return &PostgresUserRepository{
		db: db,
	}
}

func (r *PostgresUserRepository) Create(
	ctx context.Context,
	user *models.User,
) error {

	query := `
	INSERT INTO users
	(
		email,
		password_hash,
		first_name,
		last_name,
		phone
	)
	VALUES
	($1, $2, $3, $4, $5)
	RETURNING
		id,
		is_active,
		is_verified,
		created_at,
		updated_at;
	`

	err := r.db.QueryRow(
		ctx,
		query,
		user.Email,
		user.PasswordHash,
		user.FirstName,
		user.LastName,
		user.Phone,
	).Scan(
		&user.ID,
		&user.IsActive,
		&user.IsVerified,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {

		var pgErr *pgconn.PgError

		if errors.As(err, &pgErr) {

			// PostgreSQL unique_violation
			if pgErr.Code == "23505" {
				return ErrEmailAlreadyUsed
			}
		}

		return err
	}

	return nil
}

func (r *PostgresUserRepository) GetByID(ctx context.Context, id string) (*models.User, error) {

	query := `
	SELECT
		id,
		email,
		password_hash,
		first_name,
		last_name,
		phone,
		is_active,
		is_verified,
		created_at,
		updated_at,
		deleted_at
	FROM users
	WHERE id=$1
	AND deleted_at IS NULL;
	`

	var user models.User

	err := r.db.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.FirstName,
		&user.LastName,
		&user.Phone,
		&user.IsActive,
		&user.IsVerified,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.DeletedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}

	return &user, err
}

func (r *PostgresUserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {

	query := `
	SELECT
		id,
		email,
		password_hash,
		first_name,
		last_name,
		phone,
		is_active,
		is_verified,
		created_at,
		updated_at,
		deleted_at
	FROM users
	WHERE email=$1
	AND deleted_at IS NULL;
	`

	var user models.User

	err := r.db.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.FirstName,
		&user.LastName,
		&user.Phone,
		&user.IsActive,
		&user.IsVerified,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.DeletedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}

	return &user, err
}

func (r *PostgresUserRepository) Update(ctx context.Context, user *models.User) error {

	query := `
	UPDATE users
	SET
		first_name=$1,
		last_name=$2,
		phone=$3,
		updated_at=NOW()
	WHERE id=$4;
	`

	_, err := r.db.Exec(
		ctx,
		query,
		user.FirstName,
		user.LastName,
		user.Phone,
		user.ID,
	)

	return err
}

func (r *PostgresUserRepository) Delete(ctx context.Context, id string) error {

	query := `
	UPDATE users
	SET
		deleted_at=NOW()
	WHERE id=$1;
	`

	_, err := r.db.Exec(ctx, query, id)

	return err
}
