package stripe

import (
	"errors"
	"testing"
)

func TestAmountToMinorUnits(
	t *testing.T,
) {
	tests := []struct {
		name     string
		amount   string
		currency string
		want     int64
	}{
		{
			name:     "euros with cents",
			amount:   "23.45",
			currency: "EUR",
			want:     2345,
		},
		{
			name:     "whole euros",
			amount:   "23",
			currency: "EUR",
			want:     2300,
		},
		{
			name:     "single decimal",
			amount:   "23.4",
			currency: "EUR",
			want:     2340,
		},
		{
			name:     "lowercase currency",
			amount:   "1.05",
			currency: "eur",
			want:     105,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				got, err :=
					amountToMinorUnits(
						tt.amount,
						tt.currency,
					)
				if err != nil {
					t.Fatalf(
						"convert amount: %v",
						err,
					)
				}

				if got != tt.want {
					t.Fatalf(
						"got %d want %d",
						got,
						tt.want,
					)
				}
			},
		)
	}
}

func TestAmountToMinorUnitsRejectsInvalidValues(
	t *testing.T,
) {
	tests := []struct {
		name     string
		amount   string
		currency string
	}{
		{
			name:     "empty amount",
			amount:   "",
			currency: "EUR",
		},
		{
			name:     "zero",
			amount:   "0",
			currency: "EUR",
		},
		{
			name:     "negative",
			amount:   "-1.00",
			currency: "EUR",
		},
		{
			name:     "too many decimals",
			amount:   "1.001",
			currency: "EUR",
		},
		{
			name:     "malformed",
			amount:   "abc",
			currency: "EUR",
		},
		{
			name:     "unsupported currency",
			amount:   "1.00",
			currency: "USD",
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				_, err :=
					amountToMinorUnits(
						tt.amount,
						tt.currency,
					)

				if !errors.Is(
					err,
					ErrInvalidAmount,
				) {
					t.Fatalf(
						"expected ErrInvalidAmount, got %v",
						err,
					)
				}
			},
		)
	}
}
