package postgres

import "testing"

func TestFlexCentsToDecimal(t *testing.T) {
	tests := []struct {
		cents int64
		want  string
		valid bool
	}{
		{1, "0.01", true},
		{10, "0.10", true},
		{100, "1.00", true},
		{12345, "123.45", true},
		{9_999_999_999, "99999999.99", true},
		{0, "", false},
		{-1, "", false},
		{10_000_000_000, "", false},
	}

	for _, tt := range tests {
		got, err := flexCentsToDecimal(tt.cents)

		if (err == nil) != tt.valid {
			t.Fatalf(
				"cents=%d: unexpected error: %v",
				tt.cents,
				err,
			)
		}

		if tt.valid && got != tt.want {
			t.Fatalf(
				"cents=%d: got %q, want %q",
				tt.cents,
				got,
				tt.want,
			)
		}
	}
}

func TestFlexDecimalToCents(t *testing.T) {
	tests := []struct {
		value string
		want  int64
		valid bool
	}{
		{"0.01", 1, true},
		{"0.10", 10, true},
		{"1", 100, true},
		{"1.5", 150, true},
		{"123.45", 12345, true},
		{"99999999.99", 9_999_999_999, true},
		{"0", 0, false},
		{"-1.00", 0, false},
		{"1.001", 0, false},
		{"100000000.00", 0, false},
		{"abc", 0, false},
		{"1.", 0, false},
		{"", 0, false},
	}

	for _, tt := range tests {
		got, err := flexDecimalToCents(tt.value)

		if (err == nil) != tt.valid {
			t.Fatalf(
				"value=%q: unexpected error: %v",
				tt.value,
				err,
			)
		}

		if tt.valid && got != tt.want {
			t.Fatalf(
				"value=%q: got %d, want %d",
				tt.value,
				got,
				tt.want,
			)
		}
	}
}
