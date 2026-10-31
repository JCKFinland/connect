package payouts

import (
	"context"
	"errors"
	"testing"
)

const (
	ownershipActor   = "11111111-1111-4111-8111-111111111111"
	ownershipDriver  = "22222222-2222-4222-8222-222222222222"
	ownershipCompany = "33333333-3333-4333-8333-333333333333"
	ownershipBinding = "44444444-4444-4444-8444-444444444444"
	ownershipAccount = "acct_1234567890ABCDEF"
)

type fakeOwnershipProvider struct {
	evidence StripeConnectOwnershipEvidence
	err      error
	calls    int
}

func (f *fakeOwnershipProvider) LookupVerifiedOwnership(
	_ context.Context,
	_ StripeConnectOwnershipRequest,
) (StripeConnectOwnershipEvidence, error) {
	f.calls++
	return f.evidence, f.err
}

func validOwnershipFixture() (
	StripeConnectOwnershipRequest,
	StripeConnectOwnershipEvidence,
) {
	request := StripeConnectOwnershipRequest{
		ActorUserID:  ownershipActor,
		DriverUserID: ownershipDriver,
		CompanyID:    ownershipCompany,
		AccountID:    ownershipAccount,
	}

	evidence := StripeConnectOwnershipEvidence{
		ActorUserID:  ownershipActor,
		DriverUserID: ownershipDriver,
		CompanyID:    ownershipCompany,
		AccountID:    ownershipAccount,
		BindingID:    ownershipBinding,
		Verified:     true,
	}

	return request, evidence
}

func TestStripeConnectOwnershipVerified(t *testing.T) {
	request, evidence := validOwnershipFixture()
	provider := &fakeOwnershipProvider{evidence: evidence}

	verifier := StripeConnectOwnershipVerifier{
		Provider: provider,
	}

	if err := verifier.Verify(
		context.Background(),
		request,
	); err != nil {
		t.Fatalf("expected verified ownership: %v", err)
	}

	if provider.calls != 1 {
		t.Fatalf("expected one evidence lookup, got %d", provider.calls)
	}
}

func TestStripeConnectOwnershipRejectsInvalidEvidence(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*StripeConnectOwnershipEvidence)
	}{
		{
			"unverified",
			func(e *StripeConnectOwnershipEvidence) {
				e.Verified = false
			},
		},
		{
			"wrong actor",
			func(e *StripeConnectOwnershipEvidence) {
				e.ActorUserID = ownershipDriver
			},
		},
		{
			"wrong driver",
			func(e *StripeConnectOwnershipEvidence) {
				e.DriverUserID = ownershipActor
			},
		},
		{
			"wrong company",
			func(e *StripeConnectOwnershipEvidence) {
				e.CompanyID = ownershipActor
			},
		},
		{
			"wrong account",
			func(e *StripeConnectOwnershipEvidence) {
				e.AccountID = "acct_9999999999"
			},
		},
		{
			"missing binding",
			func(e *StripeConnectOwnershipEvidence) {
				e.BindingID = ""
			},
		},
		{
			"invalid binding",
			func(e *StripeConnectOwnershipEvidence) {
				e.BindingID = "not-a-uuid"
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request, evidence := validOwnershipFixture()
			tc.mutate(&evidence)

			err := VerifyStripeConnectOwnershipEvidence(
				request,
				evidence,
			)

			if !errors.Is(
				err,
				ErrStripeConnectOwnershipNotVerified,
			) {
				t.Fatalf("expected rejection, got %v", err)
			}
		})
	}
}

func TestStripeConnectOwnershipRejectsInvalidRequests(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*StripeConnectOwnershipRequest)
	}{
		{
			"missing actor",
			func(r *StripeConnectOwnershipRequest) {
				r.ActorUserID = ""
			},
		},
		{
			"wrong driver identifier",
			func(r *StripeConnectOwnershipRequest) {
				r.DriverUserID = "invalid"
			},
		},
		{
			"missing company",
			func(r *StripeConnectOwnershipRequest) {
				r.CompanyID = ""
			},
		},
		{
			"invalid account",
			func(r *StripeConnectOwnershipRequest) {
				r.AccountID = "invalid"
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request, evidence := validOwnershipFixture()
			tc.mutate(&request)

			provider := &fakeOwnershipProvider{
				evidence: evidence,
			}

			verifier := StripeConnectOwnershipVerifier{
				Provider: provider,
			}

			err := verifier.Verify(
				context.Background(),
				request,
			)

			if !errors.Is(
				err,
				ErrStripeConnectOwnershipNotVerified,
			) {
				t.Fatalf("expected rejection, got %v", err)
			}

			if provider.calls != 0 {
				t.Fatalf(
					"invalid request triggered %d lookups",
					provider.calls,
				)
			}
		})
	}
}

func TestStripeConnectOwnershipRejectsProviderFailure(t *testing.T) {
	request, evidence := validOwnershipFixture()

	provider := &fakeOwnershipProvider{
		evidence: evidence,
		err:      errors.New("evidence unavailable"),
	}

	verifier := StripeConnectOwnershipVerifier{
		Provider: provider,
	}

	err := verifier.Verify(context.Background(), request)

	if !errors.Is(
		err,
		ErrStripeConnectOwnershipNotVerified,
	) {
		t.Fatalf("expected rejection, got %v", err)
	}
}

func TestStripeConnectOwnershipRejectsMissingDependencies(t *testing.T) {
	request, _ := validOwnershipFixture()

	verifier := StripeConnectOwnershipVerifier{}

	err := verifier.Verify(context.Background(), request)
	if !errors.Is(
		err,
		ErrStripeConnectOwnershipNotVerified,
	) {
		t.Fatalf("expected missing provider rejection, got %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	provider := &fakeOwnershipProvider{}
	verifier.Provider = provider

	for _, testCtx := range []context.Context{nil, ctx} {
		err := verifier.Verify(testCtx, request)

		if !errors.Is(
			err,
			ErrStripeConnectOwnershipNotVerified,
		) {
			t.Fatalf("expected context rejection, got %v", err)
		}
	}

	if provider.calls != 0 {
		t.Fatalf("invalid contexts triggered provider lookup")
	}
}
