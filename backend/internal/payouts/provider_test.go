package payouts

import (
	"errors"
	"strings"
	"testing"
)

func validTransferRequest() TransferRequest {
	return TransferRequest{
		PayoutID:       "synthetic-payout",
		IdempotencyKey: "driver-payout-execution:synthetic-payout",
		DestinationID:  "synthetic-destination",
		AmountCents:    2165,
		Currency:       "EUR",
	}
}

func TestValidateTransferRequest(t *testing.T) {
	if err := ValidateTransferRequest(validTransferRequest()); err != nil {
		t.Fatal(err)
	}

	cases := map[string]func(*TransferRequest){
		"missing payout": func(r *TransferRequest) {
			r.PayoutID = ""
		},
		"missing key": func(r *TransferRequest) {
			r.IdempotencyKey = ""
		},
		"missing destination": func(r *TransferRequest) {
			r.DestinationID = ""
		},
		"zero amount": func(r *TransferRequest) {
			r.AmountCents = 0
		},
		"negative amount": func(r *TransferRequest) {
			r.AmountCents = -1
		},
		"wrong currency": func(r *TransferRequest) {
			r.Currency = "USD"
		},
		"whitespace key": func(r *TransferRequest) {
			r.IdempotencyKey = " invalid "
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			request := validTransferRequest()
			mutate(&request)

			if !errors.Is(
				ValidateTransferRequest(request),
				ErrInvalidRequest,
			) {
				t.Fatal("invalid request accepted")
			}
		})
	}
}

func TestValidateTransferLookup(t *testing.T) {
	if err := ValidateTransferLookup(TransferLookup{
		IdempotencyKey: "synthetic-key",
	}); err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{"", " ", " invalid "} {
		if !errors.Is(
			ValidateTransferLookup(TransferLookup{
				IdempotencyKey: key,
			}),
			ErrInvalidRequest,
		) {
			t.Fatalf("invalid lookup accepted: %q", key)
		}
	}
}

func TestValidateTransferResult(t *testing.T) {
	valid := []TransferResult{
		{
			Status:            TransferSucceeded,
			ProviderReference: "synthetic-transfer",
		},
		{Status: TransferFailed},
		{Status: TransferPending},
		{Status: TransferUnknown},
	}

	for _, result := range valid {
		if err := ValidateTransferResult(result); err != nil {
			t.Fatalf("valid result rejected: %+v %v", result, err)
		}
	}

	invalid := []TransferResult{
		{Status: TransferSucceeded},
		{Status: "UNRECOGNIZED"},
		{
			Status:            TransferSucceeded,
			ProviderReference: " invalid ",
		},
	}

	for _, result := range invalid {
		if !errors.Is(
			ValidateTransferResult(result),
			ErrInvalidRequest,
		) {
			t.Fatalf("invalid result accepted: %+v", result)
		}
	}
}

func TestValidateTransferRequestBoundaries(t *testing.T) {
	cases := map[string]func(*TransferRequest){
		"blank payout": func(r *TransferRequest) {
			r.PayoutID = "   "
		},
		"payout whitespace": func(r *TransferRequest) {
			r.PayoutID = " payout "
		},
		"destination whitespace": func(r *TransferRequest) {
			r.DestinationID = " destination "
		},
		"oversized key": func(r *TransferRequest) {
			r.IdempotencyKey = strings.Repeat("x", 256)
		},
		"empty currency": func(r *TransferRequest) {
			r.Currency = ""
		},
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			request := validTransferRequest()
			mutate(&request)

			if !errors.Is(
				ValidateTransferRequest(request),
				ErrInvalidRequest,
			) {
				t.Fatal("invalid request accepted")
			}
		})
	}
}

func TestValidateTransferLookupBoundaries(t *testing.T) {
	invalid := []string{
		strings.Repeat("x", 256),
		" key ",
		"\t",
	}

	for _, key := range invalid {
		err := ValidateTransferLookup(TransferLookup{
			IdempotencyKey: key,
		})

		if !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("invalid key accepted: %q", key)
		}
	}
}

func TestValidateTransferResultBoundaries(t *testing.T) {
	invalid := []TransferResult{
		{
			Status:            TransferSucceeded,
			ProviderReference: "",
		},
		{
			Status:            TransferSucceeded,
			ProviderReference: "   ",
		},
		{
			Status:            TransferSucceeded,
			ProviderReference: strings.Repeat("x", 256),
		},
		{
			Status:            TransferUnknown,
			ProviderReference: " invalid ",
		},
		{
			Status: TransferStatus(""),
		},
	}

	for _, result := range invalid {
		if !errors.Is(
			ValidateTransferResult(result),
			ErrInvalidRequest,
		) {
			t.Fatalf("invalid result accepted: %+v", result)
		}
	}
}
