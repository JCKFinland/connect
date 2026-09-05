package payment

import "testing"

func TestPaymentStatusTransitions(t *testing.T) {
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
			name: "pending to paid",
			from: StatusPending,
			to:   StatusPaid,
			want: true,
		},
		{
			name: "pending to cancelled",
			from: StatusPending,
			to:   StatusCancelled,
			want: true,
		},
		{
			name: "processing to authorized",
			from: StatusProcessing,
			to:   StatusAuthorized,
			want: true,
		},
		{
			name: "processing to paid",
			from: StatusProcessing,
			to:   StatusPaid,
			want: true,
		},
		{
			name: "authorized to paid",
			from: StatusAuthorized,
			to:   StatusPaid,
			want: true,
		},
		{
			name: "paid to partially refunded",
			from: StatusPaid,
			to:   StatusPartiallyRefunded,
			want: true,
		},
		{
			name: "paid to refunded",
			from: StatusPaid,
			to:   StatusRefunded,
			want: true,
		},
		{
			name: "partially refunded to refunded",
			from: StatusPartiallyRefunded,
			to:   StatusRefunded,
			want: true,
		},
		{
			name: "pending cannot refund",
			from: StatusPending,
			to:   StatusRefunded,
			want: false,
		},
		{
			name: "paid cannot return to processing",
			from: StatusPaid,
			to:   StatusProcessing,
			want: false,
		},
		{
			name: "failed is terminal",
			from: StatusFailed,
			to:   StatusProcessing,
			want: false,
		},
		{
			name: "cancelled is terminal",
			from: StatusCancelled,
			to:   StatusPending,
			want: false,
		},
		{
			name: "refunded is terminal",
			from: StatusRefunded,
			to:   StatusPaid,
			want: false,
		},

		{
			name: "pending to authorized",
			from: StatusPending,
			to:   StatusAuthorized,
			want: true,
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
						"canTransition(%s, %s) = %v, want %v",
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

func TestValidPaymentMethods(t *testing.T) {
	valid := []string{
		MethodPI,
		MethodCard,
		MethodCash,
		MethodBankTransfer,
		MethodWallet,
	}

	for _, method := range valid {
		if !isValidPaymentMethod(method) {
			t.Fatalf(
				"expected payment method %s to be valid",
				method,
			)
		}
	}

	if isValidPaymentMethod("BITCOIN") {
		t.Fatal(
			"expected unsupported payment method to be invalid",
		)
	}
}
