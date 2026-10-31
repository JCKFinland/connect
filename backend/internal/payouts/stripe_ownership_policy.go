package payouts

import (
	"context"
)

// StripeOwnershipAuthorizationFacts are independently established
// security facts. They must never be populated directly from
// untrusted request-body assertions.
type StripeOwnershipAuthorizationFacts struct {
	AuthenticatedActorUserID           string
	PermissionGranted                  bool
	CompanyMembershipConfirmed         bool
	DriverCompanyRelationshipConfirmed bool
	TrustedStripeLinkConfirmed         bool
	LinkedActorUserID                  string
	LinkedDriverUserID                 string
	LinkedCompanyID                    string
	LinkedAccountID                    string
}

// AuthorizeStripeOwnershipBinding evaluates independently established
// authorization facts. It never creates or verifies ownership evidence,
// mutates records, or communicates with Stripe.
func AuthorizeStripeOwnershipBinding(
	ctx context.Context,
	request StripeConnectOwnershipRequest,
	facts StripeOwnershipAuthorizationFacts,
) error {
	if ctx == nil || ctx.Err() != nil ||
		!validOwnershipRequest(request) ||
		facts.AuthenticatedActorUserID != request.ActorUserID ||
		!facts.PermissionGranted ||
		!facts.CompanyMembershipConfirmed ||
		!facts.DriverCompanyRelationshipConfirmed ||
		!facts.TrustedStripeLinkConfirmed ||
		facts.LinkedActorUserID != request.ActorUserID ||
		facts.LinkedDriverUserID != request.DriverUserID ||
		facts.LinkedCompanyID != request.CompanyID ||
		facts.LinkedAccountID != request.AccountID {
		return ErrStripeConnectOwnershipNotVerified
	}

	return nil
}
