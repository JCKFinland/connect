package flex_settlement

import (
	"testing"
)

func TestPostgresFlexAmountToCentsValid(t *testing.T) {
	tests := []struct {
		name   string
		amount string
		want   int64
	}{
		{
			name:   "one cent",
			amount: "0.01",
			want:   1,
		},
		{
			name:   "whole euro",
			amount: "1.00",
			want:   100,
		},
		{
			name:   "typical FLEX fare",
			amount: "23.45",
			want:   2345,
		},
		{
			name:   "large fare",
			amount: "99999999.99",
			want:   9999999999,
		},
		{
			name:   "leading zeroes",
			amount: "00023.45",
			want:   2345,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := postgresFlexAmountToCents(tt.amount)
			if err != nil {
				t.Fatalf(
					"convert %q: %v",
					tt.amount,
					err,
				)
			}

			if got != tt.want {
				t.Fatalf(
					"convert %q: got %d cents, want %d",
					tt.amount,
					got,
					tt.want,
				)
			}
		})
	}
}

func TestPostgresFlexAmountToCentsRejectsInvalid(t *testing.T) {
	tests := []struct {
		name   string
		amount string
	}{
		{"empty", ""},
		{"whitespace", " "},
		{"leading whitespace", " 23.45"},
		{"trailing whitespace", "23.45 "},
		{"zero", "0.00"},
		{"negative", "-23.45"},
		{"positive sign", "+23.45"},
		{"no decimal point", "23"},
		{"one fractional digit", "23.4"},
		{"three fractional digits", "23.456"},
		{"missing whole part", ".45"},
		{"missing fraction", "23."},
		{"multiple decimal points", "23.4.5"},
		{"comma separator", "23,45"},
		{"currency symbol", "€23.45"},
		{"alphabetic", "abc"},
		{"scientific notation", "1e2"},
		{"integer overflow", "9223372036854775808.00"},
		{"cents overflow", "92233720368547758.08"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := postgresFlexAmountToCents(tt.amount)

			if err == nil {
				t.Fatalf(
					"invalid amount %q accepted as %d cents",
					tt.amount,
					got,
				)
			}
		})
	}
}

func TestPostgresFlexAmountToCentsMaximumSafeValue(
	t *testing.T,
) {
	const amount = "92233720368547758.07"
	const want int64 = 9223372036854775807

	got, err := postgresFlexAmountToCents(amount)
	if err != nil {
		t.Fatalf("convert maximum safe amount: %v", err)
	}

	if got != want {
		t.Fatalf(
			"maximum safe amount: got %d, want %d",
			got,
			want,
		)
	}
}
