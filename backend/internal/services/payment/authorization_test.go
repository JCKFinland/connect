package payment

import (
	"testing"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func TestCanReadPayment(t *testing.T) {
	const (
		customerID  = "customer-1"
		otherUserID = "other-user"
		driverID    = "driver-1"
	)

	payment := &models.Payment{
		CustomerID: customerID,
	}

	tests := []struct {
		name   string
		roles  []string
		userID string
		want   bool
	}{
		{
			name:   "owning customer may read",
			roles:  []string{"CUSTOMER"},
			userID: customerID,
			want:   true,
		},
		{
			name:   "system admin may read",
			roles:  []string{"SYSTEM_ADMIN"},
			userID: otherUserID,
			want:   true,
		},
		{
			name:   "company admin may read",
			roles:  []string{"COMPANY_ADMIN"},
			userID: otherUserID,
			want:   true,
		},
		{
			name:   "dispatcher may read",
			roles:  []string{"DISPATCHER"},
			userID: otherUserID,
			want:   true,
		},
		{
			name:   "assigned driver has no automatic payment access",
			roles:  []string{"DRIVER"},
			userID: driverID,
			want:   false,
		},
		{
			name:   "unrelated customer denied",
			roles:  []string{"CUSTOMER"},
			userID: otherUserID,
			want:   false,
		},
		{
			name:   "no roles and not owner denied",
			roles:  nil,
			userID: otherUserID,
			want:   false,
		},
		{
			name:   "nil payment denied",
			roles:  []string{"SYSTEM_ADMIN"},
			userID: otherUserID,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				target := payment

				if tt.name == "nil payment denied" {
					target = nil
				}

				got := canReadPayment(
					tt.roles,
					tt.userID,
					target,
				)

				if got != tt.want {
					t.Fatalf(
						"canReadPayment(%v, %q) = %v, want %v",
						tt.roles,
						tt.userID,
						got,
						tt.want,
					)
				}
			},
		)
	}
}

func TestCanCreatePaymentForTrip(t *testing.T) {
	const (
		customerID  = "customer-1"
		otherUserID = "other-user"
		driverID    = "driver-1"
	)

	trip := &models.Trip{
		CustomerID: customerID,
		DriverID:   driverID,
	}

	tests := []struct {
		name   string
		roles  []string
		userID string
		want   bool
	}{
		{
			name:   "owning customer may create payment",
			roles:  []string{"CUSTOMER"},
			userID: customerID,
			want:   true,
		},
		{
			name:   "system admin may create payment",
			roles:  []string{"SYSTEM_ADMIN"},
			userID: otherUserID,
			want:   true,
		},
		{
			name:   "company admin may create payment",
			roles:  []string{"COMPANY_ADMIN"},
			userID: otherUserID,
			want:   true,
		},
		{
			name:   "dispatcher may create payment",
			roles:  []string{"DISPATCHER"},
			userID: otherUserID,
			want:   true,
		},
		{
			name:   "assigned driver may not create customer payment",
			roles:  []string{"DRIVER"},
			userID: driverID,
			want:   false,
		},
		{
			name:   "unrelated customer denied",
			roles:  []string{"CUSTOMER"},
			userID: otherUserID,
			want:   false,
		},
		{
			name:   "nil trip denied",
			roles:  []string{"SYSTEM_ADMIN"},
			userID: otherUserID,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				target := trip

				if tt.name == "nil trip denied" {
					target = nil
				}

				got := canCreatePaymentForTrip(
					tt.roles,
					tt.userID,
					target,
				)

				if got != tt.want {
					t.Fatalf(
						"canCreatePaymentForTrip(%v, %q) = %v, want %v",
						tt.roles,
						tt.userID,
						got,
						tt.want,
					)
				}
			},
		)
	}
}
