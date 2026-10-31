package payouts

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestAuthorizeStripeOwnershipBinding(t *testing.T) {
	request := StripeConnectOwnershipRequest{
		ActorUserID:  uuid.NewString(),
		DriverUserID: uuid.NewString(),
		CompanyID:    uuid.NewString(),
		AccountID:    "acct_1234567890ABCDEF",
	}

	valid := StripeOwnershipAuthorizationFacts{
		AuthenticatedActorUserID:           request.ActorUserID,
		PermissionGranted:                  true,
		CompanyMembershipConfirmed:         true,
		DriverCompanyRelationshipConfirmed: true,
		TrustedStripeLinkConfirmed:         true,
		LinkedActorUserID:                  request.ActorUserID,
		LinkedDriverUserID:                 request.DriverUserID,
		LinkedCompanyID:                    request.CompanyID,
		LinkedAccountID:                    request.AccountID,
	}

	if err := AuthorizeStripeOwnershipBinding(
		context.Background(), request, valid,
	); err != nil {
		t.Fatalf("expected valid facts to authorize: %v", err)
	}

	cases := []struct {
		name   string
		change func(*StripeOwnershipAuthorizationFacts)
	}{
		{"unauthenticated actor", func(f *StripeOwnershipAuthorizationFacts) {
			f.AuthenticatedActorUserID = ""
		}},
		{"wrong actor", func(f *StripeOwnershipAuthorizationFacts) {
			f.AuthenticatedActorUserID = uuid.NewString()
		}},
		{"missing permission", func(f *StripeOwnershipAuthorizationFacts) {
			f.PermissionGranted = false
		}},
		{"missing membership", func(f *StripeOwnershipAuthorizationFacts) {
			f.CompanyMembershipConfirmed = false
		}},
		{"wrong driver company", func(f *StripeOwnershipAuthorizationFacts) {
			f.DriverCompanyRelationshipConfirmed = false
		}},
		{"missing trusted Stripe link", func(f *StripeOwnershipAuthorizationFacts) {
			f.TrustedStripeLinkConfirmed = false
		}},
		{"linked wrong actor", func(f *StripeOwnershipAuthorizationFacts) {
			f.LinkedActorUserID = uuid.NewString()
		}},
		{"linked wrong driver", func(f *StripeOwnershipAuthorizationFacts) {
			f.LinkedDriverUserID = uuid.NewString()
		}},
		{"linked wrong company", func(f *StripeOwnershipAuthorizationFacts) {
			f.LinkedCompanyID = uuid.NewString()
		}},
		{"linked wrong account", func(f *StripeOwnershipAuthorizationFacts) {
			f.LinkedAccountID = "acct_0000000000000000"
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			facts := valid
			tc.change(&facts)

			err := AuthorizeStripeOwnershipBinding(
				context.Background(), request, facts,
			)
			if !errors.Is(err, ErrStripeConnectOwnershipNotVerified) {
				t.Fatalf("expected fail-closed rejection, got %v", err)
			}
		})
	}

	t.Run("nil context", func(t *testing.T) {
		err := AuthorizeStripeOwnershipBinding(nil, request, valid)
		if !errors.Is(err, ErrStripeConnectOwnershipNotVerified) {
			t.Fatalf("expected nil-context rejection, got %v", err)
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := AuthorizeStripeOwnershipBinding(ctx, request, valid)
		if !errors.Is(err, ErrStripeConnectOwnershipNotVerified) {
			t.Fatalf("expected cancelled-context rejection, got %v", err)
		}
	})

	t.Run("invalid request", func(t *testing.T) {
		invalid := request
		invalid.CompanyID = ""

		err := AuthorizeStripeOwnershipBinding(
			context.Background(), invalid, valid,
		)
		if !errors.Is(err, ErrStripeConnectOwnershipNotVerified) {
			t.Fatalf("expected invalid-request rejection, got %v", err)
		}
	})
}
