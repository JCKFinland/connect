package payment

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
)

func canReadPayment(
	roles []string,
	userID string,
	payment *models.Payment,
) bool {
	if payment == nil {
		return false
	}

	for _, role := range roles {
		switch role {
		case "SYSTEM_ADMIN",
			"COMPANY_ADMIN",
			"DISPATCHER":

			return true
		}
	}

	return payment.CustomerID == userID
}

func canCreatePaymentForTrip(
	roles []string,
	userID string,
	trip *models.Trip,
) bool {
	if trip == nil {
		return false
	}

	for _, role := range roles {
		switch role {
		case "SYSTEM_ADMIN",
			"COMPANY_ADMIN",
			"DISPATCHER":

			return true
		}
	}

	return trip.CustomerID == userID
}

func (s *paymentService) getUserRoles(
	ctx context.Context,
	userID string,
) ([]string, error) {
	if userID == "" {
		return nil, fmt.Errorf(
			"user ID is required",
		)
	}

	if s.userRoles == nil {
		return nil, fmt.Errorf(
			"user role repository is not configured",
		)
	}

	roles, err :=
		s.userRoles.GetUserRoles(
			ctx,
			userID,
		)
	if err != nil {
		return nil, fmt.Errorf(
			"get payment user roles: %w",
			err,
		)
	}

	return roles, nil
}
