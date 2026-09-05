package paymenttransaction

import (
	"testing"

	paymentservice "github.com/JCKFinland/connect/backend/internal/services/payment"
)

func TestCanInitiateOperation(t *testing.T) {
	tests := []struct {
		name            string
		paymentStatus   string
		transactionType string
		want            bool
	}{
		{
			name:            "pending allows sale",
			paymentStatus:   paymentservice.StatusPending,
			transactionType: TypeSale,
			want:            true,
		},
		{
			name:            "pending allows authorize",
			paymentStatus:   paymentservice.StatusPending,
			transactionType: TypeAuthorize,
			want:            true,
		},
		{
			name:            "pending rejects capture",
			paymentStatus:   paymentservice.StatusPending,
			transactionType: TypeCapture,
			want:            false,
		},
		{
			name:            "authorized allows capture",
			paymentStatus:   paymentservice.StatusAuthorized,
			transactionType: TypeCapture,
			want:            true,
		},
		{
			name:            "authorized allows void",
			paymentStatus:   paymentservice.StatusAuthorized,
			transactionType: TypeVoid,
			want:            true,
		},
		{
			name:            "authorized rejects sale",
			paymentStatus:   paymentservice.StatusAuthorized,
			transactionType: TypeSale,
			want:            false,
		},
		{
			name:            "paid allows refund",
			paymentStatus:   paymentservice.StatusPaid,
			transactionType: TypeRefund,
			want:            true,
		},
		{
			name:            "partially refunded allows refund",
			paymentStatus:   paymentservice.StatusPartiallyRefunded,
			transactionType: TypeRefund,
			want:            true,
		},
		{
			name:            "refunded rejects refund",
			paymentStatus:   paymentservice.StatusRefunded,
			transactionType: TypeRefund,
			want:            false,
		},
		{
			name:            "cancelled rejects sale",
			paymentStatus:   paymentservice.StatusCancelled,
			transactionType: TypeSale,
			want:            false,
		},
		{
			name:            "failed rejects sale",
			paymentStatus:   paymentservice.StatusFailed,
			transactionType: TypeSale,
			want:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canInitiateOperation(
				tt.paymentStatus,
				tt.transactionType,
			)

			if got != tt.want {
				t.Fatalf(
					"canInitiateOperation(%q, %q) = %v, want %v",
					tt.paymentStatus,
					tt.transactionType,
					got,
					tt.want,
				)
			}
		})
	}
}
