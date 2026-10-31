package payouts

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var ErrStripeConnectOwnershipNotVerified = errors.New(
	"Stripe Connect account ownership is not verified",
)

// StripeConnectOwnershipRequest describes the authenticated CONNECT
// actor and the exact destination ownership relationship.
//
// ActorUserID must come from trusted authentication middleware.
// The driver, company and account identifiers must be resolved
// against authoritative CONNECT records, not client assertions.
type StripeConnectOwnershipRequest struct {
	ActorUserID  string
	DriverUserID string
	CompanyID    string
	AccountID    string
}

// StripeConnectOwnershipEvidence represents an independent,
// authoritative ownership decision.
//
// An account ID, an eligibility snapshot, or a client-provided
// assertion must never be converted into this evidence.
//
// A future provider must authenticate the CONNECT actor, establish
// the driver/company relationship, and bind the Stripe account
// through a trusted onboarding or account-linking process.
type StripeConnectOwnershipEvidence struct {
	ActorUserID  string
	DriverUserID string
	CompanyID    string
	AccountID    string
	BindingID    string
	Verified     bool
}

// StripeConnectOwnershipEvidenceProvider is a security boundary.
//
// Implementations must independently retrieve authoritative
// evidence. No production implementation is provided here.
type StripeConnectOwnershipEvidenceProvider interface {
	LookupVerifiedOwnership(
		context.Context,
		StripeConnectOwnershipRequest,
	) (StripeConnectOwnershipEvidence, error)
}

// StripeConnectOwnershipVerifier validates independently obtained
// ownership evidence. It does not mutate any database records.
type StripeConnectOwnershipVerifier struct {
	Provider StripeConnectOwnershipEvidenceProvider
}

func validOwnershipRequest(
	request StripeConnectOwnershipRequest,
) bool {
	actor, err := uuid.Parse(request.ActorUserID)
	if err != nil || actor == uuid.Nil ||
		actor.String() != request.ActorUserID {
		return false
	}

	driver, err := uuid.Parse(request.DriverUserID)
	if err != nil || driver == uuid.Nil ||
		driver.String() != request.DriverUserID {
		return false
	}

	company, err := uuid.Parse(request.CompanyID)
	if err != nil || company == uuid.Nil ||
		company.String() != request.CompanyID {
		return false
	}

	return ValidateStripeConnectAccountIdentity(
		StripeConnectAccountIdentity{
			AccountID: request.AccountID,
			Country:   "FI",
			Currency:  "EUR",
		},
	) == nil
}

func VerifyStripeConnectOwnershipEvidence(
	request StripeConnectOwnershipRequest,
	evidence StripeConnectOwnershipEvidence,
) error {
	if !validOwnershipRequest(request) ||
		!evidence.Verified ||
		evidence.ActorUserID != request.ActorUserID ||
		evidence.DriverUserID != request.DriverUserID ||
		evidence.CompanyID != request.CompanyID ||
		evidence.AccountID != request.AccountID {
		return ErrStripeConnectOwnershipNotVerified
	}

	binding, err := uuid.Parse(evidence.BindingID)
	if err != nil || binding == uuid.Nil ||
		binding.String() != evidence.BindingID {
		return ErrStripeConnectOwnershipNotVerified
	}

	return nil
}

func (v StripeConnectOwnershipVerifier) Verify(
	ctx context.Context,
	request StripeConnectOwnershipRequest,
) error {
	if ctx == nil ||
		v.Provider == nil ||
		!validOwnershipRequest(request) ||
		ctx.Err() != nil {
		return ErrStripeConnectOwnershipNotVerified
	}

	evidence, err := v.Provider.LookupVerifiedOwnership(
		ctx,
		request,
	)
	if err != nil || ctx.Err() != nil {
		return ErrStripeConnectOwnershipNotVerified
	}

	return VerifyStripeConnectOwnershipEvidence(
		request,
		evidence,
	)
}
