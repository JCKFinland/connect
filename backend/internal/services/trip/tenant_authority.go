package trip

import (
	"context"
	"fmt"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository/postgres"
	"github.com/jackc/pgx/v5"
)

// hasOperationalCompanyAuthority requires an operational role and
// explicit company membership, except for SYSTEM_ADMIN.
func (s *tripService) hasOperationalCompanyAuthority(
	ctx context.Context,
	userID string,
	companyID string,
	roles []string,
) (bool, error) {
	if userID == "" || companyID == "" {
		return false, nil
	}

	operational := false

	for _, role := range roles {
		switch role {
		case "SYSTEM_ADMIN":
			return true, nil
		case "COMPANY_ADMIN", "DISPATCHER":
			operational = true
		}
	}

	if !operational {
		return false, nil
	}

	if s.companyMemberships == nil {
		return false, fmt.Errorf(
			"trip company membership repository is not configured",
		)
	}

	member, err := s.companyMemberships.Exists(
		ctx,
		userID,
		companyID,
	)
	if err != nil {
		return false, fmt.Errorf(
			"check trip company membership: %w",
			err,
		)
	}

	return member, nil
}

// canUpdateTripStatusAuthorized preserves assigned-driver lifecycle
// authority while restricting operational staff to their own tenant.
func (s *tripService) canUpdateTripStatusAuthorized(
	ctx context.Context,
	roles []string,
	userID string,
	currentTrip *models.Trip,
) (bool, error) {
	if currentTrip == nil || userID == "" {
		return false, nil
	}

	for _, role := range roles {
		if role == "DRIVER" && currentTrip.DriverID == userID {
			return true, nil
		}
	}

	return s.hasOperationalCompanyAuthority(
		ctx,
		userID,
		currentTrip.CompanyID,
		roles,
	)
}

// hasOperationalCompanyAuthorityForUpdate checks company authority
// while locking the membership row in the mutation transaction.
//
// SYSTEM_ADMIN retains global authority. Company-scoped operational
// roles require an exact membership protected by FOR SHARE.
func (s *tripService) hasOperationalCompanyAuthorityForUpdate(
	ctx context.Context,
	tx pgx.Tx,
	userID string,
	companyID string,
	roles []string,
) (bool, error) {
	if userID == "" || companyID == "" {
		return false, nil
	}

	if tx == nil {
		return false, fmt.Errorf(
			"trip authorization transaction is not configured",
		)
	}

	operational := false

	for _, role := range roles {
		switch role {
		case "SYSTEM_ADMIN":
			return true, nil
		case "COMPANY_ADMIN", "DISPATCHER":
			operational = true
		}
	}

	if !operational {
		return false, nil
	}

	memberships := postgres.NewCompanyMembershipRepository(tx)

	member, err := memberships.ExistsForShare(
		ctx,
		userID,
		companyID,
	)
	if err != nil {
		return false, fmt.Errorf(
			"lock trip company membership: %w",
			err,
		)
	}

	return member, nil
}

// canUpdateTripStatusAuthorizedForUpdate checks lifecycle authority
// within the transaction that performs the trip mutation.
//
// An assigned driver retains lifecycle authority without requiring
// company membership. Operational staff require a locked membership
// row, while SYSTEM_ADMIN retains global authority.
func (s *tripService) canUpdateTripStatusAuthorizedForUpdate(
	ctx context.Context,
	tx pgx.Tx,
	roles []string,
	userID string,
	currentTrip *models.Trip,
) (bool, error) {
	if currentTrip == nil || userID == "" {
		return false, nil
	}

	if tx == nil {
		return false, fmt.Errorf(
			"trip authorization transaction is not configured",
		)
	}

	for _, role := range roles {
		if role == "DRIVER" && currentTrip.DriverID == userID {
			return true, nil
		}
	}

	return s.hasOperationalCompanyAuthorityForUpdate(
		ctx,
		tx,
		userID,
		currentTrip.CompanyID,
		roles,
	)
}
