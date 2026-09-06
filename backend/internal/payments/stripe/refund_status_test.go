package stripe

import (
	"errors"
	"testing"

	stripego "github.com/stripe/stripe-go/v86"

	"github.com/JCKFinland/connect/backend/internal/services/paymenttransaction"
)

func TestMapRefundStatus(
	t *testing.T,
) {
	tests := []struct {
		name string

		status stripego.RefundStatus

		wantStatus string

		wantRequiresCustomerAction bool
	}{
		{
			name: "succeeded",

			status: stripego.RefundStatusSucceeded,

			wantStatus: paymenttransaction.StatusSuccess,

			wantRequiresCustomerAction: false,
		},
		{
			name: "pending",

			status: stripego.RefundStatusPending,

			wantStatus: paymenttransaction.StatusProcessing,

			wantRequiresCustomerAction: false,
		},
		{
			name: "requires action",

			status: stripego.RefundStatusRequiresAction,

			wantStatus: paymenttransaction.StatusProcessing,

			wantRequiresCustomerAction: true,
		},
		{
			name: "canceled",

			status: stripego.RefundStatusCanceled,

			wantStatus: paymenttransaction.StatusCancelled,

			wantRequiresCustomerAction: false,
		},
		{
			name: "failed",

			status: stripego.RefundStatusFailed,

			wantStatus: paymenttransaction.StatusFailed,

			wantRequiresCustomerAction: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(
			tt.name,
			func(t *testing.T) {
				gotStatus,
					gotRequiresCustomerAction,
					err :=
					mapRefundStatus(
						tt.status,
					)

				if err != nil {
					t.Fatalf(
						"map refund status %q: %v",
						tt.status,
						err,
					)
				}

				if gotStatus !=
					tt.wantStatus {

					t.Fatalf(
						"status got %q want %q",
						gotStatus,
						tt.wantStatus,
					)
				}

				if gotRequiresCustomerAction !=
					tt.wantRequiresCustomerAction {

					t.Fatalf(
						"requires customer action got %v want %v",
						gotRequiresCustomerAction,
						tt.wantRequiresCustomerAction,
					)
				}
			},
		)
	}
}

func TestMapRefundStatusRejectsUnsupportedStatus(
	t *testing.T,
) {
	status,
		requiresCustomerAction,
		err :=
		mapRefundStatus(
			stripego.RefundStatus(
				"unexpected",
			),
		)

	if status != "" {
		t.Fatalf(
			"expected empty status, got %q",
			status,
		)
	}

	if requiresCustomerAction {
		t.Fatal(
			"unsupported refund status must not require customer action",
		)
	}

	if !errors.Is(
		err,
		ErrInvalidOperation,
	) {
		t.Fatalf(
			"expected ErrInvalidOperation, got %v",
			err,
		)
	}
}
