package vehicle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JCKFinland/connect/backend/internal/models"
	"github.com/JCKFinland/connect/backend/internal/repository"
	"github.com/jackc/pgx/v5/pgconn"
)

// Register registers a vehicle for the authenticated, verified driver.
//
// Company and branch ownership are derived from the driver's profile.
// The driver may only register the vehicle into an active fleet belonging
// to that same company and branch.
func (s *Service) Register(
	ctx context.Context,
	userID string,
	req RegisterDriverVehicleRequest,
) (*VehicleResponse, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrDriverNotEligible
	}

	driver, err := s.drivers.GetByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrDriverNotEligible
		}

		return nil, fmt.Errorf("get driver profile: %w", err)
	}

	if driver == nil ||
		driver.Status != "ACTIVE" ||
		!driver.IsVerified ||
		!driver.IsActive {
		return nil, ErrDriverNotEligible
	}

	fleetID := strings.TrimSpace(req.FleetID)
	registrationNumber := strings.ToUpper(
		strings.TrimSpace(req.RegistrationNumber),
	)
	makeName := strings.TrimSpace(req.Make)
	modelName := strings.TrimSpace(req.Model)
	color := strings.TrimSpace(req.Color)
	vehicleType := strings.ToUpper(
		strings.TrimSpace(req.VehicleType),
	)
	fuelType := strings.ToUpper(
		strings.TrimSpace(req.FuelType),
	)

	if fleetID == "" ||
		registrationNumber == "" ||
		makeName == "" ||
		modelName == "" ||
		vehicleType == "" {
		return nil, ErrInvalidVehicle
	}

	switch fuelType {
	case "EV", "HYBRID", "PETROL", "DIESEL":
	default:
		return nil, ErrInvalidVehicle
	}

	currentYear := time.Now().UTC().Year()

	if req.ModelYear < 1900 ||
		req.ModelYear > currentYear+1 ||
		req.SeatingCapacity < 1 {
		return nil, ErrInvalidVehicle
	}

	fleet, err := s.fleets.GetByID(ctx, fleetID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrInvalidFleet
		}

		return nil, fmt.Errorf("get fleet: %w", err)
	}

	if fleet == nil ||
		!fleet.IsActive ||
		fleet.CompanyID != driver.CompanyID ||
		fleet.BranchID != driver.BranchID {
		return nil, ErrInvalidFleet
	}

	vehicle := &models.Vehicle{
		CompanyID:          driver.CompanyID,
		BranchID:           driver.BranchID,
		FleetID:            fleet.ID,
		RegistrationNumber: registrationNumber,
		VIN:                normalizeVIN(req.VIN),
		Make:               makeName,
		Model:              modelName,
		ModelYear:          req.ModelYear,
		Color:              color,
		VehicleType:        vehicleType,
		FuelType:           fuelType,
		SeatingCapacity:    req.SeatingCapacity,
		IsActive:           true,
	}

	if err := s.vehicles.Create(ctx, vehicle); err != nil {
		var pgErr *pgconn.PgError

		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			switch pgErr.ConstraintName {
			case "idx_vehicles_registration":
				return nil, ErrDuplicateRegistrationNumber

			case "idx_vehicles_vin":
				return nil, ErrDuplicateVIN
			}
		}

		return nil, fmt.Errorf("create driver vehicle: %w", err)
	}

	return &VehicleResponse{
		ID:                 vehicle.ID,
		CompanyID:          vehicle.CompanyID,
		BranchID:           vehicle.BranchID,
		FleetID:            vehicle.FleetID,
		RegistrationNumber: vehicle.RegistrationNumber,
		VIN:                vinValue(vehicle.VIN),
		Make:               vehicle.Make,
		Model:              vehicle.Model,
		ModelYear:          vehicle.ModelYear,
		Color:              vehicle.Color,
		VehicleType:        vehicle.VehicleType,
		FuelType:           vehicle.FuelType,
		SeatingCapacity:    vehicle.SeatingCapacity,
		IsActive:           vehicle.IsActive,
	}, nil
}
