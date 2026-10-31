package payouts

import (
	"context"
	"errors"

	stripe "github.com/stripe/stripe-go/v86"
)

var ErrStripeConnectAccountRetrieval = errors.New(
	"Stripe Connect account retrieval failed",
)

// StripeConnectAccountRetrieveFunc is an injectable read-only
// Stripe account retrieval operation.
type StripeConnectAccountRetrieveFunc func(
	context.Context,
	string,
) (*stripe.Account, error)

// StripeConnectSDKAccountReader converts authoritative Stripe
// account data into CONNECT's eligibility snapshot.
//
// It cannot create accounts or initiate financial operations.
type StripeConnectSDKAccountReader struct {
	retrieve StripeConnectAccountRetrieveFunc
}

var _ StripeConnectAccountReader = (*StripeConnectSDKAccountReader)(nil)

// NewStripeConnectSDKAccountReader accepts an injected retrieval
// operation. Tests supply a synthetic implementation.
func NewStripeConnectSDKAccountReader(
	retrieve StripeConnectAccountRetrieveFunc,
) *StripeConnectSDKAccountReader {
	return &StripeConnectSDKAccountReader{
		retrieve: retrieve,
	}
}

// NewStripeConnectSDKAccountReaderFromClient wires the existing
// Stripe SDK's GET /v1/accounts/{id} method.
//
// Construction performs no network request.
func NewStripeConnectSDKAccountReaderFromClient(
	client *stripe.Client,
) *StripeConnectSDKAccountReader {
	if client == nil {
		return NewStripeConnectSDKAccountReader(nil)
	}

	return NewStripeConnectSDKAccountReader(
		func(
			ctx context.Context,
			accountID string,
		) (*stripe.Account, error) {
			return client.V1Accounts.GetByID(
				ctx,
				accountID,
				nil,
			)
		},
	)
}

// RetrieveConnectAccount retrieves and maps Stripe's account
// response. It fails closed on absent or mismatched evidence.
//
// Account ownership and destination-to-driver binding must be
// independently established by CONNECT. This reader does not
// establish either property.
func (r *StripeConnectSDKAccountReader) RetrieveConnectAccount(
	ctx context.Context,
	accountID string,
) (StripeConnectAccountSnapshot, error) {
	var empty StripeConnectAccountSnapshot

	if r == nil || r.retrieve == nil || ctx == nil {
		return empty, ErrStripeConnectAccountRetrieval
	}

	if err := ctx.Err(); err != nil {
		return empty, ErrStripeConnectAccountRetrieval
	}

	if ValidateStripeConnectAccountIdentity(
		StripeConnectAccountIdentity{
			AccountID: accountID,
			Country:   "FI",
			Currency:  "EUR",
		},
	) != nil {
		return empty, ErrStripeConnectAccountRetrieval
	}

	account, err := r.retrieve(ctx, accountID)
	if err != nil || account == nil {
		return empty, ErrStripeConnectAccountRetrieval
	}

	if err := ctx.Err(); err != nil {
		return empty, ErrStripeConnectAccountRetrieval
	}

	if account.ID != accountID ||
		account.Deleted ||
		account.Capabilities == nil ||
		account.Requirements == nil {
		return empty, ErrStripeConnectAccountRetrieval
	}

	requirements := account.Requirements

	return StripeConnectAccountSnapshot{
		AccountID: account.ID,
		Country:   account.Country,
		TransfersCapabilityActive: account.Capabilities.Transfers ==
			stripe.AccountCapabilityStatusActive,
		ChargesEnabled:    account.ChargesEnabled,
		PayoutsEnabled:    account.PayoutsEnabled,
		DetailsSubmitted:  account.DetailsSubmitted,
		CurrentlyDueCount: len(requirements.CurrentlyDue),
		PastDueCount:      len(requirements.PastDue),
		DisabledReason:    string(requirements.DisabledReason),
	}, nil
}
