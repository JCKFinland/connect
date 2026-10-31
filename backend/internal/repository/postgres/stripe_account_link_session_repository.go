package postgres

import (
	"context"
	"errors"

	"github.com/JCKFinland/connect/backend/internal/payouts"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// StripeAccountLinkSessionClaimer is intentionally limited to
// consuming an existing server-established linking intent.
//
// A successful claim is NOT proof of Stripe account ownership.
type StripeAccountLinkSessionClaimer struct {
	db interface {
		QueryRow(
			context.Context,
			string,
			...any,
		) pgx.Row
	}
}

func NewStripeAccountLinkSessionClaimer(
	db interface {
		QueryRow(
			context.Context,
			string,
			...any,
		) pgx.Row
	},
) *StripeAccountLinkSessionClaimer {
	return &StripeAccountLinkSessionClaimer{db: db}
}

// Claim consumes an exact, active, unconsumed linking session.
//
// The caller must independently establish authenticated actor
// identity, permission, company membership, driver-company
// relationship and trusted Stripe account control.
//
// This method does not perform those checks, create ownership
// evidence, or authorize a payout.
func (r *StripeAccountLinkSessionClaimer) Claim(
	ctx context.Context,
	sessionID string,
	request payouts.StripeConnectOwnershipRequest,
) (bool, error) {
	if ctx == nil || ctx.Err() != nil ||
		r == nil || r.db == nil ||
		!validStripeLinkClaimUUID(sessionID) ||
		!validStripeLinkClaimUUID(request.ActorUserID) ||
		!validStripeLinkClaimUUID(request.DriverUserID) ||
		!validStripeLinkClaimUUID(request.CompanyID) {
		return false, payouts.ErrStripeConnectOwnershipNotVerified
	}

	if err := payouts.ValidateStripeConnectAccountIdentity(
		payouts.StripeConnectAccountIdentity{
			AccountID: request.AccountID,
			Country:   "FI",
			Currency:  "EUR",
		},
	); err != nil {
		return false, payouts.ErrStripeConnectOwnershipNotVerified
	}

	const query = `
        UPDATE stripe_connect_account_link_sessions
        SET consumed_at = clock_timestamp()
        WHERE id = $1::uuid
          AND actor_user_id = $2::uuid
          AND driver_user_id = $3::uuid
          AND company_id = $4::uuid
          AND stripe_account_id = $5
          AND consumed_at IS NULL
          AND created_at <= clock_timestamp()
          AND expires_at > clock_timestamp()
        RETURNING id
    `

	var consumedID string

	err := r.db.QueryRow(
		ctx,
		query,
		sessionID,
		request.ActorUserID,
		request.DriverUserID,
		request.CompanyID,
		request.AccountID,
	).Scan(&consumedID)

	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	if consumedID != sessionID {
		return false, payouts.ErrStripeConnectOwnershipNotVerified
	}

	return true, nil
}

func validStripeLinkClaimUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil &&
		parsed != uuid.Nil &&
		parsed.String() == value
}
