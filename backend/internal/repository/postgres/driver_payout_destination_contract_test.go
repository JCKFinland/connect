package postgres

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestValidateDriverPayoutDestinationBinding(t *testing.T) {
	valid := DriverPayoutDestinationBinding{
		DestinationID: uuid.NewString(),
		Provider:      "TEST_PROVIDER",
	}

	if err := ValidateDriverPayoutDestinationBinding(valid); err != nil {
		t.Fatalf("valid binding rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*DriverPayoutDestinationBinding)
	}{
		{
			name: "missing_destination",
			mutate: func(b *DriverPayoutDestinationBinding) {
				b.DestinationID = ""
			},
		},
		{
			name: "invalid_destination",
			mutate: func(b *DriverPayoutDestinationBinding) {
				b.DestinationID = "not-a-uuid"
			},
		},
		{
			name: "missing_provider",
			mutate: func(b *DriverPayoutDestinationBinding) {
				b.Provider = ""
			},
		},
		{
			name: "lowercase_provider",
			mutate: func(b *DriverPayoutDestinationBinding) {
				b.Provider = "test_provider"
			},
		},
		{
			name: "leading_space",
			mutate: func(b *DriverPayoutDestinationBinding) {
				b.Provider = " TEST_PROVIDER"
			},
		},
		{
			name: "trailing_space",
			mutate: func(b *DriverPayoutDestinationBinding) {
				b.Provider = "TEST_PROVIDER "
			},
		},
		{
			name: "provider_too_long",
			mutate: func(b *DriverPayoutDestinationBinding) {
				b.Provider = "ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890ABCDEFGHIJKLMNOP"
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			binding := valid
			tc.mutate(&binding)

			err := ValidateDriverPayoutDestinationBinding(binding)

			if !errors.Is(
				err,
				ErrDriverPayoutDestinationInvalidInput,
			) {
				t.Fatalf(
					"expected invalid input, got %v",
					err,
				)
			}
		})
	}
}
