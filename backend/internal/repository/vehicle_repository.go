package repository

import (
	"context"

	"github.com/JCKFinland/connect/backend/internal/models"
)

// VehicleRepository defines persistence operations for vehicles.
type VehicleRepository interface {

	// Create stores a new vehicle.
	Create(
		ctx context.Context,
		vehicle *models.Vehicle,
	) error

	// GetByID returns a vehicle by its ID.
	GetByID(
		ctx context.Context,
		id string,
	) (*models.Vehicle, error)

	// GetByIDForCompanyMember returns a vehicle only when the user has
	// explicit membership in the vehicle's company.
	GetByIDForCompanyMember(
		ctx context.Context,
		userID string,
		id string,
	) (*models.Vehicle, error)

	// List returns all non-deleted vehicles across all companies.
	// This global surface is reserved for explicitly authorized callers.
	List(
		ctx context.Context,
	) ([]models.Vehicle, error)

	// ListForCompanyMember returns non-deleted vehicles belonging only to
	// companies in which the user has explicit membership.
	ListForCompanyMember(
		ctx context.Context,
		userID string,
	) ([]models.Vehicle, error)

	// UpdateDetails modifies descriptive vehicle fields only.
	UpdateDetails(
		ctx context.Context,
		vehicle *models.Vehicle,
	) error

	// UpdateDetailsForCompanyMember modifies descriptive vehicle fields only
	// when the user has explicit membership in the vehicle's company.
	UpdateDetailsForCompanyMember(
		ctx context.Context,
		userID string,
		vehicle *models.Vehicle,
	) error

	// Deactivate marks a non-deleted vehicle operationally inactive.
	Deactivate(
		ctx context.Context,
		id string,
	) error

	// DeactivateForCompanyMember marks a vehicle inactive only when the
	// authenticated user has explicit membership in its company.
	DeactivateForCompanyMember(
		ctx context.Context,
		userID string,
		id string,
	) error

	// Reactivate marks a non-deleted vehicle operationally active only when
	// its current fleet is itself active and non-deleted.
	Reactivate(
		ctx context.Context,
		id string,
	) error

	// ReactivateForCompanyMember marks a vehicle active only when the
	// authenticated user has explicit membership in its company and its
	// current fleet is active and non-deleted.
	ReactivateForCompanyMember(
		ctx context.Context,
		userID string,
		id string,
	) error

	// HasNonDeletedByFleet reports whether the fleet still contains at least
	// one non-archived vehicle, regardless of operational activation state.
	HasNonDeletedByFleet(
		ctx context.Context,
		fleetID string,
	) (bool, error)

	// Archive hides a vehicle from active repository reads.
	//
	// This unrestricted mutation is reserved for callers that have already
	// established platform-global authority.
	Archive(
		ctx context.Context,
		id string,
	) error

	// ArchiveForCompanyMember hides a vehicle only when the authenticated user
	// has explicit membership in the vehicle's company.
	ArchiveForCompanyMember(
		ctx context.Context,
		userID string,
		id string,
	) error
}
