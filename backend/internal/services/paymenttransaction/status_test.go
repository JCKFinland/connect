package paymenttransaction

import "testing"

func TestPaymentTransactionStatusTransitions(
	t *testing.T,
) {
	tests := []struct {
		name string

		from string
		to   string

		want bool
	}{
		{
			name: "pending to processing",
			from: StatusPending,
			to:   StatusProcessing,
			want: true,
		},
		{
			name: "pending directly to success",
			from: StatusPending,
			to:   StatusSuccess,
			want: true,
		},
		{
			name: "pending to failed",
			from: StatusPending,
			to:   StatusFailed,
			want: true,
		},
		{
			name: "pending to cancelled",
			from: StatusPending,
			to:   StatusCancelled,
			want: true,
		},
		{
			name: "processing to success",
			from: StatusProcessing,
			to:   StatusSuccess,
			want: true,
		},
		{
			name: "processing to failed",
			from: StatusProcessing,
			to:   StatusFailed,
			want: true,
		},
		{
			name: "processing to cancelled",
			from: StatusProcessing,
			to:   StatusCancelled,
			want: true,
		},
		{
			name: "success is terminal",
			from: StatusSuccess,
			to:   StatusFailed,
			want: false,
		},
		{
			name: "failed is terminal",
			from: StatusFailed,
			to:   StatusSuccess,
			want: false,
		},
		{
			name: "cancelled is terminal",
			from: StatusCancelled,
			to:   StatusProcessing,
			want: false,
		},
		{
			name: "pending cannot remain pending transition",
			from: StatusPending,
			to:   StatusPending,
			want: false,
		},
		{
			name: "processing cannot return to pending",
			from: StatusProcessing,
			to:   StatusPending,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				got := canTransition(
					tt.from,
					tt.to,
				)

				if got != tt.want {
					t.Fatalf(
						"canTransition(%q, %q) = %v, want %v",
						tt.from,
						tt.to,
						got,
						tt.want,
					)
				}
			},
		)
	}
}

func TestPaymentTransactionValidStatuses(
	t *testing.T,
) {
	valid := []string{
		StatusPending,
		StatusProcessing,
		StatusSuccess,
		StatusFailed,
		StatusCancelled,
	}

	for _, status := range valid {
		if !isValidStatus(status) {
			t.Fatalf(
				"expected %q to be valid",
				status,
			)
		}
	}

	invalid := []string{
		"",
		"PAID",
		"AUTHORIZED",
		"REFUNDED",
		"UNKNOWN",
	}

	for _, status := range invalid {
		if isValidStatus(status) {
			t.Fatalf(
				"expected %q to be invalid",
				status,
			)
		}
	}
}
