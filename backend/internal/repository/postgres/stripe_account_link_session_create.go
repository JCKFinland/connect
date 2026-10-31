package postgres

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/payouts"
	"github.com/jackc/pgx/v5"
)

// StripeAccountLinkSessionCreator persists an independently authorized,
// server-established Stripe Connect account-linking intent.
//
// This repository does not authenticate actors, establish tenant
// authority, create Stripe accounts, or prove account ownership.
//
// Callers must establish those facts before invoking Create.
type StripeAccountLinkSessionCreator struct {
	db interface {
		QueryRow(
			context.Context,
			string,
			...any,
		) pgx.Row
	}
}

func NewStripeAccountLinkSessionCreator(
	db interface {
		QueryRow(
			context.Context,
			string,
			...any,
		) pgx.Row
	},
) *StripeAccountLinkSessionCreator {
	return &StripeAccountLinkSessionCreator{db: db}
}

// Create persists a short-lived, unconsumed linking intent.
//
// AccountID must originate from a trusted server-side Stripe
// workflow, never solely from client-supplied input.
//
// A created session is not ownership evidence and does not
// authorize a payout.
func (r *StripeAccountLinkSessionCreator) Create(
	ctx context.Context,
	request payouts.StripeConnectOwnershipRequest,
) (string, error) {
	if ctx == nil || ctx.Err() != nil ||
		r == nil || r.db == nil ||
		!validStripeLinkClaimUUID(request.ActorUserID) ||
		!validStripeLinkClaimUUID(request.DriverUserID) ||
		!validStripeLinkClaimUUID(request.CompanyID) {
		return "", payouts.ErrStripeConnectOwnershipNotVerified
	}

	if err := payouts.ValidateStripeConnectAccountIdentity(
		payouts.StripeConnectAccountIdentity{
			AccountID: request.AccountID,
			Country:   "FI",
			Currency:  "EUR",
		},
	); err != nil {
		return "", payouts.ErrStripeConnectOwnershipNotVerified
	}

	const query = `
        INSERT INTO stripe_connect_account_link_sessions (
            actor_user_id,
            driver_user_id,
            company_id,
            stripe_account_id,
            expires_at
        ) VALUES (
            $1::uuid,
            $2::uuid,
            $3::uuid,
            $4,
            clock_timestamp() + INTERVAL '5 minutes'
        )
        RETURNING id
    `

	var sessionID string

	err := r.db.QueryRow(
		ctx,
		query,
		request.ActorUserID,
		request.DriverUserID,
		request.CompanyID,
		request.AccountID,
	).Scan(&sessionID)

	if err != nil {
		return "", err
	}

	if !validStripeLinkClaimUUID(sessionID) {
		return "", payouts.ErrStripeConnectOwnershipNotVerified
	}

	return sessionID, nil
}
