package payouts

import (
	"context"
	"errors"
	"testing"
)

type fakeStripeConnectAccountReader struct {
	snapshot  StripeConnectAccountSnapshot
	err       error
	calls     int
	accountID string
}

func (f *fakeStripeConnectAccountReader) RetrieveConnectAccount(
	_ context.Context,
	accountID string,
) (StripeConnectAccountSnapshot, error) {
	f.calls++
	f.accountID = accountID
	return f.snapshot, f.err
}

func testStripeConnectExpectedIdentity() StripeConnectAccountIdentity {
	return StripeConnectAccountIdentity{
		AccountID: "acct_1234567890ABCDEF",
		Country:   "FI",
		Currency:  "EUR",
	}
}

func testStripeConnectEligibleSnapshot() StripeConnectAccountSnapshot {
	return StripeConnectAccountSnapshot{
		AccountID:                 "acct_1234567890ABCDEF",
		Country:                   "FI",
		TransfersCapabilityActive: true,
		PayoutsEnabled:            true,
		DetailsSubmitted:          true,
	}
}

func TestStripeConnectEligibilityVerified(t *testing.T) {
	reader := &fakeStripeConnectAccountReader{
		snapshot: testStripeConnectEligibleSnapshot(),
	}

	verifier := StripeConnectEligibilityVerifier{
		Reader: reader,
	}

	err := verifier.Verify(
		context.Background(),
		testStripeConnectExpectedIdentity(),
	)
	if err != nil {
		t.Fatalf("expected eligible account: %v", err)
	}

	if reader.calls != 1 {
		t.Fatalf("expected one provider lookup, got %d", reader.calls)
	}

	if reader.accountID != testStripeConnectExpectedIdentity().AccountID {
		t.Fatalf("wrong account lookup: %q", reader.accountID)
	}
}

func TestStripeConnectEligibilityRejectsInvalidEvidence(
	t *testing.T,
) {
	cases := []struct {
		name   string
		mutate func(*StripeConnectAccountSnapshot)
	}{
		{
			name: "account mismatch",
			mutate: func(s *StripeConnectAccountSnapshot) {
				s.AccountID = "acct_9999999999"
			},
		},
		{
			name: "country mismatch",
			mutate: func(s *StripeConnectAccountSnapshot) {
				s.Country = "SE"
			},
		},
		{
			name: "transfers inactive",
			mutate: func(s *StripeConnectAccountSnapshot) {
				s.TransfersCapabilityActive = false
			},
		},
		{
			name: "payouts disabled",
			mutate: func(s *StripeConnectAccountSnapshot) {
				s.PayoutsEnabled = false
			},
		},
		{
			name: "details missing",
			mutate: func(s *StripeConnectAccountSnapshot) {
				s.DetailsSubmitted = false
			},
		},
		{
			name: "currently due",
			mutate: func(s *StripeConnectAccountSnapshot) {
				s.CurrentlyDueCount = 1
			},
		},
		{
			name: "past due",
			mutate: func(s *StripeConnectAccountSnapshot) {
				s.PastDueCount = 1
			},
		},
		{
			name: "disabled reason",
			mutate: func(s *StripeConnectAccountSnapshot) {
				s.DisabledReason = "requirements.past_due"
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := testStripeConnectEligibleSnapshot()
			tc.mutate(&snapshot)

			err := VerifyStripeConnectAccountEligibility(
				testStripeConnectExpectedIdentity(),
				snapshot,
			)

			if !errors.Is(
				err,
				ErrStripeConnectAccountNotEligible,
			) {
				t.Fatalf("expected rejection, got %v", err)
			}
		})
	}
}

func TestStripeConnectEligibilityRejectsProviderFailure(
	t *testing.T,
) {
	reader := &fakeStripeConnectAccountReader{
		err: errors.New("provider unavailable"),
	}

	verifier := StripeConnectEligibilityVerifier{
		Reader: reader,
	}

	err := verifier.Verify(
		context.Background(),
		testStripeConnectExpectedIdentity(),
	)

	if !errors.Is(
		err,
		ErrStripeConnectAccountNotEligible,
	) {
		t.Fatalf("expected rejection, got %v", err)
	}

	if reader.calls != 1 {
		t.Fatalf("expected one lookup, got %d", reader.calls)
	}
}

func TestStripeConnectEligibilityRejectsInvalidExpectedIdentity(
	t *testing.T,
) {
	reader := &fakeStripeConnectAccountReader{
		snapshot: testStripeConnectEligibleSnapshot(),
	}

	verifier := StripeConnectEligibilityVerifier{
		Reader: reader,
	}

	err := verifier.Verify(
		context.Background(),
		StripeConnectAccountIdentity{
			AccountID: "invalid",
			Country:   "FI",
			Currency:  "EUR",
		},
	)

	if !errors.Is(
		err,
		ErrStripeConnectAccountNotEligible,
	) {
		t.Fatalf("expected rejection, got %v", err)
	}

	if reader.calls != 0 {
		t.Fatalf("invalid identity triggered provider lookup")
	}
}

func TestStripeConnectEligibilityRejectsMissingReader(
	t *testing.T,
) {
	verifier := StripeConnectEligibilityVerifier{}

	err := verifier.Verify(
		context.Background(),
		testStripeConnectExpectedIdentity(),
	)

	if !errors.Is(
		err,
		ErrStripeConnectAccountNotEligible,
	) {
		t.Fatalf("expected rejection, got %v", err)
	}
}
