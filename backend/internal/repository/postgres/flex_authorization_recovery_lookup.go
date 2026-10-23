package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GetFlexAuthorizationForRecovery returns an existing authorization
// eligible for retrieval-only Stripe recovery.
//
// This lookup never claims processing, creates a financial record,
// modifies authorization state, or contacts Stripe.
//
// Eligibility is rechecked when recovery is executed. Discovery alone
// must never be treated as authorization to contact the provider.
func GetFlexAuthorizationForRecovery(
	ctx context.Context,
	db *pgxpool.Pool,
	authorizationID string,
) (*FlexAuthorization, error) {
	if ctx == nil {
		return nil, errors.New(
			"FLEX authorization recovery context is required",
		)
	}

	if db == nil {
		return nil, errors.New(
			"FLEX authorization recovery database is required",
		)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if _, err := uuid.Parse(authorizationID); err != nil {
		return nil, ErrFlexAuthorizationNotFound
	}

	authorization, err := scanFlexAuthorization(
		db.QueryRow(ctx, `
			SELECT `+flexAuthorizationColumns+`
			FROM flex_authorizations
			WHERE id = $1
			  AND status = 'PROCESSING'
			  AND provider = 'STRIPE'
			  AND provider_authorization_id IS NOT NULL
			  AND btrim(provider_authorization_id) <> ''
		`, authorizationID),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrFlexAuthorizationNotFound
	}

	if err != nil {
		return nil, fmt.Errorf(
			"read FLEX authorization for recovery: %w",
			err,
		)
	}

	if authorization == nil ||
		authorization.Status != "PROCESSING" ||
		authorization.Provider != "STRIPE" ||
		authorization.ProviderAuthorizationID == nil ||
		strings.TrimSpace(
			*authorization.ProviderAuthorizationID,
		) == "" {
		return nil, ErrFlexAuthorizationNotFound
	}

	return authorization, nil
}
