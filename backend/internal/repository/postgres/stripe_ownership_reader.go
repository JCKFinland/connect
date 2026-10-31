package postgres

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/payouts"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type stripeOwnershipQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// StripeOwnershipEvidenceReader performs a read-only lookup.
// It does not create or verify ownership bindings.
type StripeOwnershipEvidenceReader struct {
	db stripeOwnershipQuery
}

var _ payouts.StripeConnectOwnershipEvidenceProvider = (*StripeOwnershipEvidenceReader)(nil)

func NewStripeOwnershipEvidenceReader(
	db stripeOwnershipQuery,
) *StripeOwnershipEvidenceReader {
	return &StripeOwnershipEvidenceReader{db: db}
}

var errStripeOwnershipUnavailable = payouts.ErrStripeConnectOwnershipNotVerified

func validStripeOwnershipUUID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}

// LookupVerifiedOwnership returns only a matching, independently
// recorded VERIFIED binding. It cannot establish ownership itself.
func (r *StripeOwnershipEvidenceReader) LookupVerifiedOwnership(
	ctx context.Context,
	request payouts.StripeConnectOwnershipRequest,
) (payouts.StripeConnectOwnershipEvidence, error) {
	var empty payouts.StripeConnectOwnershipEvidence

	if r == nil || r.db == nil || ctx == nil ||
		ctx.Err() != nil ||
		!validStripeOwnershipUUID(request.ActorUserID) ||
		!validStripeOwnershipUUID(request.DriverUserID) ||
		!validStripeOwnershipUUID(request.CompanyID) {
		return empty, errStripeOwnershipUnavailable
	}

	if payouts.ValidateStripeConnectAccountIdentity(
		payouts.StripeConnectAccountIdentity{
			AccountID: request.AccountID,
			Country:   "FI",
			Currency:  "EUR",
		},
	) != nil {
		return empty, errStripeOwnershipUnavailable
	}

	const query = `
        SELECT
            id::text,
            actor_user_id::text,
            driver_user_id::text,
            company_id::text,
            stripe_account_id
        FROM stripe_connect_ownership_bindings
        WHERE actor_user_id = $1::uuid
          AND driver_user_id = $2::uuid
          AND company_id = $3::uuid
          AND stripe_account_id = $4
          AND verification_status = 'VERIFIED'
          AND verified_at IS NOT NULL
          AND revoked_at IS NULL
          AND verified_by_user_id IS NOT NULL
          AND NULLIF(btrim(evidence_source), '') IS NOT NULL
          AND NULLIF(btrim(evidence_reference), '') IS NOT NULL
        LIMIT 1
    `

	var evidence payouts.StripeConnectOwnershipEvidence

	err := r.db.QueryRow(
		ctx,
		query,
		request.ActorUserID,
		request.DriverUserID,
		request.CompanyID,
		request.AccountID,
	).Scan(
		&evidence.BindingID,
		&evidence.ActorUserID,
		&evidence.DriverUserID,
		&evidence.CompanyID,
		&evidence.AccountID,
	)

	if err != nil || ctx.Err() != nil {
		return empty, errStripeOwnershipUnavailable
	}

	evidence.Verified = true

	if err := payouts.VerifyStripeConnectOwnershipEvidence(
		request,
		evidence,
	); err != nil {
		return empty, errStripeOwnershipUnavailable
	}

	return evidence, nil
}
