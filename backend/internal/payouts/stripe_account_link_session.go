package payouts

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// StripeAccountLinkSession represents a server-established,
// single-use account-linking intent.
//
// A valid session is NOT proof of Stripe account ownership.
// Persistence, cryptographic callback validation and trusted
// Stripe reconciliation are separate security boundaries.
type StripeAccountLinkSession struct {
	SessionID    string
	ActorUserID  string
	DriverUserID string
	CompanyID    string
	AccountID    string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	ConsumedAt   *time.Time
}

// ValidateStripeAccountLinkSession checks the immutable
// identity and lifecycle constraints of a linking session.
// It never consumes a session or produces ownership evidence.
func ValidateStripeAccountLinkSession(
	ctx context.Context,
	request StripeConnectOwnershipRequest,
	session StripeAccountLinkSession,
	now time.Time,
) error {
	if ctx == nil || ctx.Err() != nil ||
		!validOwnershipRequest(request) ||
		now.IsZero() {
		return ErrStripeConnectOwnershipNotVerified
	}

	id, err := uuid.Parse(session.SessionID)
	if err != nil || id == uuid.Nil ||
		id.String() != session.SessionID {
		return ErrStripeConnectOwnershipNotVerified
	}

	if session.ActorUserID != request.ActorUserID ||
		session.DriverUserID != request.DriverUserID ||
		session.CompanyID != request.CompanyID ||
		session.AccountID != request.AccountID ||
		session.CreatedAt.IsZero() ||
		session.ExpiresAt.IsZero() ||
		!session.ExpiresAt.After(session.CreatedAt) ||
		now.Before(session.CreatedAt) ||
		!now.Before(session.ExpiresAt) ||
		session.ConsumedAt != nil {
		return ErrStripeConnectOwnershipNotVerified
	}

	return nil
}
