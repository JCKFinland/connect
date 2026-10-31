package payouts

import (
	"errors"
	"testing"
)

func TestDeriveMangopayIdempotencyKey(t *testing.T) {
	payoutID := "550e8400-e29b-41d4-a716-446655440000"
	internal := "driver-payout-execution:" + payoutID

	got, err := DeriveMangopayIdempotencyKey(internal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != payoutID {
		t.Fatalf("got %q, want %q", got, payoutID)
	}

	if len(got) != 36 {
		t.Fatalf("expected 36-character provider key")
	}

	again, err := DeriveMangopayIdempotencyKey(internal)
	if err != nil || again != got {
		t.Fatal("provider key derivation is not deterministic")
	}
}

func TestDeriveMangopayIdempotencyKeyRejectsInvalid(t *testing.T) {
	cases := []string{
		"",
		"550e8400-e29b-41d4-a716-446655440000",
		"driver-payout-execution:",
		"driver-payout-execution:not-a-uuid",
		"driver-payout-execution:550E8400-E29B-41D4-A716-446655440000",
		"driver-payout-execution:550e8400-e29b-41d4-a716-446655440000 ",
		"other:550e8400-e29b-41d4-a716-446655440000",
	}

	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			got, err := DeriveMangopayIdempotencyKey(input)

			if !errors.Is(err, ErrInvalidMangopayKey) {
				t.Fatalf("expected invalid key, got %v", err)
			}

			if got != "" {
				t.Fatalf("expected empty result, got %q", got)
			}
		})
	}
}
