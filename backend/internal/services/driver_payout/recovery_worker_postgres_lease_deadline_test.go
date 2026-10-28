package driver_payout

import (
	"testing"
	"time"
)

func TestRecoveryLeaseAllowsLookup(t *testing.T) {
	now := time.Date(
		2026, time.October, 28,
		12, 0, 0, 0,
		time.UTC,
	)

	tests := []struct {
		name      string
		remaining time.Duration
		want      bool
	}{
		{
			name:      "expired lease",
			remaining: -time.Second,
			want:      false,
		},
		{
			name:      "deadline reached",
			remaining: 0,
			want:      false,
		},
		{
			name:      "insufficient time",
			remaining: 29 * time.Second,
			want:      false,
		},
		{
			name: "just below lookup timeout",
			remaining: postgresRecoveryLookupTimeout -
				time.Nanosecond,
			want: false,
		},
		{
			name:      "exact lookup timeout",
			remaining: postgresRecoveryLookupTimeout,
			want:      true,
		},
		{
			name: "sufficient time",
			remaining: postgresRecoveryLookupTimeout +
				time.Second,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deadline := now.Add(tt.remaining)

			got := recoveryLeaseAllowsLookup(
				deadline,
				now,
			)

			if got != tt.want {
				t.Fatalf(
					"remaining=%s: got=%v want=%v",
					tt.remaining,
					got,
					tt.want,
				)
			}
		})
	}
}
