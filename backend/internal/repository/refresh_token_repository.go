package repository

import (
	"context"
	"errors"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrRefreshTokenNotFound = errors.New("refresh token not found")

type RefreshTokenRepository interface {
	Create(ctx context.Context, token *models.RefreshToken) error
	GetByHash(ctx context.Context, hash string) (*models.RefreshToken, error)
	DeleteByHash(ctx context.Context, hash string) error
	DeleteByUserID(ctx context.Context, userID string) error
	DeleteExpired(ctx context.Context) error
}

type PostgresRefreshTokenRepository struct {
	db *pgxpool.Pool
}

func NewRefreshTokenRepository(db *pgxpool.Pool) RefreshTokenRepository {
	return &PostgresRefreshTokenRepository{
		db: db,
	}
}

func (r *PostgresRefreshTokenRepository) Create(
	ctx context.Context,
	token *models.RefreshToken,
) error {

	query := `
INSERT INTO refresh_tokens
(
	user_id,
	token_hash,
	expires_at
)
VALUES
($1, $2, $3)
RETURNING
	id,
	created_at;
`

	return r.db.QueryRow(
		ctx,
		query,
		token.UserID,
		token.TokenHash,
		token.ExpiresAt,
	).Scan(
		&token.ID,
		&token.CreatedAt,
	)
}

func (r *PostgresRefreshTokenRepository) GetByHash(
	ctx context.Context,
	hash string,
) (*models.RefreshToken, error) {

	query := `
SELECT
	id,
	user_id,
	token_hash,
	expires_at,
	revoked_at,
	created_at
FROM refresh_tokens
WHERE token_hash = $1;
`

	var token models.RefreshToken

	err := r.db.QueryRow(ctx, query, hash).Scan(
		&token.ID,
		&token.UserID,
		&token.TokenHash,
		&token.ExpiresAt,
		&token.RevokedAt,
		&token.CreatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRefreshTokenNotFound
	}

	if err != nil {
		return nil, err
	}

	return &token, nil
}

func (r *PostgresRefreshTokenRepository) DeleteByHash(
	ctx context.Context,
	hash string,
) error {

	cmd, err := r.db.Exec(
		ctx,
		`DELETE FROM refresh_tokens
		 WHERE token_hash = $1`,
		hash,
	)
	if err != nil {
		return err
	}

	println("Rows deleted:", cmd.RowsAffected())

	if cmd.RowsAffected() == 0 {
		return ErrRefreshTokenNotFound
	}

	return nil
}

func (r *PostgresRefreshTokenRepository) DeleteByUserID(
	ctx context.Context,
	userID string,
) error {

	cmd, err := r.db.Exec(
		ctx,
		`DELETE FROM refresh_tokens
		 WHERE user_id = $1`,
		userID,
	)

	if err != nil {
		return err
	}

	if cmd.RowsAffected() == 0 {
		return ErrRefreshTokenNotFound
	}

	return nil
}

func (r *PostgresRefreshTokenRepository) DeleteExpired(
	ctx context.Context,
) error {

	_, err := r.db.Exec(
		ctx,
		`DELETE FROM refresh_tokens
		 WHERE expires_at < $1`,
		time.Now().UTC(),
	)

	return err
}
