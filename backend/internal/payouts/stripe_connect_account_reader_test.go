package payouts

import (
	"context"
	"errors"
	"testing"

	stripe "github.com/stripe/stripe-go/v86"
)

const testConnectAccountID = "acct_1234567890ABCDEF"

func testStripeSDKAccount() *stripe.Account {
	return &stripe.Account{
		ID:      testConnectAccountID,
		Country: "FI",
		Capabilities: &stripe.AccountCapabilities{
			Transfers: stripe.AccountCapabilityStatusActive,
		},
		PayoutsEnabled:   true,
		DetailsSubmitted: true,
		Requirements:     &stripe.AccountRequirements{},
	}
}

func TestStripeConnectSDKAccountReaderEligible(t *testing.T) {
	calls := 0

	reader := NewStripeConnectSDKAccountReader(
		func(
			ctx context.Context,
			accountID string,
		) (*stripe.Account, error) {
			calls++

			if ctx == nil || accountID != testConnectAccountID {
				t.Fatal("unexpected retrieval arguments")
			}

			return testStripeSDKAccount(), nil
		},
	)

	snapshot, err := reader.RetrieveConnectAccount(
		context.Background(),
		testConnectAccountID,
	)
	if err != nil {
		t.Fatalf("retrieval failed: %v", err)
	}

	if calls != 1 {
		t.Fatalf("expected one retrieval, got %d", calls)
	}

	expected := StripeConnectAccountIdentity{
		AccountID: testConnectAccountID,
		Country:   "FI",
		Currency:  "EUR",
	}

	if err := VerifyStripeConnectAccountEligibility(
		expected,
		snapshot,
	); err != nil {
		t.Fatalf("eligible account rejected: %v", err)
	}
}

func TestStripeConnectSDKAccountReaderRejectsMissingEvidence(
	t *testing.T,
) {
	cases := []struct {
		name   string
		mutate func(*stripe.Account)
	}{
		{
			name: "account mismatch",
			mutate: func(a *stripe.Account) {
				a.ID = "acct_9999999999"
			},
		},
		{
			name: "deleted account",
			mutate: func(a *stripe.Account) {
				a.Deleted = true
			},
		},
		{
			name: "missing capabilities",
			mutate: func(a *stripe.Account) {
				a.Capabilities = nil
			},
		},
		{
			name: "missing requirements",
			mutate: func(a *stripe.Account) {
				a.Requirements = nil
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			account := testStripeSDKAccount()
			tc.mutate(account)

			reader := NewStripeConnectSDKAccountReader(
				func(
					context.Context,
					string,
				) (*stripe.Account, error) {
					return account, nil
				},
			)

			_, err := reader.RetrieveConnectAccount(
				context.Background(),
				testConnectAccountID,
			)

			if !errors.Is(
				err,
				ErrStripeConnectAccountRetrieval,
			) {
				t.Fatalf("expected retrieval rejection, got %v", err)
			}
		})
	}
}

func TestStripeConnectSDKAccountReaderMapsIneligibleEvidence(
	t *testing.T,
) {
	cases := []struct {
		name   string
		mutate func(*stripe.Account)
	}{
		{
			name: "transfers inactive",
			mutate: func(a *stripe.Account) {
				a.Capabilities.Transfers =
					stripe.AccountCapabilityStatusInactive
			},
		},
		{
			name: "payouts disabled",
			mutate: func(a *stripe.Account) {
				a.PayoutsEnabled = false
			},
		},
		{
			name: "details incomplete",
			mutate: func(a *stripe.Account) {
				a.DetailsSubmitted = false
			},
		},
		{
			name: "currently due",
			mutate: func(a *stripe.Account) {
				a.Requirements.CurrentlyDue = []string{"company.name"}
			},
		},
		{
			name: "past due",
			mutate: func(a *stripe.Account) {
				a.Requirements.PastDue = []string{"company.name"}
			},
		},
		{
			name: "disabled reason",
			mutate: func(a *stripe.Account) {
				a.Requirements.DisabledReason =
					stripe.AccountRequirementsDisabledReason(
						"requirements.past_due",
					)
			},
		},
		{
			name: "wrong country",
			mutate: func(a *stripe.Account) {
				a.Country = "SE"
			},
		},
	}

	expected := StripeConnectAccountIdentity{
		AccountID: testConnectAccountID,
		Country:   "FI",
		Currency:  "EUR",
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			account := testStripeSDKAccount()
			tc.mutate(account)

			reader := NewStripeConnectSDKAccountReader(
				func(
					context.Context,
					string,
				) (*stripe.Account, error) {
					return account, nil
				},
			)

			snapshot, err := reader.RetrieveConnectAccount(
				context.Background(),
				testConnectAccountID,
			)
			if err != nil {
				t.Fatalf("mapping failed: %v", err)
			}

			err = VerifyStripeConnectAccountEligibility(
				expected,
				snapshot,
			)

			if !errors.Is(
				err,
				ErrStripeConnectAccountNotEligible,
			) {
				t.Fatalf("expected eligibility rejection, got %v", err)
			}
		})
	}
}

func TestStripeConnectSDKAccountReaderProviderFailure(
	t *testing.T,
) {
	reader := NewStripeConnectSDKAccountReader(
		func(
			context.Context,
			string,
		) (*stripe.Account, error) {
			return nil, errors.New("provider unavailable")
		},
	)

	_, err := reader.RetrieveConnectAccount(
		context.Background(),
		testConnectAccountID,
	)

	if !errors.Is(err, ErrStripeConnectAccountRetrieval) {
		t.Fatalf("expected retrieval failure, got %v", err)
	}
}

func TestStripeConnectSDKAccountReaderRejectsInvalidInput(
	t *testing.T,
) {
	calls := 0

	reader := NewStripeConnectSDKAccountReader(
		func(
			context.Context,
			string,
		) (*stripe.Account, error) {
			calls++
			return testStripeSDKAccount(), nil
		},
	)

	cases := []struct {
		name      string
		ctx       context.Context
		accountID string
	}{
		{"nil context", nil, testConnectAccountID},
		{"invalid account", context.Background(), "invalid"},
		{"empty account", context.Background(), ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := reader.RetrieveConnectAccount(
				tc.ctx,
				tc.accountID,
			)

			if !errors.Is(
				err,
				ErrStripeConnectAccountRetrieval,
			) {
				t.Fatalf("expected rejection, got %v", err)
			}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := reader.RetrieveConnectAccount(
		ctx,
		testConnectAccountID,
	)
	if !errors.Is(err, ErrStripeConnectAccountRetrieval) {
		t.Fatalf("expected canceled context rejection, got %v", err)
	}

	if calls != 0 {
		t.Fatalf("invalid inputs triggered %d provider calls", calls)
	}
}

func TestStripeConnectSDKAccountReaderRejectsNilDependencies(
	t *testing.T,
) {
	var nilReader *StripeConnectSDKAccountReader

	readers := []*StripeConnectSDKAccountReader{
		nilReader,
		NewStripeConnectSDKAccountReader(nil),
		NewStripeConnectSDKAccountReaderFromClient(nil),
	}

	for _, reader := range readers {
		_, err := reader.RetrieveConnectAccount(
			context.Background(),
			testConnectAccountID,
		)

		if !errors.Is(err, ErrStripeConnectAccountRetrieval) {
			t.Fatalf("expected rejection, got %v", err)
		}
	}
}
