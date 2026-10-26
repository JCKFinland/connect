package payouts

import (
	"errors"
	"strings"
	"testing"
)

func validVerifiedTransferDetails() VerifiedTransferDetails {
	return VerifiedTransferDetails{
		Status:            TransferSucceeded,
		PayoutID:          "synthetic-payout",
		IdempotencyKey:    "synthetic-key",
		DestinationID:     "synthetic-destination",
		AmountCents:       2165,
		Currency:          "EUR",
		ProviderReference: "synthetic-transfer",
	}
}

func expectedTransferRequest() TransferRequest {
	return TransferRequest{
		PayoutID:       "synthetic-payout",
		IdempotencyKey: "synthetic-key",
		DestinationID:  "synthetic-destination",
		AmountCents:    2165,
		Currency:       "EUR",
	}
}

func TestVerifyTransferDetailsSuccess(t *testing.T) {
	if err := VerifyTransferDetails(
		expectedTransferRequest(),
		validVerifiedTransferDetails(),
	); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyTransferDetailsRejectsMismatch(t *testing.T) {
	cases := map[string]func(*VerifiedTransferDetails){
		"pending status": func(v *VerifiedTransferDetails) {
			v.Status = TransferPending
		},
		"failed status": func(v *VerifiedTransferDetails) {
			v.Status = TransferFailed
		},
		"unknown status": func(v *VerifiedTransferDetails) {
			v.Status = TransferUnknown
		},
		"wrong payout": func(v *VerifiedTransferDetails) {
			v.PayoutID = "other-payout"
		},
		"wrong key": func(v *VerifiedTransferDetails) {
			v.IdempotencyKey = "other-key"
		},
		"wrong destination": func(v *VerifiedTransferDetails) {
			v.DestinationID = "other-destination"
		},
		"wrong amount": func(v *VerifiedTransferDetails) {
			v.AmountCents++
		},
		"wrong currency": func(v *VerifiedTransferDetails) {
			v.Currency = "USD"
		},
		"missing reference": func(v *VerifiedTransferDetails) {
			v.ProviderReference = ""
		},
		"whitespace reference": func(v *VerifiedTransferDetails) {
			v.ProviderReference = " transfer "
		},
		"oversized reference": func(v *VerifiedTransferDetails) {
			v.ProviderReference = strings.Repeat("x", 256)
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			actual := validVerifiedTransferDetails()
			mutate(&actual)

			err := VerifyTransferDetails(
				expectedTransferRequest(),
				actual,
			)

			if !errors.Is(
				err,
				ErrTransferVerificationFailed,
			) {
				t.Fatalf(
					"mismatched transfer accepted: %+v",
					actual,
				)
			}
		})
	}
}

func TestVerifyTransferDetailsRejectsInvalidExpected(t *testing.T) {
	expected := expectedTransferRequest()
	expected.AmountCents = 0

	err := VerifyTransferDetails(
		expected,
		validVerifiedTransferDetails(),
	)

	if !errors.Is(
		err,
		ErrTransferVerificationFailed,
	) {
		t.Fatal("invalid expected request accepted")
	}
}
